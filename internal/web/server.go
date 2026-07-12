// Package web serves the cinatlas site: one search box over the same movie,
// person, and filming-location lookups the CLI answers, plus an interactive
// globe of every filming pin.
package web

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dcadolph/cinatlas/internal/imdb"
	"github.com/dcadolph/cinatlas/internal/locate"
	"github.com/dcadolph/cinatlas/internal/model"
	"github.com/dcadolph/cinatlas/internal/tmdb"
)

//go:embed templates static
var siteFS embed.FS

// maxAlternates caps the other-matches strip under a result.
const maxAlternates = 5

// creditsPerPage sizes one filmography page.
const creditsPerPage = 24

// maxCast caps the cast shelf at top billing, where photos are dependable.
const maxCast = 14

// maxShelf caps each poster shelf on the home page.
const maxShelf = 12

// maxSimilar caps the more-like-this row on a movie page.
const maxSimilar = 6

// maxPlaceMovies caps the filmed-here shelf on a place page.
const maxPlaceMovies = 24

// Server renders and serves the cinatlas site.
type Server struct {
	// tmdb answers movie and person lookups.
	tmdb *tmdb.HTTPClient
	// locator answers filming facts in both directions.
	locator locate.Atlas
	// tmpl holds the parsed page templates.
	tmpl *template.Template
	// log receives request diagnostics.
	log *slog.Logger
}

// New returns a Server. It panics on nil dependencies, which are developer
// errors, and returns an error only when the embedded templates fail to parse.
func New(client *tmdb.HTTPClient, locator locate.Atlas, log *slog.Logger) (*Server, error) {
	if client == nil {
		panic("web.New: tmdb client required")
	}
	if locator == nil {
		panic("web.New: locator required")
	}
	if log == nil {
		panic("web.New: logger required")
	}
	tmpl, err := template.New("site").Funcs(template.FuncMap{
		"runtime":   formatRuntime,
		"rating":    formatRating,
		"shortDate": formatShortDate,
		"add":       func(a, b int) int { return a + b },
		"sub":       func(a, b int) int { return a - b },
	}).ParseFS(siteFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("web: parse templates: %w", err)
	}
	return &Server{tmdb: client, locator: locator, tmpl: tmpl, log: log}, nil
}

// formatRuntime renders minutes as "1h 38m".
func formatRuntime(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dh %dm", minutes/60, minutes%60)
}

// formatRating renders a 0 to 10 vote average with one decimal.
func formatRating(rating float64) string {
	return fmt.Sprintf("%.1f", rating)
}

// formatShortDate renders an ISO date as "Dec 19", falling back to the input.
func formatShortDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("Jan 2")
}

// Routes returns the site handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /search", s.handleSearch)
	mux.HandleFunc("GET /movie", s.handleMovie)
	mux.HandleFunc("GET /person", s.handlePerson)
	mux.HandleFunc("GET /place", s.handlePlace)
	mux.HandleFunc("GET /globe", s.handleGlobe)
	mux.Handle("GET /static/", http.FileServerFS(siteFS))
	mux.HandleFunc("/", s.handleNotFound)
	return mux
}

// handleNotFound renders a styled 404 for unknown paths.
func (s *Server) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	s.render(w, http.StatusNotFound, "index.html", pageData{
		Error: "That reel does not exist. Try a search instead.",
	})
}

