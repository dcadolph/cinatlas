package cmd

import "errors"

// ErrNoSubject reports that a data command was called without a subject.
var ErrNoSubject = errors.New("no subject given")
