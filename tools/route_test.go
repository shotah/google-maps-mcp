package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouteETA(t *testing.T) {
	t.Parallel()
	srv := newDirectionsServer(t, directionsOK)
	got, err := RouteETA(context.Background(), testClient(srv), nil, "Seattle", "Portland", "now", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.DurationInTrafficSeconds != 10800 || got.ArrivalTime == "" || got.DepartureTime == "" {
		t.Fatalf("%+v", got)
	}
	if got.Mode != ModeDriving {
		t.Fatalf("mode = %q", got.Mode)
	}
	if !strings.Contains(got.URL, "google.com/maps/dir/") || !strings.Contains(got.URL, "origin=Seattle") || !strings.Contains(got.URL, "destination=Portland") {
		t.Fatalf("url = %q", got.URL)
	}
}

func TestRouteETAShareURLs(t *testing.T) {
	t.Parallel()
	srv := newDirectionsServer(t, directionsOK)
	origin := "https://www.google.com/maps/place/Seattle/@47.6,-122.3,10z"
	dest := "https://www.google.com/maps/dir/Seattle/Portland,+OR"
	got, err := RouteETA(context.Background(), testClient(srv), &stubFetcher{err: errors.New("no fetch")}, origin, dest, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Summary != "I-5 S" {
		t.Fatalf("%+v", got)
	}
}

func TestRouteETAMissingArgs(t *testing.T) {
	t.Parallel()
	_, err := RouteETA(context.Background(), NewClient("x"), nil, "", "Portland", "", "")
	if err == nil || !strings.Contains(err.Error(), "origin is required") {
		t.Fatalf("err = %v", err)
	}
	_, err = RouteETA(context.Background(), NewClient("x"), nil, "Seattle", "", "", "")
	if err == nil || !strings.Contains(err.Error(), "destination is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestRouteETANilClient(t *testing.T) {
	t.Parallel()
	_, err := RouteETA(context.Background(), nil, nil, "a", "b", "", "")
	if err == nil || !errors.Is(err, errMissingKey) {
		t.Fatalf("err = %v", err)
	}
}

func TestRouteETABadDeparture(t *testing.T) {
	t.Parallel()
	_, err := RouteETA(context.Background(), NewClient("x"), nil, "a", "b", "soon", "")
	if err == nil || !strings.Contains(err.Error(), "departure_time") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveWaypoint(t *testing.T) {
	t.Parallel()
	got, err := resolveWaypoint(context.Background(), nil, "Seattle", "origin")
	if err != nil || got != "Seattle" {
		t.Fatalf("%q %v", got, err)
	}
	dir := "https://www.google.com/maps/dir/Seattle/Portland"
	got, err = resolveWaypoint(context.Background(), nil, dir, "origin")
	if err != nil || got != "Seattle" {
		t.Fatalf("origin %q %v", got, err)
	}
	got, err = resolveWaypoint(context.Background(), nil, dir, "destination")
	if err != nil || got != "Portland" {
		t.Fatalf("dest %q %v", got, err)
	}
	place := "https://www.google.com/maps/place/Space+Needle/@47.6205,-122.3493,17z"
	got, err = resolveWaypoint(context.Background(), nil, place, "origin")
	if err != nil || got != "47.6205,-122.3493" {
		t.Fatalf("coords %q %v", got, err)
	}
}

func TestResolveWaypointNameOnly(t *testing.T) {
	t.Parallel()
	place := "https://www.google.com/maps/place/Space+Needle"
	got, err := resolveWaypoint(context.Background(), nil, place, "origin")
	if err != nil || got != "Space Needle" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestRouteETADurationFallback(t *testing.T) {
	t.Parallel()
	payload := map[string]any{
		"status": "OK",
		"routes": []map[string]any{{
			"summary": "local",
			"legs": []map[string]any{{
				"start_address": "A",
				"end_address":   "B",
				"distance":      map[string]any{"text": "1 mi", "value": 1600},
				"duration":      map[string]any{"text": "5 mins", "value": 300},
			}},
		}},
	}
	srv := newDirectionsServer(t, payload)
	got, err := RouteETA(context.Background(), testClient(srv), nil, "A", "B", "now", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.DurationSeconds != 300 || got.ArrivalTime == "" {
		t.Fatalf("%+v", got)
	}
}

func TestResolveWaypointUnknown(t *testing.T) {
	t.Parallel()
	_, err := resolveWaypoint(context.Background(), nil, "https://www.google.com/maps", "origin")
	if err == nil || !strings.Contains(err.Error(), "could not extract a waypoint") {
		t.Fatalf("err = %v", err)
	}
}

func TestTeachRoute(t *testing.T) {
	t.Parallel()
	if !strings.Contains(teachRoute(&APIError{Status: "REQUEST_DENIED"}), "GOOGLE_MAPS_API_KEY") {
		t.Fatal("denied")
	}
	if !strings.Contains(teachRoute(&APIError{Status: "ZERO_RESULTS"}), "no route found") {
		t.Fatal("zero")
	}
	if !strings.Contains(teachRoute(&APIError{Status: "OVER_QUERY_LIMIT"}), "quota") {
		t.Fatal("quota")
	}
	if !strings.Contains(teachRoute(&APIError{HTTPStatus: http.StatusUnauthorized}), "GOOGLE_MAPS_API_KEY") {
		t.Fatal("401")
	}
	if teachRoute(errors.New("boom")) != "boom" {
		t.Fatal("passthrough")
	}
}

func TestNormalizeMode(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":          ModeDriving,
		"driving":   ModeDriving,
		"DRIVE":     ModeDriving,
		"walk":      ModeWalking,
		"bike":      ModeBicycling,
		"bicycling": ModeBicycling,
		"transit":   ModeTransit,
		"train":     ModeTransit,
	}
	for in, want := range cases {
		got, err := normalizeMode(in)
		if err != nil || got != want {
			t.Errorf("normalizeMode(%q) = %q %v, want %q", in, got, err, want)
		}
	}
	if _, err := normalizeMode("hoverboard"); err == nil || !strings.Contains(err.Error(), "mode must be") {
		t.Fatalf("expected bad mode, got %v", err)
	}
}

func TestRouteETABike(t *testing.T) {
	t.Parallel()
	var gotMode, gotDepart string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMode = r.URL.Query().Get("mode")
		gotDepart = r.URL.Query().Get("departure_time")
		_ = json.NewEncoder(w).Encode(directionsOK)
	}))
	t.Cleanup(srv.Close)
	got, err := RouteETA(context.Background(), testClient(srv), nil, "Seattle", "Portland", "now", "bike")
	if err != nil {
		t.Fatal(err)
	}
	if gotMode != ModeBicycling {
		t.Fatalf("api mode = %q", gotMode)
	}
	if gotDepart != "" {
		t.Fatalf("departure_time should be omitted for bike, got %q", gotDepart)
	}
	if got.Mode != ModeBicycling || !strings.Contains(got.URL, "travelmode=bicycling") {
		t.Fatalf("%+v", got)
	}
}

func TestRouteETABadMode(t *testing.T) {
	t.Parallel()
	_, err := RouteETA(context.Background(), NewClient("x"), nil, "a", "b", "", "hoverboard")
	if err == nil || !strings.Contains(err.Error(), "mode must be") {
		t.Fatalf("err = %v", err)
	}
}

func TestHandleRouteMissingArgs(t *testing.T) {
	t.Parallel()
	text := callHandlerErr(t, handleRoute, map[string]any{})
	if !strings.Contains(text, "routes is required") {
		t.Fatalf("text = %q", text)
	}
	text = callHandlerErr(t, handleRoute, map[string]any{"origin": "Seattle", "destination": "Portland"})
	if !strings.Contains(text, "origin is not a parameter") {
		t.Fatalf("text = %q", text)
	}
}

func TestHandleRouteMissingKey(t *testing.T) {
	t.Setenv("GOOGLE_MAPS_API_KEY", "")
	old := newClient
	t.Cleanup(func() { newClient = old })
	newClient = clientFromEnv

	text := callHandlerErr(t, handleRoute, map[string]any{"routes": []map[string]any{{"origin": "a", "destination": "b"}}})
	if !strings.Contains(text, "GOOGLE_MAPS_API_KEY") {
		t.Fatalf("text = %q", text)
	}
}

func TestHandleRouteSuccess(t *testing.T) {
	srv := newDirectionsServer(t, directionsOK)
	oldC := newClient
	t.Cleanup(func() { newClient = oldC })
	newClient = func() (*Client, error) { return testClient(srv), nil }

	text := callHandlerOK(t, handleRoute, map[string]any{
		"routes": []map[string]any{{
			"origin": "Seattle", "destination": "Portland", "departure_time": "now", "mode": "walking",
		}},
	})
	var got struct {
		Results []RouteOutcome `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 1 || got.Results[0].DurationSeconds != 9900 || got.Results[0].Mode != ModeWalking {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(got.Results[0].URL, "travelmode=walking") {
		t.Fatalf("url = %q", got.Results[0].URL)
	}
}

func TestDirectionsURL(t *testing.T) {
	t.Parallel()
	got := directionsURL("Space Needle", "47.6097,-122.3425", ModeDriving)
	if !strings.HasPrefix(got, "https://www.google.com/maps/dir/?") {
		t.Fatalf("prefix = %q", got)
	}
	if !strings.Contains(got, "api=1") || !strings.Contains(got, "travelmode=driving") {
		t.Fatalf("query = %q", got)
	}
	if !strings.Contains(got, "origin=Space+Needle") || !strings.Contains(got, "destination=47.6097%2C-122.3425") {
		t.Fatalf("waypoints = %q", got)
	}
}

func TestHandleRouteAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "NOT_FOUND"})
	}))
	t.Cleanup(srv.Close)
	old := newClient
	t.Cleanup(func() { newClient = old })
	newClient = func() (*Client, error) { return testClient(srv), nil }

	text := callHandlerOK(t, handleRoute, map[string]any{"routes": []map[string]any{{"origin": "a", "destination": "b"}}})
	if !strings.Contains(text, "no route found") {
		t.Fatalf("text = %q", text)
	}
}

func TestHandleLinkMissingURL(t *testing.T) {
	t.Parallel()
	text := callHandlerErr(t, handleLink, map[string]any{})
	if !strings.Contains(text, "urls is required") || !strings.Contains(text, "link_resolve") {
		t.Fatalf("text = %q", text)
	}
}

func TestHandleLinkSuccess(t *testing.T) {
	t.Parallel()
	text := callHandlerOK(t, handleLink, map[string]any{
		"urls": []string{"https://www.google.com/maps/place/Space+Needle/@47.6205,-122.3493,17z"},
	})
	var got struct {
		Results []LinkOutcome `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 1 || got.Results[0].Kind != KindPlace || got.Results[0].Name != "Space Needle" {
		t.Fatalf("%+v", got)
	}
}

func TestHandleLinkError(t *testing.T) {
	t.Parallel()
	text := callHandlerOK(t, handleLink, map[string]any{"urls": []string{"https://example.com/x"}})
	if !strings.Contains(text, "not a Google Maps host") {
		t.Fatalf("text = %q", text)
	}
}

var directionsOK = map[string]any{
	"status": "OK",
	"routes": []map[string]any{{
		"summary": "I-5 S",
		"legs": []map[string]any{{
			"start_address":       "Seattle, WA, USA",
			"end_address":         "Portland, OR, USA",
			"distance":            map[string]any{"text": "174 mi", "value": 280000},
			"duration":            map[string]any{"text": "2 hours 45 mins", "value": 9900},
			"duration_in_traffic": map[string]any{"text": "3 hours", "value": 10800},
		}},
	}},
}

func newDirectionsServer(t *testing.T, payload map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/directions/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(srv.Close)
	return srv
}
