// Package wikidata resolves place facts for a film: filming locations (P915),
// narrative settings (P840), production countries (P495), and the English
// Wikipedia article, all through one SPARQL query with coordinates (P625).
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

// defaultEntityEndpoint is the Wikidata entity search API.
const defaultEntityEndpoint = "https://www.wikidata.org/w/api.php"

// maxPlaceCandidates caps how many entity matches a place name fans out to.
const maxPlaceCandidates = 5

// userAgent identifies cinatlas to the query service, which the service asks of clients.
const userAgent = "cinatlas/0.1 (https://github.com/dcadolph/cinatlas)"

// Result holds every place fact one film resolves to.
type Result struct {
	// Filming lists filming locations.
	Filming []model.Location
	// SetIn lists narrative settings.
	SetIn []model.Location
	// Countries lists production countries, a coarse fallback.
	Countries []model.Location
	// ArticleTitle is the English Wikipedia article title, empty when none.
	ArticleTitle string
}

// Resolver answers place facts for an IMDB title id.
type Resolver interface {
	Resolve(ctx context.Context, imdbID string) (*Result, error)
}

// ResolverFunc adapts a function to the Resolver interface.
type ResolverFunc func(ctx context.Context, imdbID string) (*Result, error)

// Resolve calls the underlying function.
func (f ResolverFunc) Resolve(ctx context.Context, imdbID string) (*Result, error) {
	return f(ctx, imdbID)
}

// Film is one film found by a reverse place lookup.
type Film struct {
	// Title is the film label.
	Title string
	// Year is the earliest known publication year, zero when unknown.
	Year int
	// IMDBID is the IMDB title identifier.
	IMDBID string
	// Place is the matched place label.
	Place string
}

// PlaceSearcher finds films shot at a named place.
type PlaceSearcher interface {
	FilmsAt(ctx context.Context, place string) ([]Film, error)
}

// PlaceSearcherFunc adapts a function to the PlaceSearcher interface.
type PlaceSearcherFunc func(ctx context.Context, place string) ([]Film, error)

