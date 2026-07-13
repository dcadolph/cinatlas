package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dcadolph/cinatlas/internal/fitfind"
	"github.com/dcadolph/cinatlas/internal/model"
)

// TestFit checks the fit listing output, the ownership mark, unverified notes, and
// the empty case.
func TestFit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantContains []string
		Results      []fitfind.Result
		Excluded     int
	}{{ // Test 0: A full listing shows count, cert, ownership, and unverified notes.
		Results: []fitfind.Result{{
			Movie:            model.Movie{Title: "Paddington", Year: 2014, Certification: "PG"},
			SubscriptionHits: 1,
			Unverified:       []string{"No data: jump-scares"},
		}},
		Excluded: 3,
		WantContains: []string{
			"1 films fit everyone (3 excluded):",
			"Paddington (2014) [PG] ✓ on your services",
			"Unverified: No data: jump-scares",
		},
	}, { // Test 1: No results prints the empty message.
		Results:      nil,
		Excluded:     5,
		WantContains: []string{"Nothing passed every constraint."},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var buf strings.Builder
			Fit(&buf, test.Results, test.Excluded)
			for _, want := range test.WantContains {
				if !strings.Contains(buf.String(), want) {
					t.Errorf("output missing %q:\n%s", want, buf.String())
				}
			}
		})
	}
}
