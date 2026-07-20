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

	"github.com/dcadolph/cinatlas/internal/ddd"
	"github.com/dcadolph/cinatlas/internal/fitfind"
	"github.com/dcadolph/cinatlas/internal/imdb"
	"github.com/dcadolph/cinatlas/internal/locate"
	"github.com/dcadolph/cinatlas/internal/model"
	"github.com/dcadolph/cinatlas/internal/sin"
	"github.com/dcadolph/cinatlas/internal/taste"
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
	// triggers answers content trigger lookups for the fit page, nil when no
	// DoesTheDogDie key is configured; hard vetoes then report as unverified.
	triggers ddd.TriggerSource
	// finder runs the family fit pipeline.
	finder *fitfind.Finder
	// sin runs the adults-only discovery lens.
	sin *sin.Finder
	// enhancer refines mood queries beyond the lexicon, nil when no model is
	// wired; the lexicon then answers on its own.
	enhancer taste.Enhancer
	// sinEnhancer refines sin-mode queries without sanitizing them, nil when
	// no model is wired.
	sinEnhancer taste.Enhancer
	// tmpl holds the parsed page templates.
	tmpl *template.Template
	// log receives request diagnostics.
	log *slog.Logger
}

// New returns a Server. It panics on nil required dependencies, which are
// developer errors, and returns an error only when the embedded templates fail
// to parse. A nil trigger source is allowed and disables content checks.
func New(client *tmdb.HTTPClient, locator locate.Atlas, triggers ddd.TriggerSource,
	log *slog.Logger) (*Server, error) {
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
	return &Server{
		tmdb:     client,
		locator:  locator,
		triggers: triggers,
		finder:   fitfind.New(client, triggers, log),
		sin:      sin.New(client, log),
		tmpl:     tmpl,
		log:      log,
	}, nil
}

// SetEnhancer wires an optional mood-query enhancer. Passing nil leaves the
// lexicon to answer on its own.
func (s *Server) SetEnhancer(e taste.Enhancer) {
	s.enhancer = e
}

// SetSinEnhancer wires an optional sin-mode query enhancer. Passing nil
// leaves the lexicon and anchor vocabulary to answer on their own.
func (s *Server) SetSinEnhancer(e taste.Enhancer) {
	s.sinEnhancer = e
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
	mux.HandleFunc("GET /find", s.handleFind)
	mux.HandleFunc("GET /sin", s.handleSin)
	mux.HandleFunc("GET /globe", s.handleGlobe)
	mux.HandleFunc("GET /globe/pins", s.handleGlobePins)
	mux.HandleFunc("GET /fit", s.handleFit)
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
// the correction. When no variant matches a title, it falls back to a
// similarity-ranked person lookup so a badly misspelled name still resolves.
// It is a no-op when nothing plausible is found.
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

	people := s.fuzzyPeople(ctx, query)
	if len(people) == 0 {
		return
	}
	data.SearchPeople = people[:min(len(people), maxShelf)]
	data.PeopleFirst = true
	data.Corrected = people[0].Name
}

