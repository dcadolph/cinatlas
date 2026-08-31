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
	}{ // Test 0: Empty query yields the most-voted baseline.
		{Query: "", WantSort: sortVoted},
		// Test 1: Feel-good animals adventure blends genres, keywords, exclusion.
		{
			Query:        "a feel-good movie with animals that go on an adventure",
			WantGenres:   []int{genreComedy, genreAdventure},
			WantExcludes: []int{genreHorror},
			WantKeywords: []string{"animal", "feel-good"},
			WantSort:     sortVoted,
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

// TestParseSexualityThemes checks sexuality and identity queries map plainly
// instead of being sanitized away.
func TestParseSexualityThemes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Query        string
		WantKeywords []string
	}{{ // Test 0: Cheating maps to the affair cluster.
		Query: "movies about cheating", WantKeywords: []string{"affair", "adultery", "infidelity"},
	}, { // Test 1: Lesbian stories resolve directly.
		Query: "a lesbian love story", WantKeywords: []string{"lesbian", "lgbt"},
	}, { // Test 2: Queer blends with other rules untouched.
		Query: "queer coming of age", WantKeywords: []string{"gay", "lgbt", "coming-of-age"},
	}, { // Test 3: Nudity is a legitimate theme filter.
		Query: "films with nudity", WantKeywords: []string{"nudity"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := Parse(test.Query)
			for _, k := range test.WantKeywords {
				if !slices.Contains(got.Keywords, k) {
					t.Errorf("keyword %q missing from %v", k, got.Keywords)
				}
			}
		})
	}
}
