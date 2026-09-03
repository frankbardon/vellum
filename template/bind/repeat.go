package bind

// A repeat splices N independent copies of one structural region — a DOCX
// table row (RepeatTargetRow), a DOCX native content-control block
// (RepeatTargetBlock), or an xlsx Excel Table's one sample data row
// (RepeatTargetTableRow) — into the single place that region occupies once
// in the template, one copy per item of Repeat.Over's evaluated list. This
// file is the whole of that mechanism, generalized in E11-S1 to cover a
// third target beyond the two DOCX shapes E10-S3 built it for:
//
//   - shapeFor names, per Target, which element kind the container is and
//     which synthetic wrapper redeclares the namespaces its bytes rely on.
//   - findContainerFor locates the region. For RepeatTargetRow and
//     RepeatTargetBlock this is the smallest <w:tr> or <w:sdt> in the current
//     byte source whose content contains every anchor the repeat's body
//     reaches, at any nesting depth — unchanged from E10-S3. For
//     RepeatTargetTableRow the region is not a strict ancestor of its
//     anchors' own spans the way a DOCX container is: every KindTableColumn
//     anchor's own Span already *is* the table's one sample <row>, shared
//     identically across every column of that row (see anchor.Anchor.Span's
//     own doc). The container is simply that shared span, re-walked once to
//     recover its xmlcopy.Element.
//   - execRepeat extracts that region's own source bytes once, and for each
//     item runs the body against a throwaway, single-part package holding
//     the extracted slice and a relocated copy of the anchors the body
//     references — coordinates translated into the slice's own frame — so
//     that every iteration starts from the same untouched original rather
//     than from a previous iteration's own output, since the real document
//     carries the region only once and has not been mutated yet. This much
//     is identical across every Target.
//
// RepeatTargetTableRow additionally needs two things neither DOCX target
// does, both because a worksheet row states its own position (a <w:tr>
// states none):
//
//   - Every iteration's row and every one of its cells carries an r
//     attribute encoding its own position ("B5"), and inserting several
//     copies of one row cannot leave them all claiming the same position —
//     planRowRenumber locates each one's own attribute-value byte span, and
//     execRepeat's own loop replaces just the row-number digits per
//     iteration, in a *second* xmlcopy.Apply pass over that iteration's own
//     already-value-spliced bytes rather than merged into the first: a
//     table-column splice regenerates a cell's whole span, which would
//     otherwise overlap a renumber replacement targeting that same cell's
//     own r attribute — see execRepeat's own inline comment at the call
//     site for the full reasoning.
//   - The table's own ref attribute, in a second OPC part
//     (xl/tables/tableN.xml), states the row extent and must stay
//     consistent with whatever the worksheet actually ends up containing —
//     tableRefReplacement computes its replacement once, after the loop,
//     against the *real* source package (never the throwaway).
//
// Before any of that runs, checkTableAtSheetBottom enforces this story's own
// reason for existing: row insertion invalidates every absolute reference
// below the insertion point, so a table whose sample row is not the sheet's
// last content is rejected before any byte is spliced.
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
	"bytes"
	"fmt"
	"strconv"

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

// nsSpreadsheet is the SpreadsheetML main namespace, the xlsx counterpart of
// nsWordprocessing above.
const nsSpreadsheet = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"

// repeatWrapOpen and repeatWrapClose bracket one iteration's extracted
// container bytes before they are stored as the throwaway package's own
// part, for a DOCX target (RepeatTargetRow or RepeatTargetBlock).
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

// repeatWrapOpenXLSX and repeatWrapCloseXLSX are repeatWrapOpen/Close's xlsx
// counterpart, for RepeatTargetTableRow: the extracted <row> relies only on
// SpreadsheetML's own default namespace (a worksheet's cells carry no
// prefix), so redeclaring that one namespace is enough — there is no
// xlsx counterpart to a DOCX picture's several drawing namespaces at this
// story's own scope, which is scalar values only.
const (
	repeatWrapOpenXLSX  = `<vellumFillRepeatContainer xmlns="` + nsSpreadsheet + `">`
	repeatWrapCloseXLSX = `</vellumFillRepeatContainer>`
)