// fuzzyPeople resolves a misspelled name to real people by searching TMDB for
// the query, each of its words, and the prefix-relaxed variants, then ranking
// every distinct candidate by name similarity to the query. It returns the
// matches above the plausibility threshold, best first, or nil when none clear
// it. This is what turns "matthew mcaughnehey" into Matthew McConaughey: the
// bare query returns nothing, but the "matthew" word search surfaces him and
// the last name still scores closest.
func (s *Server) fuzzyPeople(ctx context.Context, query string) []model.Person {
	seen := make(map[int]bool)
	var candidates []model.Person
	for _, variant := range personSearchVariants(query) {
		results, err := s.tmdb.SearchPerson(ctx, variant)
		if err != nil {
			s.log.Error("fuzzy person search failed", "variant", variant, "err", err)
			continue
		}
		for _, p := range results {
			if !seen[p.TMDBID] {
				seen[p.TMDBID] = true
				candidates = append(candidates, p)
			}
		}
		if len(candidates) >= 60 {
			break
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return nameScore(query, candidates[i].Name) > nameScore(query, candidates[j].Name)
	})
	var matches []model.Person
	for _, p := range candidates {
		if nameScore(query, p.Name) < nameMatchThreshold {
			break
		}
		matches = append(matches, p)
	}
	return matches
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
// findData is everything the mood-search page needs.
type findData struct {
	// Query is the mood text echoed back into the box.
	Query string
	// Movies are the discovered films in ranked order.
	Movies []model.Movie
	// Chips describe how the query was read, for display.
	Chips []string
	// Searched reports that a query ran, distinguishing an empty box from a
	// query that found nothing.
	Searched bool
	// Enhanced reports that a model refined the read beyond the lexicon.
	Enhanced bool
}

// maxFindResults caps the mood-search shelf.
const maxFindResults = 24

// handleFind answers a plain-language mood with discovered films. It reads the
// query into an intent with the lexicon, lets an enhancer refine it when one
// is wired, resolves theme keywords to ids, and runs a TMDB discovery.
func (s *Server) handleFind(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	data := findData{Query: query}
	if query == "" {
		s.render(w, http.StatusOK, "find.html", data)
		return
	}
	data.Searched = true

	// Pull any named cast out first so the remainder reads as pure mood.
	castIDs, castNames, mood := s.castFilter(ctx, query)

	intent := taste.Parse(mood)
	if s.enhancer != nil {
		if refined, err := s.enhancer.Enhance(ctx, mood, intent); err != nil {
			s.log.Error("mood enhance failed", "query", mood, "err", err)
		} else {
			intent = refined
			data.Enhanced = true
		}
	}
	data.Chips = castChips(castNames, intent.Labels())

	movies, err := s.discover(ctx, intent, castIDs)
	if err != nil {
		s.log.Error("discover failed", "query", query, "err", err)
		s.render(w, http.StatusBadGateway, "find.html", data)
		return
	}
	data.Movies = movies[:min(len(movies), maxFindResults)]
	s.render(w, http.StatusOK, "find.html", data)
}

// minDiscoverResults is the shelf size discovery tries to reach before it
// relaxes filters.
const minDiscoverResults = 8

// discover runs a TMDB discovery for the intent and any named cast, relaxing
// from most specific to least until the shelf fills. Keywords drop first, then
// a blended genre set narrows to its lead genre. The cast filter and quality
// floors never relax, since those are hard constraints the viewer asked for.
func (s *Server) discover(ctx context.Context, intent taste.Intent, castIDs []int) ([]model.Movie, error) {
	base := tmdb.DiscoverQuery{
		WithGenres:     intent.Genres,
		WithoutGenres:  intent.ExcludeGenres,
		WithCast:       castIDs,
		VoteAverageGTE: intent.MinRating,
		VoteCountGTE:   intent.MinVotes,
		SortBy:         intent.Sort,
	}
	if intent.YearFrom > 0 {
		base.ReleaseDateGTE = fmt.Sprintf("%04d-01-01", intent.YearFrom)
	}
	if intent.YearTo > 0 {
		base.ReleaseDateLTE = fmt.Sprintf("%04d-12-31", intent.YearTo)
	}

	attempts := make([]tmdb.DiscoverQuery, 0, 3)
	if keywords := s.resolveKeywords(ctx, intent.Keywords); len(keywords) > 0 {
		withKeywords := base
		withKeywords.WithKeywords = keywords
		attempts = append(attempts, withKeywords)
	}
	attempts = append(attempts, base)
	if len(base.WithGenres) > 1 {
		primary := base
		primary.WithGenres = base.WithGenres[:1]
		attempts = append(attempts, primary)
	}

	var best []model.Movie
	for _, attempt := range attempts {
		movies, err := s.tmdb.Discover(ctx, attempt)
		if err != nil {
			return nil, err
		}
		if len(movies) >= minDiscoverResults {
			return movies, nil
		}
		if len(movies) > len(best) {
			best = movies
		}
	}
	return best, nil
}

// castTriggers are the words that introduce a named actor in a mood query.
var castTriggers = map[string]bool{
	"with": true, "starring": true, "featuring": true, "stars": true,
}

// nameStops end a candidate actor name inside a query.
var nameStops = map[string]bool{
	"in": true, "the": true, "cast": true, "and": true, "or": true,
	"a": true, "an": true, "as": true, "that": true, "who": true,
	"from": true, "to": true, "movie": true, "movies": true,
	"film": true, "films": true,
}

// castFilter extracts named cast members from a mood query, resolves each to a
// TMDB id, and returns the ids, their display names, and the query with those
// names removed so the remainder reads as pure mood.
func (s *Server) castFilter(ctx context.Context, query string) ([]int, []string, string) {
	words := strings.Fields(query)
	var ids []int
	var names []string
	kept := make([]string, 0, len(words))
	for i := 0; i < len(words); {
		if !castTriggers[strings.ToLower(words[i])] {
			kept = append(kept, words[i])
			i++
			continue
		}
		end := i + 1
		var candidate []string
		for end < len(words) && len(candidate) < 3 && !nameStops[strings.ToLower(words[end])] {
			candidate = append(candidate, words[end])
			end++
		}
		if id, name, ok := s.resolveActor(ctx, strings.Join(candidate, " ")); ok {
			ids = append(ids, id)
			names = append(names, name)
			i = end
			continue
		}
		kept = append(kept, words[i])
		i++
	}
	return ids, names, strings.Join(kept, " ")
}

// resolveActor resolves a candidate name to a TMDB person, requiring a close
// name match so a stray word is not mistaken for a star.
func (s *Server) resolveActor(ctx context.Context, name string) (int, string, bool) {
	if strings.TrimSpace(name) == "" {
		return 0, "", false
	}
	people := s.fuzzyPeople(ctx, name)
	if len(people) == 0 {
		return 0, "", false
	}
	return people[0].TMDBID, people[0].Name, true
}

// castChips prefixes the read chips with a "with <name>" chip per named actor.
func castChips(names, labels []string) []string {
	chips := make([]string, 0, len(names)+len(labels))
	for _, name := range names {
		chips = append(chips, "with "+name)
	}
	return append(chips, labels...)
}

// resolveKeywords maps each theme term to its top TMDB keyword id, dropping
// terms that resolve to nothing.
func (s *Server) resolveKeywords(ctx context.Context, terms []string) []int {
	var ids []int
	for _, term := range terms {
		matches, err := s.tmdb.Keyword(ctx, term)
		if err != nil {
			s.log.Error("keyword resolve failed", "term", term, "err", err)
			continue
		}
		if len(matches) > 0 {
			ids = append(ids, matches[0].ID)
		}
	}
	return ids
}

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
		results = s.fuzzyPeople(ctx, data.Query)
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

// Film caps bound how many of a person's credits the globe resolves so a
// packed filmography cannot stall the page or drown the map. Focused is the
// default lens; all opens the full set at the cost of a slower cold load.
const (
	// focusedFilmCap limits the focused lens to the most-voted credits.
	focusedFilmCap = 30
	// allFilmCap bounds the full lens so an outlier career stays tractable.
	allFilmCap = 80
	// globeWorkers bounds concurrent per-film location lookups.
	globeWorkers = 8
)

// globeData is everything the movie globe page needs.
type globeData struct {
	// Mode selects the template branch: movie, person, or landing.
	Mode string
	// Title is the movie title.
	Title string
	// Year is the release year, zero when unknown.
	Year int
	// MovieURL links back to the movie page.
	MovieURL string
	// Pins is the JSON pin array, marshaled server-side from parsed data.
	Pins template.JS
}

// personGlobeData is everything the person globe shell needs. Pins load
// asynchronously from the pins endpoint, so the shell renders instantly.
type personGlobeData struct {
	// Mode is always "person" here, selecting the template branch.
	Mode string
	// PersonID is the TMDB id the pins endpoint resolves against.
	PersonID int
	// Name is the person's display name.
	Name string
	// PhotoURL is the person's profile image, empty when none exists.
	PhotoURL string
	// PersonURL links back to the person's filmography page.
	PersonURL string
	// Scope is the active lens, focused or all.
	Scope string
}

// globePin is one marker on the globe. Film fields stay empty in movie mode
// and carry the source title in person mode.
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
	// Film is the title the pin belongs to, set only in person mode.
	Film string `json:"film,omitempty"`
	// Year is the film's release year, set only in person mode.
	Year int `json:"year,omitempty"`
	// MovieURL links the film page, set only in person mode.
	MovieURL string `json:"movieUrl,omitempty"`
	// Role is the person's character or job on the film, person mode only.
	Role string `json:"role,omitempty"`
}

