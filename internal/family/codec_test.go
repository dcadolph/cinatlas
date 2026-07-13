package family

import (
	"encoding/base64"
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// TestEncodeDecodeRoundTrip checks that a profile survives the share-link codec and
// that a zero version is stamped with the current schema version.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()
	in := Profile{
		Name: "weeknight crew",
		Members: []Member{
			{Name: "Sam", Ceiling: "PG", HardVetoes: []string{"animal-death"}},
			{Name: "Alex", SoftVetoes: []string{"Musical"}},
		},
	}
	encoded, err := EncodeProfile(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := DecodeProfile(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	in.Version = SchemaVersion
	if diff := cmp.Diff(in, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

// TestEncodeProfile checks encode-side validation.
func TestEncodeProfile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want    error
		Profile Profile
	}{{ // Test 0: A profile without members is rejected.
		Profile: Profile{Name: "empty"},
		Want:    ErrNoMembers,
	}, { // Test 1: A minimal valid profile encodes.
		Profile: Profile{Members: []Member{{Name: "Sam"}}},
		Want:    nil,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			_, err := EncodeProfile(test.Profile)
			if !errors.Is(err, test.Want) {
				t.Errorf("got error %v, want %v", err, test.Want)
			}
		})
	}
}

// TestDecodeProfile checks each decode failure mode.
func TestDecodeProfile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want error
		In   string
	}{{ // Test 0: Invalid base64 is a decode error.
		In:   "!!!not-base64!!!",
		Want: ErrDecode,
	}, { // Test 1: Valid base64 holding invalid JSON is a decode error.
		In:   base64.RawURLEncoding.EncodeToString([]byte("not json")),
		Want: ErrDecode,
	}, { // Test 2: An unsupported schema version is rejected.
		In:   base64.RawURLEncoding.EncodeToString([]byte(`{"v":99,"members":[{"name":"Sam"}]}`)),
		Want: ErrVersion,
	}, { // Test 3: A payload without members is rejected.
		In:   base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"members":[]}`)),
		Want: ErrNoMembers,
	}, { // Test 4: A valid payload decodes.
		In:   base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"members":[{"name":"Sam"}]}`)),
		Want: nil,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			_, err := DecodeProfile(test.In)
			if !errors.Is(err, test.Want) {
				t.Errorf("got error %v, want %v", err, test.Want)
			}
		})
	}
}
