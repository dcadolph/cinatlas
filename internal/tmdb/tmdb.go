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

// imageBaseURL is the TMDB image CDN root.
const imageBaseURL = "https://image.tmdb.org/t/p/"

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
	movie.IMDBLocationsURL = imdb.LocationsURL(movie.IMDBID)
	return &movie, nil
}

// Trending returns this week's trending movies.
func (c *HTTPClient) Trending(ctx context.Context) ([]model.Movie, error) {
	return c.movieList(ctx, "/trending/movie/week")
}

// NowPlaying returns movies currently in theaters.
func (c *HTTPClient) NowPlaying(ctx context.Context) ([]model.Movie, error) {
	return c.movieList(ctx, "/movie/now_playing")
}

// Upcoming returns movies with upcoming releases.
func (c *HTTPClient) Upcoming(ctx context.Context) ([]model.Movie, error) {
	return c.movieList(ctx, "/movie/upcoming")
}

// Recommendations returns movies recommended alongside the given movie.
func (c *HTTPClient) Recommendations(ctx context.Context, id int) ([]model.Movie, error) {
	return c.movieList(ctx, "/movie/"+strconv.Itoa(id)+"/recommendations")
}

// movieList fetches a movie list endpoint and maps the results in order.
func (c *HTTPClient) movieList(ctx context.Context, path string) ([]model.Movie, error) {
	var out struct {
		Results []movieDTO `json:"results"`
	}
	if err := c.get(ctx, path, nil, &out); err != nil {
		return nil, err
	}
	movies := make([]model.Movie, 0, len(out.Results))
	for _, r := range out.Results {
		movies = append(movies, r.toModel())
	}
	return movies, nil
}

// SearchMulti returns movies and people matching the query in TMDB's blended
// relevance order, plus which kind ranked first: movie, person, or empty.
func (c *HTTPClient) SearchMulti(ctx context.Context, query string) ([]model.Movie, []model.Person, string, error) {
	var out struct {
		Results []multiDTO `json:"results"`
	}
	q := url.Values{"query": {query}}
	if err := c.get(ctx, "/search/multi", q, &out); err != nil {
		return nil, nil, "", err
	}
	var movies []model.Movie
	var people []model.Person
	first := ""
	for _, r := range out.Results {
		switch r.MediaType {
		case "movie":
			movies = append(movies, r.movie())
		case "person":
			people = append(people, r.person())
		default:
			continue
		}
		if first == "" {
			first = r.MediaType
		}
	}
	return movies, people, first, nil
}

// multiDTO is one blended search result, movie or person by media type.
type multiDTO struct {
	MediaType   string `json:"media_type"`
	ID          int    `json:"id"`
	Title       string `json:"title"`
	ReleaseDate string `json:"release_date"`
	PosterPath  string `json:"poster_path"`
	Name        string `json:"name"`
	ProfilePath string `json:"profile_path"`
	KnownFor    string `json:"known_for_department"`
}

// movie converts a movie-typed result to the shared movie type.
func (m multiDTO) movie() model.Movie {
	return model.Movie{
		TMDBID:      m.ID,
		Title:       m.Title,
		Year:        parseYear(m.ReleaseDate),
		ReleaseDate: m.ReleaseDate,
		PosterURL:   imageURL("w342", m.PosterPath),
	}
}

// person converts a person-typed result to the shared person type.
func (m multiDTO) person() model.Person {
	return model.Person{
		TMDBID:   m.ID,
		Name:     m.Name,
		KnownFor: m.KnownFor,
		PhotoURL: imageURL("w185", m.ProfilePath),
	}
}

// FindByIMDB returns the movie carrying the given IMDB title id, or
// ErrNotFound when TMDB does not know it.
func (c *HTTPClient) FindByIMDB(ctx context.Context, imdbID string) (*model.Movie, error) {
	var out struct {
		MovieResults []movieDTO `json:"movie_results"`
	}
	q := url.Values{"external_source": {"imdb_id"}}
	if err := c.get(ctx, "/find/"+url.PathEscape(imdbID), q, &out); err != nil {
		return nil, err
	}
	if len(out.MovieResults) == 0 {
		return nil, ErrNotFound
	}
	movie := out.MovieResults[0].toModel()
	movie.IMDBID = imdbID
	movie.IMDBURL = imdb.TitleURL(imdbID)
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
	ID           int        `json:"id"`
	Title        string     `json:"title"`
	ReleaseDate  string     `json:"release_date"`
	IMDBID       string     `json:"imdb_id"`
	Overview     string     `json:"overview"`
	Tagline      string     `json:"tagline"`
	Runtime      int        `json:"runtime"`
	VoteAverage  float64    `json:"vote_average"`
	Genres       []genreDTO `json:"genres"`
	PosterPath   string     `json:"poster_path"`
	BackdropPath string     `json:"backdrop_path"`
	Credits      creditsDTO `json:"credits"`
}

