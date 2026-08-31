package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/dcadolph/cinatlas/internal/enhance"
	"github.com/dcadolph/cinatlas/internal/logutil"
	"github.com/dcadolph/cinatlas/internal/web"
)

// runServe starts the cinatlas website and opens it in the browser when
// running locally. A PORT environment variable, the convention on hosting
// platforms, switches the default to all interfaces and skips the browser.
func runServe(ctx context.Context, args []string) int {
	var opt options
	fs := newFlagSet("serve", &opt)
	defaultAddr := "127.0.0.1:8878"
	hosted := os.Getenv("PORT") != ""
	if hosted {
		defaultAddr = ":" + os.Getenv("PORT")
	}
	addr := fs.String("addr", defaultAddr, "listen address")
	if err := fs.Parse(args); err != nil {
		return CodeUsage
	}
	log := logutil.New(opt.LogLevel)
	ctx = logutil.WithLogger(ctx, log)

	httpClient := newHTTPClient(opt)
	client, code := loadTMDB(httpClient, opt.Region)
	if code != CodeOK {
		return code
	}
	server, err := web.New(client, newLocator(httpClient, client), loadDDTD(httpClient, log), log)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cinatlas serve:", err)
		return CodeError
	}
	// The mood enhancer bills whoever holds the key, and serve has no per-visitor
	// key, so on a public deployment that is the operator on every search. Both
	// values are required rather than defaulted: naming a model here would pick a
	// price tier on the key holder's behalf, and a model id in source goes stale
	// on the vendor's release schedule. A key with no model is a misconfiguration
	// worth saying out loud, since the lexicon answers either way.
	key, model := os.Getenv("CINATLAS_ANTHROPIC_KEY"), os.Getenv("CINATLAS_ENHANCER_MODEL")
	switch {
	case key == "":
		log.Info("CINATLAS_ANTHROPIC_KEY not set: mood queries answered by the lexicon alone")
	case model == "":
		log.Warn("CINATLAS_ANTHROPIC_KEY set without CINATLAS_ENHANCER_MODEL: " +
			"mood enhancer disabled, set the model you want billed for")
	default:
		enhancer := enhance.New(key, model, log)
		server.SetEnhancer(enhancer)
		server.SetSinEnhancer(enhancer.Sin())
		log.Warn("mood enhancer enabled: every mood search on this instance bills "+
			"the configured key, with no per-visitor key and no spend cap",
			"model", model)
	}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cinatlas serve:", err)
		return CodeError
	}
	siteURL := "http://" + listener.Addr().String()
	log.Info("cinatlas serving", "url", siteURL)
	if !hosted {
		openBrowser(ctx, siteURL)
	}
	if err := http.Serve(listener, server.Routes()); err != nil {
		fmt.Fprintln(os.Stderr, "cinatlas serve:", err)
		return CodeError
	}
	return CodeOK
}

// openBrowser opens the URL in the platform browser, best effort.
func openBrowser(ctx context.Context, url string) {
	log := logutil.FromContext(ctx)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Debug("browser open failed", "err", err)
	}
}
