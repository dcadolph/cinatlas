package sin

import (
	"net/url"
	"strconv"

	"github.com/dcadolph/cinatlas/internal/imdb"
)

// Link is one outbound reference on a sin card.
type Link struct {
	// Label is the short display text.
	Label string
	// URL is the destination.
	URL string
	// Explicit marks destinations carrying explicit imagery, so the template
	// can badge them before anyone clicks.
	Explicit bool
}

// Links builds the outbound reference pack for a film: the IMDB parents guide
// whose sex and nudity section details exactly what is on screen, then the
// nudity databases and the cut-comparison archive. Sites without a stable
// public search route get a site-scoped web search so the link cannot rot.
func Links(title string, year int, imdbID string) []Link {
	var links []Link
	if guide := imdb.ParentalGuideURL(imdbID); guide != "" {
		links = append(links, Link{Label: "IMDb guide", URL: guide})
	}
	return append(links,
		Link{Label: "Mr. Skin", URL: siteSearch("mrskin.com", title, year), Explicit: true},
		Link{Label: "Mr. Man", URL: siteSearch("mrman.com", title, year), Explicit: true},
		Link{Label: "Cut versions", URL: siteSearch("movie-censorship.com", title, year)},
	)
}

// PersonLinks builds the outbound reference pack for a person: the nudity
// databases' per-celebrity coverage, scoped searches for the same rot-proof
// reason as the film links.
func PersonLinks(name string) []Link {
	return []Link{
		{Label: "Mr. Skin", URL: siteSearch("mrskin.com", name, 0), Explicit: true},
		{Label: "Mr. Man", URL: siteSearch("mrman.com", name, 0), Explicit: true},
	}
}

// siteSearch returns a DuckDuckGo search scoped to one site for the film.
func siteSearch(site, title string, year int) string {
	q := "site:" + site + " " + title
	if year > 0 {
		q += " " + strconv.Itoa(year)
	}
	return "https://duckduckgo.com/?q=" + url.QueryEscape(q)
}
