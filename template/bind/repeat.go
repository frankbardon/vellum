package bind

// A repeat splices N independent copies of one structural region — a table
// row (RepeatTargetRow) or a native content-control block
// (RepeatTargetBlock) — into the single place that region occupies once in
// the template, one copy per item of Repeat.Over's evaluated list. This file
// is the whole of that mechanism:
//
//   - findContainer locates the region: the smallest <w:tr> or <w:sdt> in the
//     current byte source whose content contains every anchor the repeat's
//     body reaches, at any nesting depth.
//   - execRepeat extracts that region's own source bytes once, and for each
//     item runs the body against a throwaway, single-part package holding
//     the extracted slice and a relocated copy of the anchors the body
//     references — coordinates translated into the slice's own frame — so
//     that every iteration starts from the same untouched original rather
//     than from a previous iteration's own output, since the real document
//     carries the region only once and has not been mutated yet.
//
// The one thing a throwaway iteration package must never be the destination
// for is a new asset: an Asset block spliced inside a repeated row would
// register its media part and relationship against a package discarded the
// moment that iteration's bytes are captured. execRepeat threads the real
// output package through as assetPkg, unchanged from what Execute itself
// received, precisely so that split never has to be reasoned about by
// anything nested further inside a repeat's own body — a doubly-nested
// repeat's own execRepeat call receives the exact same assetPkg its parent
// did.

import (
	"fmt"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/xmlcopy"
)

// nsWordprocessing is the WordprocessingML main namespace, matched by
// resolved URI rather than by a literal "w:" prefix, mirroring
// template/anchor's and template/splice's own constant of the same name and
// the same reasoning: independence from whichever prefix an authoring tool
// bound the namespace to.
const nsWordprocessing = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

// repeatWrapOpen and repeatWrapClose bracket one iteration's extracted
// container bytes before they are stored as the throwaway package's own
// part.
//
// A raw sub-slice of the source is not enough on its own: xmlcopy resolves
// every element's namespace by walking up from that element's own document,
// and a slice starting partway through the real one carries no xmlns:w
// declaration of its own to resolve the "w:" prefix its own elements use —
// exactly the hazard template/defrag's own Flatten doc comment names
// ("a sub-slice starting partway through the document would lose whatever
// ancestor xmlns declaration resolves the very namespace prefixes
// container's own runs use"). Wrapping the extracted bytes in a synthetic
// root that redeclares the namespaces fill mode's own splice output can
// reach — WordprocessingML itself, plus the drawing namespaces a spliced
// picture uses — restores that context locally, the same fix
// template/splice's own renderDrawing already applies for the same reason
// (it redeclares xmlns:r, xmlns:wp, xmlns:a and xmlns:pic on the element it
// emits rather than assuming them inherited, because splice edits a
// template it did not author). The wrapper element itself is discarded the
// moment an iteration's bytes are captured — stripped back off in
// execRepeat — so its tag name only has to be well-formed and not collide
// with a real element in a WordprocessingML document, never anything a
// caller sees.
const (
	repeatWrapOpen = `<vellumFillRepeatContainer` +
		` xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"` +
		` xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"` +
		` xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"` +
		` xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"` +
		` xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"` +
		` xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"` +
		`>`
	repeatWrapClose = `</vellumFillRepeatContainer>`
)

// containerLocalFor names the WordprocessingML element a repeat's declared
// Target splices copies of.
func containerLocalFor(target RepeatTarget) string {
	switch target {
	case RepeatTargetRow:
		return "tr"
	case RepeatTargetBlock:
		return "sdt"
	default:
		return ""
	}
}

