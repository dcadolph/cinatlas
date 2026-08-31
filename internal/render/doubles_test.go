package render

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/dcadolph/cinatlas/internal/wikidata"
)

// TestDoubles checks the heading flips with direction, that distance is shown
// only when known, and that a long film list is summarized rather than dumped.
func TestDoubles(t *testing.T) {
	t.Parallel()
	stands := []wikidata.Stand{{
		Shot: "Vancouver", Played: "New York City", KM: 3904,
		Films: []wikidata.Film{
			{Title: "Titanic", Year: 1997}, {Title: "2012", Year: 2009},
			{Title: "Fantastic Four", Year: 2005}, {Title: "Deadpool", Year: 2016},
		},
	}, {
		Shot: "Vancouver", Played: "Seattle",
		Films: []wikidata.Film{{Title: "Fifty Shades of Grey", Year: 2015}},
	}}

	tests := []struct {
		Name     string
		Reverse  bool
		Want     []string
		WantNone []string
	}{{ // Test 0: Forward reads as the place doing the pretending.
		Name: "forward", Reverse: false,
		Want: []string{
			"Vancouver has played:", "New York City  (4 films, 3,904 km apart)",
			"Titanic (1997)", "and 1 more", "Seattle  (1 film)",
		},
		WantNone: []string{"has been played by", "Seattle  (1 film,"},
	}, { // Test 1: Reverse reads as the place being impersonated, and names the
		// other side of each pair.
		Name: "reverse", Reverse: true,
		Want:     []string{"Vancouver has been played by:", "Vancouver  (4 films"},
		WantNone: []string{"Vancouver has played:"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			var b bytes.Buffer
			Doubles(&b, "Vancouver", stands, test.Reverse)
			got := b.String()
			for _, want := range test.Want {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q:\n%s", want, got)
				}
			}
			for _, none := range test.WantNone {
				if strings.Contains(got, none) {
					t.Errorf("output should not contain %q:\n%s", none, got)
				}
			}
		})
	}
}

// TestKilometers checks thousands separators across magnitudes, since the
// distance is the line that carries the point.
func TestKilometers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In   float64
		Want string
	}{ // Test 0 through 4: separators appear only above a thousand.
		{In: 7, Want: "7 km"},
		{In: 193, Want: "193 km"},
		{In: 3904, Want: "3,904 km"},
		{In: 18257, Want: "18,257 km"},
		{In: 1234567, Want: "1,234,567 km"},
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := kilometers(test.In); got != test.Want {
				t.Errorf("kilometers(%v) = %q, want %q", test.In, got, test.Want)
			}
		})
	}
}
