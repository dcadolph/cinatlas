package locate

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dcadolph/cinatlas/internal/model"
	"github.com/dcadolph/cinatlas/internal/wikidata"
	"github.com/dcadolph/cinatlas/internal/wikipedia"
)

// fixedResolver returns a canned Wikidata result.
func fixedResolver(result *wikidata.Result) wikidata.Resolver {
	return wikidata.ResolverFunc(func(_ context.Context, _ string) (*wikidata.Result, error) {
		return result, nil
	})
}

// fixedSections returns canned Wikipedia locations.
func fixedSections(locs []model.Location, err error) wikipedia.SectionLocator {
	return wikipedia.SectionLocatorFunc(func(_ context.Context, _ string) ([]model.Location, error) {
		return locs, err
	})
}

// fixedPlaces returns canned reverse-lookup films.
func fixedPlaces(films []wikidata.Film) wikidata.PlaceSearcher {
	return wikidata.PlaceSearcherFunc(func(_ context.Context, _ string) ([]wikidata.Film, error) {
		return films, nil
	})
}

// mapFinder resolves IMDB ids from a fixed map, erroring on unknown ids.
type mapFinder map[string]model.Movie

// FindByIMDB returns the mapped movie or an error for unknown ids.
func (m mapFinder) FindByIMDB(_ context.Context, imdbID string) (*model.Movie, error) {
	movie, ok := m[imdbID]
	if !ok {
		return nil, errors.New("not found")
	}
	return &movie, nil
}

// TestLocateMerges checks that mined places upgrade unresolved names, new
// names append, and resolved pins sort first.
func TestLocateMerges(t *testing.T) {
	t.Parallel()
	svc := New(
		fixedResolver(&wikidata.Result{
			Filming: []model.Location{
				model.UnresolvedLocation("Chicago", "wikidata"),
				model.ResolvedLocation("Los Angeles", "wikidata", 34.05, -118.24),
			},
			SetIn:        []model.Location{model.ResolvedLocation("Los Angeles", "wikidata", 34.05, -118.24)},
			ArticleTitle: "Heat_(1995_film)",
		}),
		fixedPlaces(nil),
		fixedSections([]model.Location{
			model.ResolvedLocation("Chicago", "wikipedia", 41.88, -87.63),
			model.ResolvedLocation("Venice, Los Angeles", "wikipedia", 33.985, -118.47),
		}, nil),
		mapFinder{},
	)
	got, err := svc.Locate(context.Background(), "tt0113277")
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	want := &Located{
		Filming: []model.Location{
			model.ResolvedLocation("Chicago", "wikipedia", 41.88, -87.63),
			model.ResolvedLocation("Los Angeles", "wikidata", 34.05, -118.24),
			model.ResolvedLocation("Venice, Los Angeles", "wikipedia", 33.985, -118.47),
		},
		SetIn: []model.Location{model.ResolvedLocation("Los Angeles", "wikidata", 34.05, -118.24)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Locate\n got %+v\nwant %+v", got, want)
	}
}

// TestLocateCountryFallback checks countries fill in when nothing else exists.
func TestLocateCountryFallback(t *testing.T) {
	t.Parallel()
	svc := New(
		fixedResolver(&wikidata.Result{
			Countries: []model.Location{model.ResolvedLocation("United States", "country", 39.8, -98.5)},
		}),
		fixedPlaces(nil),
		fixedSections(nil, nil),
		mapFinder{},
	)
	got, err := svc.Locate(context.Background(), "tt0000001")
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	want := []model.Location{model.ResolvedLocation("United States", "country", 39.8, -98.5)}
	if !reflect.DeepEqual(got.Filming, want) {
		t.Errorf("Locate fallback\n got %+v\nwant %+v", got.Filming, want)
	}
}

// TestAt checks reverse search enrichment: TMDB hits upgrade, unknown ids fall
// back to Wikidata facts, order and the limit hold.
func TestAt(t *testing.T) {
	t.Parallel()
	svc := New(
		fixedResolver(&wikidata.Result{}),
		fixedPlaces([]wikidata.Film{
			{Title: "Heat", Year: 1995, IMDBID: "tt0113277", Place: "Los Angeles"},
			{Title: "Obscure Film", Year: 1962, IMDBID: "tt0000062", Place: "Los Angeles"},
			{Title: "Beyond Limit", Year: 2000, IMDBID: "tt0000063", Place: "Los Angeles"},
		}),
		fixedSections(nil, nil),
		mapFinder{
			"tt0113277": {
				TMDBID: 1, IMDBID: "tt0113277", Title: "Heat", Year: 1995,
				PosterURL: "https://image.tmdb.org/t/p/w342/heat.jpg",
				IMDBURL:   "https://www.imdb.com/title/tt0113277/",
			},
		},
	)
	got, total, err := svc.At(context.Background(), "Los Angeles", 0, 2)
	if err != nil {
		t.Fatalf("At: %v", err)
	}
	if total != 3 {
		t.Errorf("At total = %d, want 3", total)
	}
	want := []model.Movie{
		{
			TMDBID: 1, IMDBID: "tt0113277", Title: "Heat", Year: 1995,
			PosterURL: "https://image.tmdb.org/t/p/w342/heat.jpg",
			IMDBURL:   "https://www.imdb.com/title/tt0113277/",
		},
		{
			Title: "Obscure Film", Year: 1962, IMDBID: "tt0000062",
			IMDBURL: "https://www.imdb.com/title/tt0000062/",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("At\n got %+v\nwant %+v", got, want)
	}
}

// TestLocateToleratesMiningFailure checks a Wikipedia error never blocks the
// structured answer.
func TestLocateToleratesMiningFailure(t *testing.T) {
	t.Parallel()
	svc := New(
		fixedResolver(&wikidata.Result{
			Filming:      []model.Location{model.ResolvedLocation("Los Angeles", "wikidata", 34.05, -118.24)},
			ArticleTitle: "Heat_(1995_film)",
		}),
		fixedPlaces(nil),
		fixedSections(nil, errors.New("api down")),
		mapFinder{},
	)
	got, err := svc.Locate(context.Background(), "tt0113277")
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if len(got.Filming) != 1 || got.Filming[0].Name != "Los Angeles" {
		t.Errorf("Locate under mining failure = %+v, want the wikidata pin", got.Filming)
	}
}
