package anchor

import (
	"github.com/frankbardon/vellum/artifact"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
)

// Discover finds every anchor in a template's main part.
//
// format and mainPart come from [template.Template]: format selects the
// per-format discoverer, and mainPart is the part that discoverer walks —
// passed explicitly rather than reaching back into template so that this
// package does not need to import it, which is what keeps
// template.Inspect -> anchor.Discover from being an import cycle.
//
// A format with no discoverer wired — XLSX and PPTX today, whose anchor
// kinds are E11's job — fails loudly with
// [verr.VELLUM_TEMPLATE_FORMAT_UNSUPPORTED] rather than returning an empty
// inventory. An empty inventory is indistinguishable from "this template
// genuinely has no anchors", which is a different fact from "Vellum cannot
// inspect this format yet", and conflating them would tell a caller their
// template has nothing to fill when the truth is that nobody has looked.
func Discover(pkg *opc.Package, format artifact.Format, mainPart string) (*Inventory, error) {
	switch format {
	case artifact.FormatDOCX:
		return discoverDOCX(pkg, mainPart)
	default:
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_TEMPLATE_FORMAT_UNSUPPORTED,
			"anchor discovery is not implemented for this template format",
			map[string]any{"format": string(format)})
	}
}
