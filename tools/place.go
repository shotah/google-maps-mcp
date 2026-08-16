package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

var errQueryRequired = errors.New(`query is required. ` + nextPlaceResolve)

// PlaceResult is what place_resolve returns.
type PlaceResult struct {
	Query   string  `json:"query"`
	PlaceID string  `json:"place_id,omitempty"`
	Name    string  `json:"name,omitempty"`
	Address string  `json:"address,omitempty"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
}

func registerPlace(s *mcpserver.MCPServer) {
	tool := mcp.NewTool(ToolPlace,
		mcp.WithDescription("Resolve a place name or address to place_id, coordinates, and name. Use for “what is this pin” and “where is X”. If query is a Maps share URL, expands it first (same hop as link_resolve). Needs GOOGLE_MAPS_API_KEY. Official Geocoding API only — not a scrape."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Place name, address, or Maps share / long URL.")),
		mcp.WithReadOnlyHintAnnotation(true),
	)
	registerTool(s, tool, handlePlace)
}

func handlePlace(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError(errQueryRequired.Error()), nil
	}
	client, err := newClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	result, err := ResolvePlace(ctx, client, newFetcher(), query)
	if err != nil {
		return mcp.NewToolResultError(teachPlace(err, query)), nil
	}
	b, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

// ResolvePlace geocodes query. Maps share URLs are expanded first.
func ResolvePlace(ctx context.Context, c *Client, f Fetcher, query string) (PlaceResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return PlaceResult{}, errQueryRequired
	}
	if c == nil {
		return PlaceResult{}, errMissingKey
	}
	if looksLikeMapsURL(query) {
		return resolvePlaceFromLink(ctx, c, f, query)
	}
	return c.Geocode(ctx, query)
}

func resolvePlaceFromLink(ctx context.Context, c *Client, f Fetcher, query string) (PlaceResult, error) {
	link, err := ResolveLink(ctx, f, query)
	if err != nil {
		return PlaceResult{}, err
	}
	if link.Lat != nil && link.Lng != nil {
		got, err := c.ReverseGeocode(ctx, *link.Lat, *link.Lng)
		if err != nil {
			return PlaceResult{}, err
		}
		if link.Name != "" {
			got.Name = link.Name
		}
		got.Query = query
		return got, nil
	}
	text := firstNonEmpty(link.Name, link.Destination, link.Origin)
	if text == "" {
		return PlaceResult{}, errors.New(`could not extract a place from that Maps URL. ` + nextLinkResolve)
	}
	got, err := c.Geocode(ctx, text)
	if err != nil {
		return PlaceResult{}, err
	}
	got.Query = query
	return got, nil
}

func teachPlace(err error, query string) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case "REQUEST_DENIED":
			return `GOOGLE_MAPS_API_KEY was rejected. Next: set a valid Maps Platform key on this process, then place_resolve(query="Space Needle")`
		case "ZERO_RESULTS":
			return fmt.Sprintf("no place found for %q. %s", query, nextPlaceResolve)
		case "OVER_QUERY_LIMIT":
			return `Maps API quota exceeded. Next: wait, then place_resolve(query="…")`
		}
		if apiErr.HTTPStatus == http.StatusUnauthorized || apiErr.HTTPStatus == http.StatusForbidden {
			return `GOOGLE_MAPS_API_KEY was rejected. Next: set a valid Maps Platform key on this process, then place_resolve(query="Space Needle")`
		}
	}
	return err.Error()
}
