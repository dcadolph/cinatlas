package imdb

import (
	"fmt"
	"testing"
)

// TestTitleURL checks title link building across valid and invalid ids.
func TestTitleURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         string
		WantResult string
	}{{ // Test 0: Standard title id.
		In: "tt0083658", WantResult: "https://www.imdb.com/title/tt0083658/",
	}, { // Test 1: Surrounding whitespace is trimmed.
		In: "  tt0111161 ", WantResult: "https://www.imdb.com/title/tt0111161/",
	}, { // Test 2: Empty id yields no link.
		In: "", WantResult: "",
	}, { // Test 3: Name id is not a title id.
		In: "nm0000123", WantResult: "",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := TitleURL(test.In); got != test.WantResult {
				t.Errorf("TitleURL(%q) = %q, want %q", test.In, got, test.WantResult)
			}
		})
	}
}

// TestLocationsURL checks locations link building across valid and invalid ids.
func TestLocationsURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         string
		WantResult string
	}{{ // Test 0: Standard title id.
		In: "tt0113277", WantResult: "https://www.imdb.com/title/tt0113277/locations/",
	}, { // Test 1: Empty id yields no link.
		In: "", WantResult: "",
	}, { // Test 2: Name id yields no link.
		In: "nm0000123", WantResult: "",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := LocationsURL(test.In); got != test.WantResult {
				t.Errorf("LocationsURL(%q) = %q, want %q", test.In, got, test.WantResult)
			}
		})
	}
}

// TestNameURL checks name link building across valid and invalid ids.
func TestNameURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         string
		WantResult string
	}{{ // Test 0: Standard name id.
		In: "nm0000123", WantResult: "https://www.imdb.com/name/nm0000123/",
	}, { // Test 1: Surrounding whitespace is trimmed.
		In: " nm0634240  ", WantResult: "https://www.imdb.com/name/nm0634240/",
	}, { // Test 2: Empty id yields no link.
		In: "", WantResult: "",
	}, { // Test 3: Title id is not a name id.
		In: "tt0083658", WantResult: "",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := NameURL(test.In); got != test.WantResult {
				t.Errorf("NameURL(%q) = %q, want %q", test.In, got, test.WantResult)
			}
		})
	}
}
