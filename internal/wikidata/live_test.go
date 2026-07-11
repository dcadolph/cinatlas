//go:build live

package wikidata

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/dcadolph/cinatlas/internal/httpcache"
)

// TestLiveLocations exercises the real Wikidata endpoint through the disk cache.
// It is excluded from normal runs. Run with: go test -tags live ./internal/wikidata
func TestLiveLocations(t *testing.T) {
	t.Parallel()
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: httpcache.New(t.TempDir(), time.Hour),
	}
	finder := New(WithHTTPClient(client))
	ctx := context.Background()

	start := time.Now()
	first, err := finder.Locations(ctx, "tt0116282") // Fargo.
	if err != nil {
		t.Fatalf("live locations: %v", err)
	}
	cold := time.Since(start)
	if len(first) == 0 {
		t.Fatal("live locations: no filming locations for Fargo")
	}

	start = time.Now()
	second, err := finder.Locations(ctx, "tt0116282")
	if err != nil {
		t.Fatalf("cached locations: %v", err)
	}
	warm := time.Since(start)
	if len(second) != len(first) {
		t.Errorf("cached count = %d, want %d", len(second), len(first))
	}

	t.Logf("cold %v, warm %v, %d locations", cold.Round(time.Millisecond), warm.Round(time.Millisecond), len(first))
	for i, loc := range first {
		if i == 3 {
			break
		}
		t.Logf("  %s resolved=%v %s", loc.Name, loc.Resolved, loc.MapsURL)
	}
}