// pinsResponse is the JSON payload the person pins endpoint returns.
type pinsResponse struct {
	// Pins are every resolved marker across the scoped filmography.
	Pins []globePin `json:"pins"`
	// Films counts the credits that were considered.
	Films int `json:"films"`
	// Resolved counts the films that yielded at least one pin.
	Resolved int `json:"resolved"`
	// Truncated reports that the scope capped a longer filmography.
	Truncated bool `json:"truncated"`
}

// handleGlobe dispatches the globe page: a movie's filming pins, a person's
// filmography-wide pins, or the landing prompt when neither is named.
func (s *Server) handleGlobe(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Query().Get("id") != "":
		s.movieGlobe(w, r)
	case r.URL.Query().Get("person") != "" || strings.TrimSpace(r.URL.Query().Get("q")) != "":
		s.personGlobe(w, r)
	default:
		s.render(w, http.StatusOK, "globe.html", globeData{Mode: "landing"})
	}
}

// movieGlobe renders the interactive globe with every resolved filming pin for
// one film.
func (s *Server) movieGlobe(w http.ResponseWriter, r *http.Request) {
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
		Mode:     "movie",
		Title:    movie.Title,
		Year:     movie.Year,
		MovieURL: "/movie?id=" + strconv.Itoa(movie.TMDBID),
		// Safe as trusted JS: json.Marshal output of typed data, with HTML
		// characters escaped by encoding/json.
		Pins: template.JS(raw),
	})
}

