package tools

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

var (
	errURLRequired = errors.New(`url is required. ` + nextLinkResolve)
	atCoordsRe     = regexp.MustCompile(`@(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?)`)
	dataCoordsRe   = regexp.MustCompile(`!3d(-?\d+(?:\.\d+)?)!4d(-?\d+(?:\.\d+)?)`)
	latLngPairRe   = regexp.MustCompile(`(-?\d+(?:\.\d+)?)\s*,\s*(-?\d+(?:\.\d+)?)`)
)

// newFetcher builds the share-link HTTP client. Tests replace this.
var newFetcher = func() Fetcher { return defaultFetcher() }

// LinkResult is what link_resolve returns.
type LinkResult struct {
	URL         string   `json:"url"`
	Canonical   string   `json:"canonical"`
	Kind        string   `json:"kind"`
	Name        string   `json:"name,omitempty"`
	Lat         *float64 `json:"lat,omitempty"`
	Lng         *float64 `json:"lng,omitempty"`
	Origin      string   `json:"origin,omitempty"`
	Destination string   `json:"destination,omitempty"`
}

func registerLink(s *mcpserver.MCPServer) {
	tool := mcp.NewTool(ToolLink,
		mcp.WithDescription("Turn one or more Google Maps share or short URLs (maps.app.goo.gl, goo.gl/maps, g.co/maps) into canonical maps URLs plus place name, coordinates, or directions endpoints. Pass every link in urls (max 8). No API key. Already-long google.com/maps URLs are parsed without a fetch. Not a geocoder — use place_resolve for an address."),
		mcp.WithArray("urls", mcp.Required(), mcp.Description("Maps share URLs, short URLs, or already-long google.com/maps URLs. 1–8."), mcp.WithStringItems(), mcp.MinItems(1), mcp.MaxItems(maxBatch)),
		mcp.WithReadOnlyHintAnnotation(true),
	)
	registerTool(s, tool, handleLink)
}

// LinkOutcome is one link_resolve result. Error is set when that URL failed.
type LinkOutcome struct {
	LinkResult
	Error string `json:"error,omitempty"`
}

func handleLink(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := rejectSingular(request, "url", "urls", nextLinkResolve); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	urls, err := requireStrings(request, "urls", nextLinkResolve)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	f := newFetcher()
	out := make([]LinkOutcome, 0, len(urls))
	for _, rawURL := range urls {
		got, err := ResolveLink(ctx, f, rawURL)
		if err != nil {
			out = append(out, LinkOutcome{LinkResult: LinkResult{URL: rawURL}, Error: err.Error()})
			continue
		}
		out = append(out, LinkOutcome{LinkResult: got})
	}
	return batchText(out)
}

// ResolveLink follows a Maps share / short URL (or parses a long maps URL)
// and returns the canonical google.com/maps URL plus extracted fields.
func ResolveLink(ctx context.Context, f Fetcher, rawURL string) (LinkResult, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return LinkResult{}, errURLRequired
	}
	if err := validateURL(rawURL); err != nil {
		return LinkResult{}, err
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return LinkResult{}, fmt.Errorf("url: %w", err)
	}
	if !allowedMapsHost(u.Host) {
		return LinkResult{}, errors.New(`url host is not a Google Maps host. ` + nextLinkResolve)
	}

	canonical := rawURL
	if !isLongMapsURL(u) {
		var err error
		canonical, err = followShortURL(ctx, f, rawURL)
		if err != nil {
			return LinkResult{}, err
		}
	}

	out := parseMapsURL(canonical)
	out.URL = rawURL
	out.Canonical = canonical
	return out, nil
}

func followShortURL(ctx context.Context, f Fetcher, rawURL string) (string, error) {
	if f == nil {
		f = defaultFetcher()
	}
	_, _, finalURL, getErr := f.Get(ctx, rawURL)
	canonical := rawURL
	if finalURL != "" {
		canonical = finalURL
	}
	if getErr == nil {
		return canonical, nil
	}
	cu, perr := url.Parse(canonical)
	if perr != nil || !isLongMapsURL(cu) {
		return "", getErr
	}
	return canonical, nil
}

func isLongMapsURL(u *url.URL) bool {
	if u == nil || !allowedMapsHost(u.Host) {
		return false
	}
	if isShortMapsHost(u.Host) {
		return false
	}
	return strings.Contains(strings.ToLower(u.Path), "/maps")
}

func looksLikeMapsURL(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		return false
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return allowedMapsHost(u.Host)
}

func parseMapsURL(raw string) LinkResult {
	out := LinkResult{Kind: KindUnknown}
	u, err := url.Parse(raw)
	if err != nil {
		return out
	}
	q := u.Query()
	applyQueryEndpoints(&out, q)
	applyPath(&out, pathSegments(u.Path))
	applyQueryName(&out, q)
	applyCoords(&out, raw, u.Path, q)
	return out
}

