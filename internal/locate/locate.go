// Package locate merges place facts about a film from every source cinatlas
// knows: Wikidata for structured filming locations and settings, Wikipedia
// section mining for street-level depth, and production countries as the
// coarse fallback so a film is never left with nothing.
package locate

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/dcadolph/cinatlas/internal/imdb"
	"github.com/dcadolph/cinatlas/internal/logutil"
	"github.com/dcadolph/cinatlas/internal/model"
	"github.com/dcadolph/cinatlas/internal/wikidata"
	"github.com/dcadolph/cinatlas/internal/wikipedia"
)

// maxLocations caps the merged filming list.
const maxLocations = 40

// findWorkers bounds concurrent TMDB lookups during place-search enrichment.
const findWorkers = 6

// Located holds the merged place facts for one film.
type Located struct {
	// Filming lists where the film shot, resolved pins first.
	Filming []model.Location
	// SetIn lists where the story takes place.
	SetIn []model.Location
}

// Locator answers merged place facts for an IMDB title id.
type Locator interface {
	Locate(ctx context.Context, imdbID string) (*Located, error)
}

// LocatorFunc adapts a function to the Locator interface.
type LocatorFunc func(ctx context.Context, imdbID string) (*Located, error)

// Locate calls the underlying function.
func (f LocatorFunc) Locate(ctx context.Context, imdbID string) (*Located, error) {
	return f(ctx, imdbID)
}

// Sort orders for place results.
const (
	// SortFame orders by fame, the default.
	SortFame = "fame"
	// SortAZ orders by title.
	SortAZ = "az"
	// SortNew orders newest release first.
	SortNew = "new"
	// SortOld orders oldest release first.
	SortOld = "old"
)

// AtQuery shapes one page of a place lookup.
type AtQuery struct {
	// Offset is how many matches to skip.
	Offset int
	// Limit caps the returned page.
	Limit int
	// Sort picks the ordering, SortFame when empty.
	Sort string
	// Decade keeps only releases in one decade when non-zero, such as 1990.
	Decade int
}

// AtResult is one page of a place lookup plus its facets.
type AtResult struct {
	// Movies is the requested page, enriched through TMDB.
	Movies []model.Movie
	// Total counts every match after filtering.
	Total int
	// Decades lists the release decades present across all matches, newest
	// first, for building filters.
	Decades []int
}

// Atlas answers both directions: film to places, and place to films.
type Atlas interface {
	Locator
	At(ctx context.Context, place string, query AtQuery) (*AtResult, error)
}

// IMDBFinder resolves an IMDB title id to a movie with images and ids.
type IMDBFinder interface {
	FindByIMDB(ctx context.Context, imdbID string) (*model.Movie, error)
}

// Service merges Wikidata, Wikipedia, and TMDB place facts.
type Service struct {
	// resolver answers the structured Wikidata facts.
	resolver wikidata.Resolver
	// places reverse-searches films by place name.
	places wikidata.PlaceSearcher
	// sections mines the Wikipedia filming section.
	sections wikipedia.SectionLocator
	// finder enriches reverse hits with posters and TMDB ids.
	finder IMDBFinder
}

// New returns a Service. It panics on nil dependencies, which are developer errors.
func New(resolver wikidata.Resolver, places wikidata.PlaceSearcher,
	sections wikipedia.SectionLocator, finder IMDBFinder) *Service {
	if resolver == nil {
		panic("locate.New: wikidata resolver required")
	}
	if places == nil {
		panic("locate.New: wikidata place searcher required")
	}
	if sections == nil {
		panic("locate.New: wikipedia section locator required")
	}
	if finder == nil {
		panic("locate.New: imdb finder required")
	}
	return &Service{resolver: resolver, places: places, sections: sections, finder: finder}
}

