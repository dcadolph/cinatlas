package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/dcadolph/cinatlas/internal/wikidata"
)

// maxDoublesPairs is how many stand-in pairings one page shows.
const maxDoublesPairs = 24

// maxStandFilms caps the titles listed under one pairing, so a place with
// dozens of credits does not push the next pairing off the screen. The list is
// trimmed in the template rather than here, because the card reports how many
// films made the swap and trimming first would make it report the cap.
const maxStandFilms = 6

// handleDoubles answers the body doubles page: the places a named place has
// stood in for, or with reverse set, the places that have stood in for it.
func (s *Server) handleDoubles(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	reverse := r.URL.Query().Get("reverse") != ""
	data := pageData{Query: query, Reverse: reverse}
	if query == "" {
		s.render(w, http.StatusOK, "doubles.html", data)
		return
	}
	stands, err := s.locator.Doubles(r.Context(), query, reverse)
	if err != nil {
		s.log.Error("doubles lookup failed", "place", query, "reverse", reverse, "err", err)
		data.Error = "That lookup did not come back. Try again, or try a smaller place."
		s.render(w, http.StatusBadGateway, "doubles.html", data)
		return
	}
	if len(stands) == 0 {
		data.Error = fmt.Sprintf("No recorded stand-ins for %q. This needs a film to record "+
			"both where it shot and where it is set, which is thin outside film hubs.", query)
		s.render(w, http.StatusNotFound, "doubles.html", data)
		return
	}
	data.PlaceName = query
	data.StandsTotal = len(stands)
	if len(stands) > maxDoublesPairs {
		stands = stands[:maxDoublesPairs]
	}
	data.Stands = stands
	s.render(w, http.StatusOK, "doubles.html", data)
}

// formatKM renders a distance in whole kilometers with thousands separators.
func formatKM(km float64) string {
	s := strconv.FormatFloat(km, 'f', 0, 64)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String() + " km"
}

// firstFilms returns at most n films, for a template that shows a sample while
// the surrounding card reports the true total.
func firstFilms(n int, films []wikidata.Film) []wikidata.Film {
	if len(films) <= n {
		return films
	}
	return films[:n]
}
