// Command vellum is the CLI shell over the Vellum library.
//
// It is deliberately thin: every flag definition, every facade call and
// every --json/human-output decision lives in internal/cli, one file per
// verb group. This file only builds that command tree, runs it, and turns
// its own returned error into the process's exit code — see
// internal/cli.CodeOf for the exact convention.
package main

import (
	"context"
	"os"

	"github.com/frankbardon/vellum/internal/cli"
)

// version is set by the build system via -X main.version.
var version = "dev"

func main() {
	app := cli.New(version)
	err := app.Run(context.Background(), os.Args)
	os.Exit(cli.CodeOf(err))
}