// repeatShape names, per [RepeatTarget], the container element this repeat
// splices copies of and the synthetic wrapper that restores its namespace
// context once extracted.
type repeatShape struct {
	namespace string
	local     string
	wrapOpen  string
	wrapClose string
}

// shapeFor returns target's own repeatShape, or ok=false for a target
// outside the vocabulary [ValidRepeatTarget] already validated — unreachable
// from a [Validate]d binding, guarded defensively the same way
// execStatement's own default case is.
func shapeFor(target RepeatTarget) (repeatShape, bool) {
	switch target {
	case RepeatTargetRow:
		return repeatShape{nsWordprocessing, "tr", repeatWrapOpen, repeatWrapClose}, true
	case RepeatTargetBlock:
		return repeatShape{nsWordprocessing, "sdt", repeatWrapOpen, repeatWrapClose}, true
	case RepeatTargetTableRow:
		return repeatShape{nsSpreadsheet, "row", repeatWrapOpenXLSX, repeatWrapCloseXLSX}, true
	default:
		return repeatShape{}, false
	}
}

// execRepeat evaluates r.Over to a list and splices one copy of r.Body per
// item into the single container all of r.Body's own anchors reconcile to.
//
// A zero-item list is not an error: it produces a single
// [xmlcopy.Replacement] whose Data is empty, deleting the container's own
// span from the document entirely. For RepeatTargetRow this is well-formed
// WordprocessingML even when it is a table's only row: CT_Tbl's own content
// model is tblPr, tblGrid, then zero or more w:tr, so a table left with none
// is a table Word still opens, just an empty one. For RepeatTargetTableRow,
// zero items removes the worksheet's own sample row and the table's ref is
// updated to cover the header row alone — a table with no data rows is a
// table Excel still opens.
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

	shape, ok := shapeFor(r.Target)
	if !ok {
		// Unreachable given a Validate()d binding: ValidRepeatTarget already
		// rejected anything outside the vocabulary before execution ever
		// starts.
		return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"a repeat reached execution with a target outside the vocabulary Validate already checks",
			map[string]any{"target": string(r.Target)})
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
		if r.Target == RepeatTargetTableRow && a.Kind != anchor.KindTableColumn {
			return repeatContainerErr(r.Target, names,
				"every anchor a table_row repeat's body references must be a table column anchor")
		}
		resolved[name] = a
		spans = append(spans, a.Span)
	}

	src, err := partBytes(frame.SrcPkg, part)
	if err != nil {
		return err
	}

	container, found, err := findContainerFor(r.Target, src, shape, spans)
	if err != nil {
		return err
	}
	if !found {
		return repeatContainerErr(r.Target, names,
			fmt.Sprintf("no single <%s> in the template contains every one of the repeat's own anchors", shape.local))
	}

	var tableInfo *anchor.TableColumn
	if r.Target == RepeatTargetTableRow {
		tableInfo = resolved[names[0]].Table
		if tableInfo == nil {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
				"a table_row repeat's own anchor carries no Table location info",
				map[string]any{"anchor": names[0]})
		}
		if err := checkTableAtSheetBottom(src, container, tableInfo.DisplayName); err != nil {
			return err
		}
	}

	extracted := src[container.Span.Start:container.Span.End]

	// wrapped is the throwaway part's own content for every iteration: the
	// extracted container bytes, unchanged, bracketed by a synthetic root
	// that redeclares the namespaces those bytes rely on — see
	// repeatWrapOpen's own doc comment for why the raw extracted slice is
	// not enough on its own. offset is where the extracted bytes actually
	// start inside wrapped, which every relocated anchor's Span is computed
	// relative to.
	wrapped := make([]byte, 0, len(shape.wrapOpen)+len(extracted)+len(shape.wrapClose))
	wrapped = append(wrapped, shape.wrapOpen...)
	wrapped = append(wrapped, extracted...)
	wrapped = append(wrapped, shape.wrapClose...)
	offset := int64(len(shape.wrapOpen))

	var originalRowNum int
	if r.Target == RepeatTargetTableRow {
		rowStr, ok := readAttr(container, "r")
		if !ok {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
				"the table's own sample row carries no r attribute", nil)
		}
		originalRowNum, err = strconv.Atoi(rowStr)
		if err != nil {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
				"the table's own sample row's r attribute does not parse as an integer",
				map[string]any{"r": rowStr})
		}
	}

	var out []byte
	for i, item := range items {
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
				Table: a.Table,
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

		// Row/cell renumbering is a *second*, independent Apply pass over
		// this iteration's own already-spliced bytes, not merged into the
		// pass above: a table-column splice replaces a cell's *whole* span
		// (spliceTableColumn re-emits the cell's own r attribute unchanged,
		// alongside its new typed value), which overlaps byte-for-byte with
		// a renumber replacement targeting that same attribute's value —
		// xmlcopy.Apply rejects overlapping spans outright, and there is no
		// honest way to describe "part of this element changes for one
		// reason, part of it for another" as two disjoint spans when one
		// splice already regenerates the whole element. Running the passes
		// in sequence instead — value first, renumber second, each over the
		// other's own output — sidesteps the conflict entirely, and is
		// planRowRenumber's own reason for being called fresh here rather
		// than once before the loop: attribute value spans shift between
		// iterations only because Apply's own emitted length can differ from
		// the source's (an inline string longer or shorter than
		// "placeholder"), so recomputing against each iteration's own
		// applied bytes is correct where reusing offsets captured from
		// wrapped's pristine bytes would not be.
		if r.Target == RepeatTargetTableRow {
			plan, err := planRowRenumber(applied)
			if err != nil {
				return err
			}
			applied, err = xmlcopy.Apply(applied, plan.replacementsFor(originalRowNum+i))
			if err != nil {
				return err
			}
		}

		// Every replacement above lies strictly inside
		// [offset, offset+len(extracted)) — every relocated anchor's own
		// Span does, and every renumber replacement targets an attribute
		// value inside the row or one of its cells, both inside that same
		// range — so shape.wrapOpen and shape.wrapClose themselves survive
		// both Apply passes verbatim at applied's own start and end, and can
		// be stripped back off before this iteration's bytes join the
		// others.
		out = append(out, applied[len(shape.wrapOpen):len(applied)-len(shape.wrapClose)]...)
	}

	repls.Add(part, container.Span.Replace(out))

	if r.Target == RepeatTargetTableRow {
		refRepl, err := tableRefReplacement(frame.SrcPkg, tableInfo, len(items))
		if err != nil {
			return err
		}
		repls.Add(tableInfo.TablePart, refRepl)
	}

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

