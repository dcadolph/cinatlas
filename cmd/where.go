package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dcadolph/cinatlas/internal/logutil"
	"github.com/dcadolph/cinatlas/internal/model"
	"github.com/dcadolph/cinatlas/internal/render"
	"github.com/dcadolph/cinatlas/internal/wikidata"
)

// runWhere reports where a movie was filmed.
func runWhere(ctx context.Context, args []string) int {
	var opt options
	fs := newFlagSet("where", &opt)
	if err := fs.Parse(args); err != nil {
		return CodeUsage
	}
	title := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if title == "" {
		fmt.Fprintln(os.Stderr, "cinatlas where:", ErrNoSubject)
		return CodeUsage
	}
	log := logutil.New(opt.LogLevel)
	ctx = logutil.WithLogger(ctx, log)

	httpClient := newHTTPClient(opt)
	client, code := loadTMDB(httpClient)
	if code != CodeOK {
		return code
	}
	movie, code := resolveMovie(ctx, client, title)
	if code != CodeOK {
		return code
	}
	if movie.IMDBID == "" {
		log.Warn("no imdb id for title, cannot resolve locations", "title", movie.Title)
	} else {
		finder := wikidata.New(wikidata.WithHTTPClient(httpClient))
		if locs, err := finder.Locations(ctx, movie.IMDBID); err != nil {
			log.Error("location lookup failed", "err", err)
		} else {
			movie.Locations = locs
		}
	}
	result := whereResult(movie)
	if opt.JSON {
		return emit(result, opt.Pretty)
	}
	render.Where(os.Stdout, result)
	return CodeOK
}

// whereResult is the location-focused view the where command prints.
func whereResult(m *model.Movie) model.Movie {
	return model.Movie{
		TMDBID:    m.TMDBID,
		IMDBID:    m.IMDBID,
		Title:     m.Title,
		Year:      m.Year,
		Locations: m.Locations,
		IMDBURL:   m.IMDBURL,
	}
}