// pageData is everything one page render needs.
type pageData struct {
	// Query is the search text echoed back into the box.
	Query string
	// Movie is the movie result, nil on other pages.
	Movie *model.Movie
	// Person is the person result, nil on other pages.
	Person *model.Person
	// MovieAlternates are other search matches for disambiguation.
	MovieAlternates []model.Movie
	// PersonAlternates are other search matches for disambiguation.
	PersonAlternates []model.Person
	// MoreCast reports that the cast shelf was truncated to top billing.
	MoreCast bool
	// FullCreditsURL links the IMDB full cast page when the shelf truncates.
	FullCreditsURL string
	// MapURL is the OpenStreetMap embed for the first located place.
	MapURL template.URL
	// Trending is the home-page trending wall.
	Trending []model.Movie
	// NowPlaying is the home-page in-theaters shelf.
	NowPlaying []model.Movie
	// Upcoming is the home-page coming-soon shelf, soonest first.
	Upcoming []model.Movie
	// PopularPeople is the home-page trending-people shelf.
	PopularPeople []model.Person
	// Similar is the more-like-this row under a movie.
	Similar []model.Movie
	// PlaceName is the place a reverse search matched.
	PlaceName string
	// PlaceMovies are one page of films shot at the searched place.
	PlaceMovies []model.Movie
	// PlaceTotal counts every film matched at the place.
	PlaceTotal int
	// Page is the current place page, starting at 1.
	Page int
	// TotalPages counts the place pages.
	TotalPages int
	// Sort is the active place ordering.
	Sort string
	// Decade is the active decade filter, zero for all.
	Decade int
	// Decades lists the decades available to filter by.
	Decades []int
	// Media is the active medium filter on a person: movie, tv, or empty.
	Media string
	// Role is the active role filter on a person: acting, crew, or empty.
	Role string
	// Genre is the active genre filter on a person, empty for all.
	Genre string
	// Genres lists the genres available to filter a person's credits by.
	Genres []string
	// SearchMovies are unified-search movie matches in relevance order.
	SearchMovies []model.Movie
	// SearchPeople are unified-search person matches in relevance order.
	SearchPeople []model.Person
	// AtMovies are unified-search films shot at the query as a place.
	AtMovies []model.Movie
	// PeopleFirst orders the people shelf above movies when a person
	// outranked every movie.
	PeopleFirst bool
	// Corrected is the relaxed query a fuzzy fallback matched on, empty
	// when the original query found results directly.
	Corrected string
	// Error is a human-readable failure to show instead of a result.
	Error string
}

// handleSearch renders one query across every axis at once: movies, people,
// and the query as a filming place, most relevant first.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	data := pageData{Query: query}
	if query == "" {
		data.Error = "Type a film, a name, or a place."
		s.render(w, http.StatusNotFound, "index.html", data)
		return
	}

	var wg sync.WaitGroup
	var first string
	wg.Add(2)
	go func() {
		defer wg.Done()
		movies, people, firstKind, err := s.tmdb.SearchMulti(ctx, query)
		if err != nil {
			s.log.Error("multi search failed", "query", query, "err", err)
			return
		}
		data.SearchMovies = movies[:min(len(movies), maxShelf)]
		data.SearchPeople = people[:min(len(people), maxShelf)]
		first = firstKind
	}()
	go func() {
		defer wg.Done()
		result, err := s.locator.At(ctx, query, locate.AtQuery{Limit: maxSimilar})
		if err != nil {
			s.log.Error("place search failed", "place", query, "err", err)
			return
		}
		data.AtMovies = result.Movies
	}()
	wg.Wait()

	data.PeopleFirst = first == "person"
	if len(data.SearchMovies) == 0 && len(data.SearchPeople) == 0 && len(data.AtMovies) == 0 {
		s.fuzzyFallback(ctx, query, &data)
	}
	if len(data.SearchMovies) == 0 && len(data.SearchPeople) == 0 && len(data.AtMovies) == 0 {
		data.Error = fmt.Sprintf("Nothing found for %q. Try another film, name, or place.", query)
		s.render(w, http.StatusNotFound, "index.html", data)
		return
	}
	s.render(w, http.StatusOK, "index.html", data)
}

// fuzzyFallback retries the multi search with typo-tolerant variants of the
// query and fills data with the first variant that matches, recording it as
// the correction. It is a no-op when no variant matches.
func (s *Server) fuzzyFallback(ctx context.Context, query string, data *pageData) {
	for _, variant := range relaxQuery(query) {
		movies, people, first, err := s.tmdb.SearchMulti(ctx, variant)
		if err != nil {
			s.log.Error("fuzzy search failed", "variant", variant, "err", err)
			continue
		}
		if len(movies) == 0 && len(people) == 0 {
			continue
		}
		data.SearchMovies = movies[:min(len(movies), maxShelf)]
		data.SearchPeople = people[:min(len(people), maxShelf)]
		data.PeopleFirst = first == "person"
		data.Corrected = variant
		return
	}
}

