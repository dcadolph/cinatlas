package ddd

import "errors"

var (
	// ErrNoKey reports a missing API key.
	ErrNoKey = errors.New("doesthedogdie api key required")
	// ErrRequest reports a request that could not be built or sent.
	ErrRequest = errors.New("doesthedogdie request failed")
	// ErrStatus reports a non-OK response status.
	ErrStatus = errors.New("doesthedogdie bad status")
	// ErrDecodeBody reports a response body that could not be parsed.
	ErrDecodeBody = errors.New("doesthedogdie decode failed")
)
