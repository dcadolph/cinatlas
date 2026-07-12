package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dcadolph/cinatlas/internal/logutil"
	"github.com/dcadolph/cinatlas/internal/render"
)

// defaultAtLimit is how many films the at command prints unless told otherwise.
const defaultAtLimit = 25

// runAt reports movies filmed at a named place, most famous first.
func runAt(ctx context.Context, args []string) int {
	var opt options
	fs := newFlagSet("at", &opt)
	limit := fs.Int("limit", defaultAtLimit, "max films to print, 0 for all")
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
	want := *limit
	if want <= 0 {
		want = int(^uint(0) >> 1)
	}
	movies, total, err := newLocator(httpClient, client).At(ctx, place, 0, want)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cinatlas at:", err)
		return CodeError
	}
	if total == 0 {
		fmt.Fprintf(os.Stderr, "cinatlas: no films with recorded locations at %q\n", place)
		return CodeNotFound
	}
	if opt.JSON {
		return emit(movies, opt.Pretty)
	}
	render.FilmsAt(os.Stdout, place, movies)
	if len(movies) < total {
		fmt.Fprintf(os.Stderr, "cinatlas: showing %d of %d films, use --limit 0 for all\n",
			len(movies), total)
	}
	return CodeOK
}
