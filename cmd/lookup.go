package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/dcadolph/cinatlas/internal/model"
	"github.com/dcadolph/cinatlas/internal/tmdb"
)

// resolveMovie searches for a title, then fetches full details for the top match.
// It returns the movie and CodeOK, or a nil movie and a non-OK code.
func resolveMovie(ctx context.Context, client *tmdb.HTTPClient, title string) (*model.Movie, int) {
	results, err := client.SearchMovie(ctx, title)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cinatlas: search failed:", err)
		return nil, CodeError
	}
	if len(results) == 0 {
		fmt.Fprintf(os.Stderr, "cinatlas: no movie found for %q\n", title)
		return nil, CodeNotFound
	}
	movie, err := client.Movie(ctx, results[0].TMDBID)
	if err != nil {
		if errors.Is(err, tmdb.ErrNotFound) {
			return nil, CodeNotFound
		}
		fmt.Fprintln(os.Stderr, "cinatlas: fetch failed:", err)
		return nil, CodeError
	}
	return movie, CodeOK
}

// resolvePerson searches for a person, then fetches full details for the top match.
// It returns the person and CodeOK, or a nil person and a non-OK code.
func resolvePerson(ctx context.Context, client *tmdb.HTTPClient, name string) (*model.Person, int) {
	results, err := client.SearchPerson(ctx, name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cinatlas: search failed:", err)
		return nil, CodeError
	}
	if len(results) == 0 {
		fmt.Fprintf(os.Stderr, "cinatlas: no person found for %q\n", name)
		return nil, CodeNotFound
	}
	person, err := client.Person(ctx, results[0].TMDBID)
	if err != nil {
		if errors.Is(err, tmdb.ErrNotFound) {
			return nil, CodeNotFound
		}
		fmt.Fprintln(os.Stderr, "cinatlas: fetch failed:", err)
		return nil, CodeError
	}
	return person, CodeOK
}
