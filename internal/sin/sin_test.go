package sin

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestScore checks the anchor and booster arithmetic, above all the guarantee
// that violence-only R films and theme-adjacent dramas score zero.
func TestScore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantScore int
		In        []string
	}{{ // Test 0: A war film's tags carry nothing, whatever it is rated.
		In: []string{"world war ii", "normandy", "omaha beach", "self sacrifice"}, WantScore: 0,
	}, { // Test 1: An erotic thriller stacks anchors and a booster.
		In: []string{"eroticism", "female nudity", "sex scene", "seduction"}, WantScore: 8,
	}, { // Test 2: Booster-only tags never qualify, so a divorce drama stays out.
		In: []string{"adultery", "infidelity", "divorce"}, WantScore: 0,
	}, { // Test 3: One anchor unlocks the boosters.
		In: []string{"nudity", "affair"}, WantScore: 3,
	}, { // Test 4: No tags, no heat.
		In: nil, WantScore: 0,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := Score(test.In); got != test.WantScore {
				t.Errorf("Score(%v) = %d, want %d", test.In, got, test.WantScore)
			}
		})
	}
}

// TestAnchorTerms checks the anchor list is sorted, non-empty, and free of
// booster leakage.
func TestAnchorTerms(t *testing.T) {
	t.Parallel()
	terms := AnchorTerms()
	if len(terms) == 0 {
		t.Fatal("no anchor terms")
	}
	for i := 1; i < len(terms); i++ {
		if terms[i-1] >= terms[i] {
			t.Errorf("terms not sorted: %q before %q", terms[i-1], terms[i])
		}
	}
	for _, term := range terms {
		if _, ok := boosters[term]; ok {
			t.Errorf("term %q is both anchor and booster", term)
		}
	}
}

// TestChipBySlug checks lookup hits and misses.
func TestChipBySlug(t *testing.T) {
	t.Parallel()
	chip, ok := ChipBySlug("erotic-thriller")
	if !ok || chip.Label != "Erotic Thriller" {
		t.Errorf("ChipBySlug(erotic-thriller) = %+v, %v", chip, ok)
	}
	if _, ok := ChipBySlug("wholesome"); ok {
		t.Error("ChipBySlug(wholesome) unexpectedly found a chip")
	}
}

// TestChipQuery checks a chip converts into a runnable query with its era and
// certification filters intact.
func TestChipQuery(t *testing.T) {
	t.Parallel()
	chip, ok := ChipBySlug("sleaze")
	if !ok {
		t.Fatal("sleaze chip missing")
	}
	got := chip.Query()
	if diff := cmp.Diff([]string{"sexploitation", "softcore"}, got.Terms); diff != "" {
		t.Errorf("terms mismatch (-want +got):\n%s", diff)
	}
	if got.Discover.ReleaseDateLTE != "1989-12-31" {
		t.Errorf("release ceiling = %q, want 1989-12-31", got.Discover.ReleaseDateLTE)
	}
	if got.MinScore != DefaultMinScore {
		t.Errorf("min score = %d, want %d", got.MinScore, DefaultMinScore)
	}

	club, ok := ChipBySlug("nc-17")
	if !ok {
		t.Fatal("nc-17 chip missing")
	}
	if q := club.Query(); q.Discover.CertificationGTE != "NC-17" {
		t.Errorf("certification floor = %q, want NC-17", q.Discover.CertificationGTE)
	}
}

// TestLinks checks the outbound pack: a deterministic parents guide when the
// IMDB id is known, scoped searches always, and explicit flags on the nudity
// databases.
func TestLinks(t *testing.T) {
	t.Parallel()
	got := Links("Basic Instinct", 1992, "tt0103772")
	if len(got) != 4 {
		t.Fatalf("got %d links, want 4: %+v", len(got), got)
	}
	if got[0].Label != "IMDb guide" ||
		got[0].URL != "https://www.imdb.com/title/tt0103772/parentalguide/" {
		t.Errorf("guide link = %+v", got[0])
	}
	if !got[1].Explicit || !strings.Contains(got[1].URL, "mrskin.com") ||
		!strings.Contains(got[1].URL, "Basic+Instinct+1992") {
		t.Errorf("mr skin link = %+v", got[1])
	}
	if !got[2].Explicit || !strings.Contains(got[2].URL, "mrman.com") {
		t.Errorf("mr man link = %+v", got[2])
	}
	if got[3].Explicit || !strings.Contains(got[3].URL, "movie-censorship.com") {
		t.Errorf("cut versions link = %+v", got[3])
	}

	// Without an IMDB id the guide drops and the searches remain.
	if noID := Links("Obscure Film", 0, ""); len(noID) != 3 {
		t.Errorf("got %d links without id, want 3: %+v", len(noID), noID)
	}
}

// TestPersonQuery checks the person lens pins the cast filter, keeps the
// default bar, and carries the full anchor vocabulary.
func TestPersonQuery(t *testing.T) {
	t.Parallel()
	got := PersonQuery(55)
	if diff := cmp.Diff([]int{55}, got.Discover.WithCast); diff != "" {
		t.Errorf("cast mismatch (-want +got):\n%s", diff)
	}
	if got.MinScore != DefaultMinScore {
		t.Errorf("min score = %d, want %d", got.MinScore, DefaultMinScore)
	}
	if diff := cmp.Diff(AnchorTerms(), got.Terms); diff != "" {
		t.Errorf("terms mismatch (-want +got):\n%s", diff)
	}
	if got.Discover.VoteCountGTE != 0 {
		t.Errorf("vote floor = %d, want none", got.Discover.VoteCountGTE)
	}
}

// TestPersonLinks checks the per-celebrity pack is explicit-flagged scoped
// searches.
func TestPersonLinks(t *testing.T) {
	t.Parallel()
	got := PersonLinks("Test Star")
	if len(got) != 2 {
		t.Fatalf("got %d links, want 2: %+v", len(got), got)
	}
	for i, site := range []string{"mrskin.com", "mrman.com"} {
		if !got[i].Explicit || !strings.Contains(got[i].URL, site) ||
			!strings.Contains(got[i].URL, "Test+Star") {
			t.Errorf("link %d = %+v, want explicit %s search", i, got[i], site)
		}
	}
}