// personGlobe renders the shell for a person's filming globe. The heavy pin
// resolution runs in handleGlobePins so the map paints before the lookups
// finish.
func (s *Server) personGlobe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := s.resolveGlobePerson(ctx, r)
	if !ok {
		s.handleNotFound(w, r)
		return
	}
	person, err := s.tmdb.Person(ctx, id)
	if err != nil {
		s.log.Error("person fetch failed", "id", id, "err", err)
		s.handleNotFound(w, r)
		return
	}
	scope := "focused"
	if r.URL.Query().Get("scope") == "all" {
		scope = "all"
	}
	s.render(w, http.StatusOK, "globe.html", personGlobeData{
		Mode:      "person",
		PersonID:  id,
		Name:      person.Name,
		PhotoURL:  person.PhotoURL,
		PersonURL: "/person?id=" + strconv.Itoa(id),
		Scope:     scope,
	})
}

// resolveGlobePerson picks the person id from the person parameter or the top
// name-search match.
func (s *Server) resolveGlobePerson(ctx context.Context, r *http.Request) (int, bool) {
	if idStr := r.URL.Query().Get("person"); idStr != "" {
		id, err := strconv.Atoi(idStr)
		return id, err == nil
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		return 0, false
	}
	results, err := s.tmdb.SearchPerson(ctx, query)
	if err != nil {
		s.log.Error("globe person search failed", "query", query, "err", err)
		return 0, false
	}
	if len(results) == 0 {
		results = s.fuzzyPeople(ctx, query)
	}
	if len(results) == 0 {
		return 0, false
	}
	return results[0].TMDBID, true
}