func applyQueryEndpoints(out *LinkResult, q url.Values) {
	origin := firstNonEmpty(q.Get("origin"), q.Get("saddr"))
	dest := firstNonEmpty(q.Get("destination"), q.Get("daddr"))
	if origin == "" && dest == "" {
		return
	}
	out.Kind = KindDirections
	out.Origin = origin
	out.Destination = dest
}

func applyPath(out *LinkResult, segs []string) {
	i := indexFold(segs, "maps")
	if i < 0 || i+1 >= len(segs) {
		return
	}
	switch strings.ToLower(segs[i+1]) {
	case "place":
		out.Kind = KindPlace
		if i+2 < len(segs) && !isMetaSegment(segs[i+2]) {
			out.Name = pathSegment(segs[i+2])
		}
	case "dir":
		out.Kind = KindDirections
		applyDirWaypoints(out, segs[i+2:])
	case "search":
		out.Kind = KindSearch
		if i+2 < len(segs) && !isMetaSegment(segs[i+2]) {
			out.Name = pathSegment(segs[i+2])
		}
	default:
		if strings.HasPrefix(segs[i+1], "@") && out.Kind == KindUnknown {
			out.Kind = KindView
		}
	}
}

func applyDirWaypoints(out *LinkResult, rest []string) {
	waypoints := make([]string, 0, len(rest))
	for _, s := range rest {
		if isMetaSegment(s) {
			break
		}
		waypoints = append(waypoints, pathSegment(s))
	}
	if len(waypoints) >= 1 && out.Origin == "" {
		out.Origin = waypoints[0]
	}
	if len(waypoints) >= 2 && out.Destination == "" {
		out.Destination = waypoints[len(waypoints)-1]
	}
}

func applyQueryName(out *LinkResult, q url.Values) {
	qn := firstNonEmpty(q.Get("q"), q.Get("query"))
	if qn == "" || strings.HasPrefix(strings.ToLower(qn), "place_id:") {
		return
	}
	if lat, lng, ok := parseLatLng(qn); ok {
		if out.Lat == nil {
			out.Lat, out.Lng = new(lat), new(lng)
		}
		if out.Kind == KindUnknown {
			out.Kind = KindView
		}
		return
	}
	if out.Name == "" {
		out.Name = qn
	}
	if out.Kind == KindUnknown {
		out.Kind = KindSearch
	}
}

func applyCoords(out *LinkResult, raw, path string, q url.Values) {
	if lat, lng, ok := extractDataCoords(raw); ok && (out.Kind == KindPlace || out.Lat == nil) {
		out.Lat, out.Lng = new(lat), new(lng)
	}
	if out.Lat == nil && out.Kind != KindDirections {
		if lat, lng, ok := extractAtCoords(path); ok {
			out.Lat, out.Lng = new(lat), new(lng)
		}
	}
	if out.Lat == nil {
		if lat, lng, ok := parseLatLng(q.Get("ll")); ok {
			out.Lat, out.Lng = new(lat), new(lng)
		}
	}
}

func isMetaSegment(s string) bool {
	return strings.HasPrefix(s, "@") || strings.EqualFold(s, "data")
}

func pathSegments(path string) []string {
	parts := strings.Split(path, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func pathSegment(s string) string {
	s = strings.ReplaceAll(s, "+", " ")
	dec, err := url.PathUnescape(s)
	if err != nil {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(dec)
}

func indexFold(segs []string, want string) int {
	for i, s := range segs {
		if strings.EqualFold(s, want) {
			return i
		}
	}
	return -1
}

func extractAtCoords(path string) (float64, float64, bool) {
	m := atCoordsRe.FindStringSubmatch(path)
	if m == nil {
		return 0, 0, false
	}
	return parseFloatPair(m[1], m[2])
}

func extractDataCoords(raw string) (float64, float64, bool) {
	m := dataCoordsRe.FindStringSubmatch(raw)
	if m == nil {
		return 0, 0, false
	}
	return parseFloatPair(m[1], m[2])
}

func parseLatLng(s string) (float64, float64, bool) {
	s = strings.TrimSpace(s)
	// First lat,lng pair, so maps `near` can take a `[last pin]` footer line.
	if m := latLngPairRe.FindStringSubmatch(s); m != nil {
		return parseFloatPair(m[1], m[2])
	}
	return 0, 0, false
}

func parseFloatPair(a, b string) (float64, float64, bool) {
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(a), 64)
	lng, err2 := strconv.ParseFloat(strings.TrimSpace(b), 64)
	if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return 0, 0, false
	}
	return lat, lng, true
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
