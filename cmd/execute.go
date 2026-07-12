package cmd

import (
	"context"
	"fmt"
	"os"
)

// Execute dispatches the first argument to a command and returns an exit code.
func Execute(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usageText)
		return CodeUsage
	}
	command, rest := args[0], args[1:]
	switch command {
	case "where":
		return runWhere(ctx, rest)
	case "at":
		return runAt(ctx, rest)
	case "cast":
		return runCast(ctx, rest)
	case "watch":
		return runWatch(ctx, rest)
	case "films":
		return runFilms(ctx, rest)
	case "who":
		return runWho(ctx, rest)
	case "serve":
		return runServe(ctx, rest)
	case "version":
		return runVersion()
	case "help", "-h", "--help":
		fmt.Fprint(os.Stdout, usageText)
		return CodeOK
	default:
		fmt.Fprintf(os.Stderr, "cinatlas: unknown command %q\n\n", command)
		fmt.Fprint(os.Stderr, usageText)
		return CodeUsage
	}
}
