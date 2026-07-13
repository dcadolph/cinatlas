package fitfind

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/cinatlas/internal/family"
	"github.com/dcadolph/cinatlas/internal/logutil"
	"github.com/dcadolph/cinatlas/internal/tmdb"
)

// newFinder builds a Finder against a backend serving the given routes.
func newFinder(t *testing.T, routes map[string]string) *Finder {
	t.Helper()
	mux := http.NewServeMux()
	for path, body := range routes {
		b := body
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(b))
		})
	}
	backend := httptest.NewServer(mux)
	t.Cleanup(backend.Close)
	client, err := tmdb.New("testkey", tmdb.WithBaseURL(backend.URL), tmdb.WithHTTPClient(backend.Client()))
	if err != nil {
		t.Fatalf("tmdb.New: %v", err)
	}
	return New(client, nil, logutil.New("error"))
}

// TestFindRanksAndFilters checks that failing films are excluded and passing films
// rank by subscription coverage.
func TestFindRanksAndFilters(t *testing.T) {
	t.Parallel()
	finder := newFinder(t, map[string]string{
		"/discover/movie": `{"results":[
			{"id":1,"title":"Popular Film","release_date":"2020-01-01","popularity":90},
			{"id":2,"title":"Streamable Film","release_date":"2021-01-01","popularity":10},
			{"id":3,"title":"Harsh Film","release_date":"2022-01-01","popularity":99}]}`,
		"/genre/movie/list":      `{"genres":[]}`,
		"/genre/tv/list":         `{"genres":[]}`,
		"/movie/1/release_dates": `{"results":[{"iso_3166_1":"US","release_dates":[{"certification":"PG"}]}]}`,
		"/movie/2/release_dates": `{"results":[{"iso_3166_1":"US","release_dates":[{"certification":"G"}]}]}`,
		"/movie/3/release_dates": `{"results":[{"iso_3166_1":"US","release_dates":[{"certification":"R"}]}]}`,
		"/movie/1":               `{"id":1,"title":"Popular Film"}`,
		"/movie/2": `{"id":2,"title":"Streamable Film","watch/providers":{"results":{"US":{
			"flatrate":[{"provider_name":"Netflix"}]}}}}`,
		"/movie/3": `{"id":3,"title":"Harsh Film"}`,
	})
	profile := family.Profile{Members: []family.Member{{Name: "Sam", Ceiling: "PG"}}}
	results, excluded, err := finder.Find(context.Background(), profile, []string{"netflix"})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if excluded != 1 {
		t.Errorf("excluded = %d, want 1", excluded)
	}
	titles := make([]string, 0, len(results))
	for _, r := range results {
		titles = append(titles, r.Movie.Title)
	}
	// Streamable Film ranks first: a subscription hit beats higher popularity.
	want := []string{"Streamable Film", "Popular Film"}
	if diff := cmp.Diff(want, titles); diff != "" {
		t.Errorf("order mismatch (-want +got):\n%s", diff)
	}
}

// TestFindDiscoverError checks a failing first discover page surfaces the error.
func TestFindDiscoverError(t *testing.T) {
	t.Parallel()
	finder := newFinder(t, map[string]string{})
	profile := family.Profile{Members: []family.Member{{Name: "Sam", Ceiling: "PG"}}}
	if _, _, err := finder.Find(context.Background(), profile, nil); !errors.Is(err, tmdb.ErrNotFound) {
		t.Errorf("got error %v, want tmdb.ErrNotFound", err)
	}
}
