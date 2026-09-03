package cli

import "github.com/frankbardon/vellum"

// newFacade constructs the library facade every verb calls.
//
// The zero-value [vellum.Options] is intentional: this story wires the CLI
// shell over the facade, not a configuration surface for its seams. A future
// story that wants --theme-dir, --asset-dir or similar flags builds them
// here, in the one place a Vellum is constructed, without touching any verb
// file.
func newFacade() (*vellum.Vellum, error) {
	return vellum.New(vellum.Options{})
}