// findContainerFor locates target's own splice container in src, dispatching
// on target because "which region is the container" is not the same
// question for every target — see this file's own doc comment.
func findContainerFor(target RepeatTarget, src []byte, shape repeatShape, spans []xmlcopy.Span) (xmlcopy.Element, bool, error) {
	if target == RepeatTargetTableRow {
		// Every KindTableColumn anchor referenced by the body already shares
		// one identical Span — the table's own one sample row — set once at
		// discovery (anchor.discoverXLSX) and never re-derived here. The
		// generic "smallest element whose Content strictly contains every
		// span" search below assumes the container is a strict *ancestor* of
		// every span, which holds for a DOCX <w:tr> containing several
		// markers' own paragraph spans but not here: the row *is* the span,
		// so a container search would need to look for something whose
		// Content is bigger than itself and never find one.
		for _, sp := range spans[1:] {
			if sp != spans[0] {
				return xmlcopy.Element{}, false, nil
			}
		}
		return elementAtSpan(src, shape.namespace, shape.local, spans[0])
	}
	return findContainer(src, shape.namespace, shape.local, spans)
}

// findContainer walks src once looking for every element named local (in
// namespace) whose own content entirely contains every span in spans, and
// returns the smallest one by byte width — the same "smallest containing
// span" reasoning template/anchor's own nested-w:sdt handling uses,
// generalised from "which w:sdt owns this w:tag" to "which element of this
// kind contains this whole set of spans".
func findContainer(src []byte, namespace, local string, spans []xmlcopy.Span) (xmlcopy.Element, bool, error) {
	var best xmlcopy.Element
	found := false

	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if e.Name.Space != namespace || e.Name.Local != local {
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

// elementAtSpan walks src once looking for the element named local (in
// namespace) whose own Span exactly equals sp.
func elementAtSpan(src []byte, namespace, local string, sp xmlcopy.Span) (xmlcopy.Element, bool, error) {
	var result xmlcopy.Element
	found := false
	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if found || e.Name.Space != namespace || e.Name.Local != local {
			return nil
		}
		if e.Span == sp {
			result = e
			found = true
		}
		return nil
	})
	if err != nil {
		return xmlcopy.Element{}, false, err
	}
	return result, found, nil
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

// --- xlsx table_row: row/cell renumbering, ref update, bottom-of-sheet ----

// readAttr returns the value of element e's own unprefixed attribute named
// local, from the Attr slice xmlcopy.Walk already populated — cheap, since
// it needs no byte-level search, and used everywhere this file only needs to
// *read* an attribute rather than compute a byte span to replace.
func readAttr(e xmlcopy.Element, local string) (string, bool) {
	for _, a := range e.Attr {
		if a.Name.Space == "" && a.Name.Local == local {
			return a.Value, true
		}
	}
	return "", false
}

// attrValueSpan finds the byte span of attribute local's value — the bytes
// strictly between its surrounding quote characters — within element e's own
// opening tag, by searching for `local="` preceded by a whitespace boundary.
// A leading space rules out a coincidental substring match inside a longer
// attribute name (searching for `r="` never matches inside `spans="`, whose
// letters do not contain that sequence at all); every attribute this file
// searches for on a <row>, <c> or <table> element has no other attribute
// name on the same element ending in the same letters immediately before
// "=\"", so this is exact for every shape this package's own writers and
// every fixture xmlcopy.Walk is asked to parse produce.
func attrValueSpan(src []byte, e xmlcopy.Element, local string) (xmlcopy.Span, bool) {
	tag := src[e.Span.Start:e.Content.Start]
	needle := []byte(" " + local + `="`)
	idx := bytes.Index(tag, needle)
	if idx < 0 {
		return xmlcopy.Span{}, false
	}
	valStart := e.Span.Start + int64(idx) + int64(len(needle))
	rest := src[valStart:e.Content.Start]
	end := bytes.IndexByte(rest, '"')
	if end < 0 {
		return xmlcopy.Span{}, false
	}
	return xmlcopy.Span{Start: valStart, End: valStart + int64(end)}, true
}

// cellRenumberSpot is one <c> child's own column letters (preserved) and the
// byte span of its own r attribute's value (rewritten per iteration).
type cellRenumberSpot struct {
	colLetters string
	valueSpan  xmlcopy.Span
}

// rowRenumberPlan is computed once, against the pristine wrapped bytes,
// before any iteration's splices run — every iteration's row and cells sit
// at the identical byte offsets in their own copy of wrapped, only the
// replacement *value* differs per iteration.
type rowRenumberPlan struct {
	rowValueSpan xmlcopy.Span
	cells        []cellRenumberSpot
}

// planRowRenumber walks wrapped once, locating the single <row> element it
// carries (the extracted sample row, still bracketed by its synthetic
// wrapper) and every <c> child's own r attribute.
func planRowRenumber(wrapped []byte) (*rowRenumberPlan, error) {
	var row xmlcopy.Element
	rowFound := false
	var cells []xmlcopy.Element

	err := xmlcopy.Walk(wrapped, func(e xmlcopy.Element) error {
		if e.Name.Space != nsSpreadsheet {
			return nil
		}
		switch e.Name.Local {
		case "row":
			if !rowFound {
				row = e
				rowFound = true
			}
		case "c":
			cells = append(cells, e)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !rowFound {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"a table_row repeat's own extracted container carries no <row> element", nil)
	}

	rowSpan, ok := attrValueSpan(wrapped, row, "r")
	if !ok {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"the table's own sample row carries no r attribute to renumber", nil)
	}

	plan := &rowRenumberPlan{rowValueSpan: rowSpan}
	for _, c := range cells {
		valSpan, ok := attrValueSpan(wrapped, c, "r")
		if !ok {
			continue
		}
		ref := string(wrapped[valSpan.Start:valSpan.End])
		colLetters, _, ok := anchor.ParseSimpleCellRef(ref)
		if !ok {
			continue
		}
		plan.cells = append(plan.cells, cellRenumberSpot{colLetters: colLetters, valueSpan: valSpan})
	}
	return plan, nil
}

// replacementsFor returns the row's own r replacement plus every cell's own
// r replacement for iteration row number newRow, preserving each cell's own
// column letters. Already in the ascending-by-Start order [xmlcopy.Apply]
// requires without any extra sort: the row's own r attribute always
// precedes every cell inside it, and planRowRenumber's own Walk visits
// sibling <c> cells in document order (they share no nesting relationship,
// so xmlcopy.Walk's post-order visitation coincides with left-to-right
// document order for them).
func (p *rowRenumberPlan) replacementsFor(newRow int) []xmlcopy.Replacement {
	out := make([]xmlcopy.Replacement, 0, 1+len(p.cells))
	out = append(out, p.rowValueSpan.Replace([]byte(strconv.Itoa(newRow))))
	for _, c := range p.cells {
		out = append(out, c.valueSpan.Replace([]byte(c.colLetters+strconv.Itoa(newRow))))
	}
	return out
}

// checkTableAtSheetBottom is this story's own reason for existing: row
// insertion invalidates every absolute reference below the insertion point,
// so a table_row repeat refuses to run against a template where anything —
// another table, a formula's own row, a chart source range — sits below the
// table's own sample row.
func checkTableAtSheetBottom(src []byte, row xmlcopy.Element, tableDisplayName string) error {
	rowStr, ok := readAttr(row, "r")
	if !ok {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"the table's own sample row carries no r attribute", nil)
	}
	rowNum, err := strconv.Atoi(rowStr)
	if err != nil {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"the table's own sample row's r attribute does not parse as an integer",
			map[string]any{"r": rowStr})
	}

	violating := 0
	walkErr := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if e.Name.Space != nsSpreadsheet || e.Name.Local != "row" {
			return nil
		}
		v, ok := readAttr(e, "r")
		if !ok {
			return nil
		}
		n, convErr := strconv.Atoi(v)
		if convErr != nil {
			return nil
		}
		if n > rowNum && (violating == 0 || n < violating) {
			violating = n
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	if violating > 0 {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_TEMPLATE_TABLE_NOT_AT_SHEET_BOTTOM,
			"a table_row repeat targets a table whose sample row is not the last content in the worksheet",
			map[string]any{"table": tableDisplayName, "table_row": rowNum, "content_row": violating})
	}
	return nil
}

