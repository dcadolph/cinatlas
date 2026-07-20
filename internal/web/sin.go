package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/dcadolph/cinatlas/internal/sin"
	"github.com/dcadolph/cinatlas/internal/taste"
	"github.com/dcadolph/cinatlas/internal/tmdb"
)

// sinGenreRomance is the TMDB Romance genre id, stripped from free-text sin
// reads the sexy vocabulary injected so the shelf is not all romances.
const sinGenreRomance = 10749

// sinRomanceWords are the phrases that legitimately pin the Romance genre in
// sin mode. Without one of these in the query, Romance came from the sexy
// vocabulary and would slant every search away from the thrillers.
var sinRomanceWords = []string{"romance", "romantic", "love story", "date night"}

// sinData is everything the sin page render needs.
type sinData struct {
	// Query is the mood text echoed back into the box.
	Query string
	// Chips are the curated shelves, in display order.
	Chips []sin.Chip
	// Active is the selected chip's slug, empty on free-text searches.
	Active string
	// ActiveLabel is the selected chip's label, empty on free-text searches.
	ActiveLabel string
	// Blurb is the selected chip's pitch, empty otherwise.
	Blurb string
	// PersonID is the person whose filmography the lens covers, zero on the
	// general room.
	PersonID int
	// PersonName is the person's display name, empty on the general room.
	PersonName string
	// PersonPhoto is the person's profile image, empty when none exists.
	PersonPhoto string
	// PersonLinks are the person-level outbound references.
	PersonLinks []sin.Link
	// Results are the passing films, hottest first.
	Results []sin.Result
	// Searched reports that a query ran, distinguishing the landing view from
	// a search that found nothing.
	Searched bool
	// Enhanced reports that a model refined the read.
	Enhanced bool
	// Error is a human-readable failure to show instead of results.
	Error string
}

// handleSin renders the adults-only lens: curated shelves behind the devil, or
// a free-text mood read through the same keyword scoring. Certification never
// selects here; sexual-theme tags do, so an R rating earned by violence alone
// cannot reach the shelf.
func (s *Server) handleSin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := sinData{Chips: sin.Chips()}
	slug := strings.TrimSpace(r.URL.Query().Get("chip"))
	data.Query = strings.TrimSpace(r.URL.Query().Get("q"))

	var query sin.Query
	switch {
	case r.URL.Query().Get("person") != "":
		personID, err := strconv.Atoi(r.URL.Query().Get("person"))
		if err != nil {
			s.handleNotFound(w, r)
			return
		}
		person, err := s.tmdb.Person(ctx, personID)
		if err != nil {
			s.log.Error("sin person fetch failed", "id", personID, "err", err)
			s.handleNotFound(w, r)
			return
		}
		data.PersonID = person.TMDBID
		data.PersonName = person.Name
		data.PersonPhoto = person.PhotoURL
		data.PersonLinks = sin.PersonLinks(person.Name)
		query = sin.PersonQuery(personID)
	case slug != "":
		chip, ok := sin.ChipBySlug(slug)
		if !ok {
			s.handleNotFound(w, r)
			return
		}
		data.Active, data.ActiveLabel, data.Blurb = chip.Slug, chip.Label, chip.Blurb
		query = chip.Query()
	case data.Query != "":
		query = s.sinQuery(ctx, &data)
	default:
		s.render(w, http.StatusOK, "sin.html", data)
		return
	}
	data.Searched = true

	results, err := s.sin.Find(ctx, query)
	if err != nil {
		s.log.Error("sin find failed", "chip", slug, "query", data.Query, "err", err)
		data.Error = "The back room is jammed. Try again."
		s.render(w, http.StatusBadGateway, "sin.html", data)
		return
	}
	data.Results = results
	s.render(w, http.StatusOK, "sin.html", data)
}

// sinQuery reads a free-text sin mood into a runnable query. The lexicon and
// the sin enhancer shape genres, era, and quality; every anchor tag joins the
// keyword OR so selection always requires a sexual-theme tag; and the score
// gate relaxes to rank-only when the mood named its own themes, so an
// asked-for subject is never culled for running cool.
func (s *Server) sinQuery(ctx context.Context, data *sinData) sin.Query {
	intent := taste.Parse(data.Query)
	if s.sinEnhancer != nil {
		if refined, err := s.sinEnhancer.Enhance(ctx, data.Query, intent); err != nil {
			s.log.Error("sin enhance failed", "query", data.Query, "err", err)
		} else {
			intent = refined
			data.Enhanced = true
		}
	}
	minScore := sin.DefaultMinScore
	if len(intent.Keywords) > 0 {
		minScore = 0
	}
	discover := tmdb.DiscoverQuery{
		WithGenres:     sinGenres(data.Query, intent.Genres),
		WithoutGenres:  intent.ExcludeGenres,
		VoteAverageGTE: intent.MinRating,
		VoteCountGTE:   intent.MinVotes,
		SortBy:         intent.Sort,
	}
	if intent.YearFrom > 0 {
		discover.ReleaseDateGTE = fmt.Sprintf("%04d-01-01", intent.YearFrom)
	}
	if intent.YearTo > 0 {
		discover.ReleaseDateLTE = fmt.Sprintf("%04d-12-31", intent.YearTo)
	}
	return sin.Query{
		Terms:    append(sin.AnchorTerms(), intent.Keywords...),
		Discover: discover,
		MinScore: minScore,
	}
}

// sinGenres strips the Romance genre from a sin read unless the query asked
// for romance in its own words.
func sinGenres(query string, genres []int) []int {
	lowered := strings.ToLower(query)
	for _, word := range sinRomanceWords {
		if strings.Contains(lowered, word) {
			return genres
		}
	}
	kept := make([]int, 0, len(genres))
	for _, g := range genres {
		if g != sinGenreRomance {
			kept = append(kept, g)
		}
	}
	return kept
}
