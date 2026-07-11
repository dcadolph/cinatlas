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

// topCredits caps how many filmography entries the who command shows.
const topCredits = 8

// runWho identifies a person and lists their most recent notable roles.
func runWho(ctx context.Context, args []string) int {
	var opt options
	fs := newFlagSet("who", &opt)
	if err := fs.Parse(args); err != nil {
		return CodeUsage
	}
	name := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if name == "" {
		fmt.Fprintln(os.Stderr, "cinatlas who:", ErrNoSubject)
		return CodeUsage
	}
	ctx = logutil.WithLogger(ctx, logutil.New(opt.LogLevel))

	client, code := loadTMDB(newHTTPClient(opt))
	if code != CodeOK {
		return code
	}
	person, code := resolvePerson(ctx, client, name)
	if code != CodeOK {
		return code
	}
	result := whoResult(person)
	if opt.JSON {
		return emit(result, opt.Pretty)
	}
	render.Person(os.Stdout, result)
	return CodeOK
}

// whoResult is the identity-focused view the who command prints, capped to the
// most recent notable credits.
func whoResult(p *model.Person) model.Person {
	credits := p.Credits
	if len(credits) > topCredits {
		credits = credits[:topCredits]
	}
	return model.Person{
		TMDBID:   p.TMDBID,
		IMDBID:   p.IMDBID,
		Name:     p.Name,
		KnownFor: p.KnownFor,
		Credits:  credits,
		IMDBURL:  p.IMDBURL,
	}
}
