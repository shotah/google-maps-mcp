package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultTimeout  = 15 * time.Second
	defaultMaxBytes = 2 << 20 // 2 MiB
	maxRedirects    = 5
)

// Fetcher GETs a URL. Tests inject httptest / stubs; production uses HTTPFetcher.
type Fetcher interface {
	Get(ctx context.Context, rawURL string) (body []byte, contentType, finalURL string, err error)
}

// HTTPFetcher is a size-capped http.Client. Redirects are capped and, by
// default, restricted to Google Maps hosts (SSRF).
type HTTPFetcher struct {
	Client    *http.Client
	UserAgent string
	MaxBytes  int64
	// AllowHost overrides the Maps-host allowlist. Tests set this so httptest
	// (localhost) works. Nil means allowedMapsHost.
	AllowHost func(host string) bool
}

func defaultUserAgent() string {
	return "google-maps-mcp/0.1 (+https://github.com/shotah/google-maps-mcp)"
}

func defaultFetcher() *HTTPFetcher {
	f := &HTTPFetcher{
		UserAgent: defaultUserAgent(),
		MaxBytes:  defaultMaxBytes,
	}
	f.Client = newHTTPClient(f.allow)
	return f
}

func newHTTPClient(allow func(host string) bool) *http.Client {
	if allow == nil {
		allow = allowedMapsHost
	}
	return &http.Client{
		Timeout: defaultTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			if err := validateURL(req.URL.String()); err != nil {
				return err
			}
			if !allow(req.URL.Host) {
				return fmt.Errorf("refusing redirect to non-Maps host %q", req.URL.Host)
			}
			return nil
		},
	}
}

func (f *HTTPFetcher) allow(host string) bool {
	if f != nil && f.AllowHost != nil {
		return f.AllowHost(host)
	}
	return allowedMapsHost(host)
}

func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("url must be http or https")
	}
	if u.Host == "" {
		return errors.New("url host is required")
	}
	return nil
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimSuffix(host, ".")
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return host
}

func isShortMapsHost(host string) bool {
	switch normalizeHost(host) {
	case "maps.app.goo.gl", "goo.gl", "g.co":
		return true
	default:
		return false
	}
}

func isGoogleTLD(rest string) bool {
	if rest == "com" {
		return true
	}
	if len(rest) == 2 && isAlpha(rest) {
		return true
	}
	if strings.HasPrefix(rest, "co.") && len(rest) == 5 && isAlpha(rest[3:]) {
		return true
	}
	if strings.HasPrefix(rest, "com.") && len(rest) == 6 && isAlpha(rest[4:]) {
		return true
	}
	return false
}

func isAlpha(s string) bool {
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return s != ""
}

// allowedMapsHost is the SSRF allowlist for share-link hops.
func allowedMapsHost(host string) bool {
	host = normalizeHost(host)
	switch host {
	case "maps.app.goo.gl", "goo.gl", "g.co", "google.com":
		return true
	}
	if strings.HasSuffix(host, ".google.com") {
		return true
	}
	if rest, ok := strings.CutPrefix(host, "www.google."); ok {
		return isGoogleTLD(rest)
	}
	if rest, ok := strings.CutPrefix(host, "maps.google."); ok {
		return isGoogleTLD(rest)
	}
	return false
}

// Get downloads rawURL. The caller must pass a validated http(s) URL.
func (f *HTTPFetcher) Get(ctx context.Context, rawURL string) ([]byte, string, string, error) {
	if err := validateURL(rawURL); err != nil {
		return nil, "", "", err
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", "", fmt.Errorf("url: %w", err)
	}
	if !f.allow(u.Host) {
		return nil, "", "", fmt.Errorf("refusing non-Maps host %q", u.Host)
	}

	client := f.Client
	if client == nil {
		client = newHTTPClient(f.allow)
	}
	maxBytes := f.MaxBytes
	if maxBytes < 1 {
		maxBytes = defaultMaxBytes
	}
	ua := f.UserAgent
	if ua == "" {
		ua = defaultUserAgent()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, "", "", err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer func() { _ = resp.Body.Close() }()

	final := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL.String()
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", final, fmt.Errorf("GET %s: HTTP %d", final, resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", final, err
	}
	if int64(len(body)) > maxBytes {
		return nil, "", final, fmt.Errorf("response larger than %d bytes", maxBytes)
	}
	ct := resp.Header.Get("Content-Type")
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return body, strings.ToLower(ct), final, nil
}