// FilmsAt calls the underlying function.
func (f PlaceSearcherFunc) FilmsAt(ctx context.Context, place string) ([]Film, error) {
	return f(ctx, place)
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

// WithEntityEndpoint overrides the entity search endpoint, mainly for tests.
func WithEntityEndpoint(endpoint string) Option {
	return func(c *HTTPClient) { c.entityEndpoint = endpoint }
}

// HTTPClient queries the Wikidata SPARQL and entity search endpoints.
type HTTPClient struct {
	// endpoint is the SPARQL query URL.
	endpoint string
	// entityEndpoint is the entity search API URL.
	entityEndpoint string
	// httpClient performs the requests.
	httpClient *http.Client
}

// New returns an HTTPClient with sensible defaults.
func New(opts ...Option) *HTTPClient {
	c := &HTTPClient{
		endpoint:       defaultEndpoint,
		entityEndpoint: defaultEntityEndpoint,
		httpClient:     &http.Client{Timeout: 20 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Resolve returns the place facts recorded on Wikidata for the film with the
// given IMDB title id. Places without coordinates still return with a
// text-search map link so a known name is never dropped.
func (c *HTTPClient) Resolve(ctx context.Context, imdbID string) (*Result, error) {
	imdbID = strings.TrimSpace(imdbID)
	if imdbID == "" {
		return &Result{}, nil
	}
	q := url.Values{
		"query":  {placeQuery(imdbID)},
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
	return out.result(), nil
}

// FilmsAt returns films whose recorded filming locations match the named
// place. The name fans out to entity candidates first; non-place candidates
// filter themselves out because nothing films at them.
func (c *HTTPClient) FilmsAt(ctx context.Context, place string) ([]Film, error) {
	place = strings.TrimSpace(place)
	if place == "" {
		return nil, nil
	}
	ids, err := c.searchEntities(ctx, place)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	q := url.Values{
		"query":  {filmsAtQuery(ids)},
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
	return out.films(), nil
}

// searchEntities returns candidate entity ids for a place name.
func (c *HTTPClient) searchEntities(ctx context.Context, name string) ([]string, error) {
	q := url.Values{
		"action": {"wbsearchentities"}, "search": {name}, "language": {"en"},
		"type": {"item"}, "limit": {strconv.Itoa(maxPlaceCandidates)},
		"format": {"json"}, "formatversion": {"2"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.entityEndpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %w", ErrRequest, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRequest, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: status %d", ErrRequest, resp.StatusCode)
	}
	var out struct {
		Search []struct {
			ID string `json:"id"`
		} `json:"search"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("%w: decode: %w", ErrRequest, err)
	}
	ids := make([]string, 0, len(out.Search))
	for _, s := range out.Search {
		ids = append(ids, s.ID)
	}
	return ids, nil
}

// filmsAtQuery builds the SPARQL text finding films shot at any candidate place.
func filmsAtQuery(ids []string) string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, "wd:"+id)
	}
	return `SELECT ?filmLabel ?imdb ?date ?placeLabel WHERE {
  VALUES ?place { ` + strings.Join(values, " ") + ` }
  ?film wdt:P915 ?place ; wdt:P345 ?imdb .
  OPTIONAL { ?film wdt:P577 ?date . }
  SERVICE wikibase:label { bd:serviceParam wikibase:language "en" . }
}
ORDER BY DESC(?date)
LIMIT 60`
}

// films converts reverse-lookup rows to films, newest first, one per IMDB id.
func (r sparqlResponse) films() []Film {
	films := make([]Film, 0, len(r.Results.Bindings))
	seen := map[string]bool{}
	for _, row := range r.Results.Bindings {
		imdbID := row["imdb"].Value
		title := row["filmLabel"].Value
		if imdbID == "" || title == "" || seen[imdbID] {
			continue
		}
		seen[imdbID] = true
		year := 0
		if date := row["date"].Value; len(date) >= 4 {
			if y, err := strconv.Atoi(date[:4]); err == nil {
				year = y
			}
		}
		films = append(films, Film{
			Title:  title,
			Year:   year,
			IMDBID: imdbID,
			Place:  row["placeLabel"].Value,
		})
	}
	return films
}

// placeQuery builds the SPARQL text for one film: filming locations, narrative
// settings, and production countries as a union tagged by kind, plus the
// English Wikipedia sitelink and coordinates where present.
func placeQuery(imdbID string) string {
	return `SELECT ?kind ?placeLabel ?coord ?article WHERE {
  ?film wdt:P345 "` + imdbID + `" .
  OPTIONAL { ?article schema:about ?film ; schema:isPartOf <https://en.wikipedia.org/> . }
  { ?film wdt:P915 ?place . BIND("filming" AS ?kind) }
  UNION { ?film wdt:P840 ?place . BIND("setting" AS ?kind) }
  UNION { ?film wdt:P495 ?place . BIND("country" AS ?kind) }
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

// result converts rows into the grouped place facts.
func (r sparqlResponse) result() *Result {
	out := &Result{}
	seen := map[string]bool{}
	for _, row := range r.Results.Bindings {
		if out.ArticleTitle == "" {
			out.ArticleTitle = articleTitle(row["article"].Value)
		}
		name := row["placeLabel"].Value
		kind := row["kind"].Value
		if name == "" || seen[kind+"/"+name] {
			continue
		}
		seen[kind+"/"+name] = true
		var loc model.Location
		if lat, lon, ok := parsePoint(row["coord"].Value); ok {
			loc = model.ResolvedLocation(name, "wikidata", lat, lon)
		} else {
			loc = model.UnresolvedLocation(name, "wikidata")
		}
		switch kind {
		case "filming":
			out.Filming = append(out.Filming, loc)
		case "setting":
			out.SetIn = append(out.SetIn, loc)
		case "country":
			loc.Source = "country"
			out.Countries = append(out.Countries, loc)
		}
	}
	return out
}

// articleTitle extracts a Wikipedia article title from a sitelink URL.
func articleTitle(link string) string {
	const marker = "/wiki/"
	at := strings.Index(link, marker)
	if at < 0 {
		return ""
	}
	title, err := url.PathUnescape(link[at+len(marker):])
	if err != nil {
		return ""
	}
	return title
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
