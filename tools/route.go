package tools

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

var (
	errOriginRequired = errors.New(`origin is required. ` + nextRouteETA)
	errDestRequired   = errors.New(`destination is required. ` + nextRouteETA)
	errBadMode        = errors.New(`mode must be driving, walking, bicycling, or transit. Next: route_eta(routes=[{"origin":"…","destination":"…","mode":"bicycling"}])`)
)

// Official Directions / Maps URLs travel modes.
const (
	ModeDriving   = "driving"
	ModeWalking   = "walking"
	ModeBicycling = "bicycling"
	ModeTransit   = "transit"
)

// RouteResult is what route_eta returns.
type RouteResult struct {
	Origin                   string `json:"origin"`
	Destination              string `json:"destination"`
	Mode                     string `json:"mode,omitempty"`
	DepartureTime            string `json:"departure_time,omitempty"`
	ArrivalTime              string `json:"arrival_time,omitempty"`
	DurationSeconds          int    `json:"duration_seconds"`
	DurationText             string `json:"duration_text,omitempty"`
	DurationInTrafficSeconds int    `json:"duration_in_traffic_seconds,omitempty"`
	DurationInTrafficText    string `json:"duration_in_traffic_text,omitempty"`
	DistanceMeters           int    `json:"distance_meters,omitempty"`
	DistanceText             string `json:"distance_text,omitempty"`
	Summary                  string `json:"summary,omitempty"`
	URL                      string `json:"url,omitempty"`
}

func registerRoute(s *mcpserver.MCPServer) {
	tool := mcp.NewTool(ToolRoute,
		mcp.WithDescription("Get duration, distance, and a tap-to-open Google Maps directions URL for each route. Pass every trip in routes (max 8). Use for “when do I leave?”, “how far by bike”, and “walking directions”. Origin and destination can be names, coordinates, or Maps share URLs. Optional mode (driving, walking, bicycling, transit) and departure_time (RFC3339, unix seconds, or now) on each route. Needs GOOGLE_MAPS_API_KEY. Official Directions API only — not turn-by-turn steps."),
		mcp.WithArray("routes", mcp.Required(), mcp.Description("1–8 trips. Each object has origin, destination, and optional mode and departure_time."), mcp.MinItems(1), mcp.MaxItems(maxBatch)),
		mcp.WithReadOnlyHintAnnotation(true),
	)
	registerTool(s, tool, handleRoute)
}

// RouteOutcome is one route_eta result. Error is set when that trip failed.
type RouteOutcome struct {
	RouteResult
	Error string `json:"error,omitempty"`
}

func handleRoute(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	if _, ok := args["origin"]; ok {
		return mcp.NewToolResultError(rejectSingular(request, "origin", "routes", nextRouteETA).Error()), nil
	}
	if _, ok := args["destination"]; ok {
		return mcp.NewToolResultError(rejectSingular(request, "destination", "routes", nextRouteETA).Error()), nil
	}
	trips, err := requireObjects(request, "routes", nextRouteETA)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	client, err := newClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	f := newFetcher()
	out := make([]RouteOutcome, 0, len(trips))
	for _, trip := range trips {
		origin := strField(trip, "origin")
		dest := strField(trip, "destination")
		got, err := RouteETA(ctx, client, f, origin, dest, strField(trip, "departure_time"), strField(trip, "mode"))
		if err != nil {
			out = append(out, RouteOutcome{RouteResult: RouteResult{Origin: origin, Destination: dest}, Error: teachRoute(err)})
			continue
		}
		out = append(out, RouteOutcome{RouteResult: got})
	}
	return batchText(out)
}

// RouteETA resolves waypoints (including share URLs) and returns traffic-aware duration.
func RouteETA(ctx context.Context, c *Client, f Fetcher, origin, destination, departureTime, mode string) (RouteResult, error) {
	origin = strings.TrimSpace(origin)
	destination = strings.TrimSpace(destination)
	if origin == "" {
		return RouteResult{}, errOriginRequired
	}
	if destination == "" {
		return RouteResult{}, errDestRequired
	}
	if c == nil {
		return RouteResult{}, errMissingKey
	}
	mode, err := normalizeMode(mode)
	if err != nil {
		return RouteResult{}, err
	}
	from, err := resolveWaypoint(ctx, f, origin, "origin")
	if err != nil {
		return RouteResult{}, err
	}
	to, err := resolveWaypoint(ctx, f, destination, "destination")
	if err != nil {
		return RouteResult{}, err
	}
	param, departAt, err := parseDepartureTime(departureTime)
	if err != nil {
		return RouteResult{}, err
	}
	got, err := c.Directions(ctx, from, to, param, mode)
	if err != nil {
		return RouteResult{}, err
	}
	got.Mode = mode
	got.DepartureTime = departAt.UTC().Format(time.RFC3339)
	secs := got.DurationInTrafficSeconds
	if secs < 1 {
		secs = got.DurationSeconds
	}
	if secs > 0 {
		got.ArrivalTime = departAt.UTC().Add(time.Duration(secs) * time.Second).Format(time.RFC3339)
	}
	got.URL = directionsURL(from, to, mode)
	return got, nil
}

func normalizeMode(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", ModeDriving, "drive", "car":
		return ModeDriving, nil
	case ModeWalking, "walk":
		return ModeWalking, nil
	case ModeBicycling, "bicycle", "bike", "cycling":
		return ModeBicycling, nil
	case ModeTransit, "bus", "train":
		return ModeTransit, nil
	default:
		return "", errBadMode
	}
}

// directionsURL is the official Maps URLs directions link (opens nav on a phone).
func directionsURL(origin, destination, mode string) string {
	q := url.Values{}
	q.Set("api", "1")
	q.Set("origin", origin)
	q.Set("destination", destination)
	q.Set("travelmode", mode)
	return "https://www.google.com/maps/dir/?" + q.Encode()
}

func resolveWaypoint(ctx context.Context, f Fetcher, raw, prefer string) (string, error) {
	if !looksLikeMapsURL(raw) {
		return raw, nil
	}
	link, err := ResolveLink(ctx, f, raw)
	if err != nil {
		return "", err
	}
	if prefer == "origin" && link.Origin != "" {
		return link.Origin, nil
	}
	if prefer == "destination" && link.Destination != "" {
		return link.Destination, nil
	}
	if link.Lat != nil && link.Lng != nil {
		return formatLatLng(*link.Lat, *link.Lng), nil
	}
	if name := firstNonEmpty(link.Name, link.Destination, link.Origin); name != "" {
		return name, nil
	}
	return "", errors.New(`could not extract a waypoint from that Maps URL. ` + nextLinkResolve)
}

func teachRoute(err error) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case "REQUEST_DENIED":
			return `GOOGLE_MAPS_API_KEY was rejected. Next: set a valid Maps Platform key on this process, then ` + nextRouteETA[len("Next: "):]
		case "ZERO_RESULTS", "NOT_FOUND":
			return `no route found. ` + nextRouteETA
		case "OVER_QUERY_LIMIT":
			return `Maps API quota exceeded. Next: wait, then route_eta(routes=[{"origin":"…","destination":"…"}])`
		}
		if apiErr.HTTPStatus == http.StatusUnauthorized || apiErr.HTTPStatus == http.StatusForbidden {
			return `GOOGLE_MAPS_API_KEY was rejected. Next: set a valid Maps Platform key on this process, then ` + nextRouteETA[len("Next: "):]
		}
	}
	return err.Error()
}
