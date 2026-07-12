package wikidata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/dcadolph/cinatlas/internal/model"
)

// TestResolve checks grouped mapping of filming, setting, and country rows,
// article extraction, and the unresolved-name fallback link.
func TestResolve(t *testing.T) {
	t.Parallel()
	var gotUA, gotFormat string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotFormat = r.URL.Query().Get("format")
		_, _ = w.Write([]byte(`{"results":{"bindings":[` +
			`{"kind":{"value":"filming"},"placeLabel":{"value":"Los Angeles"},` +
			`"coord":{"value":"Point(-118.2437 34.0522)"},` +
			`"article":{"value":"https://en.wikipedia.org/wiki/Heat_(1995_film)"}},` +
			`{"kind":{"value":"filming"},"placeLabel":{"value":"Chicago"},` +
			`"article":{"value":"https://en.wikipedia.org/wiki/Heat_(1995_film)"}},` +
			`{"kind":{"value":"setting"},"placeLabel":{"value":"Los Angeles"},` +
			`"coord":{"value":"Point(-118.2437 34.0522)"},` +
			`"article":{"value":"https://en.wikipedia.org/wiki/Heat_(1995_film)"}},` +
			`{"kind":{"value":"country"},"placeLabel":{"value":"United States"},` +
			`"coord":{"value":"Point(-98.5 39.8)"},` +
			`"article":{"value":"https://en.wikipedia.org/wiki/Heat_(1995_film)"}}]}}`))
	}))
	t.Cleanup(srv.Close)

	c := New(WithEndpoint(srv.URL), WithHTTPClient(srv.Client()))
	got, err := c.Resolve(context.Background(), "tt0113277")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := &Result{
		Filming: []model.Location{
			model.ResolvedLocation("Los Angeles", "wikidata", 34.0522, -118.2437),
			model.UnresolvedLocation("Chicago", "wikidata"),
		},
		SetIn: []model.Location{
			model.ResolvedLocation("Los Angeles", "wikidata", 34.0522, -118.2437),
		},
		Countries: []model.Location{
			model.ResolvedLocation("United States", "country", 39.8, -98.5),
		},
		ArticleTitle: "Heat_(1995_film)",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Resolve\n got %+v\nwant %+v", got, want)
	}
	if gotUA == "" {
		t.Error("Resolve sent no User-Agent header")
	}
	if gotFormat != "json" {
		t.Errorf("format = %q, want json", gotFormat)
	}
}

// TestResolveEmptyID checks that a blank id makes no request and returns empty facts.
func TestResolveEmptyID(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("Resolve made a request for a blank id")
	}))
	t.Cleanup(srv.Close)
	got, err := New(WithEndpoint(srv.URL), WithHTTPClient(srv.Client())).
		Resolve(context.Background(), "  ")
	if err != nil {
		t.Fatalf("Resolve(blank): %v", err)
	}
	if !reflect.DeepEqual(got, &Result{}) {
		t.Errorf("Resolve(blank) = %+v, want empty result", got)
	}
}

// TestFilmsAt checks the reverse lookup: entity fan-out, film rows mapped
// newest first, and duplicate IMDB ids collapsed.
func TestFilmsAt(t *testing.T) {
	t.Parallel()
	entity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") != "wbsearchentities" {
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"search":[{"id":"Q485716"},{"id":"Q99999"}]}`))
	}))
	t.Cleanup(entity.Close)
	sparql := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":{"bindings":[` +
			`{"filmLabel":{"value":"Heat"},"imdb":{"value":"tt0113277"},` +
			`"date":{"value":"1995-12-15T00:00:00Z"},"placeLabel":{"value":"Los Angeles"}},` +
			`{"filmLabel":{"value":"Heat"},"imdb":{"value":"tt0113277"},` +
			`"date":{"value":"1995-12-01T00:00:00Z"},"placeLabel":{"value":"Los Angeles"}},` +
			`{"filmLabel":{"value":"Collateral"},"imdb":{"value":"tt0369339"},` +
			`"placeLabel":{"value":"Los Angeles"}}]}}`))
	}))
	t.Cleanup(sparql.Close)

	c := New(WithEndpoint(sparql.URL), WithEntityEndpoint(entity.URL), WithHTTPClient(sparql.Client()))
	got, err := c.FilmsAt(context.Background(), "Los Angeles")
	if err != nil {
		t.Fatalf("FilmsAt: %v", err)
	}
	want := []Film{
		{Title: "Heat", Year: 1995, IMDBID: "tt0113277", Place: "Los Angeles"},
		{Title: "Collateral", IMDBID: "tt0369339", Place: "Los Angeles"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilmsAt\n got %+v\nwant %+v", got, want)
	}
}

// TestFilmsAtEmptyPlace checks that a blank place makes no request.
func TestFilmsAtEmptyPlace(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("request made for a blank place")
	}))
	t.Cleanup(srv.Close)
	c := New(WithEndpoint(srv.URL), WithEntityEndpoint(srv.URL), WithHTTPClient(srv.Client()))
	got, err := c.FilmsAt(context.Background(), " ")
	if err != nil || got != nil {
		t.Errorf("FilmsAt(blank) = %v, %v, want nil, nil", got, err)
	}
}

// TestParsePoint checks WKT point parsing across valid and malformed inputs.
func TestParsePoint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In      string
		WantLat float64
		WantLon float64
		WantOK  bool
	}{{ // Test 0: Standard point, longitude first.
		In: "Point(-118.2437 34.0522)", WantLat: 34.0522, WantLon: -118.2437, WantOK: true,
	}, { // Test 1: Too many components.
		In: "Point(1 2 3)", WantOK: false,
	}, { // Test 2: Not a point at all.
		In: "somewhere", WantOK: false,
	}, { // Test 3: Empty string.
		In: "", WantOK: false,
	}, { // Test 4: Non-numeric components.
		In: "Point(east north)", WantOK: false,
	}}
	for testNum, test := range tests {
		t.Run("parse", func(t *testing.T) {
			t.Parallel()
			lat, lon, ok := parsePoint(test.In)
			if ok != test.WantOK || (ok && (lat != test.WantLat || lon != test.WantLon)) {
				t.Errorf("test %d: parsePoint(%q) = %v,%v,%v, want %v,%v,%v",
					testNum, test.In, lat, lon, ok, test.WantLat, test.WantLon, test.WantOK)
			}
		})
	}
}
