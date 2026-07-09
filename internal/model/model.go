// Package model holds the shared types the cinatlas packages exchange.
package model

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
	// Director is the credited director name, empty when unknown.
	Director string `json:"director,omitempty"`
	// Cast lists the billed people in order.
	Cast []Person `json:"cast,omitempty"`
	// Locations lists resolved filming locations.
	Locations []Location `json:"locations,omitempty"`
	// IMDBURL is the deep link to the IMDB title page.
	IMDBURL string `json:"imdbUrl,omitempty"`
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
	// Credits lists notable filmography entries.
	Credits []Credit `json:"credits,omitempty"`
	// IMDBURL is the deep link to the IMDB name page.
	IMDBURL string `json:"imdbUrl,omitempty"`
}

// Credit is one filmography entry for a person.
type Credit struct {
	// Title is the film or show title.
	Title string `json:"title"`
	// Year is the release year, zero when unknown.
	Year int `json:"year,omitempty"`
	// Character is the role played, empty for crew credits.
	Character string `json:"character,omitempty"`
	// Job is the crew role such as Director, empty for acting credits.
	Job string `json:"job,omitempty"`
}

// Location is a real-world filming location for a film.
type Location struct {
	// Name is the human-readable place description.
	Name string `json:"name"`
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
