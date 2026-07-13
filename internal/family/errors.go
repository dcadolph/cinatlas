package family

import "errors"

var (
	// ErrEncode reports a profile that could not be serialized for a share link.
	ErrEncode = errors.New("profile encode failed")
	// ErrDecode reports a share-link payload that could not be parsed.
	ErrDecode = errors.New("profile decode failed")
	// ErrVersion reports an unsupported profile schema version.
	ErrVersion = errors.New("unsupported profile version")
	// ErrNoMembers reports a profile without members.
	ErrNoMembers = errors.New("profile has no members")
)
