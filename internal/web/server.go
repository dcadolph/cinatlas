// Package web serves the cinatlas site: one search box over the same movie,
// person, and filming-location lookups the CLI answers.
package web

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/dcadolph/cinatlas/internal/model"
	"github.com/dcadolph/cinatlas/internal/tmdb"
	"github.com/dcadolph/cinatlas/internal/wikidata"
)

//go:embed templates static
var siteFS embed.FS

// maxAlternates caps the other-matches strip under a result.
const maxAlternates = 5

// maxCredits caps filmography rows on a person page.
const maxCredits = 30

// maxTrending caps the trending wall on the home page.
const maxTrending = 18

// maxSimilar caps the more-like-this row on a movie page.
const maxSimilar = 6

// Server renders and serves the cinatlas site.
type Server struct {
	// tmdb answers movie and person lookups.
	tmdb *tmdb.HTTPClient
	// locations answers filming-location lookups.
	locations wikidata.LocationFinder
	// tmpl is the parsed page template.
	tmpl *template.Template
	// log receives request diagnostics.
	log *slog.Logger
}

// New returns a Server. It panics on nil dependencies, which are developer
// errors, and returns an error only when the embedded template fails to parse.
func New(client *tmdb.HTTPClient, finder wikidata.LocationFinder, log *slog.Logger) (*Server, error) {
	if client == nil {
		panic("web.New: tmdb client required")
	}
	if finder == nil {
		panic("web.New: location finder required")
	}
	if log == nil {
		panic("web.New: logger required")
	}
	tmpl, err := template.New("index.html").Funcs(template.FuncMap{
		"runtime": formatRuntime,
		"rating":  formatRating,
	}).ParseFS(siteFS, "templates/index.html")
	if err != nil {
		return nil, fmt.Errorf("web: parse template: %w", err)
	}
	return &Server{tmdb: client, locations: finder, tmpl: tmpl, log: log}, nil
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

// Routes returns the site handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /movie", s.handleMovie)
	mux.HandleFunc("GET /person", s.handlePerson)
	mux.Handle("GET /static/", http.FileServerFS(siteFS))
	mux.HandleFunc("/", s.handleNotFound)
	return mux
}

// handleNotFound renders a styled 404 for unknown paths.
func (s *Server) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	s.render(w, http.StatusNotFound, pageData{
		Kind:  "movie",
		Error: "That reel does not exist. Try a search instead.",
	})
}

// pageData is everything one page render needs.
type pageData struct {
	// Query is the search text echoed back into the box.
	Query string
	// Kind selects the active search pill, movie or person.
	Kind string
	// Movie is the movie result, nil on other pages.
	Movie *model.Movie
	// Person is the person result, nil on other pages.
	Person *model.Person
	// MovieAlternates are other search matches for disambiguation.
	MovieAlternates []model.Movie
	// PersonAlternates are other search matches for disambiguation.
	PersonAlternates []model.Person
	// MoreCredits reports that the filmography was truncated.
	MoreCredits bool
	// MapURL is the OpenStreetMap embed for the first located place.
	MapURL template.URL
	// Trending is the home-page trending wall.
	Trending []model.Movie
	// Similar is the more-like-this row under a movie.
	Similar []model.Movie
	// Error is a human-readable failure to show instead of a result.
	Error string
}

// handleHome renders the search page over a trending wall.
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	data := pageData{Kind: "movie"}
	trending, err := s.tmdb.Trending(r.Context())
	if err != nil {
		s.log.Error("trending fetch failed", "err", err)
	} else {
		data.Trending = trending[:min(len(trending), maxTrending)]
	}
	s.render(w, http.StatusOK, data)
}

// handleMovie resolves a movie by id or query and renders its full card:
// details, cast, and filming locations with a map.
func (s *Server) handleMovie(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	data := pageData{Query: query, Kind: "movie"}

	id, ok := s.resolveMovieID(ctx, r, &data)
	if !ok {
		s.render(w, http.StatusNotFound, data)
		return
	}
	movie, err := s.tmdb.Movie(ctx, id)
	if err != nil {
		s.log.Error("movie fetch failed", "id", id, "err", err)
		data.Error = "That movie could not be loaded. Try again."
		s.render(w, http.StatusBadGateway, data)
		return
	}
	if movie.IMDBID != "" {
		if locs, err := s.locations.Locations(ctx, movie.IMDBID); err != nil {
			s.log.Error("location lookup failed", "imdbId", movie.IMDBID, "err", err)
		} else {
			movie.Locations = locs
		}
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
	s.render(w, http.StatusOK, data)
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
	data := pageData{Query: query, Kind: "person"}

	id, ok := s.resolvePersonID(ctx, r, &data)
	if !ok {
		s.render(w, http.StatusNotFound, data)
		return
	}
	person, err := s.tmdb.Person(ctx, id)
	if err != nil {
		s.log.Error("person fetch failed", "id", id, "err", err)
		data.Error = "That person could not be loaded. Try again."
		s.render(w, http.StatusBadGateway, data)
		return
	}
	if len(person.Credits) > maxCredits {
		person.Credits = person.Credits[:maxCredits]
		data.MoreCredits = true
	}
	if data.Query == "" {
		data.Query = person.Name
	}
	data.Person = person
	s.render(w, http.StatusOK, data)
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

// render executes the page template into a buffer so a failure can become a
// clean 500 instead of a half-written page.
func (s *Server) render(w http.ResponseWriter, status int, data pageData) {
	var buf bytes.Buffer
	if err := s.tmpl.Execute(&buf, data); err != nil {
		s.log.Error("template render failed", "err", err)
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