// tableRefReplacement computes the single replacement needed to keep
// info.TablePart's own ref attribute consistent with a repeat that produced
// itemCount rows — read against pkg, always the real source package, never a
// repeat iteration's own throwaway view, because the table part is untouched
// until this one replacement lands.
func tableRefReplacement(pkg *opc.Package, info *anchor.TableColumn, itemCount int) (xmlcopy.Replacement, error) {
	tableSrc, err := partBytes(pkg, info.TablePart)
	if err != nil {
		return xmlcopy.Replacement{}, err
	}

	table, found, err := findRootElement(tableSrc, nsSpreadsheet, "table")
	if err != nil {
		return xmlcopy.Replacement{}, err
	}
	if !found {
		return xmlcopy.Replacement{}, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"the table part carries no root <table> element",
			map[string]any{"part": info.TablePart})
	}

	refSpan, ok := attrValueSpan(tableSrc, table, "ref")
	if !ok {
		return xmlcopy.Replacement{}, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"the table's own root element carries no ref attribute",
			map[string]any{"part": info.TablePart})
	}

	toRow := info.HeaderRow + itemCount
	newRef := anchor.CellRef(info.FromColumn, info.HeaderRow) + ":" + anchor.CellRef(info.ToColumn, toRow)
	return refSpan.Replace([]byte(newRef)), nil
}

// findRootElement walks src once for the outermost (Depth 0) element named
// local in namespace — a table part's own root <table>, in this file's only
// use of it.
func findRootElement(src []byte, namespace, local string) (xmlcopy.Element, bool, error) {
	var result xmlcopy.Element
	found := false
	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if !found && e.Depth == 0 && e.Name.Space == namespace && e.Name.Local == local {
			result = e
			found = true
		}
		return nil
	})
	if err != nil {
		return xmlcopy.Element{}, false, err
	}
	return result, found, nil
}
