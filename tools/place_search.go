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

const (
	defaultSearchLimit   = 5
	maxSearchLimit       = 8
	defaultSearchRadiusM = 5000
)

// PlaceHit is one place_search result.
type PlaceHit struct {
	PlaceID    string  `json:"place_id,omitempty"`
	Name       string  `json:"name,omitempty"`
	Address    string  `json:"address,omitempty"`
	Lat        float64 `json:"lat"`
	Lng        float64 `json:"lng"`
	Rating     float64 `json:"rating,omitempty"`
	Ratings    int     `json:"ratings,omitempty"`
	PriceLevel int     `json:"price_level,omitempty"`
	OpenNow    *bool   `json:"open_now,omitempty"`
	URL        string  `json:"url,omitempty"`
}

// PlaceSearchResult is what place_search returns.
type PlaceSearchResult struct {
	Query  string     `json:"query"`
	Near   string     `json:"near,omitempty"`
	Places []PlaceHit `json:"places"`
}

func registerSearch(s *mcpserver.MCPServer) {
	tool := mcp.NewTool(ToolSearch,
		mcp.WithDescription("Find a few places matching a query, optionally near a location. Use for “sushi restaurants near Ballard”, “coffee by the gym”, and “recommend a few spots”. Returns name, rating, address, and a Maps URL for each. Not a city-wide dump — default 5, max 8. Needs GOOGLE_MAPS_API_KEY and Places API. For one known pin use place_resolve."),
		mcp.WithString("query", mcp.Required(), mcp.Description("What to find, e.g. sushi restaurants, coffee, climbing gym.")),
		mcp.WithString("near", mcp.Description("Bias results to a place name, lat,lng, or Maps share URL.")),
		mcp.WithNumber("limit", mcp.Description("How many places to return (default 5, max 8).")),
		mcp.WithReadOnlyHintAnnotation(true),
	)
	registerTool(s, tool, handleSearch)
}

func handleSearch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError(`query is required. ` + nextPlaceSearch), nil
	}
	near := request.GetString("near", "")
	limit := request.GetInt("limit", defaultSearchLimit)
	client, err := newClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	result, err := SearchPlaces(ctx, client, newFetcher(), query, near, limit)
	if err != nil {
		return mcp.NewToolResultError(teachSearch(err, query)), nil
	}
	b, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

// SearchPlaces runs Places Text Search. near biases to a location when set.
func SearchPlaces(ctx context.Context, c *Client, f Fetcher, query, near string, limit int) (PlaceSearchResult, error) {
	query = strings.TrimSpace(query)
	near = strings.TrimSpace(near)
	if query == "" {
		return PlaceSearchResult{}, errors.New(`query is required. ` + nextPlaceSearch)
	}
	if c == nil {
		return PlaceSearchResult{}, errMissingKey
	}
	if limit < 1 {
		limit = defaultSearchLimit
	}
	limit = min(limit, maxSearchLimit)

	location, err := resolveSearchLocation(ctx, c, f, near)
	if err != nil {
		return PlaceSearchResult{}, err
	}
	hits, err := c.TextSearch(ctx, query, location, defaultSearchRadiusM)
	if err != nil {
		return PlaceSearchResult{}, err
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return PlaceSearchResult{Query: query, Near: near, Places: hits}, nil
}

func resolveSearchLocation(ctx context.Context, c *Client, f Fetcher, near string) (string, error) {
	if near == "" {
		return "", nil
	}
	if lat, lng, ok := parseLatLng(near); ok {
		return formatLatLng(lat, lng), nil
	}
	if looksLikeMapsURL(near) {
		link, err := ResolveLink(ctx, f, near)
		if err != nil {
			return "", err
		}
		if link.Lat != nil && link.Lng != nil {
			return formatLatLng(*link.Lat, *link.Lng), nil
		}
		near = firstNonEmpty(link.Name, link.Destination, link.Origin)
		if near == "" {
			return "", errors.New(`could not extract a location from that Maps URL. ` + nextLinkResolve)
		}
	}
	got, err := c.Geocode(ctx, near)
	if err != nil {
		return "", err
	}
	return formatLatLng(got.Lat, got.Lng), nil
}

func teachSearch(err error, query string) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case "REQUEST_DENIED":
			return `GOOGLE_MAPS_API_KEY was rejected or Places API is not enabled. Next: enable Places API on the key, then place_search(query="sushi restaurants", near="Ballard")`
		case "ZERO_RESULTS":
			return fmt.Sprintf("no places found for %q. %s", query, nextPlaceSearch)
		case "OVER_QUERY_LIMIT":
			return `Maps API quota exceeded. Next: wait, then place_search(query="…")`
		}
		if apiErr.HTTPStatus == http.StatusUnauthorized || apiErr.HTTPStatus == http.StatusForbidden {
			return `GOOGLE_MAPS_API_KEY was rejected or Places API is not enabled. Next: enable Places API on the key, then place_search(query="sushi restaurants", near="Ballard")`
		}
	}
	return err.Error()
}
