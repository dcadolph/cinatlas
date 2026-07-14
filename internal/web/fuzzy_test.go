package web

import (
	"fmt"
	"slices"
	"testing"
)

func TestCollapseDoubles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         string
		WantResult string
	}{
		{In: "Carrell", WantResult: "Carel"}, // Test 0: Doubled r and l.
		{In: "Steve", WantResult: "Steve"},   // Test 1: No doubles.
		{In: "", WantResult: ""},             // Test 2: Empty.
		{In: "aa bb", WantResult: "a b"},     // Test 3: Doubles across words.
		{In: "OoO", WantResult: "O"},         // Test 4: Case-insensitive run.
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := collapseDoubles(test.In)
			if got != test.WantResult {
				t.Errorf("collapseDoubles(%q) = %q, want %q", test.In, got, test.WantResult)
			}
		})
	}
}

func TestRelaxQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In       string
		WantHas  string // A variant that must appear.
		WantNone bool   // No variants expected at all.
	}{
		{In: "Steve Carrell", WantHas: "Steve Carel"}, // Test 0: Collapse fixes doubled letters.
		{In: "Villenueve", WantHas: "Villen"},         // Test 1: Prefix truncation reaches a match.
		{In: "Scorseze", WantHas: "Scorse"},           // Test 2: Prefix truncation past the typo.
		{In: "", WantNone: true},                      // Test 3: Empty query.
		{In: "  ", WantNone: true},                    // Test 4: Whitespace only.
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := relaxQuery(test.In)
			if test.WantNone {
				if len(got) != 0 {
					t.Errorf("want no variants, got %v", got)
				}
				return
			}
			if !slices.Contains(got, test.WantHas) {
				t.Errorf("want variant %q in %v", test.WantHas, got)
			}
		})
	}
}

func TestNameScore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Query   string
		Name    string
		AtLeast float64
		AtMost  float64
	}{ // Test 0: Exact name scores perfect.
		{Query: "matthew mcconaughey", Name: "Matthew McConaughey", AtLeast: 1, AtMost: 1},
		// Test 1: Correct first name, misspelled last still scores high.
		{Query: "matthew mcaughnehey", Name: "Matthew McConaughey", AtLeast: 0.6, AtMost: 1},
		// Test 2: A different Matthew stays below the misspelled real match's score.
		{Query: "matthew mcaughnehey", Name: "Matthew Perry", AtLeast: 0, AtMost: 0.72},
		// Test 3: Unrelated name scores low.
		{Query: "matthew mcaughnehey", Name: "Emma Stone", AtLeast: 0, AtMost: 0.3},
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := nameScore(test.Query, test.Name)
			if got < test.AtLeast || got > test.AtMost {
				t.Errorf("nameScore(%q, %q) = %v, want in [%v, %v]",
					test.Query, test.Name, got, test.AtLeast, test.AtMost)
			}
		})
	}
}

func TestNameScoreRanksRealMatchFirst(t *testing.T) {
	t.Parallel()
	query := "matthew mcaughnehey"
	real := nameScore(query, "Matthew McConaughey")
	other := nameScore(query, "Matthew Perry")
	if real <= other {
		t.Errorf("want McConaughey (%v) to outrank Perry (%v)", real, other)
	}
}

func TestNameScorePenalizesExtraTokens(t *testing.T) {
	t.Parallel()
	// A tight two-word match must beat a stranger who merely contains the name.
	query := "penelope cruz"
	tight := nameScore(query, "Penelope Cruz")
	padded := nameScore(query, "Aliyah Penelope Cruz")
	if tight <= padded {
		t.Errorf("want tight match (%v) to beat padded name (%v)", tight, padded)
	}
}
