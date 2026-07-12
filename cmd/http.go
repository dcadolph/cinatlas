package cmd

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/dcadolph/cinatlas/internal/httpcache"
	"github.com/dcadolph/cinatlas/internal/locate"
	"github.com/dcadolph/cinatlas/internal/wikidata"
	"github.com/dcadolph/cinatlas/internal/wikipedia"
)

// httpTimeout bounds every API request, cached or not.
const httpTimeout = 30 * time.Second

// defaultCacheTTL is how long cached API responses stay fresh.
const defaultCacheTTL = 24 * time.Hour

// newHTTPClient returns the shared HTTP client with the disk cache installed.
func newHTTPClient(opt options) *http.Client {
	transport := httpcache.New(cacheDir(), cacheTTL(), httpcache.WithRefresh(opt.Refresh))
	return &http.Client{Timeout: httpTimeout, Transport: transport}
}

// newLocator returns the merged place-facts service over the shared HTTP
// client and TMDB client.
func newLocator(h *http.Client, finder locate.IMDBFinder) *locate.Service {
	wd := wikidata.New(wikidata.WithHTTPClient(h))
	return locate.New(wd, wd, wikipedia.New(wikipedia.WithHTTPClient(h)), finder)
}

// cacheDir returns the response cache directory, honoring the env override.
// An empty string disables caching.
func cacheDir() string {
	if v := os.Getenv("CINATLAS_CACHE_DIR"); v != "" {
		return v
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "cinatlas")
}

// cacheTTL returns the cache freshness window, honoring the env override.
func cacheTTL() time.Duration {
	if v := os.Getenv("CINATLAS_CACHE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultCacheTTL
}
