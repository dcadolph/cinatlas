// Package tmdb is a client for the Movie Database API. It supplies cast, crew,
// filmography, and the IMDB ids that other packages link against.
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dcadolph/cinatlas/internal/imdb"
	"github.com/dcadolph/cinatlas/internal/model"
)

// ErrNoAPIKey reports that no TMDB key was supplied to the constructor.
var ErrNoAPIKey = errors.New("tmdb: api key required")

// ErrNotFound reports that a requested resource does not exist.
var ErrNotFound = errors.New("tmdb: not found")

// ErrRequest reports a failed or non-success API request.
var ErrRequest = errors.New("tmdb: request failed")

// defaultBaseURL is the TMDB v3 API root.
const defaultBaseURL = "https://api.themoviedb.org/3"

// MovieSearcher finds movies matching a free-text query.
type MovieSearcher interface {
	SearchMovie(ctx context.Context, query string) ([]model.Movie, error)
}

// MovieFetcher fetches full details for a single movie by TMDB id.
type MovieFetcher interface {
	Movie(ctx context.Context, id int) (*model.Movie, error)
}

// PersonSearcher finds people matching a free-text query.
type PersonSearcher interface {
	SearchPerson(ctx context.Context, query string) ([]model.Person, error)
}

// PersonFetcher fetches full details for a single person by TMDB id.
type PersonFetcher interface {
	Person(ctx context.Context, id int) (*model.Person, error)
}

// Option configures an HTTPClient at construction time.
type Option func(*HTTPClient)

// WithHTTPClient sets the underlying HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *HTTPClient) { c.httpClient = h }
}

// WithBaseURL overrides the API root, mainly for tests.
func WithBaseURL(base string) Option {
	return func(c *HTTPClient) { c.baseURL = strings.TrimRight(base, "/") }
}

// HTTPClient talks to the TMDB API over HTTP.
type HTTPClient struct {
	// key is the TMDB v3 key or v4 read access token.
	key string
	// bearer reports whether key is a v4 token sent as a bearer header.
	bearer bool
	// baseURL is the API root.
	baseURL string
	// httpClient performs the requests.
	httpClient *http.Client
}

// New returns an HTTPClient for the given key. It returns ErrNoAPIKey when the
// key is empty so importers can handle the misconfiguration.
func New(key string, opts ...Option) (*HTTPClient, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, ErrNoAPIKey
	}
	c := &HTTPClient{
		key:        key,
		bearer:     strings.Count(key, ".") == 2,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// SearchMovie returns movies matching the query, best match first.
func (c *HTTPClient) SearchMovie(ctx context.Context, query string) ([]model.Movie, error) {
	var out struct {
		Results []movieDTO `json:"results"`
	}
	q := url.Values{"query": {query}}
	if err := c.get(ctx, "/search/movie", q, &out); err != nil {
		return nil, err
	}
	movies := make([]model.Movie, 0, len(out.Results))
	for _, r := range out.Results {
		movies = append(movies, r.toModel())
	}
	return movies, nil
}

// Movie returns full details for the movie with the given id, including the
// director, billed cast, and IMDB link.
func (c *HTTPClient) Movie(ctx context.Context, id int) (*model.Movie, error) {
	var dto movieDTO
	q := url.Values{"append_to_response": {"credits"}}
	if err := c.get(ctx, "/movie/"+strconv.Itoa(id), q, &dto); err != nil {
		return nil, err
	}
	movie := dto.toModel()
	movie.Director = dto.Credits.director()
	movie.Cast = dto.Credits.castModels()
	movie.IMDBURL = imdb.TitleURL(movie.IMDBID)
	return &movie, nil
}

// SearchPerson returns people matching the query, best match first.
func (c *HTTPClient) SearchPerson(ctx context.Context, query string) ([]model.Person, error) {
	var out struct {
		Results []personDTO `json:"results"`
	}
	q := url.Values{"query": {query}}
	if err := c.get(ctx, "/search/person", q, &out); err != nil {
		return nil, err
	}
	people := make([]model.Person, 0, len(out.Results))
	for _, r := range out.Results {
		people = append(people, r.toModel())
	}
	return people, nil
}

// Person returns full details for the person with the given id, including a
// year-sorted filmography and IMDB link.
func (c *HTTPClient) Person(ctx context.Context, id int) (*model.Person, error) {
	var dto personDTO
	q := url.Values{"append_to_response": {"combined_credits"}}
	if err := c.get(ctx, "/person/"+strconv.Itoa(id), q, &dto); err != nil {
		return nil, err
	}
	person := dto.toModel()
	person.Credits = dto.CombinedCredits.creditModels()
	person.IMDBURL = imdb.NameURL(person.IMDBID)
	return &person, nil
}

// get performs a GET against the API and decodes the JSON body into out.
func (c *HTTPClient) get(ctx context.Context, path string, q url.Values, out any) error {
	if q == nil {
		q = url.Values{}
	}
	if !c.bearer {
		q.Set("api_key", c.key)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("%w: build request: %w", ErrRequest, err)
	}
	req.Header.Set("Accept", "application/json")
	if c.bearer {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRequest, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: status %d", ErrRequest, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrRequest, err)
	}
	return nil
}

// movieDTO is the subset of the TMDB movie payload cinatlas reads.
type movieDTO struct {
	ID          int        `json:"id"`
	Title       string     `json:"title"`
	ReleaseDate string     `json:"release_date"`
	IMDBID      string     `json:"imdb_id"`
	Credits     creditsDTO `json:"credits"`
}

// toModel converts the DTO to the shared movie type.
func (m movieDTO) toModel() model.Movie {
	return model.Movie{
		TMDBID: m.ID,
		IMDBID: m.IMDBID,
		Title:  m.Title,
		Year:   parseYear(m.ReleaseDate),
	}
}

// creditsDTO holds a movie's cast and crew.
type creditsDTO struct {
	Cast []castDTO `json:"cast"`
	Crew []crewDTO `json:"crew"`
}

// director returns the first credited director name, or empty when none.
func (c creditsDTO) director() string {
	for _, m := range c.Crew {
		if m.Job == "Director" {
			return m.Name
		}
	}
	return ""
}

// castModels converts billed cast to the shared person type.
func (c creditsDTO) castModels() []model.Person {
	people := make([]model.Person, 0, len(c.Cast))
	for _, m := range c.Cast {
		people = append(people, model.Person{
			TMDBID:    m.ID,
			Name:      m.Name,
			Character: m.Character,
		})
	}
	return people
}

// castDTO is one acting credit on a movie.
type castDTO struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Character string `json:"character"`
}

