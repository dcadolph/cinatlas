// Package model holds the shared types the cinatlas packages exchange.
package model

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Movie is a film with the facts cinatlas reports about it.
type Movie struct {
	// TMDBID is the Movie Database identifier.
	TMDBID int `json:"tmdbId"`
	// IMDBID is the IMDB title identifier, such as tt0083658.
	IMDBID string `json:"imdbId,omitempty"`
	// Title is the primary display title.
	Title string `json:"title"`
	// Year is the release year, zero when unknown.
	Year int `json:"year,omitempty"`
	// ReleaseDate is the ISO release date, empty when unknown.
	ReleaseDate string `json:"releaseDate,omitempty"`
	// Director is the credited director name, empty when unknown.
	Director string `json:"director,omitempty"`
	// Overview is the one-paragraph synopsis, empty when unknown.
	Overview string `json:"overview,omitempty"`
	// Tagline is the marketing one-liner, empty when unknown.
	Tagline string `json:"tagline,omitempty"`
	// Runtime is the length in minutes, zero when unknown.
	Runtime int `json:"runtime,omitempty"`
	// Rating is the TMDB average vote from 0 to 10, zero when unknown.
	Rating float64 `json:"rating,omitempty"`
	// Genres lists the genre names.
	Genres []string `json:"genres,omitempty"`
	// PosterURL is the poster image, empty when none exists.
	PosterURL string `json:"posterUrl,omitempty"`
	// BackdropURL is the wide backdrop image, empty when none exists.
	BackdropURL string `json:"backdropUrl,omitempty"`
	// Cast lists the billed people in order.
	Cast []Person `json:"cast,omitempty"`
	// Locations lists resolved filming locations.
	Locations []Location `json:"locations,omitempty"`
	// SetIn lists where the story takes place, distinct from where it filmed.
	SetIn []Location `json:"setIn,omitempty"`
	// Availability lists the ways to watch, by service and access kind, for
	// WatchRegion. Empty when nothing is on record there.
	Availability []Availability `json:"availability,omitempty"`
	// WatchRegion is the two-letter country the availability applies to.
	WatchRegion string `json:"watchRegion,omitempty"`
	// WatchURL links the region's TMDB watch page, which lists every service
	// and links out to JustWatch. Empty when the region has no data.
	WatchURL string `json:"watchUrl,omitempty"`
	// IMDBURL is the deep link to the IMDB title page.
	IMDBURL string `json:"imdbUrl,omitempty"`
	// IMDBLocationsURL is the deep link to the IMDB filming-locations page.
	IMDBLocationsURL string `json:"imdbLocationsUrl,omitempty"`
}

// Person is someone credited on a film.
type Person struct {
	// TMDBID is the Movie Database identifier.
	TMDBID int `json:"tmdbId"`
	// IMDBID is the IMDB name identifier, such as nm0000123.
	IMDBID string `json:"imdbId,omitempty"`
	// Name is the display name.
	Name string `json:"name"`
	// Character is the role played, set only in a cast context.
	Character string `json:"character,omitempty"`
	// KnownFor is a short note on why the person is recognizable.
	KnownFor string `json:"knownFor,omitempty"`
	// PhotoURL is the profile image, empty when none exists.
	PhotoURL string `json:"photoUrl,omitempty"`
	// Credits lists notable filmography entries.
	Credits []Credit `json:"credits,omitempty"`
	// IMDBURL is the deep link to the IMDB name page.
	IMDBURL string `json:"imdbUrl,omitempty"`
}

// Credit is one filmography entry for a person.
type Credit struct {
	// TMDBID is the Movie Database identifier of the title.
	TMDBID int `json:"tmdbId,omitempty"`
	// Kind is the credit medium, movie or tv, empty when unknown.
	Kind string `json:"kind,omitempty"`
	// Title is the film or show title.
	Title string `json:"title"`
	// Year is the release year, zero when unknown.
	Year int `json:"year,omitempty"`
	// Character is the role played, empty for crew credits.
	Character string `json:"character,omitempty"`
	// Job is the crew role such as Director, empty for acting credits.
	Job string `json:"job,omitempty"`
	// Votes counts TMDB votes on the title, a durable fame signal.
	Votes int `json:"votes,omitempty"`
	// PosterURL is the title poster, empty when none exists.
	PosterURL string `json:"posterUrl,omitempty"`
}

// Location is a real-world place tied to a film.
type Location struct {
	// Name is the human-readable place description.
	Name string `json:"name"`
	// Source names where the fact came from: wikidata, wikipedia, or country.
	Source string `json:"source,omitempty"`
	// Latitude is the decimal latitude, valid only when Resolved is true.
	Latitude float64 `json:"latitude,omitempty"`
	// Longitude is the decimal longitude, valid only when Resolved is true.
	Longitude float64 `json:"longitude,omitempty"`
	// Resolved reports whether coordinates were found for this place.
	Resolved bool `json:"resolved"`
	// MapsURL links the place on Google Maps.
	MapsURL string `json:"mapsUrl,omitempty"`
	// EarthURL links the place on Google Earth.
	EarthURL string `json:"earthUrl,omitempty"`
}

