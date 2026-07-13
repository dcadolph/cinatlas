package tmdb

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/dcadolph/cinatlas/internal/model"
)

// TestDiscover checks result mapping, genre id resolution, and popularity.
func TestDiscover(t *testing.T) {
	t.Parallel()
	srv := newServer(t, map[string]string{
		"/discover/movie": `{"results":[
			{"id":1,"title":"Bluey: The Movie","release_date":"2027-01-01",
			 "popularity":80.5,"genre_ids":[16,35]},
			{"id":2,"title":"Paddington","release_date":"2014-11-28",
			 "popularity":60.1,"genre_ids":[35,99]}]}`,
		"/genre/movie/list": `{"genres":[{"id":16,"name":"Animation"},{"id":35,"name":"Comedy"}]}`,
		"/genre/tv/list":    `{"genres":[]}`,
	})
	got, err := newClient(t, srv).Discover(context.Background(), DiscoverQuery{CertificationLTE: "PG"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := []model.Movie{{
		TMDBID: 1, Title: "Bluey: The Movie", Year: 2027, ReleaseDate: "2027-01-01",
		Popularity: 80.5, Genres: []string{"Animation", "Comedy"},
	}, {
		TMDBID: 2, Title: "Paddington", Year: 2014, ReleaseDate: "2014-11-28",
		Popularity: 60.1, Genres: []string{"Comedy"},
	}}
	if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

// TestDiscoverQueryParams checks the certification and page parameters reach the API.
func TestDiscoverQueryParams(t *testing.T) {
	t.Parallel()
	var query url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/discover/movie" {
			query = r.URL.Query()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	t.Cleanup(srv.Close)
	in := DiscoverQuery{CertificationLTE: "PG-13", Page: 3}
	if _, err := newClient(t, srv).Discover(context.Background(), in); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for key, want := range map[string]string{
		"certification_country": "US",
		"certification.lte":     "PG-13",
		"page":                  "3",
		"include_adult":         "false",
		"sort_by":               "popularity.desc",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("param %s = %q, want %q", key, got, want)
		}
	}
}

// TestCertification checks US selection, empty-entry skipping, and the missing case.
func TestCertification(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantCert string
		Body     string
	}{{ // Test 0: The first non-empty US certification wins.
		Body: `{"results":[
			{"iso_3166_1":"DE","release_dates":[{"certification":"12"}]},
			{"iso_3166_1":"US","release_dates":[{"certification":""},{"certification":"PG"}]}]}`,
		WantCert: "PG",
	}, { // Test 1: No US entry yields empty.
		Body:     `{"results":[{"iso_3166_1":"FR","release_dates":[{"certification":"U"}]}]}`,
		WantCert: "",
	}, { // Test 2: A US entry with only blank certifications yields empty.
		Body:     `{"results":[{"iso_3166_1":"US","release_dates":[{"certification":""}]}]}`,
		WantCert: "",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			srv := newServer(t, map[string]string{"/movie/7/release_dates": test.Body})
			got, err := newClient(t, srv).Certification(context.Background(), 7)
			if err != nil {
				t.Fatalf("Certification: %v", err)
			}
			if got != test.WantCert {
				t.Errorf("got %q, want %q", got, test.WantCert)
			}
		})
	}
}