// genreDTO is one genre tag on a movie.
type genreDTO struct {
	Name string `json:"name"`
}

// toModel converts the DTO to the shared movie type.
func (m movieDTO) toModel() model.Movie {
	var genres []string
	for _, g := range m.Genres {
		genres = append(genres, g.Name)
	}
	return model.Movie{
		TMDBID:      m.ID,
		IMDBID:      m.IMDBID,
		Title:       m.Title,
		Year:        parseYear(m.ReleaseDate),
		ReleaseDate: m.ReleaseDate,
		Overview:    m.Overview,
		Tagline:     m.Tagline,
		Runtime:     m.Runtime,
		Rating:      m.VoteAverage,
		Genres:      genres,
		PosterURL:   imageURL("w342", m.PosterPath),
		BackdropURL: imageURL("w1280", m.BackdropPath),
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
			PhotoURL:  imageURL("w185", m.ProfilePath),
		})
	}
	return people
}

// castDTO is one acting credit on a movie.
type castDTO struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Character   string `json:"character"`
	ProfilePath string `json:"profile_path"`
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
	ProfilePath     string           `json:"profile_path"`
	CombinedCredits combinedCredited `json:"combined_credits"`
}

// toModel converts the DTO to the shared person type.
func (p personDTO) toModel() model.Person {
	return model.Person{
		TMDBID:   p.ID,
		IMDBID:   p.IMDBID,
		Name:     p.Name,
		KnownFor: p.KnownFor,
		PhotoURL: imageURL("w185", p.ProfilePath),
	}
}

// combinedCredited holds a person's acting and crew credits across titles.
type combinedCredited struct {
	Cast []personCreditDTO `json:"cast"`
	Crew []personCreditDTO `json:"crew"`
}

// creditModels flattens acting and crew credits into one list ordered by
// fame, merging repeat credits on the same title into one entry with joined
// roles. Self appearances on talk shows, documentaries, and archive footage
// are dropped: they are not the person's work.
func (c combinedCredited) creditModels() []model.Credit {
	credits := make([]model.Credit, 0, len(c.Cast)+len(c.Crew))
	index := make(map[string]int, len(c.Cast)+len(c.Crew))
	add := func(credit model.Credit) {
		key := fmt.Sprintf("%d/%s", credit.TMDBID, credit.Title)
		if at, ok := index[key]; ok {
			merged := &credits[at]
			if credit.Job != "" {
				if merged.Job != "" {
					merged.Job += ", " + credit.Job
				} else {
					merged.Job = credit.Job
				}
			}
			if merged.Character == "" {
				merged.Character = credit.Character
			}
			return
		}
		index[key] = len(credits)
		credits = append(credits, credit)
	}
	for _, m := range c.Cast {
		if m.selfAppearance() {
			continue
		}
		add(m.toModel())
	}
	for _, m := range c.Crew {
		add(m.toModel())
	}
	sort.SliceStable(credits, func(i, j int) bool {
		if credits[i].Votes != credits[j].Votes {
			return credits[i].Votes > credits[j].Votes
		}
		return credits[i].Year > credits[j].Year
	})
	return credits
}

// personCreditDTO is one filmography entry, movie or television.
type personCreditDTO struct {
	ID           int     `json:"id"`
	MediaType    string  `json:"media_type"`
	Title        string  `json:"title"`
	Name         string  `json:"name"`
	Character    string  `json:"character"`
	Job          string  `json:"job"`
	ReleaseDate  string  `json:"release_date"`
	FirstAirDate string  `json:"first_air_date"`
	PosterPath   string  `json:"poster_path"`
	VoteCount    int     `json:"vote_count"`
	Popularity   float64 `json:"popularity"`
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
		TMDBID:    p.ID,
		Kind:      p.MediaType,
		Title:     title,
		Year:      parseYear(date),
		Character: p.Character,
		Job:       p.Job,
		Votes:     p.VoteCount,
		PosterURL: imageURL("w342", p.PosterPath),
	}
}

// selfAppearance reports credits where the person appears as themselves:
// talk shows, documentaries, award footage. They read as noise next to roles.
func (p personCreditDTO) selfAppearance() bool {
	c := strings.ToLower(strings.TrimSpace(p.Character))
	return c == "self" || strings.HasPrefix(c, "self ") || strings.HasPrefix(c, "self,") ||
		strings.HasPrefix(c, "self (") || strings.HasPrefix(c, "self -") ||
		c == "himself" || c == "herself" || c == "themselves" ||
		strings.Contains(c, "archive footage") || strings.Contains(c, "(archival)")
}

// imageURL builds a TMDB image CDN link for the given size and path, or an
// empty string when the path is absent.
func imageURL(size, path string) string {
	if path == "" {
		return ""
	}
	return imageBaseURL + size + path
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
