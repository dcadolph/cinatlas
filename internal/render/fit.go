package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/dcadolph/cinatlas/internal/fitfind"
)

// Fit writes the films that passed every family constraint, best fit first.
func Fit(w io.Writer, results []fitfind.Result, excluded int) {
	if len(results) == 0 {
		fmt.Fprintln(w, "Nothing passed every constraint. Loosen a ceiling or drop a veto.")
		return
	}
	fmt.Fprintf(w, "%d films fit everyone (%d excluded):\n", len(results), excluded)
	for _, r := range results {
		line := "  " + r.Movie.Title
		if r.Movie.Year != 0 {
			line += fmt.Sprintf(" (%d)", r.Movie.Year)
		}
		if r.Movie.Certification != "" {
			line += " [" + r.Movie.Certification + "]"
		}
		if r.SubscriptionHits > 0 {
			line += " ✓ on your services"
		}
		fmt.Fprintln(w, line)
		if len(r.Unverified) > 0 {
			fmt.Fprintf(w, "    Unverified: %s\n", strings.Join(r.Unverified, "; "))
		}
	}
}
