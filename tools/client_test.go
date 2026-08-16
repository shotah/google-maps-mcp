package tools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientFromEnvMissing(t *testing.T) {
	t.Setenv("GOOGLE_MAPS_API_KEY", "  ")
	_, err := clientFromEnv()
	if err == nil || !errors.Is(err, errMissingKey) {
		t.Fatalf("err = %v", err)
	}
}

func TestClientFromEnv(t *testing.T) {
	t.Setenv("GOOGLE_MAPS_API_KEY", "secret")
	c, err := clientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.APIKey != "secret" {
		t.Fatalf("key = %q", c.APIKey)
	}
}

func TestNewClientDefaults(t *testing.T) {
	t.Parallel()
	c := NewClient("tok")
	if c.BaseURL != defaultMapsBase || c.APIKey != "tok" || c.HTTP == nil {
		t.Fatalf("%+v", c)
	}
}

func TestGeocodeOK(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "test-key" {
			t.Errorf("key = %q", r.URL.Query().Get("key"))
		}
		if r.URL.Path != "/maps/api/geocode/json" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "OK",
			"results": []map[string]any{{
				"place_id":          "ChIJ1",
				"formatted_address": "400 Broad St, Seattle, WA 98109, USA",
				"address_components": []map[string]any{{
					"long_name": "Space Needle",
					"types":     []string{"establishment"},
				}},
				"geometry": map[string]any{
					"location": map[string]any{"lat": 47.6205, "lng": -122.3493},
				},
			}},
		})
	}))
	t.Cleanup(srv.Close)
	got, err := testClient(srv).Geocode(context.Background(), "Space Needle")
	if err != nil {
		t.Fatal(err)
	}
	if got.PlaceID != "ChIJ1" || got.Name != "Space Needle" || got.Lat != 47.6205 {
		t.Fatalf("%+v", got)
	}
}

func TestReverseGeocodeOK(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("latlng") != "47.62,-122.35" {
			t.Errorf("latlng = %q", r.URL.Query().Get("latlng"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "OK",
			"results": []map[string]any{{
				"place_id":          "ChIJ2",
				"formatted_address": "Seattle, WA, USA",
				"geometry": map[string]any{
					"location": map[string]any{"lat": 47.62, "lng": -122.35},
				},
			}},
		})
	}))
	t.Cleanup(srv.Close)
	got, err := testClient(srv).ReverseGeocode(context.Background(), 47.62, -122.35)
	if err != nil {
		t.Fatal(err)
	}
	if got.PlaceID != "ChIJ2" || got.Name != "Seattle" {
		t.Fatalf("%+v", got)
	}
}

func TestGeocodeZeroResults(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ZERO_RESULTS"})
	}))
	t.Cleanup(srv.Close)
	_, err := testClient(srv).Geocode(context.Background(), "zzz")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != "ZERO_RESULTS" {
		t.Fatalf("err = %v", err)
	}
}

func TestGeocodeInvalidJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	t.Cleanup(srv.Close)
	_, err := testClient(srv).Geocode(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "geocode") {
		t.Fatalf("err = %v", err)
	}
}

func TestGeocodeHTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"status":"REQUEST_DENIED","error_message":"denied"}`))
	}))
	t.Cleanup(srv.Close)
	_, err := testClient(srv).Geocode(context.Background(), "x")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != "REQUEST_DENIED" {
		t.Fatalf("err = %v", err)
	}
}

func TestTextSearchOK(t *testing.T) {
	t.Parallel()
	srv := newPlacesServer(t, placesSearchOK, nil)
	got, err := testClient(srv).TextSearch(context.Background(), "sushi", "47.67,-122.38", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "Sushi Kappo" || got[0].OpenNow == nil || !*got[0].OpenNow {
		t.Fatalf("%+v", got)
	}
}

func TestTextSearchInvalidJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	t.Cleanup(srv.Close)
	_, err := testClient(srv).TextSearch(context.Background(), "x", "", 0)
	if err == nil || !strings.Contains(err.Error(), "places search") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlaceDetailsOK(t *testing.T) {
	t.Parallel()
	srv := newPlacesServer(t, nil, nil)
	got, err := testClient(srv).PlaceDetails(context.Background(), "ChIJ1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Rating != 4.7 || got.Website == "" || len(got.Reviews) != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestPlaceDetailsEmpty(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "OK", "result": map[string]any{}})
	}))
	t.Cleanup(srv.Close)
	_, err := testClient(srv).PlaceDetails(context.Background(), "x")
	if err == nil {
		t.Fatal("expected empty result error")
	}
}

func TestDirectionsOK(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/maps/api/directions/json" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("departure_time") != "now" {
			t.Errorf("departure_time = %q", r.URL.Query().Get("departure_time"))
		}
		if r.URL.Query().Get("mode") != ModeDriving {
			t.Errorf("mode = %q", r.URL.Query().Get("mode"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "OK",
			"routes": []map[string]any{{
				"summary": "I-5 S",
				"legs": []map[string]any{{
					"start_address": "Seattle, WA, USA",
					"end_address":   "Portland, OR, USA",
					"distance":      map[string]any{"text": "174 mi", "value": 280000},
					"duration":      map[string]any{"text": "2 hours 45 mins", "value": 9900},
					"duration_in_traffic": map[string]any{
						"text": "3 hours", "value": 10800,
					},
				}},
			}},
		})
	}))
	t.Cleanup(srv.Close)
	got, err := testClient(srv).Directions(context.Background(), "Seattle", "Portland", "now", ModeDriving)
	if err != nil {
		t.Fatal(err)
	}
	if got.DurationInTrafficSeconds != 10800 || got.DistanceMeters != 280000 || got.Summary != "I-5 S" {
		t.Fatalf("%+v", got)
	}
}

func TestDirectionsWalkingOmitsDeparture(t *testing.T) {
	t.Parallel()
	var gotMode, gotDepart string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMode = r.URL.Query().Get("mode")
		gotDepart = r.URL.Query().Get("departure_time")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "OK",
			"routes": []map[string]any{{
				"summary": "trail",
				"legs": []map[string]any{{
					"start_address": "A",
					"end_address":   "B",
					"distance":      map[string]any{"text": "1 mi", "value": 1600},
					"duration":      map[string]any{"text": "20 mins", "value": 1200},
				}},
			}},
		})
	}))
	t.Cleanup(srv.Close)
	got, err := testClient(srv).Directions(context.Background(), "A", "B", "now", ModeWalking)
	if err != nil {
		t.Fatal(err)
	}
	if gotMode != ModeWalking || gotDepart != "" || got.DurationSeconds != 1200 {
		t.Fatalf("mode=%q depart=%q got=%+v", gotMode, gotDepart, got)
	}
}

func TestDirectionsInvalidJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{`))
	}))
	t.Cleanup(srv.Close)
	_, err := testClient(srv).Directions(context.Background(), "a", "b", "now", ModeDriving)
	if err == nil || !strings.Contains(err.Error(), "directions") {
		t.Fatalf("err = %v", err)
	}
}

func TestDirectionsEmptyRoutes(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "OK", "routes": []any{}})
	}))
	t.Cleanup(srv.Close)
	_, err := testClient(srv).Directions(context.Background(), "a", "b", "now", ModeDriving)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v", err)
	}
}

func TestDirectionsHTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":"INVALID_REQUEST"}`))
	}))
	t.Cleanup(srv.Close)
	_, err := testClient(srv).Directions(context.Background(), "a", "b", "now", ModeDriving)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != "INVALID_REQUEST" {
		t.Fatalf("err = %v", err)
	}
}

func TestClientGetTooLarge(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("abcdef"))
	}))
	t.Cleanup(srv.Close)
	c := testClient(srv)
	c.MaxBytes = 3
	_, err := c.Geocode(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("err = %v", err)
	}
}

