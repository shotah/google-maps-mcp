package tools

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw     string
		wantErr bool
	}{
		{"https://maps.app.goo.gl/xxxx", false},
		{"http://localhost/x", false},
		{"ftp://example.com/x", true},
		{"https://", true},
		{"://bad", true},
		{"not a url", true},
	}
	for _, tc := range cases {
		err := validateURL(tc.raw)
		if tc.wantErr && err == nil {
			t.Errorf("validateURL(%q) = nil, want error", tc.raw)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("validateURL(%q) = %v, want nil", tc.raw, err)
		}
	}
}

func TestAllowedMapsHost(t *testing.T) {
	t.Parallel()
	allow := []string{
		"maps.app.goo.gl",
		"goo.gl",
		"g.co",
		"google.com",
		"www.google.com",
		"maps.google.com",
		"www.google.com:443",
		"www.google.de",
		"www.google.co.uk",
		"maps.google.com.au",
		"GOOGLE.COM",
	}
	deny := []string{
		"example.com",
		"evil.com",
		"localhost",
		"127.0.0.1",
		"maps.app.goo.gl.evil.com",
		"goo.gl.evil.com",
		"www.google.evil.com",
		"notgoogle.com",
		"",
	}
	for _, h := range allow {
		if !allowedMapsHost(h) {
			t.Errorf("allowedMapsHost(%q) = false, want true", h)
		}
	}
	for _, h := range deny {
		if allowedMapsHost(h) {
			t.Errorf("allowedMapsHost(%q) = true, want false", h)
		}
	}
}

func TestIsShortMapsHost(t *testing.T) {
	t.Parallel()
	if !isShortMapsHost("maps.app.goo.gl") || !isShortMapsHost("goo.gl") || !isShortMapsHost("g.co") {
		t.Fatal("expected short hosts")
	}
	if isShortMapsHost("www.google.com") {
		t.Fatal("www.google.com is not short")
	}
}

func TestHTTPFetcherGetOK(t *testing.T) {
	t.Parallel()
	var gotUA, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)

	f := &HTTPFetcher{Client: srv.Client(), UserAgent: "maps-test/1", MaxBytes: 1024, AllowHost: anyHost}
	body, ct, final, err := f.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}
	if ct != "text/html" {
		t.Fatalf("content-type = %q", ct)
	}
	if final != srv.URL {
		t.Fatalf("final = %q", final)
	}
	if gotUA != "maps-test/1" {
		t.Fatalf("User-Agent = %q", gotUA)
	}
	if !strings.Contains(gotAccept, "text/html") {
		t.Fatalf("Accept = %q", gotAccept)
	}
}

func TestHTTPFetcherDefaults(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("empty User-Agent")
		}
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)

	f := &HTTPFetcher{AllowHost: anyHost}
	body, _, _, err := f.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}
}

func TestHTTPFetcherHTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	f := &HTTPFetcher{Client: srv.Client(), AllowHost: anyHost}
	_, _, _, err := f.Get(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("err = %v, want HTTP 404", err)
	}
}

func TestHTTPFetcherTooLarge(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", 64))
	}))
	t.Cleanup(srv.Close)

	f := &HTTPFetcher{Client: srv.Client(), MaxBytes: 8, AllowHost: anyHost}
	_, _, _, err := f.Get(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("err = %v, want size cap", err)
	}
}

func TestHTTPFetcherRejectsBadURL(t *testing.T) {
	t.Parallel()
	f := &HTTPFetcher{AllowHost: anyHost}
	_, _, _, err := f.Get(context.Background(), "ftp://example.com/x")
	if err == nil {
		t.Fatal("expected url scheme error")
	}
}

func TestHTTPFetcherRejectsNonMapsHost(t *testing.T) {
	t.Parallel()
	f := defaultFetcher()
	_, _, _, err := f.Get(context.Background(), "https://example.com/x")
	if err == nil || !strings.Contains(err.Error(), "non-Maps host") {
		t.Fatalf("err = %v", err)
	}
}

func TestHTTPFetcherRedirectLimit(t *testing.T) {
	t.Parallel()
	var srv *httptest.Server
	n := 0
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		http.Redirect(w, r, srv.URL+"/hop", http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	f := &HTTPFetcher{Client: newHTTPClient(anyHost), AllowHost: anyHost}
	_, _, _, err := f.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected redirect stop")
	}
	if n < 2 {
		t.Fatalf("redirects = %d, want several attempts", n)
	}
}

func TestHTTPFetcherRedirectToNonMaps(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.com/x", http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	allow := func(host string) bool {
		h := normalizeHost(host)
		return h == "127.0.0.1" || h == "localhost" || allowedMapsHost(host)
	}
	f := &HTTPFetcher{Client: newHTTPClient(allow), AllowHost: allow}
	_, _, _, err := f.Get(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "non-Maps host") {
		t.Fatalf("err = %v, want non-Maps host", err)
	}
}

func TestHTTPFetcherRedirectToBadScheme(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "ftp://example.com/x", http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	f := &HTTPFetcher{Client: newHTTPClient(anyHost), AllowHost: anyHost}
	_, _, _, err := f.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected redirect scheme error")
	}
}

func TestHTTPFetcherCanceledContext(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := &HTTPFetcher{Client: srv.Client(), AllowHost: anyHost}
	_, _, _, err := f.Get(ctx, srv.URL)
	if err == nil {
		t.Fatal("expected canceled context error")
	}
}

func TestDefaultFetcher(t *testing.T) {
	t.Parallel()
	f := defaultFetcher()
	if f == nil || f.Client == nil || f.MaxBytes != defaultMaxBytes {
		t.Fatalf("defaultFetcher = %+v", f)
	}
}

func TestDefaultUserAgent(t *testing.T) {
	t.Parallel()
	if !strings.Contains(defaultUserAgent(), "google-maps-mcp") {
		t.Fatalf("UA = %q", defaultUserAgent())
	}
}

func TestIsGoogleTLD(t *testing.T) {
	t.Parallel()
	if !isGoogleTLD("com") || !isGoogleTLD("de") || !isGoogleTLD("co.uk") || !isGoogleTLD("com.au") {
		t.Fatal("expected valid TLDs")
	}
	if isGoogleTLD("evil.com") || isGoogleTLD("") || isGoogleTLD("com.toolong") {
		t.Fatal("expected invalid TLDs")
	}
}

func anyHost(string) bool { return true }
