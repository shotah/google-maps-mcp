package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

var (
	errOriginRequired = errors.New(`origin is required. ` + nextRouteETA)
	errDestRequired   = errors.New(`destination is required. ` + nextRouteETA)
)

// RouteResult is what route_eta returns.
type RouteResult struct {
	Origin                   string `json:"origin"`
	Destination              string `json:"destination"`
	DepartureTime            string `json:"departure_time,omitempty"`
	ArrivalTime              string `json:"arrival_time,omitempty"`
	DurationSeconds          int    `json:"duration_seconds"`
	DurationText             string `json:"duration_text,omitempty"`
	DurationInTrafficSeconds int    `json:"duration_in_traffic_seconds,omitempty"`
	DurationInTrafficText    string `json:"duration_in_traffic_text,omitempty"`
	DistanceMeters           int    `json:"distance_meters,omitempty"`
	DistanceText             string `json:"distance_text,omitempty"`
	Summary                  string `json:"summary,omitempty"`
}

func registerRoute(s *mcpserver.MCPServer) {
	tool := mcp.NewTool(ToolRoute,
		mcp.WithDescription("Get driving duration in traffic and distance between origin and destination. Use for “when do I leave?” and “how long to get there”. Origin and destination can be names, coordinates, or Maps share URLs. Optional departure_time (RFC3339, unix seconds, or now). Needs GOOGLE_MAPS_API_KEY. Official Directions API only — not turn-by-turn steps."),
		mcp.WithString("origin", mcp.Required(), mcp.Description("Start: place name, lat,lng, or Maps share URL.")),
		mcp.WithString("destination", mcp.Required(), mcp.Description("End: place name, lat,lng, or Maps share URL.")),
		mcp.WithString("departure_time", mcp.Description("When you leave: RFC3339, unix seconds, or now (default now). Used for traffic-aware duration.")),
		mcp.WithReadOnlyHintAnnotation(true),
	)
	registerTool(s, tool, handleRoute)
}

func handleRoute(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	origin, err := request.RequireString("origin")
	if err != nil {
		return mcp.NewToolResultError(errOriginRequired.Error()), nil
	}
	dest, err := request.RequireString("destination")
	if err != nil {
		return mcp.NewToolResultError(errDestRequired.Error()), nil
	}
	departure := request.GetString("departure_time", "")
	client, err := newClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	result, err := RouteETA(ctx, client, newFetcher(), origin, dest, departure)
	if err != nil {
		return mcp.NewToolResultError(teachRoute(err)), nil
	}
	b, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

// RouteETA resolves waypoints (including share URLs) and returns traffic-aware duration.
func RouteETA(ctx context.Context, c *Client, f Fetcher, origin, destination, departureTime string) (RouteResult, error) {
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
	got, err := c.Directions(ctx, from, to, param)
	if err != nil {
		return RouteResult{}, err
	}
	got.DepartureTime = departAt.UTC().Format(time.RFC3339)
	secs := got.DurationInTrafficSeconds
	if secs < 1 {
		secs = got.DurationSeconds
	}
	if secs > 0 {
		got.ArrivalTime = departAt.UTC().Add(time.Duration(secs) * time.Second).Format(time.RFC3339)
	}
	return got, nil
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
			return `GOOGLE_MAPS_API_KEY was rejected. Next: set a valid Maps Platform key on this process, then route_eta(origin="Seattle", destination="Portland")`
		case "ZERO_RESULTS", "NOT_FOUND":
			return `no route found. ` + nextRouteETA
		case "OVER_QUERY_LIMIT":
			return `Maps API quota exceeded. Next: wait, then route_eta(origin="…", destination="…")`
		}
		if apiErr.HTTPStatus == http.StatusUnauthorized || apiErr.HTTPStatus == http.StatusForbidden {
			return `GOOGLE_MAPS_API_KEY was rejected. Next: set a valid Maps Platform key on this process, then route_eta(origin="Seattle", destination="Portland")`
		}
	}
	return err.Error()
}
