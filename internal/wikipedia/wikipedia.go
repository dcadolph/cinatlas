// Package wikipedia mines filming locations from a film's English Wikipedia
// article: it finds the filming or production section, collects the place
// articles linked there, and resolves their coordinates in one batch call.
// Links without coordinates are dropped, which filters people and films out
// of the result naturally.
package wikipedia

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/dcadolph/cinatlas/internal/model"
)

// ErrRequest reports a failed or non-success API request.
var ErrRequest = errors.New("wikipedia: request failed")

// defaultEndpoint is the English Wikipedia action API.
const defaultEndpoint = "https://en.wikipedia.org/w/api.php"

// userAgent identifies cinatlas to the API, which Wikimedia asks of clients.
const userAgent = "cinatlas/0.1 (https://github.com/dcadolph/cinatlas)"

// maxPlaceTitles caps one coordinate batch, the API limit for normal clients.
const maxPlaceTitles = 50

// sectionPattern matches section headings that describe where a film shot.
var sectionPattern = regexp.MustCompile(`(?i)^(filming|production|locations|filming locations|principal photography)`)

// SectionLocator mines filming locations from a Wikipedia article.
type SectionLocator interface {
	FilmingLocations(ctx context.Context, articleTitle string) ([]model.Location, error)
}

// SectionLocatorFunc adapts a function to the SectionLocator interface.
type SectionLocatorFunc func(ctx context.Context, articleTitle string) ([]model.Location, error)

// FilmingLocations calls the underlying function.
func (f SectionLocatorFunc) FilmingLocations(ctx context.Context, articleTitle string) ([]model.Location, error) {
	return f(ctx, articleTitle)
}

// Option configures an HTTPClient at construction time.
type Option func(*HTTPClient)

// WithHTTPClient sets the underlying HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *HTTPClient) { c.httpClient = h }
}

// WithEndpoint overrides the API endpoint, mainly for tests.
func WithEndpoint(endpoint string) Option {
	return func(c *HTTPClient) { c.endpoint = endpoint }
}

// HTTPClient talks to the Wikipedia action API.
type HTTPClient struct {
	// endpoint is the action API URL.
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

// FilmingLocations returns coordinate-bearing places linked from the article's
// filming or production section. It returns nothing, without error, when the
// article has no such section.
func (c *HTTPClient) FilmingLocations(ctx context.Context, articleTitle string) ([]model.Location, error) {
	articleTitle = strings.TrimSpace(articleTitle)
	if articleTitle == "" {
		return nil, nil
	}
	section, ok, err := c.filmingSection(ctx, articleTitle)
	if err != nil || !ok {
		return nil, err
	}
	titles, err := c.sectionLinks(ctx, articleTitle, section)
	if err != nil || len(titles) == 0 {
		return nil, err
	}
	if len(titles) > maxPlaceTitles {
		titles = titles[:maxPlaceTitles]
	}
	return c.coordinates(ctx, titles)
}

// filmingSection returns the index of the first section whose heading looks
// like a filming or production section.
func (c *HTTPClient) filmingSection(ctx context.Context, articleTitle string) (string, bool, error) {
	var out struct {
		Parse struct {
			Sections []struct {
				Index string `json:"index"`
				Line  string `json:"line"`
			} `json:"sections"`
		} `json:"parse"`
	}
	q := url.Values{
		"action": {"parse"}, "page": {articleTitle}, "prop": {"sections"},
		"format": {"json"}, "formatversion": {"2"}, "redirects": {"1"},
	}
	if err := c.get(ctx, q, &out); err != nil {
		return "", false, err
	}
	for _, s := range out.Parse.Sections {
		if sectionPattern.MatchString(strings.TrimSpace(s.Line)) {
			return s.Index, true, nil
		}
	}
	return "", false, nil
}

// sectionLinks returns the main-namespace article titles linked from a section.
func (c *HTTPClient) sectionLinks(ctx context.Context, articleTitle, section string) ([]string, error) {
	var out struct {
		Parse struct {
			Links []struct {
				NS     int    `json:"ns"`
				Title  string `json:"title"`
				Exists bool   `json:"exists"`
			} `json:"links"`
		} `json:"parse"`
	}
	q := url.Values{
		"action": {"parse"}, "page": {articleTitle}, "section": {section},
		"prop": {"links"}, "format": {"json"}, "formatversion": {"2"}, "redirects": {"1"},
	}
	if err := c.get(ctx, q, &out); err != nil {
		return nil, err
	}
	titles := make([]string, 0, len(out.Parse.Links))
	for _, l := range out.Parse.Links {
		if l.NS == 0 && l.Exists {
			titles = append(titles, l.Title)
		}
	}
	return titles, nil
}

// coordinates batch-resolves article titles to locations, keeping only titles
// that carry coordinates.
func (c *HTTPClient) coordinates(ctx context.Context, titles []string) ([]model.Location, error) {
	var out struct {
		Query struct {
			Pages []struct {
				Title       string `json:"title"`
				Coordinates []struct {
					Lat float64 `json:"lat"`
					Lon float64 `json:"lon"`
				} `json:"coordinates"`
			} `json:"pages"`
		} `json:"query"`
	}
	q := url.Values{
		"action": {"query"}, "titles": {strings.Join(titles, "|")},
		"prop": {"coordinates"}, "colimit": {"max"},
		"format": {"json"}, "formatversion": {"2"}, "redirects": {"1"},
	}
	if err := c.get(ctx, q, &out); err != nil {
		return nil, err
	}
	// The API returns pages in its own order; restore the article's order so
	// the list reads the way the section does.
	byTitle := make(map[string]model.Location, len(out.Query.Pages))
	for _, p := range out.Query.Pages {
		if len(p.Coordinates) == 0 {
			continue
		}
		byTitle[p.Title] = model.ResolvedLocation(p.Title, "wikipedia", p.Coordinates[0].Lat, p.Coordinates[0].Lon)
	}
	locations := make([]model.Location, 0, len(byTitle))
	for _, title := range titles {
		if loc, ok := byTitle[title]; ok {
			locations = append(locations, loc)
		}
	}
	return locations, nil
}

// get performs one API GET and decodes the JSON body into out.
func (c *HTTPClient) get(ctx context.Context, q url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("%w: build request: %w", ErrRequest, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRequest, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: status %d", ErrRequest, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrRequest, err)
	}
	return nil
}
