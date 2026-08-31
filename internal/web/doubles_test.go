package web

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/dcadolph/cinatlas/internal/wikidata"
)

// TestDoublesPage checks both directions, the empty form, and the case where a
// place has no recorded stand-ins.
func TestDoublesPage(t *testing.T) {
	t.Parallel()
	site := newSite(t)

	tests := []struct {
		Name       string
		Path       string
		WantStatus int
		Want       []string
		WantNone   []string
	}{{ // Test 0: The bare page offers the form and seeds some places to try.
		Name: "empty form", Path: "/doubles", WantStatus: http.StatusOK,
		Want: []string{"Body doubles", "Show the disguises", "/doubles?q=Prague"},
		// The results heading, not the prose, since the page description names
		// Vancouver as an example whether or not anything was searched.
		WantNone: []string{`class="doubles-head"`, "recorded pairing"},
	}, { // Test 1: A place reports what it has pretended to be.
		Name: "forward", Path: "/doubles?q=Vancouver", WantStatus: http.StatusOK,
		Want:     []string{"Vancouver has played", "New York City", "3,904 km apart", "Titanic"},
		WantNone: []string{"has been played by</h2>"},
	}, { // Test 2: The flipped question reports who pretended to be it.
		Name: "reverse", Path: "/doubles?q=Vancouver&reverse=1", WantStatus: http.StatusOK,
		Want:     []string{"Vancouver has been played by", "New York City"},
		WantNone: []string{"Vancouver has played<"},
	}, { // Test 3: A place with nothing recorded says so rather than showing an
		// empty page, and explains why the data is thin.
		Name: "no stand-ins", Path: "/doubles?q=nowhere", WantStatus: http.StatusNotFound,
		Want: []string{"No recorded stand-ins", "thin outside film hubs"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			status, body := fetch(t, site, test.Path)
			if status != test.WantStatus {
				t.Errorf("status = %d, want %d", status, test.WantStatus)
			}
			for _, want := range test.Want {
				if !strings.Contains(body, want) {
					t.Errorf("body missing %q", want)
				}
			}
			for _, none := range test.WantNone {
				if strings.Contains(body, none) {
					t.Errorf("body should not contain %q", none)
				}
			}
		})
	}
}

// TestDoublesCountsAllFilms checks that a card reports how many films made the
// swap rather than how many it had room to list, which trimming the slice
// before rendering would silently break.
func TestDoublesCountsAllFilms(t *testing.T) {
	t.Parallel()
	films := make([]wikidata.Film, 20)
	for i := range films {
		films[i] = wikidata.Film{Title: fmt.Sprintf("Film %d", i), IMDBID: fmt.Sprintf("tt%07d", i)}
	}
	got := firstFilms(maxStandFilms, films)
	if len(got) != maxStandFilms {
		t.Errorf("firstFilms returned %d, want %d", len(got), maxStandFilms)
	}
	if len(films) != 20 {
		t.Errorf("firstFilms modified the input, now %d long", len(films))
	}
	short := firstFilms(maxStandFilms, films[:2])
	if len(short) != 2 {
		t.Errorf("firstFilms shortened a list under the cap to %d", len(short))
	}
}

// TestFormatKM checks the distance helper the card headline uses.
func TestFormatKM(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In   float64
		Want string
	}{ // Test 0 through 3: separators appear only above a thousand.
		{In: 66, Want: "66 km"},
		{In: 3904, Want: "3,904 km"},
		{In: 18257, Want: "18,257 km"},
		{In: 0, Want: "0 km"},
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := formatKM(test.In); got != test.Want {
				t.Errorf("formatKM(%v) = %q, want %q", test.In, got, test.Want)
			}
		})
	}
}
