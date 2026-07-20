package sin

import (
	"context"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/dcadolph/cinatlas/internal/imdb"
	"github.com/dcadolph/cinatlas/internal/model"
	"github.com/dcadolph/cinatlas/internal/tmdb"
)

// maxPages caps the discover pages pulled per query, a buffer for culling.
const maxPages = 2

// workers bounds the concurrent per-film theme hydrations.
const workers = 8

// maxResults caps the returned shelf.
const maxResults = 24

// Query is one runnable sin search.
type Query struct {
	// Terms are keyword terms discovery ORs together, resolved to ids once and
	// cached for the process lifetime.
	Terms []string
	// Discover carries the other discovery filters: genres, era, votes, sort.
	// Resolved term ids append to its WithKeywords.
	Discover tmdb.DiscoverQuery
	// MinScore culls films scoring below it; zero keeps every candidate and
	// lets the score order the shelf instead of gating it.
	MinScore int
}

// Result is one passing film with its heat and outbound references.
type Result struct {
	// Movie is the film with certification and IMDB link attached.
	Movie model.Movie
	// Score is the film's heat against the vocabulary.
	Score int
	// Links are the outbound references for the film.
	Links []Link
	// votes breaks score ties by fame without exposing a display field.
	votes int
}

// termID is one cached keyword resolution, including known misses.
type termID struct {
	// id is the resolved TMDB keyword id.
	id int
	// ok reports whether the term resolved at all.
	ok bool
}

// Finder runs sin discovery against TMDB.
type Finder struct {
	// tmdb answers discover, keyword, and theme lookups.
	tmdb *tmdb.HTTPClient
	// log receives per-film hydration failures, which are best effort.
	log *slog.Logger
	// mu guards ids.
	mu sync.Mutex
	// ids caches term resolutions; TMDB keyword ids never change.
	ids map[string]termID
}

// New returns a Finder. It panics on a nil client or logger, which are
// developer errors.
func New(client *tmdb.HTTPClient, log *slog.Logger) *Finder {
	if client == nil {
		panic("sin.New: tmdb client required")
	}
	if log == nil {
		panic("sin.New: logger required")
	}
	return &Finder{tmdb: client, log: log, ids: make(map[string]termID)}
}

// Find runs one sin query: discover candidates on the resolved terms, hydrate
// each film's tags, score, cull, and rank hottest first.
func (f *Finder) Find(ctx context.Context, query Query) ([]Result, error) {
	discover := query.Discover
	discover.WithKeywords = append(discover.WithKeywords, f.resolveTerms(ctx, query.Terms)...)
	if len(discover.WithKeywords) == 0 {
		return nil, ErrNoAnchors
	}
	if !slices.Contains(discover.WithoutGenres, genreFamily) {
		discover.WithoutGenres = append(discover.WithoutGenres, genreFamily)
	}
	candidates, err := f.candidates(ctx, discover)
	if err != nil {
		return nil, err
	}
	results := f.evaluate(ctx, candidates, query.MinScore)
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].votes > results[j].votes
	})
	return results[:min(len(results), maxResults)], nil
}

// resolveTerms maps terms to keyword ids through the cache, dropping terms
// that resolve to nothing.
func (f *Finder) resolveTerms(ctx context.Context, terms []string) []int {
	var ids []int
	for _, term := range terms {
		f.mu.Lock()
		cached, done := f.ids[term]
		f.mu.Unlock()
		if !done {
			var durable bool
			cached, durable = f.lookup(ctx, term)
			if durable {
				f.mu.Lock()
				f.ids[term] = cached
				f.mu.Unlock()
			}
		}
		if cached.ok {
			ids = append(ids, cached.id)
		}
	}
	return ids
}

// lookup resolves one term against the keyword search, preferring the
// exact-name match over TMDB's first suggestion. It also reports whether the
// answer is durable; network failures are not, so a later query can retry.
func (f *Finder) lookup(ctx context.Context, term string) (termID, bool) {
	matches, err := f.tmdb.Keyword(ctx, term)
	if err != nil {
		f.log.Error("sin keyword resolve failed", "term", term, "err", err)
		return termID{}, false
	}
	for _, m := range matches {
		if strings.EqualFold(m.Name, term) {
			return termID{id: m.ID, ok: true}, true
		}
	}
	if len(matches) > 0 {
		return termID{id: matches[0].ID, ok: true}, true
	}
	return termID{}, true
}

// candidates pulls discovery pages for the query, deduped by id. The first
// page must succeed; later pages are best effort.
func (f *Finder) candidates(ctx context.Context, query tmdb.DiscoverQuery) ([]model.Movie, error) {
	movies, err := f.tmdb.Discover(ctx, query)
	if err != nil {
		return nil, err
	}
	for page := 2; page <= maxPages; page++ {
		next := query
		next.Page = page
		more, err := f.tmdb.Discover(ctx, next)
		if err != nil {
			f.log.Error("sin discover page failed", "page", page, "err", err)
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

// evaluate hydrates and scores every candidate concurrently, keeping films at
// or above the bar with their links attached.
func (f *Finder) evaluate(ctx context.Context, candidates []model.Movie, minScore int) []Result {
	var mu sync.Mutex
	var results []Result

	sem := make(chan struct{}, workers)
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
			result, ok := f.score(ctx, m, minScore)
			if !ok {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			results = append(results, result)
		}(candidate)
	}
	wg.Wait()
	return results
}

// score hydrates one candidate's themes and returns it when it clears the bar.
func (f *Finder) score(ctx context.Context, m model.Movie, minScore int) (Result, bool) {
	themes, err := f.tmdb.MovieThemes(ctx, m.TMDBID)
	if err != nil {
		f.log.Error("sin theme fetch failed", "id", m.TMDBID, "err", err)
		return Result{}, false
	}
	heat := Score(themes.Keywords)
	if heat < minScore {
		return Result{}, false
	}
	m.Certification = themes.Certification
	m.IMDBID = themes.IMDBID
	m.IMDBURL = imdb.TitleURL(themes.IMDBID)
	return Result{
		Movie: m,
		Score: heat,
		Links: Links(m.Title, m.Year, themes.IMDBID),
		votes: themes.Votes,
	}, true
}
