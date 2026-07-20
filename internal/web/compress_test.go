package web

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestCompressAndCacheHeaders checks the site gzips for accepting clients,
// caches static assets, and serves the crawl policy.
func TestCompressAndCacheHeaders(t *testing.T) {
	t.Parallel()
	site := newSinSite(t)

	req, err := http.NewRequest(http.MethodGet, site.URL+"/static/style.css", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := site.Client().Transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("fetch css: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != staticMaxAge {
		t.Errorf("Cache-Control = %q, want %q", got, staticMaxAge)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	body, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "marquee gold") {
		t.Error("decompressed css lost its content")
	}

	// A client without gzip support still gets a plain response.
	plainReq, err := http.NewRequest(http.MethodGet, site.URL+"/robots.txt", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	plain, err := site.Client().Transport.RoundTrip(plainReq)
	if err != nil {
		t.Fatalf("fetch robots: %v", err)
	}
	defer func() { _ = plain.Body.Close() }()
	if got := plain.Header.Get("Content-Encoding"); got != "" {
		t.Errorf("plain Content-Encoding = %q, want none", got)
	}
	robots, err := io.ReadAll(plain.Body)
	if err != nil {
		t.Fatalf("read robots: %v", err)
	}
	for _, want := range []string{"Disallow: /sin", "Disallow: /find", "Disallow: /globe"} {
		if !strings.Contains(string(robots), want) {
			t.Errorf("robots.txt missing %q", want)
		}
	}
}