// handlePlace renders one page of the films shot at a searched place. Every
// match stays reachable through pagination; nothing is hidden.
func (s *Server) handlePlace(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	data := pageData{Query: query}
	if query == "" {
		data.Error = "Type a place to find what filmed there."
		s.render(w, http.StatusNotFound, "index.html", data)
		return
	}
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}
	sortOrder := r.URL.Query().Get("sort")
	decade, _ := strconv.Atoi(r.URL.Query().Get("decade"))
	result, err := s.locator.At(r.Context(), query, locate.AtQuery{
		Offset: (page - 1) * maxPlaceMovies,
		Limit:  maxPlaceMovies,
		Sort:   sortOrder,
		Decade: decade,
	})
	if err != nil {
		s.log.Error("place search failed", "place", query, "err", err)
		data.Error = "Place search failed. Try again."
		s.render(w, http.StatusBadGateway, "index.html", data)
		return
	}
	if result.Total == 0 && decade == 0 {
		data.Error = fmt.Sprintf("No films with recorded locations at %q yet. "+
			"Location data is thin outside film hubs. Try a nearby city.", query)
		s.render(w, http.StatusNotFound, "index.html", data)
		return
	}
	data.PlaceName = query
	data.PlaceMovies = result.Movies
	data.PlaceTotal = result.Total
	data.Page = page
	data.TotalPages = (result.Total + maxPlaceMovies - 1) / maxPlaceMovies
	data.Sort = sortOrder
	data.Decade = decade
	data.Decades = result.Decades
	s.render(w, http.StatusOK, "index.html", data)
}

// handleHome renders the search page over trending, in-theaters, and
// coming-soon walls, fetched concurrently and each best effort.
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := pageData{}

	var wg sync.WaitGroup
	fetch := func(dst *[]model.Movie, name string, f func(context.Context) ([]model.Movie, error)) {
		defer wg.Done()
		movies, err := f(ctx)
		if err != nil {
			s.log.Error(name+" fetch failed", "err", err)
			return
		}
		*dst = movies
	}
	wg.Add(3)
	go fetch(&data.Trending, "trending", s.tmdb.Trending)
	go fetch(&data.NowPlaying, "now playing", s.tmdb.NowPlaying)
	go fetch(&data.Upcoming, "upcoming", s.tmdb.Upcoming)
	wg.Add(1)
	go func() {
		defer wg.Done()
		people, err := s.tmdb.PopularPeople(ctx)
		if err != nil {
			s.log.Error("popular people fetch failed", "err", err)
			return
		}
		data.PopularPeople = people
	}()
	wg.Wait()

	data.Trending = data.Trending[:min(len(data.Trending), maxShelf)]
	data.NowPlaying = data.NowPlaying[:min(len(data.NowPlaying), maxShelf)]
	data.Upcoming = futureReleases(data.Upcoming, time.Now(), maxShelf)
	data.PopularPeople = data.PopularPeople[:min(len(data.PopularPeople), maxShelf)]
	s.render(w, http.StatusOK, "index.html", data)
}

// futureReleases keeps movies releasing on or after today, soonest first,
// capped at limit. ISO dates compare correctly as strings.
func futureReleases(movies []model.Movie, now time.Time, limit int) []model.Movie {
	today := now.Format("2006-01-02")
	future := make([]model.Movie, 0, len(movies))
	for _, m := range movies {
		if m.ReleaseDate >= today {
			future = append(future, m)
		}
	}
	sort.SliceStable(future, func(i, j int) bool { return future[i].ReleaseDate < future[j].ReleaseDate })
	return future[:min(len(future), limit)]
}

