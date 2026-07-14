package taste

import (
	"fmt"
	"slices"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Query        string
		WantGenres   []int
		WantExcludes []int
		WantKeywords []string
		WantSort     string
	}{ // Test 0: Empty query yields the popular baseline.
		{Query: "", WantSort: sortPopular},
		// Test 1: Feel-good animals adventure blends genres, keywords, exclusion.
		{
			Query:        "a feel-good movie with animals that go on an adventure",
			WantGenres:   []int{genreComedy, genreAdventure},
			WantExcludes: []int{genreHorror},
			WantKeywords: []string{"animal", "feel-good"},
			WantSort:     sortPopular,
		},
		// Test 2: Blended tone stacks both genres.
		{
			Query:      "something scary but also funny",
			WantGenres: []int{genreHorror, genreComedy},
		},
		// Test 3: Quality words raise the floors and switch the sort.
		{Query: "the most acclaimed masterpiece", WantSort: sortRated},
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := Parse(test.Query)
			for _, g := range test.WantGenres {
				if !slices.Contains(got.Genres, g) {
					t.Errorf("genre %d missing from %v", g, got.Genres)
				}
			}
			for _, g := range test.WantExcludes {
				if !slices.Contains(got.ExcludeGenres, g) {
					t.Errorf("exclude %d missing from %v", g, got.ExcludeGenres)
				}
			}
			for _, k := range test.WantKeywords {
				if !slices.Contains(got.Keywords, k) {
					t.Errorf("keyword %q missing from %v", k, got.Keywords)
				}
			}
			if test.WantSort != "" && got.Sort != test.WantSort {
				t.Errorf("sort = %q, want %q", got.Sort, test.WantSort)
			}
			if got.MinVotes < baselineVotes {
				t.Errorf("min votes = %d, want >= %d", got.MinVotes, baselineVotes)
			}
		})
	}
}

func TestParseExcludesDropFromGenres(t *testing.T) {
	t.Parallel()
	// A family request excludes horror; horror must never be in the include set.
	got := Parse("a scary family horror movie")
	if slices.Contains(got.Genres, genreHorror) && slices.Contains(got.ExcludeGenres, genreHorror) {
		t.Errorf("horror is both included and excluded: %v / %v", got.Genres, got.ExcludeGenres)
	}
}

func TestMergeAI(t *testing.T) {
	t.Parallel()
	// An explicit family request reads as the Family genre, which skews childish
	// when the viewer meant something warmer.
	base := Parse("a wholesome family movie")
	if !slices.Contains(base.Genres, genreFamily) {
		t.Fatalf("expected lexicon to include Family, got %v", base.Genres)
	}

	// The model reads it as warm adult drama and comedy, no Family.
	got := base.MergeAI(AIReading{
		Genres:   []string{"Drama", "Comedy"},
		Keywords: []string{"cozy"},
		Sort:     "rating",
	})

	// AI genres replace the lexicon's, dropping Family.
	if slices.Contains(got.Genres, genreFamily) {
		t.Errorf("Family should be gone after AI override, got %v", got.Genres)
	}
	if !slices.Contains(got.Genres, genreDrama) {
		t.Errorf("Drama missing after merge, got %v", got.Genres)
	}
	// Keywords union keeps lexicon themes and adds the model's.
	if !slices.Contains(got.Keywords, "cozy") {
		t.Errorf("cozy keyword missing, got %v", got.Keywords)
	}
	if got.Sort != sortRated {
		t.Errorf("sort = %q, want %q", got.Sort, sortRated)
	}
}

func TestMergeAIEmptyGenresKeepsBase(t *testing.T) {
	t.Parallel()
	base := Parse("a funny movie")
	got := base.MergeAI(AIReading{Keywords: []string{"slapstick"}})
	if !slices.Contains(got.Genres, genreComedy) {
		t.Errorf("base Comedy dropped when AI named no genres, got %v", got.Genres)
	}
	if !slices.Contains(got.Keywords, "slapstick") {
		t.Errorf("AI keyword missing, got %v", got.Keywords)
	}
}

func TestGenreNames(t *testing.T) {
	t.Parallel()
	names := GenreNames()
	if len(names) != len(genreNames) {
		t.Errorf("got %d names, want %d", len(names), len(genreNames))
	}
	if !slices.Contains(names, "Comedy") {
		t.Errorf("Comedy missing from %v", names)
	}
	if !slices.IsSorted(names) {
		t.Errorf("names not sorted: %v", names)
	}
}

func TestGenreIDByName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In     string
		WantID int
		WantOK bool
	}{
		{In: "Comedy", WantID: genreComedy, WantOK: true}, // Test 0: Exact.
		{In: "comedy", WantID: genreComedy, WantOK: true}, // Test 1: Case-insensitive.
		{In: "Sci-Fi", WantID: genreSciFi, WantOK: true},  // Test 2: Hyphenated.
		{In: "nonsense", WantID: 0, WantOK: false},        // Test 3: Unknown.
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			id, ok := genreIDByName(test.In)
			if id != test.WantID || ok != test.WantOK {
				t.Errorf("genreIDByName(%q) = (%d, %t), want (%d, %t)",
					test.In, id, ok, test.WantID, test.WantOK)
			}
		})
	}
}
