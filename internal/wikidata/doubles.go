package wikidata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// maxDoublesRows caps one stand-in query. A film records several filming
// locations and several settings, and the query pairs every one against every
// other, so the row count runs well ahead of the film count.
const maxDoublesRows = 800

// DoublesFinder reports the places a named place has stood in for on screen,
// or with reverse set, the places that have stood in for it.
type DoublesFinder interface {
	Doubles(ctx context.Context, place string, reverse bool) ([]Stand, error)
}

// DoublesFinderFunc adapts a function to the DoublesFinder interface.
type DoublesFinderFunc func(ctx context.Context, place string, reverse bool) ([]Stand, error)

// Doubles calls the underlying function.
func (f DoublesFinderFunc) Doubles(ctx context.Context, place string, reverse bool) ([]Stand, error) {
	return f(ctx, place, reverse)
}

// Stand is one place standing in for another on screen, with the films that
// did it. A film is counted once per pair no matter how many rows it produced.
type Stand struct {
	// Shot is the place the camera was actually at.
	Shot string
	// Played is the place the story claims to be.
	Played string
	// Films are the titles that made the swap, most sitelinked first.
	Films []Film
	// Spots are the specific places inside Shot that the camera recorded, when
	// Wikidata pins a venue rather than the city. Empty when the pin already
	// named the searched place.
	Spots []string
	// KM is the great-circle distance between the two places, zero when either
	// side has no coordinates.
	KM float64
}

// Doubles returns the places a named place has stood in for on screen, the
// place playing someone else. Passing reverse flips the question to which
// places have stood in for it.
//
// Both directions read the same two Wikidata properties, filming location and
// narrative location, and report only the disagreements. A film shot where it
// is set contributes nothing.
func (c *HTTPClient) Doubles(ctx context.Context, place string, reverse bool) ([]Stand, error) {
	place = strings.TrimSpace(place)
	if place == "" {
		return nil, nil
	}
	ids, err := c.searchEntities(ctx, place)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := c.query(ctx, doublesQuery(ids, reverse))
	if err != nil {
		return nil, err
	}
	return rows.stands(place, reverse), nil
}

// query runs one SPARQL request and returns the decoded result set.
func (c *HTTPClient) query(ctx context.Context, sparql string) (sparqlResponse, error) {
	var out sparqlResponse
	q := url.Values{"query": {sparql}, "format": {"json"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return out, fmt.Errorf("%w: build request: %w", ErrRequest, err)
	}
	req.Header.Set("Accept", "application/sparql-results+json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return out, fmt.Errorf("%w: %w", ErrRequest, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("%w: status %d", ErrRequest, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, fmt.Errorf("%w: decode: %w", ErrRequest, err)
	}
	return out, nil
}

// doublesQuery builds the SPARQL text for stand-in pairs involving any
// candidate place. The place is bound on the filming side by default and on
// the setting side when reverse is set.
//
// P131* walks the administrative tree so a pin on a studio lot still answers
// for the city it sits in. Only the far side is required to differ, which is
// what keeps a film shot where it is set out of the results.
//
// The bound side is written before the film pattern on purpose. Leading with
// the film pattern makes the planner walk every film holding both properties
// and test each one, which times out on a place as busy as New York. Starting
// from the handful of matching places instead answers in a few seconds.
func doublesQuery(ids []string, reverse bool) string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, "wd:"+id)
	}
	bind := `?shot wdt:P131* ?target .`
	if reverse {
		bind = `?set wdt:P131* ?target .`
	}
	return `SELECT ?filmLabel ?imdb ?date ?links ?shotLabel ?setLabel ?shotCoord ?setCoord WHERE {
  VALUES ?target { ` + strings.Join(values, " ") + ` }
  ` + bind + `
  ?film wdt:P915 ?shot ; wdt:P840 ?set ; wdt:P345 ?imdb ; wikibase:sitelinks ?links .
  FILTER(?shot != ?set)
  OPTIONAL { ?film wdt:P577 ?date . }
  OPTIONAL { ?shot wdt:P625 ?shotCoord . }
  OPTIONAL { ?set wdt:P625 ?setCoord . }
  SERVICE wikibase:label {
    bd:serviceParam wikibase:language "en" .
    ?film rdfs:label ?filmLabel .
    ?shot rdfs:label ?shotLabel .
    ?set rdfs:label ?setLabel .
  }
}
ORDER BY DESC(?links)
LIMIT ` + strconv.Itoa(maxDoublesRows)
}

// stands folds stand-in rows into one entry per far-side place, ordered by how
// many films made the swap.
//
// Grouping is by the far side alone, with anchor standing in for the near side.
// Wikidata pins filming at whatever granularity an editor chose, so a single
// film shot across seven Prague venues arrives as seven rows naming Strahov
// Library, Wenceslas Square, and the rest. Keying on both sides would file each
// venue as its own pairing and report one film seven times. The venue labels are
// kept as Spots, since they are the interesting part once the count is right.
func (r sparqlResponse) stands(anchor string, reverse bool) []Stand {
	group := map[string]*Stand{}
	seen := map[string]bool{}
	spots := map[string]map[string]bool{}
	for _, row := range r.Results.Bindings {
		shot, played := row["shotLabel"].Value, row["setLabel"].Value
		imdbID, title := row["imdb"].Value, row["filmLabel"].Value
		if shot == "" || played == "" || shot == played || imdbID == "" || title == "" {
			continue
		}
		far, near := played, shot
		if reverse {
			far, near = shot, played
		}
		if spots[far] == nil {
			spots[far] = map[string]bool{}
		}
		if !strings.EqualFold(near, anchor) {
			spots[far][near] = true
		}
		if seen[far+"|"+imdbID] {
			continue
		}
		seen[far+"|"+imdbID] = true
		s := group[far]
		if s == nil {
			s = &Stand{Shot: anchor, Played: far,
				KM: haversine(row["shotCoord"].Value, row["setCoord"].Value)}
			if reverse {
				s.Shot, s.Played = far, anchor
			}
			group[far] = s
		}
		year := 0
		if date := row["date"].Value; len(date) >= 4 {
			if y, err := strconv.Atoi(date[:4]); err == nil {
				year = y
			}
		}
		s.Films = append(s.Films, Film{Title: title, Year: year, IMDBID: imdbID, Place: shot})
	}
	out := make([]Stand, 0, len(group))
	for far, s := range group {
		for spot := range spots[far] {
			s.Spots = append(s.Spots, spot)
		}
		sort.Strings(s.Spots)
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Films) != len(out[j].Films) {
			return len(out[i].Films) > len(out[j].Films)
		}
		return out[i].Played < out[j].Played
	})
	return out
}
