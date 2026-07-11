package render

import (
	"bytes"
	"testing"

	"github.com/dcadolph/cinatlas/internal/model"
)

// TestCast checks the cast view formatting.
func TestCast(t *testing.T) {
	t.Parallel()
	m := model.Movie{
		Title:    "Heat",
		Year:     1995,
		Director: "Michael Mann",
		Cast:     []model.Person{{Name: "Al Pacino", Character: "Vincent Hanna"}},
		IMDBURL:  "https://www.imdb.com/title/tt0113277/",
	}
	want := "Heat (1995)\n" +
		"Directed by Michael Mann\n" +
		"Cast:\n" +
		"  Al Pacino as Vincent Hanna\n" +
		"IMDB: https://www.imdb.com/title/tt0113277/\n"
	var b bytes.Buffer
	Cast(&b, m)
	if got := b.String(); got != want {
		t.Errorf("Cast\n got %q\nwant %q", got, want)
	}
}

// TestWhere checks the filming-location view formatting.
func TestWhere(t *testing.T) {
	t.Parallel()
	m := model.Movie{
		Title:     "Fargo",
		Year:      1996,
		Locations: []model.Location{{Name: "Bismarck", MapsURL: "https://maps/x"}},
		IMDBURL:   "https://www.imdb.com/title/tt0116282/",
	}
	want := "Fargo (1996)\n" +
		"Filmed in:\n" +
		"  Bismarck  https://maps/x\n" +
		"IMDB: https://www.imdb.com/title/tt0116282/\n"
	var b bytes.Buffer
	Where(&b, m)
	if got := b.String(); got != want {
		t.Errorf("Where\n got %q\nwant %q", got, want)
	}
}

// TestPerson checks the person view formatting.
func TestPerson(t *testing.T) {
	t.Parallel()
	p := model.Person{
		Name:     "Michael Mann",
		KnownFor: "Directing",
		Credits:  []model.Credit{{Title: "Collateral", Year: 2004, Job: "Director"}},
		IMDBURL:  "https://www.imdb.com/name/nm0000520/",
	}
	want := "Michael Mann (Directing)\n" +
		"  2004  Collateral (Director)\n" +
		"IMDB: https://www.imdb.com/name/nm0000520/\n"
	var b bytes.Buffer
	Person(&b, p)
	if got := b.String(); got != want {
		t.Errorf("Person\n got %q\nwant %q", got, want)
	}
}
