package cmd

import (
	"fmt"
	"os"
)

// Version is the build version. Override at link time with -ldflags.
var Version = "0.3.0-dev"

// runVersion prints the version to stdout.
func runVersion() int {
	fmt.Fprintln(os.Stdout, "cinatlas "+Version)
	return CodeOK
}
