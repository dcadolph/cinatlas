// Package imdb builds outbound deep links to IMDB. It makes no network calls.
package imdb

import "strings"

// baseURL is the IMDB site root used for all links.
const baseURL = "https://www.imdb.com"

// TitleURL returns the IMDB page for a title id such as tt0083658.
// It returns an empty string when the id is blank or not a title id.
func TitleURL(id string) string {
	id = strings.TrimSpace(id)
	if !strings.HasPrefix(id, "tt") {
		return ""
	}
	return baseURL + "/title/" + id + "/"
}

// NameURL returns the IMDB page for a name id such as nm0000123.
// It returns an empty string when the id is blank or not a name id.
func NameURL(id string) string {
	id = strings.TrimSpace(id)
	if !strings.HasPrefix(id, "nm") {
		return ""
	}
	return baseURL + "/name/" + id + "/"
}

// LocationsURL returns the IMDB filming-locations page for a title id.
// It returns an empty string when the id is blank or not a title id.
func LocationsURL(id string) string {
	title := TitleURL(id)
	if title == "" {
		return ""
	}
	return title + "locations/"
}

// FullCreditsURL returns the IMDB full cast and crew page for a title id.
// It returns an empty string when the id is blank or not a title id.
func FullCreditsURL(id string) string {
	title := TitleURL(id)
	if title == "" {
		return ""
	}
	return title + "fullcredits/"
}
