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

// TestDiscoverCertificationGTE checks the floor parameter reaches the API with
// its country and without a stray ceiling.
func TestDiscoverCertificationGTE(t *testing.T) {
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
	if _, err := newClient(t, srv).Discover(context.Background(), DiscoverQuery{CertificationGTE: "NC-17"}); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got := query.Get("certification_country"); got != "US" {
		t.Errorf("certification_country = %q, want US", got)
	}
	if got := query.Get("certification.gte"); got != "NC-17" {
		t.Errorf("certification.gte = %q, want NC-17", got)
	}
	if got := query.Get("certification.lte"); got != "" {
		t.Errorf("certification.lte = %q, want empty", got)
	}
}

// TestMovieThemes checks the one-call bundle: lowercased keywords, the US
// certification, the IMDB id, and the vote count.
func TestMovieThemes(t *testing.T) {
	t.Parallel()
	srv := newServer(t, map[string]string{
		"/movie/9": `{"id":9,"imdb_id":"tt0103772","vote_count":4200,
			"keywords":{"keywords":[{"id":1,"name":"Eroticism"},{"id":2,"name":"Female Nudity"}]},
			"release_dates":{"results":[
				{"iso_3166_1":"DE","release_dates":[{"certification":"16"}]},
				{"iso_3166_1":"US","release_dates":[{"certification":""},{"certification":"R"}]}]}}`,
	})
	got, err := newClient(t, srv).MovieThemes(context.Background(), 9)
	if err != nil {
		t.Fatalf("MovieThemes: %v", err)
	}
	want := Themes{
		Keywords:      []string{"eroticism", "female nudity"},
		Certification: "R",
		IMDBID:        "tt0103772",
		Votes:         4200,
	}
	if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

// TestKeyword checks match decoding in relevance order.
func TestKeyword(t *testing.T) {
	t.Parallel()
	srv := newServer(t, map[string]string{
		"/search/keyword": `{"results":[{"id":9999,"name":"heist gone wrong"},{"id":5,"name":"heist"}]}`,
	})
	got, err := newClient(t, srv).Keyword(context.Background(), "heist")
	if err != nil {
		t.Fatalf("Keyword: %v", err)
	}
	want := []KeywordMatch{{ID: 9999, Name: "heist gone wrong"}, {ID: 5, Name: "heist"}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