// crewDTO is one crew credit on a movie.
type crewDTO struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Job  string `json:"job"`
}

// personDTO is the subset of the TMDB person payload cinatlas reads.
type personDTO struct {
	ID              int              `json:"id"`
	Name            string           `json:"name"`
	IMDBID          string           `json:"imdb_id"`
	KnownFor        string           `json:"known_for_department"`
	CombinedCredits combinedCredited `json:"combined_credits"`
}

// toModel converts the DTO to the shared person type.
func (p personDTO) toModel() model.Person {
	return model.Person{
		TMDBID:   p.ID,
		IMDBID:   p.IMDBID,
		Name:     p.Name,
		KnownFor: p.KnownFor,
	}
}

// combinedCredited holds a person's acting and crew credits across titles.
type combinedCredited struct {
	Cast []personCreditDTO `json:"cast"`
	Crew []personCreditDTO `json:"crew"`
}

// creditModels flattens acting and crew credits into a year-sorted list.
func (c combinedCredited) creditModels() []model.Credit {
	credits := make([]model.Credit, 0, len(c.Cast)+len(c.Crew))
	for _, m := range c.Cast {
		credits = append(credits, m.toModel())
	}
	for _, m := range c.Crew {
		credits = append(credits, m.toModel())
	}
	sort.SliceStable(credits, func(i, j int) bool { return credits[i].Year > credits[j].Year })
	return credits
}

// personCreditDTO is one filmography entry, movie or television.
type personCreditDTO struct {
	Title        string `json:"title"`
	Name         string `json:"name"`
	Character    string `json:"character"`
	Job          string `json:"job"`
	ReleaseDate  string `json:"release_date"`
	FirstAirDate string `json:"first_air_date"`
}

// toModel converts the credit DTO to the shared credit type, preferring the
// movie title and release date but falling back to television fields.
func (p personCreditDTO) toModel() model.Credit {
	title := p.Title
	if title == "" {
		title = p.Name
	}
	date := p.ReleaseDate
	if date == "" {
		date = p.FirstAirDate
	}
	return model.Credit{
		Title:     title,
		Year:      parseYear(date),
		Character: p.Character,
		Job:       p.Job,
	}
}

// parseYear extracts the leading four-digit year from a date string.
func parseYear(date string) int {
	if len(date) < 4 {
		return 0
	}
	year, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return year
}
