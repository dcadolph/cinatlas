package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dcadolph/cinatlas/internal/logutil"
	"github.com/dcadolph/cinatlas/internal/render"
)

// maxAtResults caps the reverse place search list.
const maxAtResults = 25

// runAt reports movies filmed at a named place.
func runAt(ctx context.Context, args []string) int {
	var opt options
	fs := newFlagSet("at", &opt)
	if err := fs.Parse(args); err != nil {
		return CodeUsage
	}
	place := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if place == "" {
		fmt.Fprintln(os.Stderr, "cinatlas at:", ErrNoSubject)
		return CodeUsage
	}
	ctx = logutil.WithLogger(ctx, logutil.New(opt.LogLevel))

	httpClient := newHTTPClient(opt)
	client, code := loadTMDB(httpClient)
	if code != CodeOK {
		return code
	}
	movies, err := newLocator(httpClient, client).At(ctx, place, maxAtResults)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cinatlas at:", err)
		return CodeError
	}
	if len(movies) == 0 {
		fmt.Fprintf(os.Stderr, "cinatlas: no films with recorded locations at %q\n", place)
		return CodeNotFound
	}
	if opt.JSON {
		return emit(movies, opt.Pretty)
	}
	render.FilmsAt(os.Stdout, place, movies)
	return CodeOK
}
