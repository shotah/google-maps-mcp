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
	got, err := RouteETA(context.Background(), testClient(srv), nil, "Seattle", "Portland", "now")
	if err != nil {
		t.Fatal(err)
	}
	if got.DurationInTrafficSeconds != 10800 || got.ArrivalTime == "" || got.DepartureTime == "" {
		t.Fatalf("%+v", got)
	}
}

func TestRouteETAShareURLs(t *testing.T) {
	t.Parallel()
	srv := newDirectionsServer(t, directionsOK)
	origin := "https://www.google.com/maps/place/Seattle/@47.6,-122.3,10z"
	dest := "https://www.google.com/maps/dir/Seattle/Portland,+OR"
	got, err := RouteETA(context.Background(), testClient(srv), &stubFetcher{err: errors.New("no fetch")}, origin, dest, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Summary != "I-5 S" {
		t.Fatalf("%+v", got)
	}
}

func TestRouteETAMissingArgs(t *testing.T) {
	t.Parallel()
	_, err := RouteETA(context.Background(), NewClient("x"), nil, "", "Portland", "")
	if err == nil || !strings.Contains(err.Error(), "origin is required") {
		t.Fatalf("err = %v", err)
	}
	_, err = RouteETA(context.Background(), NewClient("x"), nil, "Seattle", "", "")
	if err == nil || !strings.Contains(err.Error(), "destination is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestRouteETANilClient(t *testing.T) {
	t.Parallel()
	_, err := RouteETA(context.Background(), nil, nil, "a", "b", "")
	if err == nil || !errors.Is(err, errMissingKey) {
		t.Fatalf("err = %v", err)
	}
}

func TestRouteETABadDeparture(t *testing.T) {
	t.Parallel()
	_, err := RouteETA(context.Background(), NewClient("x"), nil, "a", "b", "soon")
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
	got, err := RouteETA(context.Background(), testClient(srv), nil, "A", "B", "now")
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

func TestHandleRouteMissingArgs(t *testing.T) {
	t.Parallel()
	text := callHandlerErr(t, handleRoute, map[string]any{})
	if !strings.Contains(text, "origin is required") {
		t.Fatalf("text = %q", text)
	}
	text = callHandlerErr(t, handleRoute, map[string]any{"origin": "Seattle"})
	if !strings.Contains(text, "destination is required") {
		t.Fatalf("text = %q", text)
	}
}

func TestHandleRouteMissingKey(t *testing.T) {
	t.Setenv("GOOGLE_MAPS_API_KEY", "")
	old := newClient
	t.Cleanup(func() { newClient = old })
	newClient = clientFromEnv

	text := callHandlerErr(t, handleRoute, map[string]any{"origin": "a", "destination": "b"})
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
		"origin": "Seattle", "destination": "Portland", "departure_time": "now",
	})
	var got RouteResult
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatal(err)
	}
	if got.DurationSeconds != 9900 {
		t.Fatalf("%+v", got)
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

	text := callHandlerErr(t, handleRoute, map[string]any{"origin": "a", "destination": "b"})
	if !strings.Contains(text, "no route found") {
		t.Fatalf("text = %q", text)
	}
}

func TestHandleLinkMissingURL(t *testing.T) {
	t.Parallel()
	text := callHandlerErr(t, handleLink, map[string]any{})
	if !strings.Contains(text, "url is required") || !strings.Contains(text, "link_resolve") {
		t.Fatalf("text = %q", text)
	}
}

func TestHandleLinkSuccess(t *testing.T) {
	t.Parallel()
	text := callHandlerOK(t, handleLink, map[string]any{
		"url": "https://www.google.com/maps/place/Space+Needle/@47.6205,-122.3493,17z",
	})
	var got LinkResult
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindPlace || got.Name != "Space Needle" {
		t.Fatalf("%+v", got)
	}
}

func TestHandleLinkError(t *testing.T) {
	t.Parallel()
	text := callHandlerErr(t, handleLink, map[string]any{"url": "https://example.com/x"})
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
