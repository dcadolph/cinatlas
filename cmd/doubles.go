package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dcadolph/cinatlas/internal/logutil"
	"github.com/dcadolph/cinatlas/internal/render"
	"github.com/dcadolph/cinatlas/internal/wikidata"
)

// defaultDoublesLimit is how many pairings the doubles command prints unless
// told otherwise.
const defaultDoublesLimit = 15

// runDoubles reports the places a named place has stood in for on screen, or
// with --played-by, the places that have stood in for it.
func runDoubles(ctx context.Context, args []string) int {
	var opt options
	fs := newFlagSet("doubles", &opt)
	limit := fs.Int("limit", defaultDoublesLimit, "max pairings to print, 0 for all")
	reverse := fs.Bool("played-by", false, "flip the question: who has stood in for this place")
	if err := fs.Parse(args); err != nil {
		return CodeUsage
	}
	place := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if place == "" {
		fmt.Fprintln(os.Stderr, "cinatlas doubles:", ErrNoSubject)
		return CodeUsage
	}
	ctx = logutil.WithLogger(ctx, logutil.New(opt.LogLevel))

	wd := wikidata.New(wikidata.WithHTTPClient(newHTTPClient(opt)))
	stands, err := wd.Doubles(ctx, place, *reverse)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cinatlas doubles:", err)
		return CodeError
	}
	if len(stands) == 0 {
		fmt.Fprintf(os.Stderr, "cinatlas: no recorded stand-ins for %q\n", place)
		return CodeNotFound
	}
	total := len(stands)
	if *limit > 0 && *limit < total {
		stands = stands[:*limit]
	}
	if opt.JSON {
		return emit(stands, opt.Pretty)
	}
	render.Doubles(os.Stdout, place, stands, *reverse)
	if len(stands) < total {
		fmt.Fprintf(os.Stderr, "cinatlas: showing %d of %d pairings, use --limit 0 for all\n",
			len(stands), total)
	}
	return CodeOK
}
