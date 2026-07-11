package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dcadolph/cinatlas/internal/logutil"
	"github.com/dcadolph/cinatlas/internal/render"
)

// runFilms reports what else a person was in or directed.
func runFilms(ctx context.Context, args []string) int {
	var opt options
	fs := newFlagSet("films", &opt)
	if err := fs.Parse(args); err != nil {
		return CodeUsage
	}
	name := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if name == "" {
		fmt.Fprintln(os.Stderr, "cinatlas films:", ErrNoSubject)
		return CodeUsage
	}
	ctx = logutil.WithLogger(ctx, logutil.New(opt.LogLevel))

	client, code := loadTMDB()
	if code != CodeOK {
		return code
	}
	person, code := resolvePerson(ctx, client, name)
	if code != CodeOK {
		return code
	}
	if opt.JSON {
		return emit(person, opt.Pretty)
	}
	render.Person(os.Stdout, *person)
	return CodeOK
}
