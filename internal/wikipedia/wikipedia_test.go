package wikipedia

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/dcadolph/cinatlas/internal/model"
)

// newAPI returns a fake action API serving canned sections, links, and coordinates.
func newAPI(t *testing.T, sections, links, coords string) *HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("prop") == "sections":
			_, _ = w.Write([]byte(sections))
		case q.Get("prop") == "links":
			_, _ = w.Write([]byte(links))
		case q.Get("prop") == "coordinates":
			_, _ = w.Write([]byte(coords))
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)
	return New(WithEndpoint(srv.URL), WithHTTPClient(srv.Client()))
}

// TestFilmingLocations checks the full mine: section match, link collection,
// coordinate batch, coordinate-less links dropped, article order preserved.
func TestFilmingLocations(t *testing.T) {
	t.Parallel()
	c := newAPI(t,
		`{"parse":{"sections":[{"index":"2","line":"Plot"},{"index":"5","line":"Filming"}]}}`,
		`{"parse":{"links":[`+
			`{"ns":0,"title":"Venice, Los Angeles","exists":true},`+
			`{"ns":0,"title":"Michael Mann","exists":true},`+
			`{"ns":0,"title":"Broadway (Los Angeles)","exists":true},`+
			`{"ns":14,"title":"Category:Films","exists":true}]}}`,
		`{"query":{"pages":[`+
			`{"title":"Broadway (Los Angeles)","coordinates":[{"lat":34.04,"lon":-118.25}]},`+
			`{"title":"Michael Mann"},`+
			`{"title":"Venice, Los Angeles","coordinates":[{"lat":33.985,"lon":-118.47}]}]}}`,
	)
	got, err := c.FilmingLocations(context.Background(), "Heat_(1995_film)")
	if err != nil {
		t.Fatalf("FilmingLocations: %v", err)
	}
	want := []model.Location{
		model.ResolvedLocation("Venice, Los Angeles", "wikipedia", 33.985, -118.47),
		model.ResolvedLocation("Broadway (Los Angeles)", "wikipedia", 34.04, -118.25),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilmingLocations\n got %+v\nwant %+v", got, want)
	}
}

// TestNoFilmingSection checks that an article without a matching section
// yields nothing without error.
func TestNoFilmingSection(t *testing.T) {
	t.Parallel()
	c := newAPI(t,
		`{"parse":{"sections":[{"index":"1","line":"Plot"},{"index":"2","line":"Reception"}]}}`,
		`{}`, `{}`,
	)
	got, err := c.FilmingLocations(context.Background(), "Some_Film")
	if err != nil || got != nil {
		t.Errorf("FilmingLocations = %v, %v, want nil, nil", got, err)
	}
}

// TestEmptyTitle checks that a blank title makes no request.
func TestEmptyTitle(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("request made for a blank title")
	}))
	t.Cleanup(srv.Close)
	c := New(WithEndpoint(srv.URL), WithHTTPClient(srv.Client()))
	got, err := c.FilmingLocations(context.Background(), " ")
	if err != nil || got != nil {
		t.Errorf("FilmingLocations(blank) = %v, %v, want nil, nil", got, err)
	}
}
