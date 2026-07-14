package web

import (
	"strings"
	"unicode"
)

// minRelaxLen is the shortest a word may be truncated to when generating
// relaxed variants. Below this, prefix matches turn to noise.
const minRelaxLen = 5

// maxRelaxSteps caps how many trailing runes a single word is truncated by,
// bounding the number of fallback API calls per search.
const maxRelaxSteps = 4

// relaxQuery returns typo-tolerant variants of q, most conservative first.
// It is a fallback for when an exact search finds nothing: TMDB matches
// prefixes but not transpositions or doubled letters, so the variants
// collapse repeats and shorten the longest word toward a matching prefix.
func relaxQuery(q string) []string {
	words := strings.Fields(q)
	if len(words) == 0 {
		return nil
	}
	seen := map[string]bool{strings.ToLower(strings.Join(words, " ")): true}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		key := strings.ToLower(s)
		if s == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, s)
	}

	// Collapse doubled letters across the whole query.
	add(collapseDoubles(q))

	// Prefix-truncate the longest word from one rune shorter down to the floor.
	li := longestWordIndex(words)
	long := []rune(words[li])
	for step := 1; step <= maxRelaxSteps; step++ {
		n := len(long) - step
		if n < minRelaxLen {
			break
		}
		add(joinReplacing(words, li, string(long[:n])))
	}

	// Collapse the longest word and, if still long enough, offer that too.
	if cw := collapseDoubles(words[li]); len([]rune(cw)) >= minRelaxLen {
		add(joinReplacing(words, li, cw))
	}
	return out
}

// collapseDoubles removes consecutive duplicate letters, ignoring case, so
// "Carrell" becomes "Carel". Non-letter runes and word breaks pass through.
func collapseDoubles(s string) string {
	var b strings.Builder
	var prev rune
	for i, r := range s {
		if i > 0 && unicode.IsLetter(r) && unicode.ToLower(r) == unicode.ToLower(prev) {
			continue
		}
		b.WriteRune(r)
		prev = r
	}
	return b.String()
}

// longestWordIndex returns the index of the longest word by rune count, the
// first such word on a tie.
func longestWordIndex(words []string) int {
	best, bestLen := 0, 0
	for i, w := range words {
		if n := len([]rune(w)); n > bestLen {
			best, bestLen = i, n
		}
	}
	return best
}

// joinReplacing joins words with single spaces after swapping index i for repl.
func joinReplacing(words []string, i int, repl string) string {
	swapped := make([]string, len(words))
	copy(swapped, words)
	swapped[i] = repl
	return strings.Join(swapped, " ")
}

// nameMatchThreshold is the lowest per-token name score treated as a plausible
// fuzzy person match. Below it the candidate is more noise than name.
const nameMatchThreshold = 0.6

// personSearchVariants expands a query into the terms worth searching TMDB by
// name when the exact query finds nobody: the whole query, each long-enough
// word on its own so a correct first or last name still anchors the lookup,
// and the prefix-relaxed variants. Order is preserved and duplicates dropped.
func personSearchVariants(query string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		key := strings.ToLower(s)
		if s == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, s)
	}
	add(query)
	for _, w := range strings.Fields(query) {
		if len([]rune(w)) >= 3 {
			add(w)
		}
	}
	for _, v := range relaxQuery(query) {
		add(v)
	}
	return out
}

// nameScore rates how well a candidate name answers the query, from 0 to 1. It
// averages, over each query word, the best similarity to any word of the name,
// so a correct first name paired with a misspelled last name still scores high.
func nameScore(query, name string) float64 {
	q := strings.Fields(strings.ToLower(query))
	n := strings.Fields(strings.ToLower(name))
	if len(q) == 0 || len(n) == 0 {
		return 0
	}
	var sum float64
	for _, qt := range q {
		best := 0.0
		for _, nt := range n {
			if r := similarity(qt, nt); r > best {
				best = r
			}
		}
		sum += best
	}
	return sum / float64(len(q))
}

// similarity returns the edit-distance similarity of two words, from 0 for
// unrelated to 1 for identical.
func similarity(a, b string) float64 {
	if a == b {
		return 1
	}
	ra, rb := []rune(a), []rune(b)
	longest := len(ra)
	if len(rb) > longest {
		longest = len(rb)
	}
	if longest == 0 {
		return 1
	}
	return 1 - float64(levenshtein(ra, rb))/float64(longest)
}

// levenshtein returns the edit distance between two rune slices using a single
// rolling row of costs.
func levenshtein(a, b []rune) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr := make([]int, len(b)+1)
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev = curr
	}
	return prev[len(b)]
}

// min3 returns the smallest of three ints.
func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
