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

func TestSearchPlaces(t *testing.T) {
	t.Parallel()
	srv := newPlacesServer(t, placesSearchOK, nil)
	got, err := SearchPlaces(context.Background(), testClient(srv), nil, "sushi restaurants", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Places) != 2 || got.Places[0].Name != "Sushi Kappo" || got.Places[0].Rating != 4.6 {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(got.Places[0].URL, "query_place_id=ChIJ-sushi") {
		t.Fatalf("url = %q", got.Places[0].URL)
	}
}

func TestSearchPlacesNearCoords(t *testing.T) {
	t.Parallel()
	var gotLoc, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/place/textsearch/") {
			gotLoc = r.URL.Query().Get("location")
			gotQuery = r.URL.Query().Get("query")
			_ = json.NewEncoder(w).Encode(placesSearchOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	got, err := SearchPlaces(context.Background(), testClient(srv), nil, "sushi", "47.67,-122.38", 5)
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "sushi" || gotLoc != "47.67,-122.38" || got.Near != "47.67,-122.38" {
		t.Fatalf("query=%q loc=%q near=%q", gotQuery, gotLoc, got.Near)
	}
}

func TestSearchPlacesNearName(t *testing.T) {
	t.Parallel()
	srv := newPlacesServer(t, placesSearchOK, geocodeOK)
	got, err := SearchPlaces(context.Background(), testClient(srv), nil, "coffee", "Ballard", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Query != "coffee" || len(got.Places) != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestSearchPlacesLimit(t *testing.T) {
	t.Parallel()
	srv := newPlacesServer(t, placesSearchOK, nil)
	got, err := SearchPlaces(context.Background(), testClient(srv), nil, "sushi", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Places) != 1 {
		t.Fatalf("len = %d", len(got.Places))
	}
}

func TestSearchPlacesEmpty(t *testing.T) {
	t.Parallel()
	_, err := SearchPlaces(context.Background(), NewClient("x"), nil, "  ", "", 5)
	if err == nil || !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestSearchPlacesNilClient(t *testing.T) {
	t.Parallel()
	_, err := SearchPlaces(context.Background(), nil, nil, "sushi", "", 5)
	if err == nil || !errors.Is(err, errMissingKey) {
		t.Fatalf("err = %v", err)
	}
}

func TestSearchPlacesZeroResults(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ZERO_RESULTS"})
	}))
	t.Cleanup(srv.Close)
	_, err := SearchPlaces(context.Background(), testClient(srv), nil, "zzz", "", 5)
	if err == nil || !strings.Contains(teachSearch(err, "zzz"), `"zzz"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestTeachSearch(t *testing.T) {
	t.Parallel()
	if !strings.Contains(teachSearch(&APIError{Status: "REQUEST_DENIED"}, "x"), "Places API") {
		t.Fatal("denied")
	}
	if !strings.Contains(teachSearch(&APIError{Status: "OVER_QUERY_LIMIT"}, "x"), "quota") {
		t.Fatal("quota")
	}
	if !strings.Contains(teachSearch(&APIError{HTTPStatus: http.StatusForbidden}, "x"), "Places API") {
		t.Fatal("403")
	}
	if teachSearch(errors.New("boom"), "x") != "boom" {
		t.Fatal("passthrough")
	}
}

func TestHandleSearchMissingQuery(t *testing.T) {
	t.Parallel()
	text := callHandlerErr(t, handleSearch, map[string]any{})
	if !strings.Contains(text, "queries is required") || !strings.Contains(text, "place_search") {
		t.Fatalf("text = %q", text)
	}
}

func TestHandleSearchSuccess(t *testing.T) {
	srv := newPlacesServer(t, placesSearchOK, nil)
	old := newClient
	t.Cleanup(func() { newClient = old })
	newClient = func() (*Client, error) { return testClient(srv), nil }

	text := callHandlerOK(t, handleSearch, map[string]any{"queries": []string{"sushi"}, "limit": 2})
	var got struct {
		Results []SearchOutcome `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 1 || len(got.Results[0].Places) != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestHandleSearchMissingKey(t *testing.T) {
	t.Setenv("GOOGLE_MAPS_API_KEY", "")
	old := newClient
	t.Cleanup(func() { newClient = old })
	newClient = clientFromEnv

	text := callHandlerErr(t, handleSearch, map[string]any{"queries": []string{"sushi"}})
	if !strings.Contains(text, "GOOGLE_MAPS_API_KEY") {
		t.Fatalf("text = %q", text)
	}
}

func TestResolvePlaceEnrichesReviews(t *testing.T) {
	t.Parallel()
	srv := newPlacesServer(t, nil, geocodeOK)
	got, err := ResolvePlace(context.Background(), testClient(srv), nil, "Space Needle")
	if err != nil {
		t.Fatal(err)
	}
	if got.Rating != 4.7 || len(got.Reviews) != 1 || got.Reviews[0].Author != "Sam" {
		t.Fatalf("%+v", got)
	}
}

func TestClipReviews(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", maxReviewChars+20)
	got := clipReviews([]reviewJSON{
		{AuthorName: "a", Rating: 5, Text: long, RelativeTimeDescription: "1w"},
		{AuthorName: "b", Rating: 4, Text: "ok", RelativeTimeDescription: "2w"},
		{AuthorName: "c", Rating: 3, Text: "meh", RelativeTimeDescription: "3w"},
		{AuthorName: "d", Rating: 2, Text: "no", RelativeTimeDescription: "4w"},
	})
	if len(got) != maxReviews {
		t.Fatalf("len = %d", len(got))
	}
	if !strings.HasSuffix(got[0].Text, "…") || len(got[0].Text) < maxReviewChars {
		t.Fatalf("clip = %q", got[0].Text)
	}
}

var placesSearchOK = map[string]any{
	"status": "OK",
	"results": []map[string]any{
		{
			"place_id":           "ChIJ-sushi",
			"name":               "Sushi Kappo",
			"formatted_address":  "123 Ballard Ave, Seattle, WA",
			"rating":             4.6,
			"user_ratings_total": 210,
			"geometry":           map[string]any{"location": map[string]any{"lat": 47.67, "lng": -122.38}},
			"opening_hours":      map[string]any{"open_now": true},
		},
		{
			"place_id": "ChIJ-omakase",
			"name":     "Omakase Bar",
			"vicinity": "Ballard",
			"rating":   4.4,
			"geometry": map[string]any{"location": map[string]any{"lat": 47.66, "lng": -122.37}},
		},
	},
}

var placeDetailsOK = map[string]any{
	"status": "OK",
	"result": map[string]any{
		"place_id":           "ChIJ1",
		"name":               "Space Needle",
		"formatted_address":  "400 Broad St, Seattle, WA 98109, USA",
		"rating":             4.7,
		"user_ratings_total": 90000,
		"url":                "https://maps.google.com/?cid=1",
		"website":            "https://www.spaceneedle.com/",
		"geometry":           map[string]any{"location": map[string]any{"lat": 47.6205, "lng": -122.3493}},
		"reviews": []map[string]any{{
			"author_name":               "Sam",
			"rating":                    5,
			"text":                      "Great views.",
			"relative_time_description": "2 months ago",
		}},
	},
}

func newPlacesServer(t *testing.T, search, geocode map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/place/textsearch/"):
			if search == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(search)
		case strings.Contains(r.URL.Path, "/place/details/"):
			_ = json.NewEncoder(w).Encode(placeDetailsOK)
		case strings.Contains(r.URL.Path, "/geocode/"):
			if geocode == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(geocode)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}
