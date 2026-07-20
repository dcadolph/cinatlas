package tmdb

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/dcadolph/cinatlas/internal/model"
)

// DiscoverQuery filters the TMDB discover endpoint. Zero-valued fields are
// omitted, so the empty query returns the most popular films overall.
type DiscoverQuery struct {
	// CertificationLTE keeps films at or under this US certification, empty for all.
	CertificationLTE string
	// CertificationGTE keeps films at or above this US certification, empty for all.
	CertificationGTE string
	// WithGenres are genre ids the film must all have.
	WithGenres []int
	// WithoutGenres are genre ids the film must not have.
	WithoutGenres []int
	// WithKeywords are keyword ids the film may have any of.
	WithKeywords []int
	// WithCast are person ids that must all appear in the billed cast.
	WithCast []int
	// VoteAverageGTE is the minimum TMDB vote average.
	VoteAverageGTE float64
	// VoteCountGTE is the minimum TMDB vote count.
	VoteCountGTE int
	// ReleaseDateGTE bounds the earliest release, as yyyy-mm-dd.
	ReleaseDateGTE string
	// ReleaseDateLTE bounds the latest release, as yyyy-mm-dd.
	ReleaseDateLTE string
	// SortBy is the TMDB ordering, defaulting to popularity.desc.
	SortBy string
	// Page is the one-based result page, zero for the first.
	Page int
}

// Discover returns movies matching the query, most popular first by default.
// Genre ids on the list payload resolve to names through the genre map so
// downstream fit checks see the same genre strings as full movie lookups.
func (c *HTTPClient) Discover(ctx context.Context, query DiscoverQuery) ([]model.Movie, error) {
	sortBy := query.SortBy
	if sortBy == "" {
		sortBy = "popularity.desc"
	}
	q := url.Values{
		"sort_by":       {sortBy},
		"include_adult": {"false"},
	}
	if query.CertificationLTE != "" || query.CertificationGTE != "" {
		q.Set("certification_country", "US")
	}
	if query.CertificationLTE != "" {
		q.Set("certification.lte", query.CertificationLTE)
	}
	if query.CertificationGTE != "" {
		q.Set("certification.gte", query.CertificationGTE)
	}
	if len(query.WithGenres) > 0 {
		q.Set("with_genres", joinIDs(query.WithGenres, ","))
	}
	if len(query.WithoutGenres) > 0 {
		q.Set("without_genres", joinIDs(query.WithoutGenres, ","))
	}
	if len(query.WithKeywords) > 0 {
		q.Set("with_keywords", joinIDs(query.WithKeywords, "|"))
	}
	if len(query.WithCast) > 0 {
		q.Set("with_cast", joinIDs(query.WithCast, ","))
	}
	if query.VoteAverageGTE > 0 {
		q.Set("vote_average.gte", strconv.FormatFloat(query.VoteAverageGTE, 'f', 1, 64))
	}
	if query.VoteCountGTE > 0 {
		q.Set("vote_count.gte", strconv.Itoa(query.VoteCountGTE))
	}
	if query.ReleaseDateGTE != "" {
		q.Set("primary_release_date.gte", query.ReleaseDateGTE)
	}
	if query.ReleaseDateLTE != "" {
		q.Set("primary_release_date.lte", query.ReleaseDateLTE)
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
	return usCertification(out.Results), nil
}

// Themes is the per-film fact bundle theme scoring reads: the community
// keyword tags, the US certification, the IMDB id for outbound links, and the
// vote count for ranking. One request carries all of it.
type Themes struct {
	// Keywords are the film's TMDB keyword tag names, lowercased.
	Keywords []string
	// Certification is the US certification, empty when unrated.
	Certification string
	// IMDBID is the IMDB title id, empty when TMDB has none.
	IMDBID string
	// Votes counts TMDB votes on the film, a durable fame signal.
	Votes int
}

// MovieThemes returns the film's keyword tags, US certification, IMDB id, and
// vote count in one call, for callers that score films by theme.
func (c *HTTPClient) MovieThemes(ctx context.Context, id int) (Themes, error) {
	var out struct {
		IMDBID   string `json:"imdb_id"`
		Votes    int    `json:"vote_count"`
		Keywords struct {
			Keywords []struct {
				Name string `json:"name"`
			} `json:"keywords"`
		} `json:"keywords"`
		ReleaseDates struct {
			Results []releaseDatesDTO `json:"results"`
		} `json:"release_dates"`
	}
	q := url.Values{"append_to_response": {"keywords,release_dates"}}
	if err := c.get(ctx, "/movie/"+strconv.Itoa(id), q, &out); err != nil {
		return Themes{}, err
	}
	themes := Themes{
		Certification: usCertification(out.ReleaseDates.Results),
		IMDBID:        out.IMDBID,
		Votes:         out.Votes,
	}
	for _, k := range out.Keywords.Keywords {
		themes.Keywords = append(themes.Keywords, strings.ToLower(k.Name))
	}
	return themes, nil
}

// usCertification returns the first non-empty US certification in a release
// list, empty when none is on record.
func usCertification(results []releaseDatesDTO) string {
	for _, r := range results {
		if r.CountryCode != "US" {
			continue
		}
		for _, rd := range r.ReleaseDates {
			if rd.Certification != "" {
				return rd.Certification
			}
		}
	}
	return ""
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
