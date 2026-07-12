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
	// PosterURL is the title poster, empty when none exists.
	PosterURL string `json:"posterUrl,omitempty"`
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
