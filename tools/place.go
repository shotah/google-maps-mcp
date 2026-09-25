package tools

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

var errQueryRequired = errors.New(`query is required. ` + nextPlaceResolve)

const (
	maxReviews     = 3
	maxReviewChars = 400
)

// PlaceReview is one Google review snippet.
type PlaceReview struct {
	Author string `json:"author,omitempty"`
	Rating int    `json:"rating,omitempty"`
	Text   string `json:"text,omitempty"`
	When   string `json:"when,omitempty"`
}

// PlaceResult is what place_resolve returns.
type PlaceResult struct {
	Query   string        `json:"query"`
	PlaceID string        `json:"place_id,omitempty"`
	Name    string        `json:"name,omitempty"`
	Address string        `json:"address,omitempty"`
	Lat     float64       `json:"lat"`
	Lng     float64       `json:"lng"`
	Rating  float64       `json:"rating,omitempty"`
	Ratings int           `json:"ratings,omitempty"`
	OpenNow *bool         `json:"open_now,omitempty"`
	URL     string        `json:"url,omitempty"`
	Website string        `json:"website,omitempty"`
	Reviews []PlaceReview `json:"reviews,omitempty"`
}

func registerPlace(s *mcpserver.MCPServer) {
	tool := mcp.NewTool(ToolPlace,
		mcp.WithDescription("Look up one or more places by name, address, or Maps share URL. Pass every lookup in queries (max 8). Returns place_id, coordinates, rating, a few reviews, and a Maps URL for each. Use for “what is this pin” and “tell me about X”. For “sushi near Ballard” use place_search. Needs GOOGLE_MAPS_API_KEY."),
		mcp.WithArray("queries", mcp.Required(), mcp.Description("Place names, addresses, or Maps share / long URLs. 1–8."), mcp.WithStringItems(), mcp.MinItems(1), mcp.MaxItems(maxBatch)),
		mcp.WithReadOnlyHintAnnotation(true),
	)
	registerTool(s, tool, handlePlace)
}

// PlaceOutcome is one place_resolve result. Error is set when that query failed.
type PlaceOutcome struct {
	PlaceResult
	Error string `json:"error,omitempty"`
}

func handlePlace(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := rejectSingular(request, "query", "queries", nextPlaceResolve); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	queries, err := requireStrings(request, "queries", nextPlaceResolve)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	client, err := newClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	f := newFetcher()
	out := make([]PlaceOutcome, 0, len(queries))
	for _, query := range queries {
		got, err := ResolvePlace(ctx, client, f, query)
		if err != nil {
			out = append(out, PlaceOutcome{PlaceResult: PlaceResult{Query: query}, Error: teachPlace(err, query)})
			continue
		}
		out = append(out, PlaceOutcome{PlaceResult: got})
	}
	return batchText(out)
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
		got, err := resolvePlaceFromLink(ctx, c, f, query)
		if err != nil {
			return PlaceResult{}, err
		}
		return enrichPlace(ctx, c, got), nil
	}
	got, err := c.Geocode(ctx, query)
	if err != nil {
		return PlaceResult{}, err
	}
	return enrichPlace(ctx, c, got), nil
}

func enrichPlace(ctx context.Context, c *Client, got PlaceResult) PlaceResult {
	if strings.TrimSpace(got.PlaceID) == "" {
		return got
	}
	details, err := c.PlaceDetails(ctx, got.PlaceID)
	if err != nil {
		return got
	}
	if details.Name != "" {
		got.Name = details.Name
	}
	if details.Address != "" {
		got.Address = details.Address
	}
	got.Rating = details.Rating
	got.Ratings = details.Ratings
	got.OpenNow = details.OpenNow
	got.URL = firstNonEmpty(details.URL, placeMapsURL(got.Name, got.PlaceID))
	got.Website = details.Website
	got.Reviews = details.Reviews
	return got
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
			return `GOOGLE_MAPS_API_KEY was rejected. Next: set a valid Maps Platform key on this process, then ` + nextPlaceResolve[len("Next: "):]
		case "ZERO_RESULTS":
			return fmt.Sprintf("no place found for %q. %s", query, nextPlaceResolve)
		case "OVER_QUERY_LIMIT":
			return `Maps API quota exceeded. Next: wait, then place_resolve(queries=["…"])`
		}
		if apiErr.HTTPStatus == http.StatusUnauthorized || apiErr.HTTPStatus == http.StatusForbidden {
			return `GOOGLE_MAPS_API_KEY was rejected. Next: set a valid Maps Platform key on this process, then ` + nextPlaceResolve[len("Next: "):]
		}
	}
	return err.Error()
}
