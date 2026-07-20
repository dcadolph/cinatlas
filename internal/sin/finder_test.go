package sin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/dcadolph/cinatlas/internal/logutil"
	"github.com/dcadolph/cinatlas/internal/tmdb"
)

// newBackend serves a fake TMDB: keyword search with an exact and a noisy
// match, one discover page of three candidates, and per-film themes. It
// records the discover query for assertions.
func newBackend(t *testing.T, discoverQuery *url.Values) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/search/keyword", func(w http.ResponseWriter, r *http.Request) {
		var body string
		switch r.URL.Query().Get("query") {
		case "eroticism":
			body = `{"results":[{"id":501,"name":"eroticism"}]}`
		case "sex scene":
			// The exact name ranks second; resolution must still pick it.
			body = `{"results":[{"id":9999,"name":"sex scene gone wrong"},{"id":502,"name":"sex scene"}]}`
		default:
			body = `{"results":[]}`
		}
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("/discover/movie", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "" {
			_, _ = w.Write([]byte(`{"results":[]}`))
			return
		}
		if discoverQuery != nil {
			*discoverQuery = r.URL.Query()
		}
		_, _ = w.Write([]byte(`{"results":[
			{"id":31,"title":"Basic Instinct","release_date":"1992-03-20","popularity":80},
			{"id":32,"title":"Beach Landing","release_date":"1998-07-24","popularity":95},
			{"id":33,"title":"Quiet Affair","release_date":"2019-11-06","popularity":40}]}`))
	})
	mux.HandleFunc("/movie/31", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":31,"imdb_id":"tt0103772","vote_count":5000,
			"keywords":{"keywords":[{"name":"Eroticism"},{"name":"Female Nudity"},{"name":"Seduction"}]},
			"release_dates":{"results":[{"iso_3166_1":"US","release_dates":[{"certification":"R"}]}]}}`))
	})
	mux.HandleFunc("/movie/32", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":32,"imdb_id":"tt0120815","vote_count":9000,
			"keywords":{"keywords":[{"name":"World War II"},{"name":"Normandy"}]},
			"release_dates":{"results":[{"iso_3166_1":"US","release_dates":[{"certification":"R"}]}]}}`))
	})
	mux.HandleFunc("/movie/33", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":33,"imdb_id":"tt7653254","vote_count":800,
			"keywords":{"keywords":[{"name":"Adultery"},{"name":"Divorce"}]},
			"release_dates":{"results":[{"iso_3166_1":"US","release_dates":[{"certification":"R"}]}]}}`))
	})
	backend := httptest.NewServer(mux)
	t.Cleanup(backend.Close)
	return backend
}

// newFinder returns a Finder against the backend.
func newFinder(t *testing.T, backend *httptest.Server) *Finder {
	t.Helper()
	client, err := tmdb.New("testkey", tmdb.WithBaseURL(backend.URL), tmdb.WithHTTPClient(backend.Client()))
	if err != nil {
		t.Fatalf("tmdb.New: %v", err)
	}
	return New(client, logutil.New("error"))
}

// TestFindCullsAndRanks checks the whole pipeline: exact keyword resolution,
// the Family exclusion, the score gate dropping the war film despite its R
// rating and the booster-only drama, and hydration onto the survivor.
func TestFindCullsAndRanks(t *testing.T) {
	t.Parallel()
	var discoverQuery url.Values
	finder := newFinder(t, newBackend(t, &discoverQuery))

	got, err := finder.Find(context.Background(), Query{
		Terms:    []string{"eroticism", "sex scene"},
		MinScore: DefaultMinScore,
	})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if kw := discoverQuery.Get("with_keywords"); kw != "501|502" {
		t.Errorf("with_keywords = %q, want 501|502", kw)
	}
	if wg := discoverQuery.Get("without_genres"); wg != "10751" {
		t.Errorf("without_genres = %q, want 10751", wg)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %+v", len(got), got)
	}
	r := got[0]
	if r.Movie.Title != "Basic Instinct" || r.Score != 6 {
		t.Errorf("survivor = %q score %d, want Basic Instinct score 6", r.Movie.Title, r.Score)
	}
	if r.Movie.Certification != "R" || r.Movie.IMDBID != "tt0103772" {
		t.Errorf("hydration = cert %q imdb %q", r.Movie.Certification, r.Movie.IMDBID)
	}
	if len(r.Links) != 4 {
		t.Errorf("got %d links, want 4", len(r.Links))
	}
}

// TestFindRankOnly checks a zero bar keeps every candidate and orders by heat
// first, then fame, which is how theme shelves like Forbidden Affairs run.
func TestFindRankOnly(t *testing.T) {
	t.Parallel()
	finder := newFinder(t, newBackend(t, nil))

	got, err := finder.Find(context.Background(), Query{Terms: []string{"eroticism"}})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d results, want 3", len(got))
	}
	want := []string{"Basic Instinct", "Beach Landing", "Quiet Affair"}
	for i, title := range want {
		if got[i].Movie.Title != title {
			t.Errorf("rank %d = %q, want %q", i, got[i].Movie.Title, title)
		}
	}
}

// TestFindNoAnchors checks an unresolvable vocabulary is an explicit error,
// never a silent shelf of unfiltered popular films.
func TestFindNoAnchors(t *testing.T) {
	t.Parallel()
	finder := newFinder(t, newBackend(t, nil))
	_, err := finder.Find(context.Background(), Query{Terms: []string{"nonsense term"}})
	if !errors.Is(err, ErrNoAnchors) {
		t.Errorf("err = %v, want ErrNoAnchors", err)
	}
}

// TestNewPanics checks nil dependencies are developer errors.
func TestNewPanics(t *testing.T) {
	t.Parallel()
	client, err := tmdb.New("testkey")
	if err != nil {
		t.Fatalf("tmdb.New: %v", err)
	}
	tests := []struct {
		Name string
		Run  func()
	}{{ // Test 0: Nil client.
		Name: "client", Run: func() { New(nil, logutil.New("error")) },
	}, { // Test 1: Nil logger.
		Name: "logger", Run: func() { New(client, nil) },
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			defer func() {
				if recover() == nil {
					t.Errorf("New with nil %s did not panic", test.Name)
				}
			}()
			test.Run()
		})
	}
}
