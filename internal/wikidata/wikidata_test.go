package wikidata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/dcadolph/cinatlas/internal/model"
)

// TestLocations checks SPARQL result mapping for places with and without coordinates.
func TestLocations(t *testing.T) {
	t.Parallel()
	var gotUA, gotFormat string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotFormat = r.URL.Query().Get("format")
		_, _ = w.Write([]byte(`{"results":{"bindings":[` +
			`{"placeLabel":{"value":"Los Angeles"},"coord":{"value":"Point(-118.2437 34.0522)"}},` +
			`{"placeLabel":{"value":"Chicago"}}]}}`))
	}))
	t.Cleanup(srv.Close)

	c := New(WithEndpoint(srv.URL), WithHTTPClient(srv.Client()))
	got, err := c.Locations(context.Background(), "tt0113277")
	if err != nil {
		t.Fatalf("Locations: %v", err)
	}
	want := []model.Location{
		{
			Name:      "Los Angeles",
			Latitude:  34.0522,
			Longitude: -118.2437,
			Resolved:  true,
			MapsURL:   "https://www.google.com/maps/search/?api=1&query=34.0522,-118.2437",
			EarthURL:  "https://earth.google.com/web/@34.0522,-118.2437,1000a,1000d",
		},
		{
			Name:     "Chicago",
			Resolved: false,
			MapsURL:  "https://www.google.com/maps/search/?api=1&query=Chicago",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Locations\n got %+v\nwant %+v", got, want)
	}
	if gotUA == "" {
		t.Error("Locations sent no User-Agent header")
	}
	if gotFormat != "json" {
		t.Errorf("format = %q, want json", gotFormat)
	}
}

// TestLocationsEmptyID checks that a blank id makes no request and returns no locations.
func TestLocationsEmptyID(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("Locations made a request for a blank id")
	}))
	t.Cleanup(srv.Close)
	got, err := New(WithEndpoint(srv.URL), WithHTTPClient(srv.Client())).
		Locations(context.Background(), "  ")
	if err != nil || got != nil {
		t.Errorf("Locations(blank) = %v, %v, want nil, nil", got, err)
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
