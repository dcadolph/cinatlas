package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/dcadolph/cinatlas/internal/logutil"
	"github.com/dcadolph/cinatlas/internal/web"
	"github.com/dcadolph/cinatlas/internal/wikidata"
)

// runServe starts the cinatlas website locally and opens it in the browser.
func runServe(ctx context.Context, args []string) int {
	var opt options
	fs := newFlagSet("serve", &opt)
	addr := fs.String("addr", "127.0.0.1:8878", "listen address")
	if err := fs.Parse(args); err != nil {
		return CodeUsage
	}
	log := logutil.New(opt.LogLevel)
	ctx = logutil.WithLogger(ctx, log)

	httpClient := newHTTPClient(opt)
	client, code := loadTMDB(httpClient)
	if code != CodeOK {
		return code
	}
	finder := wikidata.New(wikidata.WithHTTPClient(httpClient))
	server, err := web.New(client, finder, log)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cinatlas serve:", err)
		return CodeError
	}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cinatlas serve:", err)
		return CodeError
	}
	siteURL := "http://" + listener.Addr().String()
	log.Info("cinatlas serving", "url", siteURL)
	openBrowser(ctx, siteURL)
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
