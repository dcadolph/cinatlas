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

// runWatch reports where a movie is streaming right now in the chosen region,
// tagging the services the viewer already subscribes to.
func runWatch(ctx context.Context, args []string) int {
	var opt options
	fs := newFlagSet("watch", &opt)
	if err := fs.Parse(args); err != nil {
		return CodeUsage
	}
	title := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if title == "" {
		fmt.Fprintln(os.Stderr, "cinatlas watch:", ErrNoSubject)
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
	if services := splitServices(opt.Services); len(services) > 0 {
		model.TagOwnership(movie.Availability, services...)
	}
	result := watchResult(movie)
	if opt.JSON {
		return emit(result, opt.Pretty)
	}
	render.Watch(os.Stdout, result)
	return CodeOK
}

// watchResult is the availability-focused view the watch command prints.
func watchResult(m *model.Movie) model.Movie {
	return model.Movie{
		TMDBID:       m.TMDBID,
		IMDBID:       m.IMDBID,
		Title:        m.Title,
		Year:         m.Year,
		Availability: m.Availability,
		WatchRegion:  m.WatchRegion,
		WatchURL:     m.WatchURL,
		IMDBURL:      m.IMDBURL,
	}
}

// splitServices parses a comma-separated services list, dropping blanks.
func splitServices(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
