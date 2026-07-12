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

// runCast reports the billed cast of a movie.
func runCast(ctx context.Context, args []string) int {
	var opt options
	fs := newFlagSet("cast", &opt)
	if err := fs.Parse(args); err != nil {
		return CodeUsage
	}
	title := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if title == "" {
		fmt.Fprintln(os.Stderr, "cinatlas cast:", ErrNoSubject)
		return CodeUsage
	}
	ctx = logutil.WithLogger(ctx, logutil.New(opt.LogLevel))

	client, code := loadTMDB(newHTTPClient(opt), opt.Region)
	if code != CodeOK {
		return code
	}
	movie, code := resolveMovie(ctx, client, title)
	if code != CodeOK {
		return code
	}
	result := castResult(movie)
	if opt.JSON {
		return emit(result, opt.Pretty)
	}
	render.Cast(os.Stdout, result)
	return CodeOK
}

// castResult is the cast-focused view the cast command prints.
func castResult(m *model.Movie) model.Movie {
	return model.Movie{
		TMDBID:   m.TMDBID,
		IMDBID:   m.IMDBID,
		Title:    m.Title,
		Year:     m.Year,
		Director: m.Director,
		Cast:     m.Cast,
		IMDBURL:  m.IMDBURL,
	}
}
