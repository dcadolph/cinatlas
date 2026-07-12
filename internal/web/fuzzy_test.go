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
