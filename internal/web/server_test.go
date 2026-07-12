package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dcadolph/cinatlas/internal/locate"
	"github.com/dcadolph/cinatlas/internal/logutil"
	"github.com/dcadolph/cinatlas/internal/model"
	"github.com/dcadolph/cinatlas/internal/tmdb"
)

// fakeAtlas answers fixed place facts in both directions.
type fakeAtlas struct{}

// Locate returns fixed filming and setting facts.
func (fakeAtlas) Locate(_ context.Context, _ string) (*locate.Located, error) {
	return &locate.Located{
		Filming: []model.Location{
			model.ResolvedLocation("Los Angeles", "wikidata", 34.05, -118.24),
			model.ResolvedLocation("Venice Beach", "wikipedia", 33.985, -118.47),
		},
		SetIn: []model.Location{model.UnresolvedLocation("Los Angeles", "wikidata")},
	}, nil
}

// At returns one fixed film for any place.
func (fakeAtlas) At(_ context.Context, place string, _ int) ([]model.Movie, error) {
	if place == "nowhere" {
		return nil, nil
	}
	return []model.Movie{{
		TMDBID: 1, Title: "Heat", Year: 1995,
		PosterURL: "https://image.tmdb.org/t/p/w342/heat.jpg",
	}}, nil
}

// newSite returns a test site backed by a fake TMDB and a fixed location finder.
func newSite(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/search/movie", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") == "zzz" {
			_, _ = w.Write([]byte(`{"results":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[` +
			`{"id":1,"title":"Heat","release_date":"1995-12-15"},` +
			`{"id":2,"title":"Heat","release_date":"2013-06-14"}]}`))
	})
	mux.HandleFunc("/movie/1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":1,"title":"Heat","release_date":"1995-12-15",` +
			`"imdb_id":"tt0113277","overview":"A heist crew and a detective collide.",` +
			`"tagline":"A Los Angeles crime saga.","runtime":170,"vote_average":7.9,` +
			`"genres":[{"id":80,"name":"Crime"}],` +
			`"poster_path":"/heat.jpg","backdrop_path":"/heat-wide.jpg",` +
			`"credits":{"cast":[{"id":10,"name":"Al Pacino","character":"Vincent Hanna"}],` +
			`"crew":[{"id":20,"name":"Michael Mann","job":"Director"}]}}`))
	})
	mux.HandleFunc("/search/multi", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[` +
			`{"media_type":"movie","id":1,"title":"Heat","release_date":"1995-12-15",` +
			`"poster_path":"/heat.jpg"},` +
			`{"media_type":"person","id":5,"name":"Michael Mann",` +
			`"known_for_department":"Directing"}]}`))
	})
	mux.HandleFunc("/trending/movie/week", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"id":3,"title":"Trending Now",` +
			`"release_date":"2026-04-04","poster_path":"/trend.jpg"}]}`))
	})
	mux.HandleFunc("/movie/now_playing", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"id":6,"title":"Theater Feature",` +
			`"release_date":"2026-06-06","poster_path":"/theater.jpg"}]}`))
	})
	mux.HandleFunc("/movie/upcoming", func(w http.ResponseWriter, r *http.Request) {
		future := time.Now().AddDate(0, 1, 0).Format("2006-01-02")
		_, _ = fmt.Fprintf(w, `{"results":[{"id":8,"title":"Future Film",`+
			`"release_date":"%s","poster_path":"/future.jpg"}]}`, future)
	})
	mux.HandleFunc("/movie/1/recommendations", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"id":4,"title":"Companion Piece",` +
			`"release_date":"2019-09-09","poster_path":"/companion.jpg"}]}`))
	})
	mux.HandleFunc("/search/person", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"id":5,"name":"Michael Mann"}]}`))
	})
	mux.HandleFunc("/person/5", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":5,"name":"Michael Mann","imdb_id":"nm0000520",` +
			`"known_for_department":"Directing","combined_credits":{"cast":[],` +
			`"crew":[{"id":1,"media_type":"movie","title":"Heat","job":"Director",` +
			`"release_date":"1995-12-15","poster_path":"/heat.jpg"}]}}`))
	})
	backend := httptest.NewServer(mux)
	t.Cleanup(backend.Close)

	client, err := tmdb.New("testkey", tmdb.WithBaseURL(backend.URL), tmdb.WithHTTPClient(backend.Client()))
	if err != nil {
		t.Fatalf("tmdb.New: %v", err)
	}
	server, err := New(client, fakeAtlas{}, logutil.New("error"))
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}
	site := httptest.NewServer(server.Routes())
	t.Cleanup(site.Close)
	return site
}

