// Command vellum is the CLI shell over the Vellum library.
//
// It is deliberately thin: it parses flags and calls the library facade, and
// contains no business logic of its own. The full verb set — compose, fill,
// inspect, validate, boxes, capabilities, schema, provenance, mcp, doctor —
// lands with the surfaces epic; until then this binary reports its version so
// the build, the version stamp and the release path are exercised from the
// first commit rather than wired up at the end.
package main

import (
	"fmt"
	"os"
)

// version is set by the build system via -X main.version.
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			fmt.Fprintln(os.Stdout, version)
			return
		}
	}
	fmt.Fprintf(os.Stderr, "vellum %s\n\nThe CLI is not yet implemented; see CLAUDE.md for the build order.\n", version)
	os.Exit(2)
}
