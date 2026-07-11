// Package cmd holds command dispatch, flags, and output for the cinatlas CLI.
package cmd

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dcadolph/cinatlas/internal/jsonutil"
	"github.com/dcadolph/cinatlas/internal/tmdb"
)

// usageText is the top-level help shown for help and misuse.
const usageText = `cinatlas - quick movie facts

Usage:
  cinatlas <command> [flags] <subject>

Commands:
  where    Where a movie was filmed
  cast     Who is in a movie
  films    What else a person was in or directed
  who      Identify a person and their notable roles
  version  Print the build version
  help     Show this help

Flags:
  --json              Output JSON instead of human-readable text
  --pretty            Indent JSON output (with --json)
  --log-level string  Stderr log level: debug, info, warn, error (default "info")

Environment:
  CINATLAS_TMDB_KEY   TMDB API key, required for data commands
  CINATLAS_JSON       Set to 1 or true to output JSON by default
  CINATLAS_PRETTY     Set to 1 or true to indent JSON output
  CINATLAS_LOG_LEVEL  Default stderr log level
`

// options holds flags common to every command.
type options struct {
	// JSON emits machine JSON instead of human-readable text when true.
	JSON bool
	// Pretty indents JSON output when true.
	Pretty bool
	// LogLevel sets stderr verbosity.
	LogLevel string
}

// newFlagSet returns a flag set preloaded with the common flags and env defaults.
func newFlagSet(name string, opt *options) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.BoolVar(&opt.JSON, "json", envBool("CINATLAS_JSON"), "output JSON instead of text")
	fs.BoolVar(&opt.Pretty, "pretty", envBool("CINATLAS_PRETTY"), "indent JSON output")
	fs.StringVar(&opt.LogLevel, "log-level", envOr("CINATLAS_LOG_LEVEL", "info"), "stderr log level")
	return fs
}

// emit writes v to stdout as JSON and returns the success code.
func emit(v any, pretty bool) int {
	b, err := jsonutil.Marshal(v, pretty)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cinatlas: encode output:", err)
		return CodeError
	}
	fmt.Fprintln(os.Stdout, string(b))
	return CodeOK
}

// loadTMDB builds a TMDB client from the environment key. On misconfiguration it
// names the variable to set and returns a config code.
func loadTMDB() (*tmdb.HTTPClient, int) {
	client, err := tmdb.New(os.Getenv("CINATLAS_TMDB_KEY"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "cinatlas: set CINATLAS_TMDB_KEY to a TMDB API key from themoviedb.org")
		return nil, CodeConfig
	}
	return client, CodeOK
}

// envOr returns the environment value for key, or def when unset.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envBool reports whether the environment value for key parses as true.
func envBool(key string) bool {
	b, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv(key)))
	return b
}
