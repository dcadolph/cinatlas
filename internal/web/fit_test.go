package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dcadolph/cinatlas/internal/ddd"
	"github.com/dcadolph/cinatlas/internal/family"
	"github.com/dcadolph/cinatlas/internal/logutil"
	"github.com/dcadolph/cinatlas/internal/tmdb"
)

// newFitSite builds a site whose backend serves two discover candidates: Gentle
// Film (PG) and Harsh Film (R). The trigger source flags animal death in Sad Dog
// Film titles only.
func newFitSite(t *testing.T, triggers ddd.TriggerSource) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/discover/movie", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "" {
			_, _ = w.Write([]byte(`{"results":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[
			{"id":21,"title":"Gentle Film","release_date":"2020-01-01","popularity":50,"genre_ids":[35]},
			{"id":22,"title":"Harsh Film","release_date":"2021-01-01","popularity":90,"genre_ids":[27]}]}`))
	})
	mux.HandleFunc("/genre/movie/list", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"genres":[{"id":35,"name":"Comedy"},{"id":27,"name":"Horror"}]}`))
	})
	mux.HandleFunc("/genre/tv/list", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"genres":[]}`))
	})
	mux.HandleFunc("/movie/21/release_dates", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"iso_3166_1":"US","release_dates":[{"certification":"PG"}]}]}`))
	})
	mux.HandleFunc("/movie/22/release_dates", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"iso_3166_1":"US","release_dates":[{"certification":"R"}]}]}`))
	})
	mux.HandleFunc("/movie/21", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":21,"title":"Gentle Film","release_date":"2020-01-01"}`))
	})
	mux.HandleFunc("/movie/22", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":22,"title":"Harsh Film","release_date":"2021-01-01"}`))
	})
	backend := httptest.NewServer(mux)
	t.Cleanup(backend.Close)

	client, err := tmdb.New("testkey", tmdb.WithBaseURL(backend.URL), tmdb.WithHTTPClient(backend.Client()))
	if err != nil {
		t.Fatalf("tmdb.New: %v", err)
	}
	server, err := New(client, fakeAtlas{}, triggers, logutil.New("error"))
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}
	site := httptest.NewServer(server.Routes())
	t.Cleanup(site.Close)
	return site
}

// encodeTestProfile encodes a profile or fails the test.
func encodeTestProfile(t *testing.T, p family.Profile) string {
	t.Helper()
	encoded, err := family.EncodeProfile(p)
	if err != nil {
		t.Fatalf("encode profile: %v", err)
	}
	return encoded
}

// TestFitBuilderPage checks the empty fit page renders the builder.
func TestFitBuilderPage(t *testing.T) {
	t.Parallel()
	site := newFitSite(t, nil)
	status, body := fetch(t, site, "/fit")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	for _, want := range []string{"Family fit", "member-template", "Add a person"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

// TestFitMalformedProfile checks a bad payload renders a clean error.
func TestFitMalformedProfile(t *testing.T) {
	t.Parallel()
	site := newFitSite(t, nil)
	status, body := fetch(t, site, "/fit?p=%21%21%21")
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if !strings.Contains(body, "malformed") {
		t.Errorf("body missing malformed-profile error")
	}
}

// TestFitCeilingFilters checks a PG ceiling passes the PG film and excludes the R
// film, counting the exclusion.
func TestFitCeilingFilters(t *testing.T) {
	t.Parallel()
	site := newFitSite(t, nil)
	p := encodeTestProfile(t, family.Profile{Members: []family.Member{{Name: "Sam", Ceiling: "PG"}}})
	status, body := fetch(t, site, "/fit?p="+p)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !strings.Contains(body, "Gentle Film") {
		t.Errorf("body missing passing film")
	}
	if strings.Contains(body, "Harsh Film") {
		t.Errorf("body contains excluded film")
	}
	if !strings.Contains(body, "1 popular titles") {
		t.Errorf("body missing exclusion count")
	}
}

// TestFitTriggerVeto checks a hard veto flagged by the trigger source excludes an
// otherwise passing film.
func TestFitTriggerVeto(t *testing.T) {
	t.Parallel()
	triggers := ddd.TriggerSourceFunc(func(_ context.Context, title string, _ int) (map[string]family.Trigger, error) {
		if title == "Gentle Film" {
			return map[string]family.Trigger{"animal-death": family.TriggerYes}, nil
		}
		return map[string]family.Trigger{}, nil
	})
	site := newFitSite(t, triggers)
	p := encodeTestProfile(t, family.Profile{Members: []family.Member{
		{Name: "Sam", Ceiling: "PG", HardVetoes: []string{"animal-death"}},
	}})
	status, body := fetch(t, site, "/fit?p="+p)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if strings.Contains(body, "Gentle Film") {
		t.Errorf("body contains film that should be vetoed")
	}
	if !strings.Contains(body, "2 popular titles") {
		t.Errorf("body missing exclusion count for both films")
	}
}

// TestFitUnverifiedWithoutTriggers checks a hard veto with no trigger source still
// passes films but marks them unverified.
func TestFitUnverifiedWithoutTriggers(t *testing.T) {
	t.Parallel()
	site := newFitSite(t, nil)
	p := encodeTestProfile(t, family.Profile{Members: []family.Member{
		{Name: "Sam", Ceiling: "PG", HardVetoes: []string{"animal-death"}},
	}})
	status, body := fetch(t, site, "/fit?p="+p)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !strings.Contains(body, "Gentle Film") {
		t.Errorf("body missing passing film")
	}
	if !strings.Contains(body, "unverified") {
		t.Errorf("body missing unverified badge")
	}
}
