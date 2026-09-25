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

func TestResolvePlaceText(t *testing.T) {
	t.Parallel()
	srv := newGeocodeServer(t, geocodeOK)
	got, err := ResolvePlace(context.Background(), testClient(srv), nil, "Space Needle")
	if err != nil {
		t.Fatal(err)
	}
	if got.PlaceID != "ChIJ1" || got.Name != "Space Needle" {
		t.Fatalf("%+v", got)
	}
}

func TestResolvePlaceShareURL(t *testing.T) {
	t.Parallel()
	srv := newGeocodeServer(t, geocodeOK)
	query := "https://www.google.com/maps/place/Space+Needle/@47.6205,-122.3493,17z"
	got, err := ResolvePlace(context.Background(), testClient(srv), &stubFetcher{err: errors.New("no fetch")}, query)
	if err != nil {
		t.Fatal(err)
	}
	if got.Query != query || got.Name != "Space Needle" {
		t.Fatalf("%+v", got)
	}
}

func TestResolvePlaceShareURLNameOnly(t *testing.T) {
	t.Parallel()
	srv := newGeocodeServer(t, geocodeOK)
	query := "https://www.google.com/maps/place/Space+Needle"
	got, err := ResolvePlace(context.Background(), testClient(srv), nil, query)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Space Needle" {
		t.Fatalf("%+v", got)
	}
}

func TestResolvePlaceShareURLReverseError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ZERO_RESULTS"})
	}))
	t.Cleanup(srv.Close)
	query := "https://www.google.com/maps/place/X/@47.6,-122.3,17z"
	_, err := ResolvePlace(context.Background(), testClient(srv), nil, query)
	if err == nil {
		t.Fatal("expected geocode error")
	}
}

func TestResolvePlaceShareURLNoPlace(t *testing.T) {
	t.Parallel()
	_, err := ResolvePlace(context.Background(), NewClient("x"), nil, "https://www.google.com/maps")
	if err == nil || !strings.Contains(err.Error(), "could not extract a place") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolvePlaceEmpty(t *testing.T) {
	t.Parallel()
	_, err := ResolvePlace(context.Background(), NewClient("x"), nil, "  ")
	if err == nil || !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolvePlaceNilClient(t *testing.T) {
	t.Parallel()
	_, err := ResolvePlace(context.Background(), nil, nil, "Seattle")
	if err == nil || !errors.Is(err, errMissingKey) {
		t.Fatalf("err = %v", err)
	}
}

func TestResolvePlaceBadLink(t *testing.T) {
	t.Parallel()
	_, err := ResolvePlace(context.Background(), NewClient("x"), &stubFetcher{err: errors.New("boom")}, "https://maps.app.goo.gl/x")
	if err == nil || err.Error() != "boom" {
		t.Fatalf("err = %v", err)
	}
}

func TestTeachPlace(t *testing.T) {
	t.Parallel()
	if !strings.Contains(teachPlace(&APIError{Status: "REQUEST_DENIED"}, "x"), "GOOGLE_MAPS_API_KEY") {
		t.Fatal("denied")
	}
	if !strings.Contains(teachPlace(&APIError{Status: "ZERO_RESULTS"}, "zzz"), `"zzz"`) {
		t.Fatal("zero")
	}
	if !strings.Contains(teachPlace(&APIError{Status: "OVER_QUERY_LIMIT"}, "x"), "quota") {
		t.Fatal("quota")
	}
	if !strings.Contains(teachPlace(&APIError{HTTPStatus: http.StatusForbidden}, "x"), "GOOGLE_MAPS_API_KEY") {
		t.Fatal("403")
	}
	if teachPlace(errors.New("boom"), "x") != "boom" {
		t.Fatal("passthrough")
	}
}

func TestHandlePlaceMissingQuery(t *testing.T) {
	t.Parallel()
	text := callHandlerErr(t, handlePlace, map[string]any{})
	if !strings.Contains(text, "queries is required") || !strings.Contains(text, "place_resolve") {
		t.Fatalf("teach-in = %q", text)
	}
}

func TestHandlePlaceMissingKey(t *testing.T) {
	t.Setenv("GOOGLE_MAPS_API_KEY", "")
	old := newClient
	t.Cleanup(func() { newClient = old })
	newClient = clientFromEnv

	text := callHandlerErr(t, handlePlace, map[string]any{"queries": []string{"Seattle"}})
	if !strings.Contains(text, "GOOGLE_MAPS_API_KEY") {
		t.Fatalf("text = %q", text)
	}
}

func TestHandlePlaceSuccess(t *testing.T) {
	srv := newGeocodeServer(t, geocodeOK)
	oldC, oldF := newClient, newFetcher
	t.Cleanup(func() { newClient = oldC; newFetcher = oldF })
	newClient = func() (*Client, error) { return testClient(srv), nil }
	newFetcher = func() Fetcher { return &stubFetcher{} }

	text := callHandlerOK(t, handlePlace, map[string]any{"queries": []string{"Space Needle"}})
	var got struct {
		Results []PlaceOutcome `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 1 || got.Results[0].PlaceID != "ChIJ1" {
		t.Fatalf("%+v", got)
	}
}

func TestHandlePlaceAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ZERO_RESULTS"})
	}))
	t.Cleanup(srv.Close)
	old := newClient
	t.Cleanup(func() { newClient = old })
	newClient = func() (*Client, error) { return testClient(srv), nil }

	text := callHandlerOK(t, handlePlace, map[string]any{"queries": []string{"zzz"}})
	if !strings.Contains(text, "no place found") {
		t.Fatalf("text = %q", text)
	}
}

var geocodeOK = map[string]any{
	"status": "OK",
	"results": []map[string]any{{
		"place_id":          "ChIJ1",
		"formatted_address": "400 Broad St, Seattle, WA 98109, USA",
		"address_components": []map[string]any{{
			"long_name": "Space Needle",
			"types":     []string{"point_of_interest"},
		}},
		"geometry": map[string]any{
			"location": map[string]any{"lat": 47.6205, "lng": -122.3493},
		},
	}},
}

func newGeocodeServer(t *testing.T, payload map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/geocode/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(srv.Close)
	return srv
}