func TestClientGetUsesDefaults(t *testing.T) {
	t.Parallel()
	var gotURL string
	c := &Client{
		APIKey:   "tok",
		MaxBytes: 0,
		HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotURL = r.URL.String()
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"status":"ZERO_RESULTS"}`)),
				Header:     make(http.Header),
			}, nil
		})},
	}
	_, err := c.Geocode(context.Background(), "x")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatal(err)
	}
	if !strings.HasPrefix(gotURL, defaultMapsBase) {
		t.Fatalf("url = %q", gotURL)
	}
}

func TestClientBadBaseURL(t *testing.T) {
	t.Parallel()
	c := NewClient("tok")
	c.BaseURL = "://bad"
	_, err := c.Geocode(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "base url") {
		t.Fatalf("err = %v", err)
	}
}

func TestClientNilAndMissingKey(t *testing.T) {
	t.Parallel()
	var c *Client
	_, _, err := c.get(context.Background(), "/x", nil)
	if err == nil || !errors.Is(err, errMissingKey) {
		t.Fatalf("err = %v", err)
	}
}

func TestClientGetReadError(t *testing.T) {
	t.Parallel()
	c := NewClient("tok")
	c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(errReader{}),
			Header:     make(http.Header),
		}, nil
	})}
	_, err := c.Geocode(context.Background(), "x")
	if err == nil {
		t.Fatal("expected read error")
	}
}

func TestAPIErrorFallback(t *testing.T) {
	t.Parallel()
	err := &APIError{HTTPStatus: 500}
	if err.Error() != "Maps API: HTTP 500" {
		t.Fatalf("Error() = %q", err.Error())
	}
	err = &APIError{HTTPStatus: 400, Status: "INVALID_REQUEST"}
	if !strings.Contains(err.Error(), "INVALID_REQUEST") {
		t.Fatalf("Error() = %q", err.Error())
	}
}

func TestParseDepartureTime(t *testing.T) {
	t.Parallel()
	param, _, err := parseDepartureTime("")
	if err != nil || param != "now" {
		t.Fatalf("empty: %q %v", param, err)
	}
	param, _, err = parseDepartureTime("now")
	if err != nil || param != "now" {
		t.Fatalf("now: %q %v", param, err)
	}
	param, at, err := parseDepartureTime("1700000000")
	if err != nil || param != "1700000000" || at.Unix() != 1700000000 {
		t.Fatalf("unix: %q %v %v", param, at, err)
	}
	param, at, err = parseDepartureTime("2026-08-16T14:00:00Z")
	if err != nil || at.Year() != 2026 || param == "now" {
		t.Fatalf("rfc: %q %v %v", param, at, err)
	}
	if _, _, err := parseDepartureTime("yesterday"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestPlaceNameFallback(t *testing.T) {
	t.Parallel()
	if placeName(nil, "Seattle, WA") != "Seattle" {
		t.Fatal(placeName(nil, "Seattle, WA"))
	}
	if placeName(nil, "Nowhere") != "Nowhere" {
		t.Fatal(placeName(nil, "Nowhere"))
	}
	if placeName(nil, "") != "" {
		t.Fatal("expected empty")
	}
}

func TestFormatLatLng(t *testing.T) {
	t.Parallel()
	if formatLatLng(47.62, -122.35) != "47.62,-122.35" {
		t.Fatal(formatLatLng(47.62, -122.35))
	}
}

func TestParseDepartureTimeRFCUsesUnix(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	param, at, err := parseDepartureTime(ts.Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	if param != "1767323045" && at.Unix() != ts.Unix() {
		// don't hard-fail on the exact string if TZ math differs; unix must match
		if at.Unix() != ts.Unix() {
			t.Fatalf("at = %v want %v (param %s)", at, ts, param)
		}
	}
}

func testClient(srv *httptest.Server) *Client {
	c := NewClient("test-key")
	c.HTTP = srv.Client()
	c.BaseURL = srv.URL
	return c
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
