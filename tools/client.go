package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultMapsBase = "https://maps.googleapis.com"

var errMissingKey = errors.New(`GOOGLE_MAPS_API_KEY is not set. Next: set GOOGLE_MAPS_API_KEY on this process (Maps Platform key), then place_resolve(query="Space Needle") or place_search(query="sushi near Ballard")`)

// newClient builds the Maps Platform client. Tests replace this.
var newClient = clientFromEnv

// Client talks to official Google Maps Platform HTTP APIs (Geocoding, Places, Directions).
type Client struct {
	HTTP     *http.Client
	BaseURL  string
	APIKey   string
	MaxBytes int64
}

// APIError is a non-OK Maps Platform response (JSON status or HTTP).
type APIError struct {
	HTTPStatus int
	Status     string
	Detail     string
}

func (e *APIError) Error() string {
	msg := strings.TrimSpace(e.Detail)
	if msg == "" {
		msg = strings.TrimSpace(e.Status)
	}
	if msg == "" {
		msg = fmt.Sprintf("HTTP %d", e.HTTPStatus)
	}
	return "Maps API: " + msg
}

type addressComponent struct {
	LongName string   `json:"long_name"`
	Types    []string `json:"types"`
}

type geocodeResponse struct {
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message"`
	Results      []struct {
		PlaceID           string             `json:"place_id"`
		FormattedAddress  string             `json:"formatted_address"`
		Types             []string           `json:"types"`
		AddressComponents []addressComponent `json:"address_components"`
		Geometry          struct {
			Location struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			} `json:"location"`
		} `json:"geometry"`
	} `json:"results"`
}

type directionsResponse struct {
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message"`
	Routes       []struct {
		Summary string `json:"summary"`
		Legs    []struct {
			StartAddress      string    `json:"start_address"`
			EndAddress        string    `json:"end_address"`
			Distance          textValue `json:"distance"`
			Duration          textValue `json:"duration"`
			DurationInTraffic textValue `json:"duration_in_traffic"`
		} `json:"legs"`
	} `json:"routes"`
}

type textValue struct {
	Text  string `json:"text"`
	Value int    `json:"value"`
}

type latLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type placeJSON struct {
	PlaceID          string  `json:"place_id"`
	Name             string  `json:"name"`
	FormattedAddress string  `json:"formatted_address"`
	Vicinity         string  `json:"vicinity"`
	Rating           float64 `json:"rating"`
	UserRatingsTotal int     `json:"user_ratings_total"`
	PriceLevel       int     `json:"price_level"`
	URL              string  `json:"url"`
	Website          string  `json:"website"`
	Geometry         struct {
		Location latLng `json:"location"`
	} `json:"geometry"`
	OpeningHours *struct {
		OpenNow bool `json:"open_now"`
	} `json:"opening_hours"`
	Reviews []reviewJSON `json:"reviews"`
}

type reviewJSON struct {
	AuthorName              string `json:"author_name"`
	Rating                  int    `json:"rating"`
	Text                    string `json:"text"`
	RelativeTimeDescription string `json:"relative_time_description"`
}

type placesSearchResponse struct {
	Status       string      `json:"status"`
	ErrorMessage string      `json:"error_message"`
	Results      []placeJSON `json:"results"`
}

type placeDetailsResponse struct {
	Status       string    `json:"status"`
	ErrorMessage string    `json:"error_message"`
	Result       placeJSON `json:"result"`
}

const placeDetailsFields = "name,place_id,formatted_address,geometry,rating,user_ratings_total,url,reviews,website,price_level,opening_hours"

func clientFromEnv() (*Client, error) {
	key := strings.TrimSpace(os.Getenv("GOOGLE_MAPS_API_KEY"))
	if key == "" {
		return nil, errMissingKey
	}
	return NewClient(key), nil
}

// NewClient returns a Maps Platform client using apiKey.
func NewClient(apiKey string) *Client {
	return &Client{
		HTTP:     &http.Client{Timeout: defaultTimeout},
		BaseURL:  defaultMapsBase,
		APIKey:   apiKey,
		MaxBytes: defaultMaxBytes,
	}
}

// Geocode resolves address text via the Geocoding API.
func (c *Client) Geocode(ctx context.Context, address string) (PlaceResult, error) {
	q := url.Values{}
	q.Set("address", address)
	return c.geocode(ctx, q, address)
}

// ReverseGeocode resolves coordinates via the Geocoding API.
func (c *Client) ReverseGeocode(ctx context.Context, lat, lng float64) (PlaceResult, error) {
	q := url.Values{}
	q.Set("latlng", formatLatLng(lat, lng))
	return c.geocode(ctx, q, formatLatLng(lat, lng))
}

