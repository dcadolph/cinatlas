// Package sin runs the adults-only discovery lens over mainstream cinema.
// Candidates are selected by sexual-theme keyword tags, never by rating alone,
// then each film's full tag set is scored so the shelf ranks by heat. The
// vocabulary holds no violence, so a war film rated R for combat cannot enter:
// it carries none of the tags discovery selects on and scores zero besides.
package sin

import (
	"fmt"
	"sort"

	"github.com/dcadolph/cinatlas/internal/tmdb"
)

// DefaultMinScore is the bar a film must clear on the general shelf: at least
// one anchor tag. Chips can raise it or waive it for theme shelves.
const DefaultMinScore = 2

// sortVoted ranks by TMDB vote count, favoring the canon over whatever spiked
// in popularity this week.
const sortVoted = "vote_count.desc"

// genreFamily is the TMDB Family genre id, excluded from every sin query.
const genreFamily = 10751

// anchors are keyword tags allowed to select a film on their own, weighted by
// how strongly the tag marks erotic cinema rather than a film that merely
// contains a scene.
var anchors = map[string]int{
	"erotic thriller":     4,
	"erotica":             4,
	"sexploitation":       4,
	"softcore":            4,
	"unsimulated sex":     4,
	"bdsm":                3,
	"eroticism":           3,
	"full frontal nudity": 3,
	"sex comedy":          3,
	"female nudity":       2,
	"male nudity":         2,
	"nudity":              2,
	"sex scene":           2,
	"stripper":            2,
	"striptease":          2,
	"voyeurism":           2,
}

// boosters are keyword tags that sharpen the ranking but never qualify a film
// alone, so a chaste drama about an affair stays off the general shelf.
var boosters = map[string]int{
	"adultery":             1,
	"affair":               1,
	"extramarital affair":  1,
	"forbidden love":       1,
	"gay relationship":     1,
	"infidelity":           1,
	"lesbian relationship": 1,
	"lust":                 1,
	"one night stand":      1,
	"seduction":            1,
	"sensuality":           1,
	"sexuality":            1,
	"skinny dipping":       1,
	"strip club":           1,
}

// AnchorTerms returns every anchor tag name, sorted, ready to resolve into a
// discovery keyword filter.
func AnchorTerms() []string {
	terms := make([]string, 0, len(anchors))
	for term := range anchors {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	return terms
}

// Score rates a film's keyword tags against the vocabulary. Boosters count
// only when an anchor is present, so theme-adjacent dramas score zero and the
// heat ordering stays honest.
func Score(keywords []string) int {
	anchored, boosted := 0, 0
	for _, k := range keywords {
		anchored += anchors[k]
		boosted += boosters[k]
	}
	if anchored == 0 {
		return 0
	}
	return anchored + boosted
}

// Chip is one curated shelf behind the devil: a label, the keyword terms its
// discovery ORs together, and the filters that shape it.
type Chip struct {
	// Slug identifies the chip in the URL.
	Slug string
	// Label is the shelf name shown on the page.
	Label string
	// Blurb is the one-line pitch under the active shelf.
	Blurb string
	// Terms are the keyword terms discovery ORs together.
	Terms []string
	// CertificationGTE pins a US certification floor, empty for none.
	CertificationGTE string
	// YearFrom bounds the earliest release year, zero for none.
	YearFrom int
	// YearTo bounds the latest release year, zero for none.
	YearTo int
	// MinVotes is the vote-count floor keeping obscurities off the shelf.
	MinVotes int
	// MinScore is the keep bar; zero trusts the chip's terms and only ranks.
	MinScore int
}

// chips is the curated shelf list, in display order.
var chips = []Chip{{
	Slug:     "steamy",
	Label:    "Steamy",
	Blurb:    "Eroticism played straight, arthouse to multiplex.",
	Terms:    []string{"eroticism", "erotica", "sex scene"},
	MinVotes: 300,
	MinScore: DefaultMinScore,
}, {
	Slug:     "erotic-thriller",
	Label:    "Erotic Thriller",
	Blurb:    "Neon, saxophone, and someone who should not be trusted.",
	Terms:    []string{"erotic thriller"},
	MinVotes: 100,
	MinScore: DefaultMinScore,
}, {
	Slug:     "sleaze",
	Label:    "Grindhouse Sleaze",
	Blurb:    "Exploitation and softcore from the drive-in era.",
	Terms:    []string{"sexploitation", "softcore"},
	YearTo:   1989,
	MinVotes: 20,
	MinScore: DefaultMinScore,
}, {
	Slug:             "nc-17",
	Label:            "NC-17 Club",
	Blurb:            "The rating studios dread, worn here like a medal.",
	Terms:            []string{"eroticism", "female nudity", "male nudity", "nudity", "sex scene"},
	CertificationGTE: "NC-17",
	MinVotes:         50,
	MinScore:         DefaultMinScore,
}, {
	Slug:     "unsimulated",
	Label:    "Unsimulated",
	Blurb:    "Festival scandals where nobody was pretending.",
	Terms:    []string{"unsimulated sex"},
	MinVotes: 50,
	MinScore: DefaultMinScore,
}, {
	Slug:     "affairs",
	Label:    "Forbidden Affairs",
	Blurb:    "Cheating hearts and the wreckage they leave.",
	Terms:    []string{"adultery", "affair", "extramarital affair", "infidelity"},
	MinVotes: 300,
}, {
	Slug:     "queer",
	Label:    "Queer Desire",
	Blurb:    "Desire across the spectrum, ranked by heat.",
	Terms:    []string{"lgbt", "lesbian relationship", "gay relationship"},
	MinVotes: 300,
}}

// Chips returns the curated shelves in display order.
func Chips() []Chip {
	return chips
}

// ChipBySlug returns the chip with the given slug.
func ChipBySlug(slug string) (Chip, bool) {
	for _, c := range chips {
		if c.Slug == slug {
			return c, true
		}
	}
	return Chip{}, false
}

// PersonQuery builds the query behind a person's devil: their billed films
// carrying anchor tags, gated at the default bar and ranked by heat. No vote
// floor applies, since an actor's obscure sleaze is half the fun.
func PersonQuery(personID int) Query {
	return Query{
		Terms: AnchorTerms(),
		Discover: tmdb.DiscoverQuery{
			WithCast: []int{personID},
			SortBy:   sortVoted,
		},
		MinScore: DefaultMinScore,
	}
}

// Query converts the chip into a runnable sin query.
func (c Chip) Query() Query {
	q := Query{
		Terms: c.Terms,
		Discover: tmdb.DiscoverQuery{
			CertificationGTE: c.CertificationGTE,
			VoteCountGTE:     c.MinVotes,
			SortBy:           sortVoted,
		},
		MinScore: c.MinScore,
	}
	if c.YearFrom > 0 {
		q.Discover.ReleaseDateGTE = fmt.Sprintf("%04d-01-01", c.YearFrom)
	}
	if c.YearTo > 0 {
		q.Discover.ReleaseDateLTE = fmt.Sprintf("%04d-12-31", c.YearTo)
	}
	return q
}
