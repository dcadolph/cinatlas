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