// fetch returns the status and body for a site path.
func fetch(t *testing.T, site *httptest.Server, path string) (int, string) {
	t.Helper()
	resp, err := http.Get(site.URL + path)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return resp.StatusCode, string(b)
}

// TestPages checks each page renders the expected content.
func TestPages(t *testing.T) {
	t.Parallel()
	site := newSite(t)
	tests := []struct {
		Path         string
		WantStatus   int
		WantContains []string
	}{{ // Test 0: Home page renders the hero, search, and all three walls.
		Path: "/", WantStatus: http.StatusOK,
		WantContains: []string{
			"cin", "atlas", "Ask the quick question",
			"Now trending", "Trending Now", "image.tmdb.org/t/p/w342/trend.jpg",
			"In theaters", "Theater Feature",
			"Coming soon", "Future Film",
		},
	}, { // Test 1: Movie page renders hero, chips, cast, locations, map, globe
		// link, set-in, source badges, similar, and alternates.
		Path: "/movie?q=heat", WantStatus: http.StatusOK,
		WantContains: []string{
			"Heat", "1995", "Michael Mann", "Al Pacino", "Vincent Hanna",
			"Los Angeles", "Venice Beach", "wikipedia", "openstreetmap.org",
			"Open the globe", "/globe?id=1", "Set in",
			"image.tmdb.org/t/p/w342/heat.jpg",
			"image.tmdb.org/t/p/w1280/heat-wide.jpg", "A Los Angeles crime saga.",
			"2h 50m", "★ 7.9", "Crime",
			"More like this", "Companion Piece",
			"Not the one?", "2013", "imdb.com/title/tt0113277",
		},
	}, { // Test 2: Movie page by id skips search.
		Path: "/movie?id=1", WantStatus: http.StatusOK,
		WantContains: []string{"Heat", "Al Pacino"},
	}, { // Test 3: Person page renders a poster-shelf filmography with movie links.
		Path: "/person?q=mann", WantStatus: http.StatusOK,
		WantContains: []string{
			"Michael Mann", "Directing", "Heat", "/movie?id=1", "Director",
			"image.tmdb.org/t/p/w342/heat.jpg", "year-badge",
		},
	}, { // Test 4: No match renders a friendly not-found page.
		Path: "/movie?q=zzz", WantStatus: http.StatusNotFound,
		WantContains: []string{"No movie found"},
	}, { // Test 5: Static stylesheet serves.
		Path: "/static/style.css", WantStatus: http.StatusOK,
		WantContains: []string{"--gold"},
	}, { // Test 6: Unknown paths render the styled 404.
		Path: "/bogus", WantStatus: http.StatusNotFound,
		WantContains: []string{"That reel does not exist"},
	}, { // Test 7: Globe page renders every resolved pin for the map script.
		Path: "/globe?id=1", WantStatus: http.StatusOK,
		WantContains: []string{
			"maplibre-gl", "Los Angeles", "Venice Beach",
			"back to the film", "/movie?id=1",
		},
	}, { // Test 8: Place page renders the filmed-here shelf.
		Path: "/place?q=los+angeles", WantStatus: http.StatusOK,
		WantContains: []string{"Filmed here", "los angeles", "Heat", "/movie?id=1"},
	}, { // Test 9: Place with no recorded films renders the honest empty state.
		Path: "/place?q=nowhere", WantStatus: http.StatusNotFound,
		WantContains: []string{"No films with recorded locations"},
	}, { // Test 10: Unified search renders movies, people, and the place shelf.
		Path: "/search?q=heat", WantStatus: http.StatusOK,
		WantContains: []string{
			"Movies", "Heat", "/movie?id=1",
			"People", "Michael Mann", "/person?id=5",
			"Filmed at", "filmed-here page",
		},
	}, { // Test 11: Unified search with no query renders guidance.
		Path: "/search", WantStatus: http.StatusNotFound,
		WantContains: []string{"Type a film, a name, or a place."},
	}}
	for testNum, test := range tests {
		t.Run(test.Path, func(t *testing.T) {
			t.Parallel()
			status, body := fetch(t, site, test.Path)
			if status != test.WantStatus {
				t.Errorf("test %d: status = %d, want %d", testNum, status, test.WantStatus)
			}
			for _, want := range test.WantContains {
				if !strings.Contains(body, want) {
					t.Errorf("test %d: body missing %q", testNum, want)
				}
			}
		})
	}
}
