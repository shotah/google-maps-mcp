package tools

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
)

func TestParseMapsURLPlace(t *testing.T) {
	t.Parallel()
	raw := "https://www.google.com/maps/place/Space+Needle/@47.6205,-122.3493,17z"
	got := parseMapsURL(raw)
	if got.Kind != KindPlace {
		t.Fatalf("kind = %q", got.Kind)
	}
	if got.Name != "Space Needle" {
		t.Fatalf("name = %q", got.Name)
	}
	if got.Lat == nil || *got.Lat != 47.6205 || got.Lng == nil || *got.Lng != -122.3493 {
		t.Fatalf("coords = %v,%v", got.Lat, got.Lng)
	}
}

func TestParseMapsURLPlacePrefersDataCoords(t *testing.T) {
	t.Parallel()
	raw := "https://www.google.com/maps/place/Space+Needle/@47.6205,-122.3513,17z/data=!3m1!4b1!8m2!3d47.6205099!4d-122.3493894"
	got := parseMapsURL(raw)
	if got.Lat == nil || *got.Lat != 47.6205099 || got.Lng == nil || *got.Lng != -122.3493894 {
		t.Fatalf("coords = %v,%v", got.Lat, got.Lng)
	}
}

func TestParseMapsURLDirections(t *testing.T) {
	t.Parallel()
	raw := "https://www.google.com/maps/dir/Seattle,+WA/Portland,+OR/@45.5,-122.0,8z"
	got := parseMapsURL(raw)
	if got.Kind != KindDirections {
		t.Fatalf("kind = %q", got.Kind)
	}
	if got.Origin != "Seattle, WA" || got.Destination != "Portland, OR" {
		t.Fatalf("endpoints = %q → %q", got.Origin, got.Destination)
	}
	if got.Lat != nil {
		t.Fatalf("directions should not use viewport @ coords, got %v", *got.Lat)
	}
}

func TestParseMapsURLDirectionsAPI(t *testing.T) {
	t.Parallel()
	raw := "https://www.google.com/maps/dir/?api=1&origin=Seattle&destination=Portland"
	got := parseMapsURL(raw)
	if got.Kind != KindDirections || got.Origin != "Seattle" || got.Destination != "Portland" {
		t.Fatalf("%+v", got)
	}
}

func TestParseMapsURLSearch(t *testing.T) {
	t.Parallel()
	raw := "https://www.google.com/maps/search/coffee+shops/@47.6,-122.3,14z"
	got := parseMapsURL(raw)
	if got.Kind != KindSearch || got.Name != "coffee shops" {
		t.Fatalf("%+v", got)
	}
}

func TestParseMapsURLView(t *testing.T) {
	t.Parallel()
	raw := "https://www.google.com/maps/@47.6205,-122.3493,17z"
	got := parseMapsURL(raw)
	if got.Kind != KindView {
		t.Fatalf("kind = %q", got.Kind)
	}
	if got.Lat == nil || *got.Lat != 47.6205 {
		t.Fatalf("lat = %v", got.Lat)
	}
}

func TestParseMapsURLQueryPin(t *testing.T) {
	t.Parallel()
	raw := "https://maps.google.com/maps?q=47.6205,-122.3493"
	got := parseMapsURL(raw)
	if got.Kind != KindView || got.Lat == nil || *got.Lat != 47.6205 {
		t.Fatalf("%+v", got)
	}
}

func TestParseMapsURLOldDirections(t *testing.T) {
	t.Parallel()
	raw := "https://maps.google.com/maps?saddr=Seattle&daddr=Portland"
	got := parseMapsURL(raw)
	if got.Kind != KindDirections || got.Origin != "Seattle" || got.Destination != "Portland" {
		t.Fatalf("%+v", got)
	}
}

func TestParseMapsURLSearchQuery(t *testing.T) {
	t.Parallel()
	raw := "https://www.google.com/maps?q=Space+Needle"
	got := parseMapsURL(raw)
	if got.Kind != KindSearch || got.Name != "Space Needle" {
		t.Fatalf("%+v", got)
	}
}

func TestParseMapsURLUnknown(t *testing.T) {
	t.Parallel()
	got := parseMapsURL("https://www.google.com/maps")
	if got.Kind != KindUnknown {
		t.Fatalf("kind = %q", got.Kind)
	}
}

func TestParseMapsURLBad(t *testing.T) {
	t.Parallel()
	got := parseMapsURL("://bad")
	if got.Kind != KindUnknown {
		t.Fatalf("kind = %q", got.Kind)
	}
}

