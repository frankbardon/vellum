// Command validatorpin prints the veraPDF reference the conformance gate runs.
//
// It exists so the digest is stated once, in Go, and the CI workflow asks for
// it rather than repeating it. A digest written in two places is one that will
// eventually disagree with itself, and the disagreement would show up as a
// conformance job silently running a validator nobody pinned.
//
// TestNoUnpinnedValidatorImage checks the workflow goes through this rather
// than naming a reference of its own.
package main

import (
	"fmt"

	"github.com/frankbardon/vellum/internal/pdfvalidate"
)

func main() {
	fmt.Println(pdfvalidate.ContainerPrefix + pdfvalidate.PinnedImage)
}
