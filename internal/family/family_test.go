package family

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// TestFit checks certification ceilings, hard veto triggers, unverified reporting,
// and soft veto penalties across members.
func TestFit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantResult FitResult
		Profile    Profile
		Facts      FilmFacts
	}{{ // Test 0: No constraints hit, film passes clean.
		Profile: Profile{Members: []Member{{Name: "Sam", Ceiling: "PG"}}},
		Facts: FilmFacts{
			Certification: "G",
			Triggers:      map[string]Trigger{"animal-death": TriggerNo},
		},
		WantResult: FitResult{Passed: true},
	}, { // Test 1: Certification over one member's ceiling fails with a reason.
		Profile: Profile{Members: []Member{
			{Name: "Sam", Ceiling: "PG"},
			{Name: "Dad", Ceiling: "R"},
		}},
		Facts: FilmFacts{Certification: "PG-13"},
		WantResult: FitResult{
			Passed:   false,
			Failures: []string{"Fails Sam: rated PG-13, ceiling PG"},
		},
	}, { // Test 2: Hard veto flagged present fails.
		Profile: Profile{Members: []Member{{Name: "Sam", HardVetoes: []string{"animal-death"}}}},
		Facts: FilmFacts{
			Certification: "G",
			Triggers:      map[string]Trigger{"animal-death": TriggerYes},
		},
		WantResult: FitResult{
			Passed:   false,
			Failures: []string{"Fails Sam: animal-death"},
		},
	}, { // Test 3: Hard veto with no data passes but reports unverified once.
		Profile: Profile{Members: []Member{
			{Name: "Sam", HardVetoes: []string{"jump-scares"}},
			{Name: "Alex", HardVetoes: []string{"jump-scares"}},
		}},
		Facts: FilmFacts{Certification: "G"},
		WantResult: FitResult{
			Passed:     true,
			Unverified: []string{"No data: jump-scares"},
		},
	}, { // Test 4: Unknown certification with a ceiling reports unverified once.
		Profile: Profile{Members: []Member{
			{Name: "Sam", Ceiling: "PG"},
			{Name: "Alex", Ceiling: "G"},
		}},
		Facts: FilmFacts{},
		WantResult: FitResult{
			Passed:     true,
			Unverified: []string{"Certification unknown"},
		},
	}, { // Test 5: Soft veto genre hits accumulate as penalty without excluding.
		Profile: Profile{Members: []Member{
			{Name: "Sam", SoftVetoes: []string{"Musical"}},
			{Name: "Alex", SoftVetoes: []string{"musical", "Horror"}},
		}},
		Facts: FilmFacts{
			Certification: "G",
			Genres:        []string{"Musical", "Comedy"},
		},
		WantResult: FitResult{Passed: true, Penalty: 2},
	}, { // Test 6: Loose certification spellings normalize before comparison.
		Profile:    Profile{Members: []Member{{Name: "Sam", Ceiling: " pg13 "}}},
		Facts:      FilmFacts{Certification: "PG-13"},
		WantResult: FitResult{Passed: true},
	}, { // Test 7: The same trigger fails every member who vetoes it.
		Profile: Profile{Members: []Member{
			{Name: "Sam", HardVetoes: []string{"animal-death"}},
			{Name: "Alex", HardVetoes: []string{"animal-death"}},
		}},
		Facts: FilmFacts{
			Certification: "G",
			Triggers:      map[string]Trigger{"animal-death": TriggerYes},
		},
		WantResult: FitResult{
			Passed:   false,
			Failures: []string{"Fails Sam: animal-death", "Fails Alex: animal-death"},
		},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := Fit(test.Profile, test.Facts)
			if diff := cmp.Diff(test.WantResult, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestRank checks the ordering: penalty ascending, subscription hits descending,
// then popularity descending.
func TestRank(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantOrder []string
		In        []Ranked
	}{{ // Test 0: Lower penalty ranks first regardless of popularity.
		In: []Ranked{
			{Facts: FilmFacts{Title: "b", Popularity: 99}, Result: FitResult{Penalty: 1}},
			{Facts: FilmFacts{Title: "a", Popularity: 1}, Result: FitResult{Penalty: 0}},
		},
		WantOrder: []string{"a", "b"},
	}, { // Test 1: Equal penalty falls back to subscription hits.
		In: []Ranked{
			{Facts: FilmFacts{Title: "b", SubscriptionHits: 0}},
			{Facts: FilmFacts{Title: "a", SubscriptionHits: 2}},
		},
		WantOrder: []string{"a", "b"},
	}, { // Test 2: Equal penalty and hits fall back to popularity.
		In: []Ranked{
			{Facts: FilmFacts{Title: "b", Popularity: 5}},
			{Facts: FilmFacts{Title: "a", Popularity: 50}},
		},
		WantOrder: []string{"a", "b"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			Rank(test.In)
			got := make([]string, 0, len(test.In))
			for _, r := range test.In {
				got = append(got, r.Facts.Title)
			}
			if diff := cmp.Diff(test.WantOrder, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