func TestResolveLinkLongURLNoFetch(t *testing.T) {
	t.Parallel()
	raw := "https://www.google.com/maps/place/Space+Needle/@47.6205,-122.3493,17z"
	got, err := ResolveLink(context.Background(), &stubFetcher{err: io.EOF}, raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != raw || got.Canonical != raw || got.Kind != KindPlace || got.Name != "Space Needle" {
		t.Fatalf("%+v", got)
	}
}

func TestResolveLinkShortURL(t *testing.T) {
	t.Parallel()
	short := "https://maps.app.goo.gl/xxxx"
	canonical := "https://www.google.com/maps/place/Space+Needle/@47.6205,-122.3493,17z"
	got, err := ResolveLink(context.Background(), &stubFetcher{final: canonical}, short)
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != short || got.Canonical != canonical {
		t.Fatalf("url/canonical = %q %q", got.URL, got.Canonical)
	}
	if got.Kind != KindPlace || got.Name != "Space Needle" {
		t.Fatalf("%+v", got)
	}
}

func TestResolveLinkShortURLGetErrorStillParsesFinal(t *testing.T) {
	t.Parallel()
	short := "https://goo.gl/maps/xxxx"
	canonical := "https://www.google.com/maps/place/Pike+Place/@47.6,-122.3,17z"
	got, err := ResolveLink(context.Background(), &stubFetcher{final: canonical, err: errors.New("HTTP 403")}, short)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Pike Place" {
		t.Fatalf("%+v", got)
	}
}

func TestResolveLinkShortURLGetError(t *testing.T) {
	t.Parallel()
	_, err := ResolveLink(context.Background(), &stubFetcher{err: errors.New("boom")}, "https://g.co/maps/xxxx")
	if err == nil || err.Error() != "boom" {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveLinkRejectsNonMaps(t *testing.T) {
	t.Parallel()
	_, err := ResolveLink(context.Background(), nil, "https://example.com/x")
	if err == nil || !strings.Contains(err.Error(), "not a Google Maps host") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveLinkEmpty(t *testing.T) {
	t.Parallel()
	_, err := ResolveLink(context.Background(), nil, "  ")
	if err == nil || !strings.Contains(err.Error(), "url is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveLinkBadScheme(t *testing.T) {
	t.Parallel()
	_, err := ResolveLink(context.Background(), nil, "ftp://maps.app.goo.gl/x")
	if err == nil {
		t.Fatal("expected scheme error")
	}
}

func TestResolveLinkNilFetcherUsesDefaultOnShort(t *testing.T) {
	t.Parallel()
	// defaultFetcher refuses example.com; short host is allowed but live GET
	// is not used — we pass a host that fails closed without network if Get
	// is somehow invoked. Use a stub via ResolveLink's nil→default only when
	// we can avoid a live call: long URL path.
	raw := "https://www.google.com/maps/@1,2,3z"
	got, err := ResolveLink(context.Background(), nil, raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindView {
		t.Fatalf("%+v", got)
	}
}

func TestLooksLikeMapsURL(t *testing.T) {
	t.Parallel()
	if !looksLikeMapsURL("https://maps.app.goo.gl/x") {
		t.Fatal("short")
	}
	if looksLikeMapsURL("Space Needle") || looksLikeMapsURL("https://example.com") {
		t.Fatal("non-maps")
	}
}

func TestIsLongMapsURL(t *testing.T) {
	t.Parallel()
	short, err := url.Parse("https://maps.app.goo.gl/x")
	if err != nil {
		t.Fatal(err)
	}
	if isLongMapsURL(short) {
		t.Fatal("short must fetch")
	}
	long, err := url.Parse("https://www.google.com/maps/place/X")
	if err != nil {
		t.Fatal(err)
	}
	if !isLongMapsURL(long) {
		t.Fatal("long place")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	t.Parallel()
	if firstNonEmpty(" ", "", "ok") != "ok" {
		t.Fatal(firstNonEmpty(" ", "", "ok"))
	}
	if firstNonEmpty("", "  ") != "" {
		t.Fatal("expected empty")
	}
}

func TestParseLatLngRejects(t *testing.T) {
	t.Parallel()
	if _, _, ok := parseLatLng("91,0"); ok {
		t.Fatal("lat out of range")
	}
	if _, _, ok := parseLatLng("0,200"); ok {
		t.Fatal("lng out of range")
	}
	if _, _, ok := parseLatLng("nope"); ok {
		t.Fatal("not a pair")
	}
}

func TestParseLatLng_LastPinFooter(t *testing.T) {
	t.Parallel()
	lat, lng, ok := parseLatLng("[last pin ±8m] 47.600000, -122.300000 at Wed Sep 9, 2026 9:44 PM PDT (just now)")
	if !ok || lat != 47.6 || lng != -122.3 {
		t.Fatalf("last pin footer: lat=%v lng=%v ok=%v", lat, lng, ok)
	}
	lat, lng, ok = parseLatLng("47.6, -122.3")
	if !ok || lat != 47.6 || lng != -122.3 {
		t.Fatalf("bare pair: lat=%v lng=%v ok=%v", lat, lng, ok)
	}
}

func TestPathSegmentBadEscape(t *testing.T) {
	t.Parallel()
	if pathSegment("%zz") == "" {
		t.Fatal("expected raw fallback")
	}
}

type stubFetcher struct {
	body  []byte
	ct    string
	final string
	err   error
	got   string
}

func (s *stubFetcher) Get(_ context.Context, rawURL string) ([]byte, string, string, error) {
	s.got = rawURL
	if s.err != nil {
		final := s.final
		if final == "" {
			final = rawURL
		}
		return nil, "", final, s.err
	}
	final := s.final
	if final == "" {
		final = rawURL
	}
	return s.body, s.ct, final, nil
}
