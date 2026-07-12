package tmdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/dcadolph/cinatlas/internal/model"
)

// newServer returns a test server serving the given path-to-body routes.
func newServer(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, body := range routes {
		b := body
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(b))
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newClient returns a client pointed at the test server.
func newClient(t *testing.T, srv *httptest.Server) *HTTPClient {
	t.Helper()
	c, err := New("testkey", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// TestNewRejectsEmptyKey checks the constructor guards against a missing key.
func TestNewRejectsEmptyKey(t *testing.T) {
	t.Parallel()
	if _, err := New("  "); !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("New(empty) err = %v, want ErrNoAPIKey", err)
	}
}

// TestSearchMovie checks search result mapping.
func TestSearchMovie(t *testing.T) {
	t.Parallel()
	srv := newServer(t, map[string]string{
		"/search/movie": `{"results":[{"id":1,"title":"Heat","release_date":"1995-12-15"}]}`,
	})
	got, err := newClient(t, srv).SearchMovie(context.Background(), "heat")
	if err != nil {
		t.Fatalf("SearchMovie: %v", err)
	}
	want := []model.Movie{{TMDBID: 1, Title: "Heat", Year: 1995, ReleaseDate: "1995-12-15"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SearchMovie\n got %+v\nwant %+v", got, want)
	}
}

// TestMovie checks details mapping, director extraction, cast, and IMDB link.
func TestMovie(t *testing.T) {
	t.Parallel()
	srv := newServer(t, map[string]string{
		"/movie/1": `{"id":1,"title":"Heat","release_date":"1995-12-15","imdb_id":"tt0113277",` +
			`"tagline":"A Los Angeles crime saga.","runtime":170,"vote_average":7.9,` +
			`"genres":[{"id":80,"name":"Crime"},{"id":18,"name":"Drama"}],` +
			`"poster_path":"/heat.jpg","backdrop_path":"/heat-wide.jpg",` +
			`"credits":{"cast":[{"id":10,"name":"Al Pacino","character":"Vincent Hanna",` +
			`"profile_path":"/pacino.jpg"}],` +
			`"crew":[{"id":20,"name":"Michael Mann","job":"Director"},` +
			`{"id":21,"name":"Art Linson","job":"Producer"}]}}`,
	})
	got, err := newClient(t, srv).Movie(context.Background(), 1)
	if err != nil {
		t.Fatalf("Movie: %v", err)
	}
	want := &model.Movie{
		TMDBID:      1,
		IMDBID:      "tt0113277",
		Title:       "Heat",
		Year:        1995,
		ReleaseDate: "1995-12-15",
		Director:    "Michael Mann",
		Tagline:     "A Los Angeles crime saga.",
		Runtime:     170,
		Rating:      7.9,
		Genres:      []string{"Crime", "Drama"},
		PosterURL:   "https://image.tmdb.org/t/p/w342/heat.jpg",
		BackdropURL: "https://image.tmdb.org/t/p/w1280/heat-wide.jpg",
		Cast: []model.Person{{
			TMDBID: 10, Name: "Al Pacino", Character: "Vincent Hanna",
			PhotoURL: "https://image.tmdb.org/t/p/w185/pacino.jpg",
		}},
		IMDBURL:          "https://www.imdb.com/title/tt0113277/",
		IMDBLocationsURL: "https://www.imdb.com/title/tt0113277/locations/",
		WatchRegion:      "US",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Movie\n got %+v\nwant %+v", got, want)
	}
}

// TestMovieWatchProviders checks watch availability mapping for the client
// region, kind ordering, and the watch-page link, ignoring other regions.
func TestMovieWatchProviders(t *testing.T) {
	t.Parallel()
	srv := newServer(t, map[string]string{
		"/movie/1": `{"id":1,"title":"Heat","imdb_id":"tt0113277",` +
			`"watch/providers":{"results":{` +
			`"US":{"link":"https://justwatch.example/us",` +
			`"buy":[{"provider_name":"Apple TV","logo_path":"/apple.jpg"}],` +
			`"flatrate":[{"provider_name":"Max","logo_path":"/max.jpg"}],` +
			`"rent":[{"provider_name":"Amazon Video","logo_path":"/amz.jpg"}]},` +
			`"GB":{"link":"https://justwatch.example/gb",` +
			`"flatrate":[{"provider_name":"Netflix","logo_path":"/nflx.jpg"}]}}}}`,
	})
	c, err := New("testkey", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithRegion("us"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := c.Movie(context.Background(), 1)
	if err != nil {
		t.Fatalf("Movie: %v", err)
	}
	// Stream sorts before rent before buy; GB is not mixed in.
	wantAvail := []model.Availability{
		{Provider: "Max", Kind: model.AccessStream, LogoURL: "https://image.tmdb.org/t/p/w92/max.jpg"},
		{Provider: "Amazon Video", Kind: model.AccessRent, LogoURL: "https://image.tmdb.org/t/p/w92/amz.jpg"},
		{Provider: "Apple TV", Kind: model.AccessBuy, LogoURL: "https://image.tmdb.org/t/p/w92/apple.jpg"},
	}
	if got.WatchRegion != "US" {
		t.Errorf("WatchRegion = %q, want US", got.WatchRegion)
	}
	if got.WatchURL != "https://justwatch.example/us" {
		t.Errorf("WatchURL = %q", got.WatchURL)
	}
	if !reflect.DeepEqual(got.Availability, wantAvail) {
		t.Errorf("Availability\n got %+v\nwant %+v", got.Availability, wantAvail)
	}
}

// TestMovieWatchProvidersMissingRegion checks a region with no data yields no
// availability and no link.
func TestMovieWatchProvidersMissingRegion(t *testing.T) {
	t.Parallel()
	srv := newServer(t, map[string]string{
		"/movie/1": `{"id":1,"title":"Heat",` +
			`"watch/providers":{"results":{"GB":{"link":"x","flatrate":[` +
			`{"provider_name":"Netflix"}]}}}}`,
	})
	c, err := New("testkey", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithRegion("US"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := c.Movie(context.Background(), 1)
	if err != nil {
		t.Fatalf("Movie: %v", err)
	}
	if len(got.Availability) != 0 || got.WatchURL != "" {
		t.Errorf("want empty availability, got %+v link %q", got.Availability, got.WatchURL)
	}
}

// TestPerson checks person mapping with a year-sorted combined filmography.
func TestPerson(t *testing.T) {
	t.Parallel()
	srv := newServer(t, map[string]string{
		"/person/5": `{"id":5,"name":"Michael Mann","imdb_id":"nm0000520",` +
			`"known_for_department":"Directing","combined_credits":{"cast":[` +
			`{"id":7,"media_type":"tv","name":"Late Night Talk","character":"Self",` +
			`"first_air_date":"2020-01-01","vote_count":40}],` +
			`"crew":[{"id":1,"media_type":"movie","title":"Heat","job":"Director",` +
			`"release_date":"1995-12-15","vote_count":7000},` +
			`{"id":1,"media_type":"movie","title":"Heat","job":"Screenplay",` +
			`"release_date":"1995-12-15","vote_count":7000},` +
			`{"id":2,"media_type":"movie","title":"Collateral","job":"Director",` +
			`"release_date":"2004-08-06","vote_count":3000}]}}`,
	})
	got, err := newClient(t, srv).Person(context.Background(), 5)
	if err != nil {
		t.Fatalf("Person: %v", err)
	}
	// Fame orders Heat above the newer Collateral; the Self talk-show
	// appearance is dropped entirely.
	want := &model.Person{
		TMDBID:   5,
		IMDBID:   "nm0000520",
		Name:     "Michael Mann",
		KnownFor: "Directing",
		Credits: []model.Credit{
			{TMDBID: 1, Kind: "movie", Title: "Heat", Year: 1995, Job: "Director, Screenplay", Votes: 7000},
			{TMDBID: 2, Kind: "movie", Title: "Collateral", Year: 2004, Job: "Director", Votes: 3000},
		},
		IMDBURL: "https://www.imdb.com/name/nm0000520/",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Person\n got %+v\nwant %+v", got, want)
	}
}

// TestTrending checks trending result mapping.
func TestTrending(t *testing.T) {
	t.Parallel()
	srv := newServer(t, map[string]string{
		"/trending/movie/week": `{"results":[{"id":7,"title":"Weekly Hit",` +
			`"release_date":"2026-05-01","poster_path":"/hit.jpg"}]}`,
	})
	got, err := newClient(t, srv).Trending(context.Background())
	if err != nil {
		t.Fatalf("Trending: %v", err)
	}
	want := []model.Movie{{
		TMDBID: 7, Title: "Weekly Hit", Year: 2026, ReleaseDate: "2026-05-01",
		PosterURL: "https://image.tmdb.org/t/p/w342/hit.jpg",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Trending\n got %+v\nwant %+v", got, want)
	}
}

// TestRecommendations checks recommendation result mapping.
func TestRecommendations(t *testing.T) {
	t.Parallel()
	srv := newServer(t, map[string]string{
		"/movie/1/recommendations": `{"results":[{"id":9,"title":"Neighbor Film",` +
			`"release_date":"2020-02-02"}]}`,
	})
	got, err := newClient(t, srv).Recommendations(context.Background(), 1)
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	want := []model.Movie{{TMDBID: 9, Title: "Neighbor Film", Year: 2020, ReleaseDate: "2020-02-02"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Recommendations\n got %+v\nwant %+v", got, want)
	}
}

// TestSearchMulti checks blended search splitting, mapping, and first-kind.
func TestSearchMulti(t *testing.T) {
	t.Parallel()
	srv := newServer(t, map[string]string{
		"/search/multi": `{"results":[` +
			`{"media_type":"person","id":5,"name":"Michael Mann",` +
			`"known_for_department":"Directing","profile_path":"/mann.jpg"},` +
			`{"media_type":"movie","id":1,"title":"Heat","release_date":"1995-12-15",` +
			`"poster_path":"/heat.jpg"},` +
			`{"media_type":"tv","id":9,"name":"Heat TV"}]}`,
	})
	movies, people, first, err := newClient(t, srv).SearchMulti(context.Background(), "heat mann")
	if err != nil {
		t.Fatalf("SearchMulti: %v", err)
	}
	wantMovies := []model.Movie{{
		TMDBID: 1, Title: "Heat", Year: 1995, ReleaseDate: "1995-12-15",
		PosterURL: "https://image.tmdb.org/t/p/w342/heat.jpg",
	}}
	wantPeople := []model.Person{{
		TMDBID: 5, Name: "Michael Mann", KnownFor: "Directing",
		PhotoURL: "https://image.tmdb.org/t/p/w185/mann.jpg",
	}}
	if !reflect.DeepEqual(movies, wantMovies) {
		t.Errorf("SearchMulti movies\n got %+v\nwant %+v", movies, wantMovies)
	}
	if !reflect.DeepEqual(people, wantPeople) {
		t.Errorf("SearchMulti people\n got %+v\nwant %+v", people, wantPeople)
	}
	if first != "person" {
		t.Errorf("SearchMulti first = %q, want person", first)
	}
}

// TestFindByIMDB checks IMDB id resolution to a movie with links set.
func TestFindByIMDB(t *testing.T) {
	t.Parallel()
	srv := newServer(t, map[string]string{
		"/find/tt0113277": `{"movie_results":[{"id":1,"title":"Heat",` +
			`"release_date":"1995-12-15","poster_path":"/heat.jpg"}]}`,
		"/find/tt9999999": `{"movie_results":[]}`,
	})
	c := newClient(t, srv)
	got, err := c.FindByIMDB(context.Background(), "tt0113277")
	if err != nil {
		t.Fatalf("FindByIMDB: %v", err)
	}
	want := &model.Movie{
		TMDBID: 1, IMDBID: "tt0113277", Title: "Heat", Year: 1995,
		ReleaseDate: "1995-12-15",
		PosterURL:   "https://image.tmdb.org/t/p/w342/heat.jpg",
		IMDBURL:     "https://www.imdb.com/title/tt0113277/",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FindByIMDB\n got %+v\nwant %+v", got, want)
	}
	if _, err := c.FindByIMDB(context.Background(), "tt9999999"); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindByIMDB(unknown) err = %v, want ErrNotFound", err)
	}
}

// TestMovieNotFound checks that a 404 maps to ErrNotFound.
func TestMovieNotFound(t *testing.T) {
	t.Parallel()
	srv := newServer(t, map[string]string{}) // No routes, so any path returns 404.
	if _, err := newClient(t, srv).Movie(context.Background(), 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("Movie(missing) err = %v, want ErrNotFound", err)
	}
}

// TestAuth checks that a v3 key rides as a query param and a v4 token as a bearer header.
func TestAuth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Key      string
		WantKey  string
		WantAuth string
	}{{ // Test 0: v3 key uses the api_key query parameter.
		Key: "v3key", WantKey: "v3key", WantAuth: "",
	}, { // Test 1: v4 token uses the Authorization bearer header.
		Key: "aa.bb.cc", WantKey: "", WantAuth: "Bearer aa.bb.cc",
	}}
	for testNum, test := range tests {
		t.Run("auth", func(t *testing.T) {
			t.Parallel()
			var gotKey, gotAuth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotKey = r.URL.Query().Get("api_key")
				gotAuth = r.Header.Get("Authorization")
				_, _ = w.Write([]byte(`{"results":[]}`))
			}))
			t.Cleanup(srv.Close)
			c, err := New(test.Key, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if _, err := c.SearchMovie(context.Background(), "x"); err != nil {
				t.Fatalf("SearchMovie: %v", err)
			}
			if gotKey != test.WantKey || gotAuth != test.WantAuth {
				t.Errorf("test %d: key=%q auth=%q, want key=%q auth=%q",
					testNum, gotKey, gotAuth, test.WantKey, test.WantAuth)
			}
		})
	}
}
