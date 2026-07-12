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

// Watch writes where a movie streams now in its region, grouped by whether
// access is included in a subscription or costs extra.
func Watch(w io.Writer, m model.Movie) {
	writeTitle(w, m)
	region := m.WatchRegion
	if region == "" {
		region = "your region"
	}
	if len(m.Availability) == 0 {
		fmt.Fprintf(w, "Not streaming in %s right now.\n", region)
		if m.WatchURL != "" {
			fmt.Fprintf(w, "Check watch options: %s\n", m.WatchURL)
		}
		writeIMDB(w, m.IMDBURL)
		return
	}
	fmt.Fprintf(w, "Streaming in %s:\n", region)
	writeAvailability(w, "Included", m.Availability, true)
	writeAvailability(w, "Rent or buy", m.Availability, false)
	if m.WatchURL != "" {
		fmt.Fprintf(w, "All watch options: %s\n", m.WatchURL)
	}
	writeIMDB(w, m.IMDBURL)
}

// writeAvailability writes a heading and the availability entries matching the
// included filter, marking services the viewer owns. It writes nothing when no
// entry matches.
func writeAvailability(w io.Writer, heading string, av []model.Availability, included bool) {
	var lines []model.Availability
	for _, a := range av {
		if a.Included() == included {
			lines = append(lines, a)
		}
	}
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(w, "%s:\n", heading)
	for _, a := range lines {
		mark := ""
		if a.Owned {
			mark = " ✓ you have this"
		}
		fmt.Fprintf(w, "  %s (%s)%s\n", a.Provider, a.Kind, mark)
	}
}

// Where writes a movie's filming locations and settings with map links.
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
			label := loc.Name
			if loc.Source != "" {
				label += " [" + loc.Source + "]"
			}
			if loc.MapsURL != "" {
				fmt.Fprintf(w, "  %s  %s\n", label, loc.MapsURL)
			} else {
				fmt.Fprintf(w, "  %s\n", label)
			}
		}
	}
	if len(m.SetIn) > 0 {
		fmt.Fprintln(w, "Set in:")
		for _, loc := range m.SetIn {
			fmt.Fprintf(w, "  %s\n", loc.Name)
		}
	}
	writeIMDB(w, m.IMDBURL)
}

// FilmsAt writes the movies filmed at a place, newest first.
func FilmsAt(w io.Writer, place string, movies []model.Movie) {
	fmt.Fprintf(w, "Filmed at %s:\n", place)
	for _, m := range movies {
		year := "----"
		if m.Year > 0 {
			year = strconv.Itoa(m.Year)
		}
		if m.IMDBURL != "" {
			fmt.Fprintf(w, "  %s  %s  %s\n", year, m.Title, m.IMDBURL)
		} else {
			fmt.Fprintf(w, "  %s  %s\n", year, m.Title)
		}
	}
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