// execRepeat evaluates r.Over to a list and splices one copy of r.Body per
// item into the single container all of r.Body's own anchors reconcile to.
//
// A zero-item list is not an error: it produces a single
// [xmlcopy.Replacement] whose Data is empty, deleting the container's own
// span — its <w:tr> or <w:sdt> — from the document entirely. For
// RepeatTargetRow this is well-formed WordprocessingML even when it is a
// table's only row: CT_Tbl's own content model is tblPr, tblGrid, then zero
// or more w:tr, so a table left with none is a table Word still opens, just
// an empty one.
func execRepeat(r *Repeat, scope Scope, ev Evaluator, frame Frame, assetPkg *opc.Package, repls *ReplacementSet) error {
	items, err := EvaluateList(ev, r.Over, scope)
	if err != nil {
		return err
	}

	names := collectAnchorNames(r.Body)
	if len(names) == 0 {
		return repeatContainerErr(r.Target, names,
			"the repeat's body names no anchor anywhere in it, so there is no structural position to find a splice container from")
	}

	resolved := make(map[string]anchor.Anchor, len(names))
	spans := make([]xmlcopy.Span, 0, len(names))
	part := ""
	for _, name := range names {
		a, ok := frame.Anchors[name]
		if !ok {
			return unknownAnchorErr(name)
		}
		if part == "" {
			part = a.Part
		} else if part != a.Part {
			return repeatContainerErr(r.Target, names,
				"the repeat's body anchors are not all in the same part")
		}
		resolved[name] = a
		spans = append(spans, a.Span)
	}

	src, err := partBytes(frame.SrcPkg, part)
	if err != nil {
		return err
	}

	local := containerLocalFor(r.Target)
	container, found, err := findContainer(src, local, spans)
	if err != nil {
		return err
	}
	if !found {
		return repeatContainerErr(r.Target, names,
			fmt.Sprintf("no single <w:%s> in the template contains every one of the repeat's own anchors", local))
	}

	extracted := src[container.Span.Start:container.Span.End]

	// wrapped is the throwaway part's own content for every iteration: the
	// extracted container bytes, unchanged, bracketed by a synthetic root
	// that redeclares the namespaces those bytes rely on — see
	// repeatWrapOpen's own doc comment for why the raw extracted slice is
	// not enough on its own. offset is where the extracted bytes actually
	// start inside wrapped, which every relocated anchor's Span is computed
	// relative to.
	wrapped := make([]byte, 0, len(repeatWrapOpen)+len(extracted)+len(repeatWrapClose))
	wrapped = append(wrapped, repeatWrapOpen...)
	wrapped = append(wrapped, extracted...)
	wrapped = append(wrapped, repeatWrapClose...)
	offset := int64(len(repeatWrapOpen))

	var out []byte
	for _, item := range items {
		iterScope := extendScope(scope, r.As, item)

		relocated := make(map[string]anchor.Anchor, len(names))
		for _, name := range names {
			a := resolved[name]
			relocated[name] = anchor.Anchor{
				Name:  a.Name,
				Kind:  a.Kind,
				Alias: a.Alias,
				// Part stays the *real* owner part name, not a synthetic one
				// invented for the throwaway package below — an Asset block
				// spliced during this iteration registers its relationship
				// against assetPkg.Relationships(a.Part), and that must be
				// the relationships part the final document actually
				// serialises, not one nobody ever writes.
				Part: part,
				Span: xmlcopy.Span{
					Start: a.Span.Start - container.Span.Start + offset,
					End:   a.Span.End - container.Span.Start + offset,
				},
			}
		}

		// A fresh, discarded-after-use package whose only content is this
		// one iteration's own wrapped, extracted slice, stored under the
		// real part name so relocated.Part above and this Put agree:
		// SpliceInto reads a.Part's bytes from *this* package (the
		// throwaway view) while any new asset it produces registers
		// against assetPkg (the real output package, threaded through
		// unchanged) — see this file's own doc comment.
		throwaway := opc.New()
		if err := throwaway.Put(&opc.Part{Name: part, Data: wrapped}); err != nil {
			return err
		}

		childFrame := Frame{SrcPkg: throwaway, Anchors: relocated}
		childRepls := NewReplacementSet()
		if err := Execute(r.Body, iterScope, ev, childFrame, assetPkg, childRepls); err != nil {
			return err
		}

		applied, err := xmlcopy.Apply(wrapped, childRepls.For(part))
		if err != nil {
			return err
		}
		// Every replacement Execute produced above lies strictly inside
		// [offset, offset+len(extracted)) — every relocated anchor's own
		// Span does — so repeatWrapOpen and repeatWrapClose themselves
		// survive Apply verbatim at applied's own start and end, and can be
		// stripped back off before this iteration's bytes join the others.
		out = append(out, applied[len(repeatWrapOpen):len(applied)-len(repeatWrapClose)]...)
	}

	repls.Add(part, container.Span.Replace(out))
	return nil
}

// collectAnchorNames returns every Bind.Anchor reachable from stmts, at any
// nesting depth, in first-seen order with duplicates removed — the set a
// repeat's own container search reconciles against.
func collectAnchorNames(stmts []Statement) []string {
	seen := make(map[string]bool)
	var out []string
	Walk(stmts, func(s *Statement) error {
		if s.Kind == StatementBind && s.Bind != nil && s.Bind.Anchor != "" && !seen[s.Bind.Anchor] {
			seen[s.Bind.Anchor] = true
			out = append(out, s.Bind.Anchor)
		}
		return nil
	})
	return out
}

// findContainer walks src once looking for every WordprocessingML element
// named local (in the WordprocessingML namespace) whose own content
// entirely contains every span in spans, and returns the smallest one by
// byte width — the same "smallest containing span" reasoning
// template/anchor's own nested-w:sdt handling uses, generalised from "which
// w:sdt owns this w:tag" to "which element of this kind contains this whole
// set of spans".
func findContainer(src []byte, local string, spans []xmlcopy.Span) (xmlcopy.Element, bool, error) {
	var best xmlcopy.Element
	found := false

	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if e.Name.Space != nsWordprocessing || e.Name.Local != local {
			return nil
		}
		for _, sp := range spans {
			if sp.Start < e.Content.Start || sp.End > e.Content.End {
				return nil
			}
		}
		if !found || width(e.Span) < width(best.Span) {
			best = e
			found = true
		}
		return nil
	})
	if err != nil {
		return xmlcopy.Element{}, false, err
	}
	return best, found, nil
}

func width(s xmlcopy.Span) int64 { return s.End - s.Start }

func partBytes(pkg *opc.Package, name string) ([]byte, error) {
	p, ok := pkg.Get(name)
	if !ok {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"repeat execution's own frame package does not contain the anchor's part",
			map[string]any{"part": name})
	}
	return p.Bytes()
}

func repeatContainerErr(target RepeatTarget, anchors []string, reason string) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_TEMPLATE_REPEAT_CONTAINER_INVALID,
		"a repeat's body anchors cannot be reconciled to one splice container",
		map[string]any{
			"target":  string(target),
			"anchors": anchors,
			"reason":  reason,
		})
}