func (c *Client) geocode(ctx context.Context, q url.Values, query string) (PlaceResult, error) {
	body, status, err := c.get(ctx, "/maps/api/geocode/json", q)
	if err != nil {
		return PlaceResult{}, err
	}
	if status != http.StatusOK {
		return PlaceResult{}, parseAPIError(status, body)
	}
	var resp geocodeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return PlaceResult{}, fmt.Errorf("geocode: %w", err)
	}
	if resp.Status != "OK" || len(resp.Results) == 0 {
		return PlaceResult{}, mapsStatusError(http.StatusOK, resp.Status, resp.ErrorMessage)
	}
	r := resp.Results[0]
	return PlaceResult{
		Query:   query,
		PlaceID: r.PlaceID,
		Name:    placeName(r.AddressComponents, r.FormattedAddress),
		Address: r.FormattedAddress,
		Lat:     r.Geometry.Location.Lat,
		Lng:     r.Geometry.Location.Lng,
	}, nil
}

// TextSearch finds places via the Places Text Search API.
func (c *Client) TextSearch(ctx context.Context, query, location string, radiusM int) ([]PlaceHit, error) {
	q := url.Values{}
	q.Set("query", query)
	if location != "" {
		q.Set("location", location)
		if radiusM < 1 {
			radiusM = defaultSearchRadiusM
		}
		q.Set("radius", strconv.Itoa(radiusM))
	}
	body, status, err := c.get(ctx, "/maps/api/place/textsearch/json", q)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, parseAPIError(status, body)
	}
	var resp placesSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("places search: %w", err)
	}
	if resp.Status != "OK" || len(resp.Results) == 0 {
		return nil, mapsStatusError(http.StatusOK, resp.Status, resp.ErrorMessage)
	}
	out := make([]PlaceHit, 0, len(resp.Results))
	for _, r := range resp.Results {
		out = append(out, placeHitFromJSON(r))
	}
	return out, nil
}

// PlaceDetails loads rating and reviews via the Places Details API.
func (c *Client) PlaceDetails(ctx context.Context, placeID string) (PlaceResult, error) {
	q := url.Values{}
	q.Set("place_id", placeID)
	q.Set("fields", placeDetailsFields)
	body, status, err := c.get(ctx, "/maps/api/place/details/json", q)
	if err != nil {
		return PlaceResult{}, err
	}
	if status != http.StatusOK {
		return PlaceResult{}, parseAPIError(status, body)
	}
	var resp placeDetailsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return PlaceResult{}, fmt.Errorf("place details: %w", err)
	}
	if resp.Status != "OK" || (strings.TrimSpace(resp.Result.PlaceID) == "" && strings.TrimSpace(resp.Result.Name) == "") {
		return PlaceResult{}, mapsStatusError(http.StatusOK, resp.Status, resp.ErrorMessage)
	}
	return placeResultFromJSON(resp.Result), nil
}

func placeHitFromJSON(r placeJSON) PlaceHit {
	addr := firstNonEmpty(r.FormattedAddress, r.Vicinity)
	hit := PlaceHit{
		PlaceID:    r.PlaceID,
		Name:       r.Name,
		Address:    addr,
		Lat:        r.Geometry.Location.Lat,
		Lng:        r.Geometry.Location.Lng,
		Rating:     r.Rating,
		Ratings:    r.UserRatingsTotal,
		PriceLevel: r.PriceLevel,
		URL:        firstNonEmpty(r.URL, placeMapsURL(r.Name, r.PlaceID)),
	}
	if r.OpeningHours != nil {
		open := r.OpeningHours.OpenNow
		hit.OpenNow = &open
	}
	return hit
}

func placeResultFromJSON(r placeJSON) PlaceResult {
	hit := placeHitFromJSON(r)
	return PlaceResult{
		PlaceID: hit.PlaceID,
		Name:    hit.Name,
		Address: hit.Address,
		Lat:     hit.Lat,
		Lng:     hit.Lng,
		Rating:  hit.Rating,
		Ratings: hit.Ratings,
		URL:     hit.URL,
		Website: r.Website,
		OpenNow: hit.OpenNow,
		Reviews: clipReviews(r.Reviews),
	}
}

func clipReviews(in []reviewJSON) []PlaceReview {
	if len(in) == 0 {
		return nil
	}
	n := min(len(in), maxReviews)
	out := make([]PlaceReview, 0, n)
	for _, r := range in[:n] {
		text := strings.TrimSpace(r.Text)
		if len(text) > maxReviewChars {
			text = strings.TrimSpace(text[:maxReviewChars]) + "…"
		}
		out = append(out, PlaceReview{
			Author: r.AuthorName,
			Rating: r.Rating,
			Text:   text,
			When:   r.RelativeTimeDescription,
		})
	}
	return out
}

