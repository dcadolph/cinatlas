// Command cinatlas answers quick movie facts from the command line.
package main

import (
	"context"
	"os"

	"github.com/dcadolph/cinatlas/cmd"
)

// main runs the root command and exits with its status code.
func main() {
	os.Exit(cmd.Execute(context.Background(), os.Args[1:]))
}
