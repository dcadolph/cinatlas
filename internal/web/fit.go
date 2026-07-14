package web

import (
	"net/http"
	"strings"

	"github.com/dcadolph/cinatlas/internal/ddd"
	"github.com/dcadolph/cinatlas/internal/family"
	"github.com/dcadolph/cinatlas/internal/fitfind"
)

// fitGenres lists the TMDB genre names offered as soft vetoes.
var fitGenres = []string{
	"Horror", "Thriller", "Music", "Romance", "Documentary",
	"Science Fiction", "Animation", "War", "Western",
}

// fitData is everything the fit page render needs.
type fitData struct {
	// ProfileParam is the encoded profile echoed into share links.
	ProfileParam string
	// Profile is the decoded profile, nil before one is built.
	Profile *family.Profile
	// Categories groups the curated hard vetoes for the builder.
	Categories []ddd.Category
	// Genres lists the genre names offered as soft vetoes.
	Genres []string
	// Results are the passing films, best fit first.
	Results []fitfind.Result
	// Excluded counts the candidates that failed someone's constraints.
	Excluded int
	// Services is the comma-separated streaming services the family has.
	Services string
	// ContentChecks reports whether a trigger source is wired, so the page can
	// say when hard vetoes could not be verified at all.
	ContentChecks bool
	// Error is a human-readable failure to show instead of results.
	Error string
}

// handleFit renders the family fit page: a profile builder, and when a profile
// arrives, the films that pass every member's constraints.
func (s *Server) handleFit(w http.ResponseWriter, r *http.Request) {
	data := fitData{Categories: ddd.Catalog, Genres: fitGenres, ContentChecks: s.triggers != nil}
	data.Services = strings.TrimSpace(r.URL.Query().Get("services"))
	param := strings.TrimSpace(r.URL.Query().Get("p"))
	if param == "" {
		s.render(w, http.StatusOK, "fit.html", data)
		return
	}
	profile, err := family.DecodeProfile(param)
	if err != nil {
		s.log.Error("profile decode failed", "err", err)
		data.Error = "That profile link is malformed. Rebuild it below."
		s.render(w, http.StatusBadRequest, "fit.html", data)
		return
	}
	data.ProfileParam = param
	data.Profile = &profile

	results, excluded, err := s.finder.Find(r.Context(), profile, splitServices(data.Services))
	if err != nil {
		s.log.Error("fit discover failed", "err", err)
		data.Error = "Film search failed. Try again."
		s.render(w, http.StatusBadGateway, "fit.html", data)
		return
	}
	data.Results, data.Excluded = results, excluded
	s.render(w, http.StatusOK, "fit.html", data)
}

// splitServices parses the comma-separated services parameter into clean tokens.
func splitServices(services string) []string {
	parts := strings.Split(services, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
