package sin

import "errors"

// ErrNoAnchors reports that no query term resolved to a TMDB keyword, leaving
// discovery with nothing to select on.
var ErrNoAnchors = errors.New("sin: no anchor keywords resolved")
