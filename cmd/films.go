package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dcadolph/cinatlas/internal/logutil"
	"github.com/dcadolph/cinatlas/internal/model"
	"github.com/dcadolph/cinatlas/internal/render"
)

// defaultFilmsLimit is how many credits the films command prints unless told otherwise.
const defaultFilmsLimit = 30

// runFilms reports what else a person was in or directed, most famous first.
func runFilms(ctx context.Context, args []string) int {
	var opt options
	fs := newFlagSet("films", &opt)
	limit := fs.Int("limit", defaultFilmsLimit, "max credits to print, 0 for all")
	sortOrder := fs.String("sort", "", "order: fame (default), az, new, old")
	decade := fs.Int("decade", 0, "keep one release decade, such as 1990")
	if err := fs.Parse(args); err != nil {
		return CodeUsage
	}
	name := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if name == "" {
		fmt.Fprintln(os.Stderr, "cinatlas films:", ErrNoSubject)
		return CodeUsage
	}
	ctx = logutil.WithLogger(ctx, logutil.New(opt.LogLevel))

	client, code := loadTMDB(newHTTPClient(opt), opt.Region)
	if code != CodeOK {
		return code
	}
	person, code := resolvePerson(ctx, client, name)
	if code != CodeOK {
		return code
	}
	person.Credits = model.FilterCreditsByDecade(person.Credits, *decade)
	model.SortCredits(person.Credits, *sortOrder)
	total := len(person.Credits)
	if *limit > 0 && total > *limit {
		person.Credits = person.Credits[:*limit]
	}
	if opt.JSON {
		return emit(person, opt.Pretty)
	}
	render.Person(os.Stdout, *person)
	if len(person.Credits) < total {
		fmt.Fprintf(os.Stderr, "cinatlas: showing %d of %d credits, use --limit 0 for all\n",
			len(person.Credits), total)
	}
	return CodeOK
}
