package tmdb

import (
	"context"
	"net/url"
	"strconv"

	"github.com/dcadolph/cinatlas/internal/model"
)

// DiscoverQuery filters the TMDB discover endpoint.
type DiscoverQuery struct {
	// CertificationLTE keeps films at or under this US certification, empty for all.
	CertificationLTE string
	// Page is the one-based result page, zero for the first.
	Page int
}

// Discover returns popular movies matching the query, most popular first. Genre ids
// on the list payload resolve to names through the genre map so downstream fit
// checks see the same genre strings as full movie lookups.
func (c *HTTPClient) Discover(ctx context.Context, query DiscoverQuery) ([]model.Movie, error) {
	q := url.Values{
		"sort_by":       {"popularity.desc"},
		"include_adult": {"false"},
	}
	if query.CertificationLTE != "" {
		q.Set("certification_country", "US")
		q.Set("certification.lte", query.CertificationLTE)
	}
	if query.Page > 1 {
		q.Set("page", strconv.Itoa(query.Page))
	}
	var out struct {
		Results []movieDTO `json:"results"`
	}
	if err := c.get(ctx, "/discover/movie", q, &out); err != nil {
		return nil, err
	}
	genres := c.genreMap(ctx)
	movies := make([]model.Movie, 0, len(out.Results))
	for _, r := range out.Results {
		m := r.toModel()
		if len(m.Genres) == 0 {
			for _, id := range r.GenreIDs {
				if name, ok := genres[id]; ok {
					m.Genres = append(m.Genres, name)
				}
			}
		}
		movies = append(movies, m)
	}
	return movies, nil
}

// Certification returns the US certification for a movie, empty when TMDB has none
// on record.
func (c *HTTPClient) Certification(ctx context.Context, id int) (string, error) {
	var out struct {
		Results []releaseDatesDTO `json:"results"`
	}
	if err := c.get(ctx, "/movie/"+strconv.Itoa(id)+"/release_dates", nil, &out); err != nil {
		return "", err
	}
	for _, r := range out.Results {
		if r.CountryCode != "US" {
			continue
		}
		for _, rd := range r.ReleaseDates {
			if rd.Certification != "" {
				return rd.Certification, nil
			}
		}
	}
	return "", nil
}

// releaseDatesDTO is the per-country release list on the release dates endpoint.
type releaseDatesDTO struct {
	// CountryCode is the two-letter country the releases belong to.
	CountryCode string `json:"iso_3166_1"`
	// ReleaseDates lists the country's releases with their certifications.
	ReleaseDates []releaseDateDTO `json:"release_dates"`
}

// releaseDateDTO is one release entry carrying a certification.
type releaseDateDTO struct {
	// Certification is the rating body label, empty when unrated.
	Certification string `json:"certification"`
}