func placeMapsURL(name, placeID string) string {
	q := url.Values{}
	q.Set("api", "1")
	q.Set("query", firstNonEmpty(name, placeID))
	if placeID != "" {
		q.Set("query_place_id", placeID)
	}
	return "https://www.google.com/maps/search/?" + q.Encode()
}

// Directions returns the first route via the Directions API.
func (c *Client) Directions(ctx context.Context, origin, destination, departureParam, mode string) (RouteResult, error) {
	if mode == "" {
		mode = ModeDriving
	}
	q := url.Values{}
	q.Set("origin", origin)
	q.Set("destination", destination)
	q.Set("mode", mode)
	if departureParam != "" && (mode == ModeDriving || mode == ModeTransit) {
		q.Set("departure_time", departureParam)
	}
	body, status, err := c.get(ctx, "/maps/api/directions/json", q)
	if err != nil {
		return RouteResult{}, err
	}
	if status != http.StatusOK {
		return RouteResult{}, parseAPIError(status, body)
	}
	var resp directionsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return RouteResult{}, fmt.Errorf("directions: %w", err)
	}
	if resp.Status != "OK" || len(resp.Routes) == 0 || len(resp.Routes[0].Legs) == 0 {
		return RouteResult{}, mapsStatusError(http.StatusOK, resp.Status, resp.ErrorMessage)
	}
	route := resp.Routes[0]
	leg := route.Legs[0]
	out := RouteResult{
		Origin:                   firstNonEmpty(leg.StartAddress, origin),
		Destination:              firstNonEmpty(leg.EndAddress, destination),
		DurationSeconds:          leg.Duration.Value,
		DurationText:             leg.Duration.Text,
		DurationInTrafficSeconds: leg.DurationInTraffic.Value,
		DurationInTrafficText:    leg.DurationInTraffic.Text,
		DistanceMeters:           leg.Distance.Value,
		DistanceText:             leg.Distance.Text,
		Summary:                  route.Summary,
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values) ([]byte, int, error) {
	if c == nil || strings.TrimSpace(c.APIKey) == "" {
		return nil, 0, errMissingKey
	}
	base := c.BaseURL
	if base == "" {
		base = defaultMapsBase
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, 0, fmt.Errorf("base url: %w", err)
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	if query == nil {
		query = url.Values{}
	}
	query.Set("key", c.APIKey)
	u.RawQuery = query.Encode()

	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	maxBytes := c.MaxBytes
	if maxBytes < 1 {
		maxBytes = defaultMaxBytes
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", defaultUserAgent())

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	limited := io.LimitReader(resp.Body, maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if int64(len(body)) > maxBytes {
		return nil, resp.StatusCode, fmt.Errorf("response larger than %d bytes", maxBytes)
	}
	return body, resp.StatusCode, nil
}

func parseAPIError(httpStatus int, body []byte) error {
	var parsed struct {
		Status       string `json:"status"`
		ErrorMessage string `json:"error_message"`
	}
	_ = json.Unmarshal(body, &parsed)
	return mapsStatusError(httpStatus, parsed.Status, parsed.ErrorMessage)
}

func mapsStatusError(httpStatus int, status, detail string) error {
	if status == "" && detail == "" {
		detail = fmt.Sprintf("HTTP %d", httpStatus)
	}
	return &APIError{HTTPStatus: httpStatus, Status: status, Detail: strings.TrimSpace(detail)}
}

func placeName(components []addressComponent, formatted string) string {
	for _, c := range components {
		for _, typ := range c.Types {
			switch typ {
			case "establishment", "point_of_interest", "premise", "airport", "park":
				if strings.TrimSpace(c.LongName) != "" {
					return c.LongName
				}
			}
		}
	}
	if formatted == "" {
		return ""
	}
	if i := strings.Index(formatted, ","); i > 0 {
		return strings.TrimSpace(formatted[:i])
	}
	return formatted
}

func formatLatLng(lat, lng float64) string {
	return strconv.FormatFloat(lat, 'f', -1, 64) + "," + strconv.FormatFloat(lng, 'f', -1, 64)
}

func parseDepartureTime(s string) (param string, at time.Time, err error) {
	s = strings.TrimSpace(s)
	now := time.Now().UTC()
	if s == "" || strings.EqualFold(s, "now") {
		return "now", now, nil
	}
	if unix, perr := strconv.ParseInt(s, 10, 64); perr == nil && unix > 1e8 {
		t := time.Unix(unix, 0).UTC()
		return strconv.FormatInt(unix, 10), t, nil
	}
	t, perr := time.Parse(time.RFC3339, s)
	if perr != nil {
		return "", time.Time{}, errors.New(`departure_time must be RFC3339, unix seconds, or "now". ` + nextRouteETA)
	}
	return strconv.FormatInt(t.Unix(), 10), t.UTC(), nil
}
