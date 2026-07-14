package ddd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/dcadolph/cinatlas/internal/family"
)

// testServer serves the testdata fixtures and records the media ids requested. It
// rejects requests missing the API key header.
func testServer(t *testing.T, requested *[]int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/dddsearch":
			serveFixture(t, w, "search.json")
		case strings.HasPrefix(r.URL.Path, "/media/"):
			id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/media/")) //nolint:errcheck // Test path.
			*requested = append(*requested, id)
			serveFixture(t, w, "media.json")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// serveFixture writes one testdata file as a JSON response.
func serveFixture(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b) //nolint:errcheck // Irrelevant for this test.
}

// TestNew checks key validation.
func TestNew(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want error
		Key  string
	}{{ // Test 0: A blank key is rejected.
		Key: "  ", Want: ErrNoKey,
	}, { // Test 1: A real key constructs.
		Key: "test-key", Want: nil,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			_, err := New(test.Key)
			if !errors.Is(err, test.Want) {
				t.Errorf("got error %v, want %v", err, test.Want)
			}
		})
	}
}

// TestSearch checks result decoding and year parsing.
func TestSearch(t *testing.T) {
	t.Parallel()
	var requested []int
	srv := testServer(t, &requested)
	defer srv.Close()
	c, err := New("test-key", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	got, err := c.Search(context.Background(), "old yeller")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	want := []Media{
		{ID: 11, Name: "Old Yeller", Year: 1957},
		{ID: 12, Name: "Old Yeller 2", Year: 1963},
		{ID: 13, Name: "Old Yeller", Year: 2020},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

// TestTriggers checks topic mapping, majority voting, yes-over-no merging, and that
// unknown and unmapped topics are dropped.
func TestTriggers(t *testing.T) {
	t.Parallel()
	var requested []int
	srv := testServer(t, &requested)
	defer srv.Close()
	c, err := New("test-key", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	got, err := c.Triggers(context.Background(), 11)
	if err != nil {
		t.Fatalf("triggers: %v", err)
	}
	// media.json: "A Dog Dies" 50y/2n, "a cat dies" 0y/30n, jump scares 3y/10n,
	// spiders 0y/0n (dropped as unknown), "unmapped topic" (dropped, no key).
	want := map[string]family.Trigger{
		"dog-death":   family.TriggerYes,
		"cat-death":   family.TriggerNo,
		"jump-scares": family.TriggerNo,
	}
	if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

// TestTriggersFor checks best-match selection by title and year.
func TestTriggersFor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Title  string
		WantID int
		Year   int
	}{{ // Test 0: Exact title and year wins.
		Title: "Old Yeller", Year: 1957, WantID: 11,
	}, { // Test 1: Exact title and year wins over an earlier title-only match.
		Title: "Old Yeller", Year: 2020, WantID: 13,
	}, { // Test 2: Title matches case-insensitively when no year matches.
		Title: "old yeller 2", Year: 1999, WantID: 12,
	}, { // Test 3: No title match falls back to the first result.
		Title: "Turner and Hooch", Year: 1989, WantID: 11,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var requested []int
			srv := testServer(t, &requested)
			defer srv.Close()
			c, err := New("test-key", WithBaseURL(srv.URL))
			if err != nil {
				t.Fatalf("new: %v", err)
			}
			if _, err := c.TriggersFor(context.Background(), test.Title, test.Year); err != nil {
				t.Fatalf("triggers for: %v", err)
			}
			if diff := cmp.Diff([]int{test.WantID}, requested); diff != "" {
				t.Errorf("requested ids mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestTriggersForNoResults checks that an empty search yields an empty map with no
// media lookup.
func TestTriggersForNoResults(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[]}`)) //nolint:errcheck // Irrelevant for this test.
	}))
	defer srv.Close()
	c, err := New("test-key", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	got, err := c.TriggersFor(context.Background(), "Nothing", 2000)
	if err != nil {
		t.Fatalf("triggers for: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty map", got)
	}
}

// TestGetErrors checks the status and decode failure paths.
func TestGetErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Handler http.HandlerFunc
		Want    error
	}{{ // Test 0: A non-OK status is a status error.
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		},
		Want: ErrStatus,
	}, { // Test 1: A malformed body is a decode error.
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not json")) //nolint:errcheck // Irrelevant for this test.
		},
		Want: ErrDecodeBody,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(test.Handler)
			defer srv.Close()
			c, err := New("test-key", WithBaseURL(srv.URL))
			if err != nil {
				t.Fatalf("new: %v", err)
			}
			if _, err := c.Search(context.Background(), "x"); !errors.Is(err, test.Want) {
				t.Errorf("got error %v, want %v", err, test.Want)
			}
		})
	}
}