// handleGlobePins resolves and returns every filming pin across a person's
// scoped filmography as JSON.
func (s *Server) handleGlobePins(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.Atoi(r.URL.Query().Get("person"))
	if err != nil {
		http.Error(w, "bad person id", http.StatusBadRequest)
		return
	}
	person, err := s.tmdb.Person(ctx, id)
	if err != nil {
		s.log.Error("globe person fetch failed", "id", id, "err", err)
		http.Error(w, "person load failed", http.StatusBadGateway)
		return
	}

	limit := focusedFilmCap
	if r.URL.Query().Get("scope") == "all" {
		limit = allFilmCap
	}
	credits := personFilmCredits(person.Credits)
	truncated := len(credits) > limit
	if truncated {
		credits = credits[:limit]
	}

	pins, resolved := s.resolveCreditPins(ctx, credits)
	s.writeJSON(w, pinsResponse{
		Pins:      pins,
		Films:     len(credits),
		Resolved:  resolved,
		Truncated: truncated,
	})
}

// personFilmCredits keeps a person's film credits, most famous first, deduped
// by title so the same film is never resolved twice.
func personFilmCredits(credits []model.Credit) []model.Credit {
	seen := make(map[int]bool, len(credits))
	films := make([]model.Credit, 0, len(credits))
	for _, c := range credits {
		if c.Kind != "movie" || c.TMDBID == 0 || seen[c.TMDBID] {
			continue
		}
		seen[c.TMDBID] = true
		films = append(films, c)
	}
	sort.SliceStable(films, func(i, j int) bool { return films[i].Votes > films[j].Votes })
	return films
}

// resolveCreditPins looks up filming locations for each credit concurrently
// and flattens them into globe pins tagged with their film. It returns the
// pins and the count of films that produced at least one.
func (s *Server) resolveCreditPins(ctx context.Context, credits []model.Credit) ([]globePin, int) {
	type filmPins struct {
		pins []globePin
	}
	results := make([]filmPins, len(credits))

	sem := make(chan struct{}, globeWorkers)
	var wg sync.WaitGroup
	for i, credit := range credits {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			continue
		}
		wg.Add(1)
		go func(idx int, c model.Credit) {
			defer wg.Done()
			defer func() { <-sem }()
			results[idx] = filmPins{pins: s.filmPins(ctx, c)}
		}(i, credit)
	}
	wg.Wait()

	pins := make([]globePin, 0)
	resolved := 0
	for _, r := range results {
		if len(r.pins) > 0 {
			resolved++
			pins = append(pins, r.pins...)
		}
	}
	return pins, resolved
}

// filmPins resolves one credit to its located filming pins, tagged with the
// film's title, year, page link, and the person's role. It returns nil when
// the film has no IMDB id or no resolved coordinates.
func (s *Server) filmPins(ctx context.Context, c model.Credit) []globePin {
	movie, err := s.tmdb.Movie(ctx, c.TMDBID)
	if err != nil || movie.IMDBID == "" {
		return nil
	}
	located, err := s.locator.Locate(ctx, movie.IMDBID)
	if err != nil {
		return nil
	}
	role := c.Character
	if role == "" {
		role = c.Job
	}
	movieURL := "/movie?id=" + strconv.Itoa(c.TMDBID)
	pins := make([]globePin, 0, len(located.Filming))
	for _, loc := range located.Filming {
		if !loc.Resolved {
			continue
		}
		pins = append(pins, globePin{
			Name: loc.Name, Source: loc.Source,
			Lat: loc.Latitude, Lon: loc.Longitude,
			Maps: loc.MapsURL, Earth: loc.EarthURL,
			Film: c.Title, Year: c.Year, MovieURL: movieURL, Role: role,
		})
	}
	return pins
}

// writeJSON marshals a value as the JSON body of a successful response.
func (s *Server) writeJSON(w http.ResponseWriter, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		s.log.Error("json marshal failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(raw)
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
