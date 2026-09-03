package splice

import (
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/xmlcopy"
)

// Splice renders seq into a's part and returns the single [xmlcopy.Replacement]
// needed to bind it, dispatching on a.Kind to [spliceNative] or [spliceMarker]
// — see the package doc for why the two are genuinely different shapes.
//
// pkg is read from (a.Part's own bytes) and, for an Asset block only, also
// mutated in place: a new media part and its relationship are added directly,
// because "add a whole new part" has no honest representation as a byte-span
// replacement. Every other change Splice computes is returned, not applied —
// applying the Replacement to a's part (xmlcopy.Apply) and writing the result
// back with pkg.Put, across every anchor a binding fills, is E10's
// orchestration, not this call's job.
func Splice(pkg *opc.Package, a anchor.Anchor, seq fragment.Sequence) (xmlcopy.Replacement, error) {
	if pkg == nil {
		return xmlcopy.Replacement{}, verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT, "nil package")
	}
	part, ok := pkg.Get(a.Part)
	if !ok {
		return xmlcopy.Replacement{}, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"splice was given an anchor whose part the package does not contain",
			map[string]any{"anchor": a.Name, "part": a.Part})
	}
	src, err := part.Bytes()
	if err != nil {
		return xmlcopy.Replacement{}, err
	}

	switch a.Kind {
	case anchor.KindNative:
		return spliceNative(pkg, a, src, seq)
	case anchor.KindMarker:
		return spliceMarker(a, src, seq)
	default:
		return xmlcopy.Replacement{}, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"the anchor carries a kind splice does not recognise",
			map[string]any{"anchor": a.Name, "kind": string(a.Kind)})
	}
}
