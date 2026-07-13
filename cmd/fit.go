package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/dcadolph/cinatlas/internal/family"
	"github.com/dcadolph/cinatlas/internal/fitfind"
	"github.com/dcadolph/cinatlas/internal/logutil"
	"github.com/dcadolph/cinatlas/internal/render"
)

// runFit finds popular films that pass every member of a family profile. The
// profile comes from a web share link's payload, or a single ceiling shortcut.
func runFit(ctx context.Context, args []string) int {
	var opt options
	fs := newFlagSet("fit", &opt)
	profileFlag := fs.String("profile", "", "encoded profile payload from a fit share link")
	ceiling := fs.String("ceiling", "", "single rating ceiling shortcut, e.g. PG")
	if err := fs.Parse(args); err != nil {
		return CodeUsage
	}
	profile, code := fitProfile(*profileFlag, *ceiling)
	if code != CodeOK {
		return code
	}
	log := logutil.New(opt.LogLevel)
	ctx = logutil.WithLogger(ctx, log)

	httpClient := newHTTPClient(opt)
	client, code := loadTMDB(httpClient, opt.Region)
	if code != CodeOK {
		return code
	}
	triggers := loadDDTD(httpClient, log)
	finder := fitfind.New(client, triggers, log)
	results, excluded, err := finder.Find(ctx, profile, splitServices(opt.Services))
	if err != nil {
		fmt.Fprintln(os.Stderr, "cinatlas fit:", err)
		return CodeError
	}
	if opt.JSON {
		return emit(struct {
			Results  []fitfind.Result `json:"results"`
			Excluded int              `json:"excluded"`
		}{results, excluded}, opt.Pretty)
	}
	render.Fit(os.Stdout, results, excluded)
	return CodeOK
}

// fitProfile builds the family profile from the profile payload or the ceiling
// shortcut, exactly one of which is required.
func fitProfile(payload, ceiling string) (family.Profile, int) {
	switch {
	case payload != "" && ceiling != "":
		fmt.Fprintln(os.Stderr, "cinatlas fit: use --profile or --ceiling, not both")
		return family.Profile{}, CodeUsage
	case payload != "":
		profile, err := family.DecodeProfile(payload)
		if err != nil {
			fmt.Fprintln(os.Stderr, "cinatlas fit:", err)
			return family.Profile{}, CodeUsage
		}
		return profile, CodeOK
	case ceiling != "":
		return family.Profile{Members: []family.Member{{Name: "Everyone", Ceiling: ceiling}}}, CodeOK
	default:
		fmt.Fprintln(os.Stderr, "cinatlas fit: pass --profile <payload> or --ceiling <rating>")
		return family.Profile{}, CodeUsage
	}
}
