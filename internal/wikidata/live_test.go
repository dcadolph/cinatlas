//go:build live

package wikidata

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/dcadolph/cinatlas/internal/httpcache"
)

// TestLiveResolve exercises the real Wikidata endpoint through the disk cache.
// It is excluded from normal runs. Run with: go test -tags live ./internal/wikidata
func TestLiveResolve(t *testing.T) {
	t.Parallel()
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: httpcache.New(t.TempDir(), time.Hour),
	}
	resolver := New(WithHTTPClient(client))
	ctx := context.Background()

	start := time.Now()
	first, err := resolver.Resolve(ctx, "tt0116282") // Fargo.
	if err != nil {
		t.Fatalf("live resolve: %v", err)
	}
	cold := time.Since(start)
	if len(first.Filming) == 0 {
		t.Fatal("live resolve: no filming locations for Fargo")
	}
	if first.ArticleTitle == "" {
		t.Error("live resolve: no Wikipedia article for Fargo")
	}

	start = time.Now()
	second, err := resolver.Resolve(ctx, "tt0116282")
	if err != nil {
		t.Fatalf("cached resolve: %v", err)
	}
	warm := time.Since(start)
	if len(second.Filming) != len(first.Filming) {
		t.Errorf("cached count = %d, want %d", len(second.Filming), len(first.Filming))
	}

	t.Logf("cold %v, warm %v, filming %d, set-in %d, countries %d, article %q",
		cold.Round(time.Millisecond), warm.Round(time.Millisecond),
		len(first.Filming), len(first.SetIn), len(first.Countries), first.ArticleTitle)
}
