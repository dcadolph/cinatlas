package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dcadolph/cinatlas/internal/wikidata"
)

// maxDoublesTitles is how many film titles print beneath one pairing before
// the rest are summarized as a count.
const maxDoublesTitles = 3

// Doubles writes the places a place has stood in for, most-used pairing first.
// Reverse flips the heading to read as the place being impersonated.
func Doubles(w io.Writer, place string, stands []wikidata.Stand, reverse bool) {
	if reverse {
		fmt.Fprintf(w, "%s has been played by:\n", place)
	} else {
		fmt.Fprintf(w, "%s has played:\n", place)
	}
	for _, s := range stands {
		other, count := s.Played, len(s.Films)
		if reverse {
			other = s.Shot
		}
		fmt.Fprintf(w, "\n  %s  (%s%s)\n", other, plural(count, "film"), distance(s.KM))
		for i, f := range s.Films {
			if i == maxDoublesTitles {
				fmt.Fprintf(w, "      and %d more\n", count-maxDoublesTitles)
				break
			}
			year := ""
			if f.Year > 0 {
				year = " (" + strconv.Itoa(f.Year) + ")"
			}
			fmt.Fprintf(w, "      %s%s\n", f.Title, year)
		}
	}
}

// distance renders the gap between the two places, empty when either lacks
// coordinates or the places sit on top of each other.
func distance(km float64) string {
	if km < 1 {
		return ""
	}
	return fmt.Sprintf(", %s apart", kilometers(km))
}

// kilometers formats a distance with thousands separators and no decimals.
func kilometers(km float64) string {
	s := strconv.FormatFloat(km, 'f', 0, 64)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String() + " km"
}

// plural renders a count with its noun, adding an s when the count is not one.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