// handleMovie resolves a movie by id or query and renders its full card:
// details, top-billed cast, filming locations with a map, and similar titles.
func (s *Server) handleMovie(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	data := pageData{Query: query}

	id, ok := s.resolveMovieID(ctx, r, &data)
	if !ok {
		s.render(w, http.StatusNotFound, "index.html", data)
		return
	}
	movie, err := s.tmdb.Movie(ctx, id)
	if err != nil {
		s.log.Error("movie fetch failed", "id", id, "err", err)
		data.Error = "That movie could not be loaded. Try again."
		s.render(w, http.StatusBadGateway, "index.html", data)
		return
	}
	s.attachPlaces(ctx, movie)
	if len(movie.Cast) > maxCast {
		movie.Cast = movie.Cast[:maxCast]
		data.MoreCast = true
		data.FullCreditsURL = imdb.FullCreditsURL(movie.IMDBID)
	}
	if data.Query == "" {
		data.Query = movie.Title
	}
	if similar, err := s.tmdb.Recommendations(ctx, movie.TMDBID); err != nil {
		s.log.Error("recommendations fetch failed", "id", movie.TMDBID, "err", err)
	} else {
		data.Similar = similar[:min(len(similar), maxSimilar)]
	}
	data.Movie = movie
	data.MapURL = mapEmbedURL(movie.Locations)
	s.render(w, http.StatusOK, "index.html", data)
}

// attachPlaces fills filming and setting facts on the movie, best effort.
func (s *Server) attachPlaces(ctx context.Context, movie *model.Movie) {
	if movie.IMDBID == "" {
		return
	}
	located, err := s.locator.Locate(ctx, movie.IMDBID)
	if err != nil {
		s.log.Error("location lookup failed", "imdbId", movie.IMDBID, "err", err)
		return
	}
	movie.Locations = located.Filming
	movie.SetIn = located.SetIn
}

// resolveMovieID picks the movie id from the id parameter or the top search
// match, filling alternates and the error message as it goes.
func (s *Server) resolveMovieID(ctx context.Context, r *http.Request, data *pageData) (int, bool) {
	if idStr := r.URL.Query().Get("id"); idStr != "" {
		id, err := strconv.Atoi(idStr)
		if err != nil {
			data.Error = "That link looks malformed."
			return 0, false
		}
		return id, true
	}
	if data.Query == "" {
		data.Error = "Type a movie title to look it up."
		return 0, false
	}
	results, err := s.tmdb.SearchMovie(ctx, data.Query)
	if err != nil {
		s.log.Error("movie search failed", "query", data.Query, "err", err)
		data.Error = "Search failed. Try again."
		return 0, false
	}
	if len(results) == 0 {
		data.Error = fmt.Sprintf("No movie found for %q.", data.Query)
		return 0, false
	}
	if len(results) > 1 {
		end := min(len(results), maxAlternates+1)
		data.MovieAlternates = results[1:end]
	}
	return results[0].TMDBID, true
}

// handlePerson resolves a person by id or query and renders their card with a
// capped filmography.
func (s *Server) handlePerson(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	data := pageData{Query: query}

	id, ok := s.resolvePersonID(ctx, r, &data)
	if !ok {
		s.render(w, http.StatusNotFound, "index.html", data)
		return
	}
	person, err := s.tmdb.Person(ctx, id)
	if err != nil {
		s.log.Error("person fetch failed", "id", id, "err", err)
		data.Error = "That person could not be loaded. Try again."
		s.render(w, http.StatusBadGateway, "index.html", data)
		return
	}
	if data.Query == "" {
		data.Query = person.Name
	}

	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}
	data.Sort = r.URL.Query().Get("sort")
	data.Decade, _ = strconv.Atoi(r.URL.Query().Get("decade"))
	data.Media = r.URL.Query().Get("media")
	data.Role = r.URL.Query().Get("role")
	data.Genre = r.URL.Query().Get("genre")
	// Filter options come from the full credit set so a choice never hides the
	// others.
	data.Decades = model.CreditDecades(person.Credits)
	data.Genres = model.CreditGenres(person.Credits)

	credits := model.FilterCreditsByMedia(person.Credits, data.Media)
	credits = model.FilterCreditsByRole(credits, data.Role)
	credits = model.FilterCreditsByGenre(credits, data.Genre)
	credits = model.FilterCreditsByDecade(credits, data.Decade)
	model.SortCredits(credits, data.Sort)
	total := len(credits)
	data.Page = page
	data.TotalPages = (total + creditsPerPage - 1) / creditsPerPage
	start := (page - 1) * creditsPerPage
	if start >= total {
		credits = nil
	} else {
		credits = credits[start:min(total, start+creditsPerPage)]
	}
	person.Credits = credits
	data.Person = person
	s.render(w, http.StatusOK, "index.html", data)
}

