// Package wikidata resolves filming locations for a film. It queries the
// Wikidata SPARQL endpoint for the filming-location property (P915) and the
// coordinates (P625) of each place, then builds map and earth links.
package wikidata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dcadolph/cinatlas/internal/model"
)

// ErrRequest reports a failed or non-success query.
var ErrRequest = errors.New("wikidata: request failed")

// defaultEndpoint is the Wikidata SPARQL query service.
const defaultEndpoint = "https://query.wikidata.org/sparql"

// userAgent identifies cinatlas to the query service, which the service asks of clients.
const userAgent = "cinatlas/0.1 (https://github.com/dcadolph/cinatlas)"

// LocationFinder returns filming locations for an IMDB title id.
type LocationFinder interface {
	Locations(ctx context.Context, imdbID string) ([]model.Location, error)
}

// LocationFinderFunc adapts a function to the LocationFinder interface.
type LocationFinderFunc func(ctx context.Context, imdbID string) ([]model.Location, error)

// Locations calls the underlying function.
func (f LocationFinderFunc) Locations(ctx context.Context, imdbID string) ([]model.Location, error) {
	return f(ctx, imdbID)
}

// Option configures an HTTPClient at construction time.
type Option func(*HTTPClient)

// WithHTTPClient sets the underlying HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *HTTPClient) { c.httpClient = h }
}

// WithEndpoint overrides the SPARQL endpoint, mainly for tests.
func WithEndpoint(endpoint string) Option {
	return func(c *HTTPClient) { c.endpoint = endpoint }
}

// HTTPClient queries the Wikidata SPARQL endpoint.
type HTTPClient struct {
	// endpoint is the SPARQL query URL.
	endpoint string
	// httpClient performs the requests.
	httpClient *http.Client
}

// New returns an HTTPClient with sensible defaults.
func New(opts ...Option) *HTTPClient {
	c := &HTTPClient{
		endpoint:   defaultEndpoint,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Locations returns the filming locations recorded on Wikidata for the film
// with the given IMDB title id. Places without coordinates are still returned
// with a text-search map link so the caller never drops a known location.
func (c *HTTPClient) Locations(ctx context.Context, imdbID string) ([]model.Location, error) {
	imdbID = strings.TrimSpace(imdbID)
	if imdbID == "" {
		return nil, nil
	}
	q := url.Values{
		"query":  {locationQuery(imdbID)},
		"format": {"json"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %w", ErrRequest, err)
	}
	req.Header.Set("Accept", "application/sparql-results+json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRequest, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: status %d", ErrRequest, resp.StatusCode)
	}
	var out sparqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("%w: decode: %w", ErrRequest, err)
	}
	return out.locations(), nil
}

// locationQuery builds the SPARQL text that finds filming locations for an
// IMDB title id and their coordinates when present.
func locationQuery(imdbID string) string {
	return `SELECT ?placeLabel ?coord WHERE {
  ?film wdt:P345 "` + imdbID + `" .
  ?film wdt:P915 ?place .
  OPTIONAL { ?place wdt:P625 ?coord . }
  SERVICE wikibase:label { bd:serviceParam wikibase:language "en" . }
}`
}

// sparqlResponse is the shape of a SPARQL JSON result set.
type sparqlResponse struct {
	Results struct {
		Bindings []map[string]sparqlValue `json:"bindings"`
	} `json:"results"`
}

// sparqlValue is one bound variable in a result row.
type sparqlValue struct {
	Value string `json:"value"`
}

// locations converts result rows to the shared location type.
func (r sparqlResponse) locations() []model.Location {
	locations := make([]model.Location, 0, len(r.Results.Bindings))
	for _, row := range r.Results.Bindings {
		name := row["placeLabel"].Value
		if name == "" {
			continue
		}
		loc := model.Location{Name: name}
		if lat, lon, ok := parsePoint(row["coord"].Value); ok {
			loc.Latitude = lat
			loc.Longitude = lon
			loc.Resolved = true
			loc.MapsURL = mapsURL(lat, lon)
			loc.EarthURL = earthURL(lat, lon)
		} else {
			loc.MapsURL = mapsSearchURL(name)
		}
		locations = append(locations, loc)
	}
	return locations
}

// parsePoint reads a WKT point of the form "Point(lon lat)" into coordinates.
func parsePoint(wkt string) (lat, lon float64, ok bool) {
	wkt = strings.TrimSpace(wkt)
	if !strings.HasPrefix(wkt, "Point(") || !strings.HasSuffix(wkt, ")") {
		return 0, 0, false
	}
	inner := wkt[len("Point(") : len(wkt)-1]
	parts := strings.Fields(inner)
	if len(parts) != 2 {
		return 0, 0, false
	}
	lon, errLon := strconv.ParseFloat(parts[0], 64)
	lat, errLat := strconv.ParseFloat(parts[1], 64)
	if errLon != nil || errLat != nil {
		return 0, 0, false
	}
	return lat, lon, true
}

// mapsURL links coordinates on Google Maps.
func mapsURL(lat, lon float64) string {
	return "https://www.google.com/maps/search/?api=1&query=" + coordPair(lat, lon)
}

// mapsSearchURL links a place name text search on Google Maps.
func mapsSearchURL(name string) string {
	return "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(name)
}

// earthURL links coordinates on Google Earth.
func earthURL(lat, lon float64) string {
	return "https://earth.google.com/web/@" + coordPair(lat, lon) + ",1000a,1000d"
}

// coordPair formats a latitude and longitude as "lat,lon".
func coordPair(lat, lon float64) string {
	return strconv.FormatFloat(lat, 'f', -1, 64) + "," + strconv.FormatFloat(lon, 'f', -1, 64)
}
