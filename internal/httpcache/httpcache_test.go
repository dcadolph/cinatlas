package httpcache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newBackend returns a test server that counts hits and serves the given response.
func newBackend(t *testing.T, status int, body string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// get performs one GET through the transport and returns the body.
func get(t *testing.T, rt http.RoundTripper, rawURL string) string {
	t.Helper()
	client := &http.Client{Transport: rt}
	resp, err := client.Get(rawURL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

// TestHitServesFromCache checks the second identical GET never reaches the backend.
func TestHitServesFromCache(t *testing.T) {
	t.Parallel()
	srv, hits := newBackend(t, http.StatusOK, `{"ok":true}`)
	tr := New(t.TempDir(), time.Hour)
	u := srv.URL + "/search/movie?query=heat"
	for range 2 {
		if got := get(t, tr, u); got != `{"ok":true}` {
			t.Fatalf("body = %q, want ok json", got)
		}
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("backend hits = %d, want 1", n)
	}
}

// TestExpiredEntryRefetches checks entries past the TTL fall back to the backend.
func TestExpiredEntryRefetches(t *testing.T) {
	t.Parallel()
	srv, hits := newBackend(t, http.StatusOK, "x")
	tr := New(t.TempDir(), time.Hour)
	get(t, tr, srv.URL)
	tr.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	get(t, tr, srv.URL)
	if n := hits.Load(); n != 2 {
		t.Errorf("backend hits = %d, want 2", n)
	}
}

// TestKeyIgnoresAPIKey checks requests differing only by api_key share one entry.
func TestKeyIgnoresAPIKey(t *testing.T) {
	t.Parallel()
	srv, hits := newBackend(t, http.StatusOK, "x")
	tr := New(t.TempDir(), time.Hour)
	get(t, tr, srv.URL+"/movie?api_key=aaa&query=heat")
	get(t, tr, srv.URL+"/movie?api_key=bbb&query=heat")
	if n := hits.Load(); n != 1 {
		t.Errorf("backend hits = %d, want 1", n)
	}
}

// TestRefreshBypassesRead checks refresh refetches but still updates the cache.
func TestRefreshBypassesRead(t *testing.T) {
	t.Parallel()
	srv, hits := newBackend(t, http.StatusOK, "x")
	dir := t.TempDir()
	plain := New(dir, time.Hour)
	forced := New(dir, time.Hour, WithRefresh(true))
	get(t, plain, srv.URL)
	get(t, forced, srv.URL)
	if n := hits.Load(); n != 2 {
		t.Fatalf("backend hits after refresh = %d, want 2", n)
	}
	get(t, plain, srv.URL)
	if n := hits.Load(); n != 2 {
		t.Errorf("backend hits after re-read = %d, want 2", n)
	}
}

// TestNonGetNotCached checks POST requests always pass through.
func TestNonGetNotCached(t *testing.T) {
	t.Parallel()
	srv, hits := newBackend(t, http.StatusOK, "x")
	client := &http.Client{Transport: New(t.TempDir(), time.Hour)}
	for range 2 {
		resp, err := client.Post(srv.URL, "text/plain", strings.NewReader("x"))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		_ = resp.Body.Close()
	}
	if n := hits.Load(); n != 2 {
		t.Errorf("backend hits = %d, want 2", n)
	}
}

// TestNon200NotCached checks failed responses are never stored.
func TestNon200NotCached(t *testing.T) {
	t.Parallel()
	srv, hits := newBackend(t, http.StatusInternalServerError, "boom")
	tr := New(t.TempDir(), time.Hour)
	get(t, tr, srv.URL)
	get(t, tr, srv.URL)
	if n := hits.Load(); n != 2 {
		t.Errorf("backend hits = %d, want 2", n)
	}
}

// TestCorruptEntryIsMiss checks unreadable cache files fall back to the backend.
func TestCorruptEntryIsMiss(t *testing.T) {
	t.Parallel()
	srv, hits := newBackend(t, http.StatusOK, "x")
	dir := t.TempDir()
	tr := New(dir, time.Hour)
	get(t, tr, srv.URL)
	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	path := filepath.Join(dir, cacheKey(parsed)+".json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("corrupt entry: %v", err)
	}
	get(t, tr, srv.URL)
	if n := hits.Load(); n != 2 {
		t.Errorf("backend hits = %d, want 2", n)
	}
}

// TestEmptyDirPassthrough checks an empty cache dir disables caching cleanly.
func TestEmptyDirPassthrough(t *testing.T) {
	t.Parallel()
	srv, hits := newBackend(t, http.StatusOK, "x")
	tr := New("", time.Hour)
	get(t, tr, srv.URL)
	get(t, tr, srv.URL)
	if n := hits.Load(); n != 2 {
		t.Errorf("backend hits = %d, want 2", n)
	}
}
