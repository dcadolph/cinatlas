package wikidata

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// row builds one stand-in binding for the tests.
func row(film, imdb, date, links, shot, set, shotCoord, setCoord string) map[string]sparqlValue {
	return map[string]sparqlValue{
		"filmLabel": {Value: film}, "imdb": {Value: imdb}, "date": {Value: date},
		"links": {Value: links}, "shotLabel": {Value: shot}, "setLabel": {Value: set},
		"shotCoord": {Value: shotCoord}, "setCoord": {Value: setCoord},
	}
}

// TestStandsGroups checks that rows fold into one entry per shot and played
// pair, ordered by film count.
func TestStandsGroups(t *testing.T) {
	t.Parallel()
	var r sparqlResponse
	r.Results.Bindings = []map[string]sparqlValue{
		row("Deadpool", "tt1431045", "2016-02-09", "90", "Vancouver", "New York City", "", ""),
		row("Titanic", "tt0120338", "1997-12-19", "180", "Vancouver", "New York City", "", ""),
		row("Fifty Shades of Grey", "tt2322441", "2015-02-11", "70", "Vancouver", "Seattle", "", ""),
	}
	got := r.stands()
	if len(got) != 2 {
		t.Fatalf("stands() returned %d pairs, want 2: %+v", len(got), got)
	}
	if got[0].Played != "New York City" || len(got[0].Films) != 2 {
		t.Errorf("first pair = %q with %d films, want New York City with 2",
			got[0].Played, len(got[0].Films))
	}
	if got[1].Played != "Seattle" || len(got[1].Films) != 1 {
		t.Errorf("second pair = %q with %d films, want Seattle with 1",
			got[1].Played, len(got[1].Films))
	}
}

// TestStandsCountsEachFilmOnce checks the fix for the shape of this data: a
// film recording several filming locations and several settings emits a row
// for every combination, so a naive count reports one film many times.
func TestStandsCountsEachFilmOnce(t *testing.T) {
	t.Parallel()
	var r sparqlResponse
	// The Piano lists four filming pins against the same setting.
	for range 4 {
		r.Results.Bindings = append(r.Results.Bindings,
			row("The Piano", "tt0107822", "1993-05-19", "60", "New Zealand", "Scotland", "", ""))
	}
	got := r.stands()
	if len(got) != 1 {
		t.Fatalf("stands() returned %d pairs, want 1", len(got))
	}
	if len(got[0].Films) != 1 {
		t.Errorf("counted the same film %d times, want 1", len(got[0].Films))
	}
}

// TestStandsSkipsIncomplete checks that rows missing an identifier, a label, or
// naming the same place on both sides are dropped rather than reported.
func TestStandsSkipsIncomplete(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Name string
		Row  map[string]sparqlValue
	}{{ // Test 0: A film shot where it is set is not a stand-in.
		Name: "same place", Row: row("Heat", "tt0113277", "", "10", "Los Angeles", "Los Angeles", "", ""),
	}, { // Test 1: No IMDB id means nothing to link or dedupe on.
		Name: "no imdb", Row: row("Heat", "", "", "10", "Vancouver", "Seattle", "", ""),
	}, { // Test 2: No title is not printable.
		Name: "no title", Row: row("", "tt0113277", "", "10", "Vancouver", "Seattle", "", ""),
	}, { // Test 3: No filming label leaves the pair half known.
		Name: "no shot", Row: row("Heat", "tt0113277", "", "10", "", "Seattle", "", ""),
	}, { // Test 4: No setting label leaves the pair half known.
		Name: "no set", Row: row("Heat", "tt0113277", "", "10", "Vancouver", "", "", ""),
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			var r sparqlResponse
			r.Results.Bindings = []map[string]sparqlValue{test.Row}
			if got := r.stands(); len(got) != 0 {
				t.Errorf("stands() kept %+v, want it dropped", got)
			}
		})
	}
}

// TestStandsDistance checks that a pair carries the distance between its two
// places, and stays at zero when either side has no coordinates.
func TestStandsDistance(t *testing.T) {
	t.Parallel()
	var r sparqlResponse
	r.Results.Bindings = []map[string]sparqlValue{
		row("Titanic", "tt0120338", "", "180", "Vancouver", "New York City",
			"Point(-123.1207 49.2827)", "Point(-74.0060 40.7128)"),
		row("Heat", "tt0113277", "", "10", "Vancouver", "Seattle", "Point(-123.1207 49.2827)", ""),
	}
	got := r.stands()
	var withCoords, without float64
	for _, s := range got {
		if s.Played == "New York City" {
			withCoords = s.KM
		} else {
			without = s.KM
		}
	}
	if math.Abs(withCoords-3904) > 40 {
		t.Errorf("Vancouver to New York = %.0f km, want about 3904", withCoords)
	}
	if without != 0 {
		t.Errorf("distance without coordinates = %v, want 0", without)
	}
}

// TestDoublesQueryDirection checks that the query binds the filming side by
// default and the setting side in reverse, and that the bound side is written
// before the film pattern, which is what keeps a busy place from timing out.
func TestDoublesQueryDirection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Name    string
		Reverse bool
		WantAt  string
	}{{ // Test 0: Forward binds the place to where the camera was.
		Name: "forward", Reverse: false, WantAt: "?shot wdt:P131* ?target .",
	}, { // Test 1: Reverse binds the place to where the story is set.
		Name: "reverse", Reverse: true, WantAt: "?set wdt:P131* ?target .",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			q := doublesQuery([]string{"Q24639"}, test.Reverse)
			bound := strings.Index(q, test.WantAt)
			if bound < 0 {
				t.Fatalf("query does not bind %q:\n%s", test.WantAt, q)
			}
			film := strings.Index(q, "?film wdt:P915")
			if film < 0 || bound > film {
				t.Errorf("bound side must precede the film pattern, got bind at %d and film at %d",
					bound, film)
			}
			if !strings.Contains(q, "FILTER(?shot != ?set)") {
				t.Error("query does not exclude films shot where they are set")
			}
			if !strings.Contains(q, "wd:Q24639") {
				t.Error("query does not carry the candidate place id")
			}
		})
	}
}

// TestHaversine checks the distance helper against known separations and on
// input it cannot read.
func TestHaversine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Name   string
		A, B   string
		WantKM float64
		Tol    float64
	}{{ // Test 0: Vancouver to New York, a well-known stand-in pair.
		Name: "vancouver to new york", A: "Point(-123.1207 49.2827)", B: "Point(-74.0060 40.7128)",
		WantKM: 3904, Tol: 40,
	}, { // Test 1: The same point is no distance at all.
		Name: "same point", A: "Point(-123.1207 49.2827)", B: "Point(-123.1207 49.2827)",
		WantKM: 0, Tol: 0.001,
	}, { // Test 2: Unparsable input yields zero rather than a wrong number.
		Name: "bad input", A: "not a point", B: "Point(-74.0060 40.7128)", WantKM: 0, Tol: 0.001,
	}, { // Test 3: An empty side yields zero.
		Name: "empty", A: "", B: "", WantKM: 0, Tol: 0.001,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			got := haversine(test.A, test.B)
			if math.Abs(got-test.WantKM) > test.Tol {
				t.Errorf("haversine = %.1f, want %.1f within %.3f", got, test.WantKM, test.Tol)
			}
		})
	}
}