// Access kinds rank how a service offers a title, cheapest for the viewer
// first. Stream means included in a subscription; free and ads cost nothing;
// rent and buy are transactional.
const (
	// AccessStream is included with a paid subscription to the service.
	AccessStream = "stream"
	// AccessFree is watchable at no cost with no ads.
	AccessFree = "free"
	// AccessAds is watchable at no cost with ads.
	AccessAds = "ads"
	// AccessRent is a time-limited paid rental.
	AccessRent = "rent"
	// AccessBuy is a paid purchase to keep.
	AccessBuy = "buy"
)

// accessRank orders access kinds from best to worst for the viewer.
var accessRank = map[string]int{
	AccessStream: 0,
	AccessFree:   1,
	AccessAds:    2,
	AccessRent:   3,
	AccessBuy:    4,
}

// Availability is one way to watch a movie on one service in one region.
type Availability struct {
	// Provider is the streaming service name, such as Netflix or Hulu.
	Provider string `json:"provider"`
	// Kind is how the service offers it: stream, free, ads, rent, or buy.
	Kind string `json:"kind"`
	// LogoURL is the service logo image, empty when none exists.
	LogoURL string `json:"logoUrl,omitempty"`
	// Owned reports that the viewer subscribes to this service, set by
	// TagOwnership. Only meaningful for stream, free, and ads access.
	Owned bool `json:"owned,omitempty"`
}

// Included reports whether the access needs no extra payment beyond any
// subscription the viewer already holds.
func (a Availability) Included() bool {
	return a.Kind == AccessStream || a.Kind == AccessFree || a.Kind == AccessAds
}

// SortAvailability orders entries in place by access kind, best for the viewer
// first, then by provider name so output is stable.
func SortAvailability(av []Availability) {
	sort.SliceStable(av, func(i, j int) bool {
		if accessRank[av[i].Kind] != accessRank[av[j].Kind] {
			return accessRank[av[i].Kind] < accessRank[av[j].Kind]
		}
		return strings.ToLower(av[i].Provider) < strings.ToLower(av[j].Provider)
	})
}

// TagOwnership marks each availability whose provider the viewer subscribes
// to. A token matches a provider case-insensitively as a substring, so "prime"
// matches "Amazon Prime Video". It returns how many entries were tagged.
func TagOwnership(av []Availability, services ...string) int {
	tokens := make([]string, 0, len(services))
	for _, s := range services {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			tokens = append(tokens, s)
		}
	}
	tagged := 0
	for i := range av {
		name := strings.ToLower(av[i].Provider)
		for _, t := range tokens {
			if strings.Contains(name, t) {
				av[i].Owned = true
				tagged++
				break
			}
		}
	}
	return tagged
}

// SortCredits orders credits in place: "az" by title, "new" and "old" by
// year with unknown years last, anything else keeps fame order.
func SortCredits(credits []Credit, order string) {
	switch order {
	case "az":
		sort.SliceStable(credits, func(i, j int) bool {
			return strings.ToLower(credits[i].Title) < strings.ToLower(credits[j].Title)
		})
	case "new":
		sort.SliceStable(credits, func(i, j int) bool { return credits[i].Year > credits[j].Year })
	case "old":
		sort.SliceStable(credits, func(i, j int) bool {
			if credits[i].Year == 0 || credits[j].Year == 0 {
				return credits[j].Year == 0 && credits[i].Year != 0
			}
			return credits[i].Year < credits[j].Year
		})
	}
}

// CreditDecades lists the release decades present, newest first.
func CreditDecades(credits []Credit) []int {
	seen := map[int]bool{}
	for _, c := range credits {
		if c.Year > 0 {
			seen[c.Year/10*10] = true
		}
	}
	out := make([]int, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}

// FilterCreditsByDecade keeps credits released in the given decade, or all
// credits when decade is zero.
func FilterCreditsByDecade(credits []Credit, decade int) []Credit {
	if decade == 0 {
		return credits
	}
	kept := make([]Credit, 0, len(credits))
	for _, c := range credits {
		if c.Year >= decade && c.Year < decade+10 {
			kept = append(kept, c)
		}
	}
	return kept
}

// ResolvedLocation builds a Location with coordinates and map links.
func ResolvedLocation(name, source string, lat, lon float64) Location {
	pair := strconv.FormatFloat(lat, 'f', -1, 64) + "," + strconv.FormatFloat(lon, 'f', -1, 64)
	return Location{
		Name:      name,
		Source:    source,
		Latitude:  lat,
		Longitude: lon,
		Resolved:  true,
		MapsURL:   "https://www.google.com/maps/search/?api=1&query=" + pair,
		EarthURL:  "https://earth.google.com/web/@" + pair + ",1000a,1000d",
	}
}

// UnresolvedLocation builds a Location without coordinates, linking a name
// text search on Google Maps so a known place is never a dead end.
func UnresolvedLocation(name, source string) Location {
	return Location{
		Name:    name,
		Source:  source,
		MapsURL: "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(name),
	}
}
