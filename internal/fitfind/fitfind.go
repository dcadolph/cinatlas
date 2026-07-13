// Package fitfind runs the family fit pipeline: discover popular candidates, hydrate
// each with certification, trigger flags, and availability, and keep what passes.
package fitfind

import (
	"context"
	"log/slog"
	"sort"
	"sync"

	"github.com/dcadolph/cinatlas/internal/ddd"
	"github.com/dcadolph/cinatlas/internal/family"
	"github.com/dcadolph/cinatlas/internal/model"
	"github.com/dcadolph/cinatlas/internal/tmdb"
)

// maxPages caps the discover pages pulled per find.
const maxPages = 2

// workers bounds the concurrent per-film hydration calls.
const workers = 8

// maxResults caps the returned result list.
const maxResults = 24

// Result is one passing film with its fit outcome.
type Result struct {
	// Movie is the film with certification and availability attached.
	Movie model.Movie `json:"movie"`
	// Unverified lists the constraints that had no data for this film.
	Unverified []string `json:"unverified,omitempty"`
	// Penalty counts soft veto hits, lower first.
	Penalty int `json:"penalty,omitempty"`
	// SubscriptionHits counts services the family already has carrying the film.
	SubscriptionHits int `json:"subscriptionHits,omitempty"`
}

// Finder discovers candidates and scores them against a family profile.
type Finder struct {
	// tmdb answers discover, certification, and availability lookups.
	tmdb *tmdb.HTTPClient
	// triggers answers content trigger lookups, nil to skip content checks.
	triggers ddd.TriggerSource
	// log receives per-film hydration failures, which are best effort.
	log *slog.Logger
}

// New returns a Finder. It panics on a nil client or logger, which are developer
// errors. A nil trigger source is allowed and leaves hard vetoes unverified.
func New(client *tmdb.HTTPClient, triggers ddd.TriggerSource, log *slog.Logger) *Finder {
	if client == nil {
		panic("fitfind.New: tmdb client required")
	}
	if log == nil {
		panic("fitfind.New: logger required")
	}
	return &Finder{tmdb: client, triggers: triggers, log: log}
}

// Find returns the films passing every member's constraints, best fit first, plus
// the count of candidates someone's limits excluded.
func (f *Finder) Find(ctx context.Context, profile family.Profile,
	services []string) ([]Result, int, error) {
	candidates, err := f.candidates(ctx, profile)
	if err != nil {
		return nil, 0, err
	}
	results, excluded := f.evaluate(ctx, profile, candidates, services)
	return results, excluded, nil
}

// candidates pulls popular films at or under the profile's lowest ceiling, deduped.
// The first page must succeed; later pages are best effort.
func (f *Finder) candidates(ctx context.Context, profile family.Profile) ([]model.Movie, error) {
	ceiling := profile.LowestCeiling()
	movies, err := f.tmdb.Discover(ctx, tmdb.DiscoverQuery{CertificationLTE: ceiling})
	if err != nil {
		return nil, err
	}
	for page := 2; page <= maxPages; page++ {
		more, err := f.tmdb.Discover(ctx, tmdb.DiscoverQuery{CertificationLTE: ceiling, Page: page})
		if err != nil {
			f.log.Error("fit discover page failed", "page", page, "err", err)
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

// evaluate hydrates and scores every candidate concurrently, returning the passing
// films ranked best first plus the excluded count.
func (f *Finder) evaluate(ctx context.Context, profile family.Profile,
	candidates []model.Movie, services []string) ([]Result, int) {
	var mu sync.Mutex
	var results []Result
	excluded := 0

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
			result, ok := f.score(ctx, profile, m, services)
			mu.Lock()
			defer mu.Unlock()
			if !ok {
				excluded++
				return
			}
			results = append(results, result)
		}(candidate)
	}
	wg.Wait()

	sort.SliceStable(results, func(i, j int) bool {
		return family.Less(ranked(results[i]), ranked(results[j]))
	})
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return results, excluded
}

// ranked rebuilds the ranking inputs from a scored result.
func ranked(r Result) family.Ranked {
	return family.Ranked{
		Facts: family.FilmFacts{
			SubscriptionHits: r.SubscriptionHits,
			Popularity:       r.Movie.Popularity,
		},
		Result: family.FitResult{Penalty: r.Penalty},
	}
}

// score hydrates one candidate, returning it when it passes the profile.
func (f *Finder) score(ctx context.Context, profile family.Profile, m model.Movie,
	services []string) (Result, bool) {
	cert, err := f.tmdb.Certification(ctx, m.TMDBID)
	if err != nil {
		f.log.Error("certification fetch failed", "id", m.TMDBID, "err", err)
	}
	triggers := map[string]family.Trigger{}
	if f.triggers != nil {
		triggers, err = f.triggers.TriggersFor(ctx, m.Title, m.Year)
		if err != nil {
			f.log.Error("trigger fetch failed", "title", m.Title, "err", err)
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
	fit := family.Fit(profile, facts)
	if !fit.Passed {
		return Result{}, false
	}
	m.Certification = cert
	if full, err := f.tmdb.Movie(ctx, m.TMDBID); err != nil {
		f.log.Error("fit movie fetch failed", "id", m.TMDBID, "err", err)
	} else {
		full.Certification = cert
		full.Genres = m.Genres
		full.Popularity = m.Popularity
		m = *full
	}
	hits := model.TagOwnership(m.Availability, services...)
	return Result{
		Movie:            m,
		Unverified:       fit.Unverified,
		Penalty:          fit.Penalty,
		SubscriptionHits: hits,
	}, true
}