// resolvePersonID picks the person id from the id parameter or the top search
// match, filling alternates and the error message as it goes.
func (s *Server) resolvePersonID(ctx context.Context, r *http.Request, data *pageData) (int, bool) {
	if idStr := r.URL.Query().Get("id"); idStr != "" {
		id, err := strconv.Atoi(idStr)
		if err != nil {
			data.Error = "That link looks malformed."
			return 0, false
		}
		return id, true
	}
	if data.Query == "" {
		data.Error = "Type a name to look it up."
		return 0, false
	}
	results, err := s.tmdb.SearchPerson(ctx, data.Query)
	if err != nil {
		s.log.Error("person search failed", "query", data.Query, "err", err)
		data.Error = "Search failed. Try again."
		return 0, false
	}
	if len(results) == 0 {
		data.Error = fmt.Sprintf("No person found for %q.", data.Query)
		return 0, false
	}
	if len(results) > 1 {
		end := min(len(results), maxAlternates+1)
		data.PersonAlternates = results[1:end]
	}
	return results[0].TMDBID, true
}

// globeData is everything the globe page needs.
type globeData struct {
	// Title is the movie title.
	Title string
	// Year is the release year, zero when unknown.
	Year int
	// MovieURL links back to the movie page.
	MovieURL string
	// Pins is the JSON pin array, marshaled server-side from parsed data.
	Pins template.JS
}

// globePin is one marker on the globe.
type globePin struct {
	// Name is the place label.
	Name string `json:"name"`
	// Source names where the fact came from.
	Source string `json:"source"`
	// Lat is the decimal latitude.
	Lat float64 `json:"lat"`
	// Lon is the decimal longitude.
	Lon float64 `json:"lon"`
	// Maps links the place on Google Maps.
	Maps string `json:"maps"`
	// Earth links the place on Google Earth.
	Earth string `json:"earth"`
}

// handleGlobe renders the interactive globe with every resolved filming pin.
func (s *Server) handleGlobe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil {
		s.handleNotFound(w, r)
		return
	}
	movie, err := s.tmdb.Movie(ctx, id)
	if err != nil {
		s.log.Error("movie fetch failed", "id", id, "err", err)
		s.handleNotFound(w, r)
		return
	}
	s.attachPlaces(ctx, movie)

	pins := make([]globePin, 0, len(movie.Locations))
	for _, loc := range movie.Locations {
		if !loc.Resolved {
			continue
		}
		pins = append(pins, globePin{
			Name: loc.Name, Source: loc.Source,
			Lat: loc.Latitude, Lon: loc.Longitude,
			Maps: loc.MapsURL, Earth: loc.EarthURL,
		})
	}
	raw, err := json.Marshal(pins)
	if err != nil {
		s.log.Error("pin marshal failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.render(w, http.StatusOK, "globe.html", globeData{
		Title:    movie.Title,
		Year:     movie.Year,
		MovieURL: "/movie?id=" + strconv.Itoa(movie.TMDBID),
		// Safe as trusted JS: json.Marshal output of typed data, with HTML
		// characters escaped by encoding/json.
		Pins: template.JS(raw),
	})
}

// render executes a page template into a buffer so a failure can become a
// clean 500 instead of a half-written page.
func (s *Server) render(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		s.log.Error("template render failed", "template", name, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// mapEmbedURL builds the OpenStreetMap iframe URL for the first location with
// coordinates. The value is built only from parsed floats, so it is safe to
// mark as a trusted template URL.
func mapEmbedURL(locs []model.Location) template.URL {
	for _, loc := range locs {
		if !loc.Resolved {
			continue
		}
		bbox := fmt.Sprintf("%.4f,%.4f,%.4f,%.4f",
			loc.Longitude-0.35, loc.Latitude-0.25, loc.Longitude+0.35, loc.Latitude+0.25)
		v := url.Values{
			"bbox":   {bbox},
			"layer":  {"mapnik"},
			"marker": {fmt.Sprintf("%.4f,%.4f", loc.Latitude, loc.Longitude)},
		}
		return template.URL("https://www.openstreetmap.org/export/embed.html?" + v.Encode())
	}
	return ""
}
