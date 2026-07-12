package model

import (
	"fmt"
	"reflect"
	"testing"
)

// TestTagOwnership checks case-insensitive substring matching, the tagged
// count, and that non-matching entries stay untagged.
func TestTagOwnership(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Services  []string
		In        []Availability
		WantOwned []bool
		WantCount int
	}{{ // Test 0: A token matches a provider as a substring, case-insensitively.
		Services: []string{"PRIME", "hulu"},
		In: []Availability{
			{Provider: "Amazon Prime Video"},
			{Provider: "Hulu"},
			{Provider: "Netflix"},
		},
		WantOwned: []bool{true, true, false},
		WantCount: 2,
	}, { // Test 1: Blank tokens are ignored and match nothing.
		Services:  []string{"", "   "},
		In:        []Availability{{Provider: "Max"}},
		WantOwned: []bool{false},
		WantCount: 0,
	}, { // Test 2: No services tags nothing.
		Services:  nil,
		In:        []Availability{{Provider: "Max"}},
		WantOwned: []bool{false},
		WantCount: 0,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			gotCount := TagOwnership(test.In, test.Services...)
			if gotCount != test.WantCount {
				t.Errorf("count = %d, want %d", gotCount, test.WantCount)
			}
			for i, want := range test.WantOwned {
				if test.In[i].Owned != want {
					t.Errorf("entry %d owned = %v, want %v", i, test.In[i].Owned, want)
				}
			}
		})
	}
}

// TestSortAvailability checks kind ordering then provider name, best access
// for the viewer first.
func TestSortAvailability(t *testing.T) {
	t.Parallel()
	in := []Availability{
		{Provider: "Apple TV", Kinds: []string{AccessBuy}},
		{Provider: "Netflix", Kinds: []string{AccessStream}},
		{Provider: "Tubi", Kinds: []string{AccessFree}},
		{Provider: "Amazon Video", Kinds: []string{AccessRent, AccessBuy}},
		{Provider: "Max", Kinds: []string{AccessStream}},
	}
	SortAvailability(in)
	want := []Availability{
		{Provider: "Max", Kinds: []string{AccessStream}},
		{Provider: "Netflix", Kinds: []string{AccessStream}},
		{Provider: "Tubi", Kinds: []string{AccessFree}},
		{Provider: "Amazon Video", Kinds: []string{AccessRent, AccessBuy}},
		{Provider: "Apple TV", Kinds: []string{AccessBuy}},
	}
	if !reflect.DeepEqual(want, in) {
		t.Errorf("SortAvailability\n got %+v\nwant %+v", in, want)
	}
}
