package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dcadolph/cinatlas/internal/logutil"
	"github.com/dcadolph/cinatlas/internal/model"
	"github.com/dcadolph/cinatlas/internal/tmdb"
	"github.com/dcadolph/cinatlas/internal/wikidata"
)

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
	mux.HandleFunc("/trending/movie/week", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"id":3,"title":"Trending Now",` +
			`"release_date":"2026-04-04","poster_path":"/trend.jpg"}]}`))
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
	finder := wikidata.LocationFinderFunc(func(_ context.Context, _ string) ([]model.Location, error) {
		return []model.Location{{
			Name: "Los Angeles", Latitude: 34.05, Longitude: -118.24, Resolved: true,
			MapsURL: "https://maps.example/la",
		}}, nil
	})
	server, err := New(client, finder, logutil.New("error"))
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
	}{{ // Test 0: Home page renders the hero, search, and trending wall.
		Path: "/", WantStatus: http.StatusOK,
		WantContains: []string{
			"cin", "atlas", "Ask the quick question",
			"Now trending", "Trending Now", "image.tmdb.org/t/p/w342/trend.jpg",
		},
	}, { // Test 1: Movie page renders hero, chips, cast, locations, map, similar, alternates.
		Path: "/movie?q=heat", WantStatus: http.StatusOK,
		WantContains: []string{
			"Heat", "1995", "Michael Mann", "Al Pacino", "Vincent Hanna",
			"Los Angeles", "openstreetmap.org", "image.tmdb.org/t/p/w342/heat.jpg",
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