// At returns one page of movies filmed at the named place, plus totals and
// decade facets. Only the requested page is enriched through TMDB, so deep
// pages stay as cheap as the first.
func (s *Service) At(ctx context.Context, place string, query AtQuery) (*AtResult, error) {
	hits, err := s.places.FilmsAt(ctx, place)
	if err != nil {
		return nil, err
	}
	result := &AtResult{Decades: decades(hits)}
	if query.Decade != 0 {
		kept := hits[:0]
		for _, hit := range hits {
			if hit.Year >= query.Decade && hit.Year < query.Decade+10 {
				kept = append(kept, hit)
			}
		}
		hits = kept
	}
	sortHits(hits, query.Sort)
	result.Total = len(hits)
	if query.Offset >= result.Total {
		return result, nil
	}
	hits = hits[query.Offset:min(result.Total, query.Offset+query.Limit)]

	log := logutil.FromContext(ctx)
	movies := make([]model.Movie, len(hits))
	sem := make(chan struct{}, findWorkers)
	var wg sync.WaitGroup
	for i, hit := range hits {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			continue
		}
		wg.Add(1)
		go func(at int, hit wikidata.Film) {
			defer wg.Done()
			defer func() { <-sem }()
			movies[at] = enrichHit(ctx, s.finder, log, hit)
		}(i, hit)
	}
	wg.Wait()
	result.Movies = movies
	return result, nil
}

// sortHits orders hits in place. Fame keeps the source order; unknown years
// always sink to the end of date sorts.
func sortHits(hits []wikidata.Film, order string) {
	switch order {
	case SortAZ:
		sort.SliceStable(hits, func(i, j int) bool {
			return strings.ToLower(hits[i].Title) < strings.ToLower(hits[j].Title)
		})
	case SortNew:
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].Year > hits[j].Year })
	case SortOld:
		sort.SliceStable(hits, func(i, j int) bool {
			if hits[i].Year == 0 || hits[j].Year == 0 {
				return hits[j].Year == 0 && hits[i].Year != 0
			}
			return hits[i].Year < hits[j].Year
		})
	}
}

// decades lists the release decades present, newest first.
func decades(hits []wikidata.Film) []int {
	seen := map[int]bool{}
	for _, hit := range hits {
		if hit.Year > 0 {
			seen[hit.Year/10*10] = true
		}
	}
	out := make([]int, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}

// enrichHit upgrades one reverse hit through TMDB, falling back to the
// Wikidata facts when TMDB does not know the film.
func enrichHit(ctx context.Context, finder IMDBFinder, log *slog.Logger, hit wikidata.Film) model.Movie {
	movie, err := finder.FindByIMDB(ctx, hit.IMDBID)
	if err != nil || movie == nil {
		if err != nil {
			log.Debug("tmdb find failed", "imdbId", hit.IMDBID, "err", err)
		}
		return model.Movie{
			Title:   hit.Title,
			Year:    hit.Year,
			IMDBID:  hit.IMDBID,
			IMDBURL: imdb.TitleURL(hit.IMDBID),
		}
	}
	if movie.Year == 0 {
		movie.Year = hit.Year
	}
	return *movie
}

// Locate merges every source for one film. Wikipedia mining is best effort:
// its failure is logged and never blocks the structured answer.
func (s *Service) Locate(ctx context.Context, imdbID string) (*Located, error) {
	facts, err := s.resolver.Resolve(ctx, imdbID)
	if err != nil {
		return nil, err
	}
	merged := facts.Filming
	if facts.ArticleTitle != "" {
		mined, err := s.sections.FilmingLocations(ctx, facts.ArticleTitle)
		if err != nil {
			logutil.FromContext(ctx).Error("wikipedia mining failed",
				"article", facts.ArticleTitle, "err", err)
		} else {
			merged = mergeLocations(merged, mined)
		}
	}
	if len(merged) == 0 {
		merged = facts.Countries
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Resolved && !merged[j].Resolved })
	if len(merged) > maxLocations {
		merged = merged[:maxLocations]
	}
	return &Located{Filming: merged, SetIn: facts.SetIn}, nil
}

// mergeLocations folds mined places into the base list. A mined place with
// the same name upgrades an unresolved base entry with coordinates; new names
// append after the base list.
func mergeLocations(base, mined []model.Location) []model.Location {
	index := make(map[string]int, len(base))
	for i, loc := range base {
		index[locationKey(loc.Name)] = i
	}
	for _, loc := range mined {
		key := locationKey(loc.Name)
		if at, ok := index[key]; ok {
			if !base[at].Resolved && loc.Resolved {
				base[at] = loc
			}
			continue
		}
		index[key] = len(base)
		base = append(base, loc)
	}
	return base
}

// locationKey normalizes a place name for dedupe.
func locationKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
