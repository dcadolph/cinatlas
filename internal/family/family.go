// Package family models family viewing profiles and scores films against them.
package family

import (
	"fmt"
	"sort"
	"strings"
)

// Trigger is the crowdsourced answer for one content topic on one film.
type Trigger int

const (
	// TriggerUnknown means the topic has no data on record for the film.
	TriggerUnknown Trigger = iota
	// TriggerNo means the topic is flagged as not present in the film.
	TriggerNo
	// TriggerYes means the topic is flagged as present in the film.
	TriggerYes
)

// Member is one person in a family profile with their viewing constraints.
type Member struct {
	// Name labels the member in results, such as "Sam" or "the 6 year old".
	Name string `json:"name"`
	// Ceiling is the highest US certification the member may watch, empty for no limit.
	Ceiling string `json:"ceiling,omitempty"`
	// HardVetoes lists content topic keys that disqualify a film outright.
	HardVetoes []string `json:"hard,omitempty"`
	// SoftVetoes lists genre names that penalize a film in ranking without excluding it.
	SoftVetoes []string `json:"soft,omitempty"`
}

// Profile is a named set of members whose constraints a film must satisfy together.
type Profile struct {
	// Version is the schema version carried in share links.
	Version int `json:"v"`
	// Name labels the profile, such as "weeknight crew".
	Name string `json:"name,omitempty"`
	// Members lists everyone the film must fit.
	Members []Member `json:"members"`
}

// FilmFacts holds the per-film inputs the fit engine evaluates.
type FilmFacts struct {
	// Title is the display title carried through to ranking consumers.
	Title string
	// Certification is the US certification, empty when unknown.
	Certification string
	// Genres lists the film's genre names.
	Genres []string
	// Triggers maps content topic keys to their crowdsourced state.
	Triggers map[string]Trigger
	// SubscriptionHits counts availability entries the family's services already cover.
	SubscriptionHits int
	// Popularity is the TMDB popularity used as the final ranking tiebreaker.
	Popularity float64
}

// FitResult reports whether a film fits a profile and why.
type FitResult struct {
	// Passed reports whether every member's hard constraints are satisfied.
	Passed bool
	// Failures lists the reasons the film was excluded, one per member and constraint.
	Failures []string
	// Unverified lists constraints that could not be checked because data was missing.
	Unverified []string
	// Penalty counts soft veto hits across members; lower ranks earlier.
	Penalty int
}

// Ranked pairs a film's facts with its fit result for ordering.
type Ranked struct {
	// Facts holds the film inputs used by the ranking tiebreakers.
	Facts FilmFacts
	// Result is the fit outcome used for the primary ordering.
	Result FitResult
}

// certRanks orders US certifications from mildest to strictest.
var certRanks = map[string]int{"G": 1, "PG": 2, "PG-13": 3, "R": 4, "NC-17": 5}

// Fit evaluates a film's facts against every member of the profile. A film passes
// when its certification is at or under every member's ceiling and no member's hard
// veto topic is flagged present. Constraints with no data land in Unverified rather
// than silently passing.
func Fit(p Profile, f FilmFacts) FitResult {
	res := FitResult{Passed: true}
	filmRank := certRank(f.Certification)
	seen := map[string]bool{}
	for _, m := range p.Members {
		if ceil := certRank(m.Ceiling); ceil > 0 {
			switch {
			case filmRank == 0:
				if !seen["cert"] {
					seen["cert"] = true
					res.Unverified = append(res.Unverified, "Certification unknown")
				}
			case filmRank > ceil:
				res.Passed = false
				res.Failures = append(res.Failures, fmt.Sprintf("Fails %s: rated %s, ceiling %s",
					m.Name, normCert(f.Certification), normCert(m.Ceiling)))
			}
		}
		for _, topic := range m.HardVetoes {
			switch f.Triggers[topic] {
			case TriggerYes:
				res.Passed = false
				res.Failures = append(res.Failures, fmt.Sprintf("Fails %s: %s", m.Name, topic))
			case TriggerUnknown:
				if !seen[topic] {
					seen[topic] = true
					res.Unverified = append(res.Unverified, fmt.Sprintf("No data: %s", topic))
				}
			}
		}
		for _, g := range m.SoftVetoes {
			for _, fg := range f.Genres {
				if strings.EqualFold(g, fg) {
					res.Penalty++
				}
			}
		}
	}
	return res
}

// Rank orders films by fewest soft veto penalties, then most availability already
// covered by the family's services, then TMDB popularity.
func Rank(list []Ranked) {
	sort.SliceStable(list, func(i, j int) bool { return Less(list[i], list[j]) })
}

// Less reports whether a ranks before b: fewer penalties, more subscription hits,
// then higher popularity.
func Less(a, b Ranked) bool {
	if a.Result.Penalty != b.Result.Penalty {
		return a.Result.Penalty < b.Result.Penalty
	}
	if a.Facts.SubscriptionHits != b.Facts.SubscriptionHits {
		return a.Facts.SubscriptionHits > b.Facts.SubscriptionHits
	}
	return a.Facts.Popularity > b.Facts.Popularity
}

// LowestCeiling returns the strictest certification ceiling across members, empty
// when no member has one.
func (p Profile) LowestCeiling() string {
	low, out := 0, ""
	for _, m := range p.Members {
		if r := certRank(m.Ceiling); r > 0 && (low == 0 || r < low) {
			low, out = r, normCert(m.Ceiling)
		}
	}
	return out
}

// certRank returns the severity rank of a US certification, zero when unrecognized.
func certRank(s string) int {
	return certRanks[normCert(s)]
}

// normCert uppercases and trims a certification so lookups tolerate loose input.
func normCert(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "PG13":
		return "PG-13"
	case "NC17":
		return "NC-17"
	}
	return s
}
