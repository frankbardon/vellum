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
//
// Splice is SpliceInto with the same package read from a's bytes and written
// to for any new asset. See [SpliceInto]'s doc for why a caller might want
// those to be two different packages.
func Splice(pkg *opc.Package, a anchor.Anchor, seq fragment.Sequence) (xmlcopy.Replacement, error) {
	return SpliceInto(pkg, pkg, a, seq)
}

// SpliceInto is [Splice] with the byte source and the asset destination
// named separately: srcPkg supplies a.Part's own bytes to splice against,
// and assetPkg is where an Asset block's new media part and relationship
// land.
//
// The split exists for E10-S3's repeat execution: a repeated table row or
// native content control is filled once per item against an *extracted*,
// relocated copy of the row/control's own source bytes — wrapped in a
// throwaway package that is discarded the moment that one iteration's bytes
// are captured, because the same region in the real document exists only
// once and has not been mutated yet. An Asset block's media part and
// relationship must not land in that throwaway package: they would vanish
// with it. srcPkg is the throwaway view in that case; assetPkg is always the
// real output package, so a picture embedded by the third iteration of a
// repeated row is still present in the file a caller actually writes.
// Splice's own single-package form is the degenerate case where the two
// happen to be the same package, which is every splice outside a repeat's
// own body.
func SpliceInto(srcPkg, assetPkg *opc.Package, a anchor.Anchor, seq fragment.Sequence) (xmlcopy.Replacement, error) {
	if srcPkg == nil || assetPkg == nil {
		return xmlcopy.Replacement{}, verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT, "nil package")
	}
	part, ok := srcPkg.Get(a.Part)
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
		return spliceNative(assetPkg, a, src, seq)
	case anchor.KindMarker:
		return spliceMarker(a, src, seq)
	case anchor.KindShape:
		return spliceShape(a, src, seq)
	case anchor.KindDefinedName, anchor.KindTableColumn:
		// An xlsx cell anchor splices a typed numfmt.Value, not a rendered
		// fragment.Sequence — see [SpliceCell]. Reaching here is a caller bug
		// (template/bind's own execBind routes these two kinds to SpliceCell
		// directly), never something an untrusted template or binding can
		// trigger.
		return xmlcopy.Replacement{}, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"an xlsx cell anchor was routed through SpliceInto's fragment.Sequence path instead of SpliceCell's typed-value path",
			map[string]any{"anchor": a.Name, "kind": string(a.Kind)})
	default:
		return xmlcopy.Replacement{}, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"the anchor carries a kind splice does not recognise",
			map[string]any{"anchor": a.Name, "kind": string(a.Kind)})
	}
}
