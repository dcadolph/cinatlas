package family

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// SchemaVersion is the current profile share-link schema version.
const SchemaVersion = 1

// EncodeProfile serializes a profile into a base64url payload for share links. A zero
// Version is stamped with the current schema version.
func EncodeProfile(p Profile) (string, error) {
	if len(p.Members) == 0 {
		return "", ErrNoMembers
	}
	if p.Version == 0 {
		p.Version = SchemaVersion
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrEncode, err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// DecodeProfile parses a share-link payload produced by EncodeProfile.
func DecodeProfile(s string) (Profile, error) {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return Profile{}, fmt.Errorf("%w: %w", ErrDecode, err)
	}
	var p Profile
	if err := json.Unmarshal(b, &p); err != nil {
		return Profile{}, fmt.Errorf("%w: %w", ErrDecode, err)
	}
	if p.Version != SchemaVersion {
		return Profile{}, fmt.Errorf("%w: %d", ErrVersion, p.Version)
	}
	if len(p.Members) == 0 {
		return Profile{}, ErrNoMembers
	}
	return p, nil
}
