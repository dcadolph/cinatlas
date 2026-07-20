// Package httpcache is a caching http.RoundTripper. It stores successful GET
// responses on disk so repeat questions answer instantly and stay under API
// rate limits. Cache keys hash the request URL with the api_key parameter
// stripped, so entries survive key rotation and the key never shapes identity.
package httpcache

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dcadolph/cinatlas/internal/logutil"
)

// defaultMaxBytes caps the cache at 512 MB, comfortably under the small
// ephemeral disks hosting platforms give containers.
const defaultMaxBytes = 512 << 20

// tmpMaxAge is how long an orphaned temp file may linger before a sweep
// removes it. A healthy store renames its temp file within milliseconds.
const tmpMaxAge = time.Hour

// Option configures a Transport at construction time.
type Option func(*Transport)

// WithBase sets the transport that performs real requests.
func WithBase(base http.RoundTripper) Option {
	return func(t *Transport) { t.base = base }
}

// WithRefresh makes the transport bypass cache reads while still storing
// fresh responses, forcing a refetch.
func WithRefresh(refresh bool) Option {
	return func(t *Transport) { t.refresh = refresh }
}

// WithMaxBytes caps the cache directory at n bytes, evicting oldest entries
// first. Zero or negative disables the cap.
func WithMaxBytes(n int64) Option {
	return func(t *Transport) { t.maxBytes = n }
}

// Transport is a caching HTTP round tripper backed by a directory of files.
type Transport struct {
	// dir is the cache directory. Empty disables caching entirely.
	dir string
	// ttl is how long entries stay fresh.
	ttl time.Duration
	// maxBytes bounds the directory size; zero or negative means no cap.
	maxBytes int64
	// refresh bypasses cache reads while still storing fresh responses.
	refresh bool
	// base performs the real requests.
	base http.RoundTripper
	// now returns the current time, overridable in tests.
	now func() time.Time
	// sweepMu lets one goroutine sweep while concurrent stores skip it.
	sweepMu sync.Mutex
}

// New returns a Transport caching into dir with the given freshness window
// and the default size cap. An empty dir yields a passthrough transport that
// never caches.
func New(dir string, ttl time.Duration, opts ...Option) *Transport {
	t := &Transport{
		dir:      dir,
		ttl:      ttl,
		maxBytes: defaultMaxBytes,
		base:     http.DefaultTransport,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// RoundTrip serves fresh cached responses for GET requests and stores new
// successful ones. Anything else passes straight through to the base transport.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.dir == "" || req.Method != http.MethodGet {
		return t.base.RoundTrip(req)
	}
	log := logutil.FromContext(req.Context())
	path := filepath.Join(t.dir, cacheKey(req.URL)+".json")
	if !t.refresh {
		if resp, ok := t.load(path, req); ok {
			log.Debug("cache hit", "host", req.URL.Host, "path", req.URL.Path)
			return resp, nil
		}
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return resp, nil
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("httpcache: read body: %w", err)
	}
	if err := t.store(path, resp, body); err != nil {
		log.Debug("cache store failed", "err", err)
	} else {
		log.Debug("cache store", "host", req.URL.Host, "path", req.URL.Path)
		t.sweep()
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}

// sweep deletes expired entries, stale temp files, and then the oldest
// entries until the directory fits the byte cap, so the cache can never grow
// without bound and fill a host's disk. It runs after stores; when one sweep
// is already running, concurrent callers skip theirs.
func (t *Transport) sweep() {
	if !t.sweepMu.TryLock() {
		return
	}
	defer t.sweepMu.Unlock()
	dirents, err := os.ReadDir(t.dir)
	if err != nil {
		return
	}
	type cacheFile struct {
		// path locates the file for removal.
		path string
		// size counts its bytes toward the cap.
		size int64
		// mod orders eviction, oldest first.
		mod time.Time
	}
	files := make([]cacheFile, 0, len(dirents))
	var total int64
	now := t.now()
	for _, d := range dirents {
		info, err := d.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(t.dir, d.Name())
		if strings.HasPrefix(d.Name(), ".tmp-") {
			if now.Sub(info.ModTime()) > tmpMaxAge {
				_ = os.Remove(path)
			}
			continue
		}
		if !strings.HasSuffix(d.Name(), ".json") {
			continue
		}
		if now.Sub(info.ModTime()) > t.ttl {
			_ = os.Remove(path)
			continue
		}
		files = append(files, cacheFile{path: path, size: info.Size(), mod: info.ModTime()})
		total += info.Size()
	}
	if t.maxBytes <= 0 || total <= t.maxBytes {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	for _, f := range files {
		if total <= t.maxBytes {
			break
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}

// load reads a cache file and rebuilds the response. It reports false for
// missing, corrupt, or expired entries.
func (t *Transport) load(path string, req *http.Request) (*http.Response, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var e entry
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, false
	}
	if t.now().Sub(e.SavedAt) > t.ttl {
		return nil, false
	}
	header := http.Header{}
	if e.ContentType != "" {
		header.Set("Content-Type", e.ContentType)
	}
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", e.Status, http.StatusText(e.Status)),
		StatusCode:    e.Status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(e.Body)),
		ContentLength: int64(len(e.Body)),
		Request:       req,
	}, true
}

// store writes a cache entry atomically via a temp file rename.
func (t *Transport) store(path string, resp *http.Response, body []byte) error {
	if err := os.MkdirAll(t.dir, 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(entry{
		SavedAt:     t.now(),
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Body:        body,
	})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(t.dir, ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// cacheKey hashes the URL with the api_key parameter stripped and the query
// re-encoded in sorted order, so parameter order never splits entries.
func cacheKey(u *url.URL) string {
	q := u.Query()
	q.Del("api_key")
	c := *u
	c.RawQuery = q.Encode()
	sum := sha256.Sum256([]byte(c.String()))
	return hex.EncodeToString(sum[:])
}

// entry is the on-disk form of one cached response.
type entry struct {
	// SavedAt is when the response was stored.
	SavedAt time.Time `json:"savedAt"`
	// Status is the upstream HTTP status code.
	Status int `json:"status"`
	// ContentType is the upstream Content-Type header value.
	ContentType string `json:"contentType"`
	// Body is the raw response body.
	Body []byte `json:"body"`
}
