package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dcadolph/cinatlas/internal/logutil"
	"github.com/dcadolph/cinatlas/internal/tmdb"
)

// newSinSite builds a site whose backend serves two sin candidates: Hot Film,
// tagged with anchor keywords, and Cold Front, a war film that must never
// survive the score gate despite its R rating.
func newSinSite(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/search/keyword", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"results":[{"id":700,"name":%q}]}`, r.URL.Query().Get("query"))
	})
	mux.HandleFunc("/discover/movie", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "" {
			_, _ = w.Write([]byte(`{"results":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[
			{"id":41,"title":"Hot Film","release_date":"1993-05-01","popularity":70},
			{"id":42,"title":"Cold Front","release_date":"1998-07-24","popularity":95}]}`))
	})
	mux.HandleFunc("/movie/41", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":41,"imdb_id":"tt0000041","vote_count":1000,
			"keywords":{"keywords":[{"name":"eroticism"},{"name":"sex scene"}]},
			"release_dates":{"results":[{"iso_3166_1":"US","release_dates":[{"certification":"NC-17"}]}]}}`))
	})
	mux.HandleFunc("/movie/42", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":42,"imdb_id":"tt0000042","vote_count":2000,
			"keywords":{"keywords":[{"name":"world war ii"},{"name":"beach landing"}]},
			"release_dates":{"results":[{"iso_3166_1":"US","release_dates":[{"certification":"R"}]}]}}`))
	})
	mux.HandleFunc("/person/55", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":55,"name":"Test Star","profile_path":"/t.jpg"}`))
	})
	backend := httptest.NewServer(mux)
	t.Cleanup(backend.Close)

	client, err := tmdb.New("testkey", tmdb.WithBaseURL(backend.URL), tmdb.WithHTTPClient(backend.Client()))
	if err != nil {
		t.Fatalf("tmdb.New: %v", err)
	}
	server, err := New(client, fakeAtlas{}, nil, logutil.New("error"))
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}
	site := httptest.NewServer(server.Routes())
	t.Cleanup(site.Close)
	return site
}

// TestSinLanding checks the landing view carries the age gate and the shelves.
func TestSinLanding(t *testing.T) {
	t.Parallel()
	site := newSinSite(t)
	status, body := fetch(t, site, "/sin")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	for _, want := range []string{"The back room is 18+", "Erotic Thriller", "Forbidden Affairs", `name="robots"`} {
		if !strings.Contains(body, want) {
			t.Errorf("landing missing %q", want)
		}
	}
}

// TestSinChipShelf checks a chip search keeps the anchored film with its
// certification and link pack, and culls the war film its R rating alone
// would have admitted.
func TestSinChipShelf(t *testing.T) {
	t.Parallel()
	site := newSinSite(t)
	status, body := fetch(t, site, "/sin?chip=steamy")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	for _, want := range []string{"Hot Film", "NC-17", "parentalguide", "mrskin.com", "movie-censorship.com"} {
		if !strings.Contains(body, want) {
			t.Errorf("shelf missing %q", want)
		}
	}
	if strings.Contains(body, "Cold Front") {
		t.Error("war film reached the shelf on its rating")
	}
}

// TestSinUnknownChip checks a bad slug is a 404, not an empty shelf.
func TestSinUnknownChip(t *testing.T) {
	t.Parallel()
	site := newSinSite(t)
	if status, _ := fetch(t, site, "/sin?chip=wholesome"); status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}

// TestSinPerson checks a person's devil filters their films through the same
// gate, shows the per-celebrity links, and keeps the war film out.
func TestSinPerson(t *testing.T) {
	t.Parallel()
	site := newSinSite(t)
	status, body := fetch(t, site, "/sin?person=55")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	for _, want := range []string{"Test Star, after dark", "Hot Film", "Full filmography", "mrskin.com"} {
		if !strings.Contains(body, want) {
			t.Errorf("person lens missing %q", want)
		}
	}
	if strings.Contains(body, "Cold Front") {
		t.Error("war film reached the person shelf on its rating")
	}
}

// TestSinPersonBadID checks malformed and unknown person ids both 404.
func TestSinPersonBadID(t *testing.T) {
	t.Parallel()
	site := newSinSite(t)
	for _, path := range []string{"/sin?person=abc", "/sin?person=777"} {
		if status, _ := fetch(t, site, path); status != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", path, status)
		}
	}
}

// TestSinFreeText checks a free-text mood ranks the anchored film above the
// cold one instead of hiding what the lexicon asked for.
func TestSinFreeText(t *testing.T) {
	t.Parallel()
	site := newSinSite(t)
	status, body := fetch(t, site, "/sin?q=something+steamy")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	hot := strings.Index(body, "Hot Film")
	if hot < 0 {
		t.Fatal("free text lost the anchored film")
	}
	if cold := strings.Index(body, "Cold Front"); cold >= 0 && cold < hot {
		t.Error("cold film ranked above the anchored one")
	}
}
