// Package render writes human-readable views of cinatlas results to a writer.
package render

import (
	"fmt"
	"io"
	"strconv"

	"github.com/dcadolph/cinatlas/internal/model"
)

// Cast writes a movie's title, director, and billed cast.
func Cast(w io.Writer, m model.Movie) {
	writeTitle(w, m)
	if m.Director != "" {
		fmt.Fprintf(w, "Directed by %s\n", m.Director)
	}
	if len(m.Cast) > 0 {
		fmt.Fprintln(w, "Cast:")
		for _, p := range m.Cast {
			if p.Character != "" {
				fmt.Fprintf(w, "  %s as %s\n", p.Name, p.Character)
			} else {
				fmt.Fprintf(w, "  %s\n", p.Name)
			}
		}
	}
	writeIMDB(w, m.IMDBURL)
}

// Where writes a movie's filming locations with map links.
func Where(w io.Writer, m model.Movie) {
	writeTitle(w, m)
	if len(m.Locations) == 0 {
		fmt.Fprintln(w, "No filming locations found.")
		if m.IMDBLocationsURL != "" {
			fmt.Fprintf(w, "Check IMDB: %s\n", m.IMDBLocationsURL)
		}
	} else {
		fmt.Fprintln(w, "Filmed in:")
		for _, loc := range m.Locations {
			if loc.MapsURL != "" {
				fmt.Fprintf(w, "  %s  %s\n", loc.Name, loc.MapsURL)
			} else {
				fmt.Fprintf(w, "  %s\n", loc.Name)
			}
		}
	}
	writeIMDB(w, m.IMDBURL)
}

// Person writes a person's name, notable department, and filmography.
func Person(w io.Writer, p model.Person) {
	if p.KnownFor != "" {
		fmt.Fprintf(w, "%s (%s)\n", p.Name, p.KnownFor)
	} else {
		fmt.Fprintln(w, p.Name)
	}
	for _, c := range p.Credits {
		year := "----"
		if c.Year > 0 {
			year = strconv.Itoa(c.Year)
		}
		if role := creditRole(c); role != "" {
			fmt.Fprintf(w, "  %s  %s (%s)\n", year, c.Title, role)
		} else {
			fmt.Fprintf(w, "  %s  %s\n", year, c.Title)
		}
	}
	writeIMDB(w, p.IMDBURL)
}

// creditRole returns the crew job when present, otherwise the played character.
func creditRole(c model.Credit) string {
	if c.Job != "" {
		return c.Job
	}
	return c.Character
}

// writeTitle writes a movie title with its year when known.
func writeTitle(w io.Writer, m model.Movie) {
	if m.Year > 0 {
		fmt.Fprintf(w, "%s (%d)\n", m.Title, m.Year)
	} else {
		fmt.Fprintln(w, m.Title)
	}
}

// writeIMDB writes an IMDB link line when a link is present.
func writeIMDB(w io.Writer, url string) {
	if url != "" {
		fmt.Fprintf(w, "IMDB: %s\n", url)
	}
}
