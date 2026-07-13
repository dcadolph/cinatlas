package web

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/dcadolph/cinatlas/internal/ddd"
	"github.com/dcadolph/cinatlas/internal/family"
	"github.com/dcadolph/cinatlas/internal/model"
	"github.com/dcadolph/cinatlas/internal/tmdb"
)

// maxFitPages caps the discover pages pulled per fit request.
const maxFitPages = 2

// fitWorkers bounds the concurrent per-film hydration calls.
const fitWorkers = 8

// maxFitResults caps the rendered fit grid.
const maxFitResults = 24

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
	// Topics lists the curated hard vetoes for the builder.
	Topics []ddd.Topic
	// Genres lists the genre names offered as soft vetoes.
	Genres []string
	// Results are the passing films, best fit first.
	Results []fitFilm
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

// fitFilm pairs a passing movie with its fit outcome.
type fitFilm struct {
	// Movie is the film with certification and availability attached.
	Movie model.Movie
	// Unverified lists the constraints that had no data for this film.
	Unverified []string
	// Penalty counts soft veto hits, lower first.
	Penalty int
	// SubscriptionHits counts services the family already has carrying the film.
	SubscriptionHits int
}

// handleFit renders the family fit page: a profile builder, and when a profile
// arrives, the films that pass every member's constraints.
func (s *Server) handleFit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := fitData{Topics: ddd.Topics, Genres: fitGenres, ContentChecks: s.triggers != nil}
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

	candidates, err := s.fitCandidates(ctx, profile)
	if err != nil {
		s.log.Error("fit discover failed", "err", err)
		data.Error = "Film search failed. Try again."
		s.render(w, http.StatusBadGateway, "fit.html", data)
		return
	}
	data.Results, data.Excluded = s.evaluateFit(ctx, profile, candidates, data.Services)
	s.render(w, http.StatusOK, "fit.html", data)
}

// fitCandidates pulls popular films at or under the profile's lowest ceiling.
// The first page must succeed; later pages are best effort.
func (s *Server) fitCandidates(ctx context.Context, profile family.Profile) ([]model.Movie, error) {
	ceiling := profile.LowestCeiling()
	first, err := s.tmdb.Discover(ctx, tmdb.DiscoverQuery{CertificationLTE: ceiling})
	if err != nil {
		return nil, err
	}
	movies := first
	for page := 2; page <= maxFitPages; page++ {
		more, err := s.tmdb.Discover(ctx, tmdb.DiscoverQuery{CertificationLTE: ceiling, Page: page})
		if err != nil {
			s.log.Error("fit discover page failed", "page", page, "err", err)
			break
		}
		movies = append(movies, more...)
	}
	seen := make(map[int]bool, len(movies))
	unique := movies[:0]
	for _, m := range movies {
		if seen[m.TMDBID] {
			continue
		}
		seen[m.TMDBID] = true
		unique = append(unique, m)
	}
	return unique, nil
}

// evaluateFit hydrates each candidate with its certification, trigger flags, and
// availability, runs the fit engine, and returns the passing films ranked best
// first plus the count of excluded candidates.
func (s *Server) evaluateFit(ctx context.Context, profile family.Profile,
	candidates []model.Movie, services string) ([]fitFilm, int) {
	serviceList := splitServices(services)
	var mu sync.Mutex
	var results []fitFilm
	excluded := 0

	sem := make(chan struct{}, fitWorkers)
	var wg sync.WaitGroup
	for _, candidate := range candidates {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			continue
		}
		wg.Add(1)
		go func(m model.Movie) {
			defer wg.Done()
			defer func() { <-sem }()
			film, ok := s.fitFilm(ctx, profile, m, serviceList)
			mu.Lock()
			defer mu.Unlock()
			if !ok {
				excluded++
				return
			}
			results = append(results, film)
		}(candidate)
	}
	wg.Wait()

	sort.SliceStable(results, func(i, j int) bool {
		return family.Less(
			family.Ranked{Facts: fitFacts(results[i]), Result: family.FitResult{Penalty: results[i].Penalty}},
			family.Ranked{Facts: fitFacts(results[j]), Result: family.FitResult{Penalty: results[j].Penalty}},
		)
	})
	if len(results) > maxFitResults {
		results = results[:maxFitResults]
	}
	return results, excluded
}

// fitFacts rebuilds the ranking inputs from a scored film.
func fitFacts(f fitFilm) family.FilmFacts {
	return family.FilmFacts{
		SubscriptionHits: f.SubscriptionHits,
		Popularity:       f.Movie.Popularity,
	}
}

// fitFilm scores one candidate, returning it hydrated when it passes.
func (s *Server) fitFilm(ctx context.Context, profile family.Profile, m model.Movie,
	services []string) (fitFilm, bool) {
	cert, err := s.tmdb.Certification(ctx, m.TMDBID)
	if err != nil {
		s.log.Error("certification fetch failed", "id", m.TMDBID, "err", err)
	}
	triggers := map[string]family.Trigger{}
	if s.triggers != nil {
		triggers, err = s.triggers.TriggersFor(ctx, m.Title, m.Year)
		if err != nil {
			s.log.Error("trigger fetch failed", "title", m.Title, "err", err)
			triggers = map[string]family.Trigger{}
		}
	}
	facts := family.FilmFacts{
		Title:         m.Title,
		Certification: cert,
		Genres:        m.Genres,
		Triggers:      triggers,
		Popularity:    m.Popularity,
	}
	result := family.Fit(profile, facts)
	if !result.Passed {
		return fitFilm{}, false
	}
	m.Certification = cert
	if full, err := s.tmdb.Movie(ctx, m.TMDBID); err != nil {
		s.log.Error("fit movie fetch failed", "id", m.TMDBID, "err", err)
	} else {
		full.Certification = cert
		full.Genres = m.Genres
		full.Popularity = m.Popularity
		m = *full
	}
	hits := model.TagOwnership(m.Availability, services...)
	return fitFilm{
		Movie:            m,
		Unverified:       result.Unverified,
		Penalty:          result.Penalty,
		SubscriptionHits: hits,
	}, true
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
