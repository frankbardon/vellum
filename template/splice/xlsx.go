package splice

// SpliceCell is the xlsx counterpart of Splice/SpliceInto: a defined-name or
// table-column anchor's own splice site is one worksheet <c> cell, and what
// lands there is a typed [numfmt.Value] — text, number, boolean or date —
// rendered as the matching cell shape, not a rendered run of text the way
// every DOCX anchor kind is. See this file's own doc comment on renderCell
// for the exact shapes and why fill mode prefers an inline string over
// growing the shared-string table.

import (
	"strconv"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/numfmt"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/xmlcopy"
)

// nsSpreadsheet is the SpreadsheetML main namespace, matched by resolved URI
// rather than a literal default-namespace guess — this package's own copy,
// mirroring xml.go's own "own your own constants" convention rather than
// importing template/bind's identical constant (template/bind imports this
// package; the reverse would be a cycle).
const nsSpreadsheet = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"

// SpliceCell renders val into a's own worksheet cell and returns the single
// [xmlcopy.Replacement] needed to bind it, dispatching on a.Kind to
// [spliceDefinedName] or [spliceTableColumn].
//
// Unlike [SpliceInto], SpliceCell never touches assetPkg — a spreadsheet
// cell's own scalar value never registers a new media part or relationship
// — so it takes only the package a's own bytes are read from.
func SpliceCell(srcPkg *opc.Package, a anchor.Anchor, val numfmt.Value) (xmlcopy.Replacement, error) {
	if srcPkg == nil {
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
	case anchor.KindDefinedName:
		return spliceDefinedName(a, src, val)
	case anchor.KindTableColumn:
		return spliceTableColumn(a, src, val)
	default:
		return xmlcopy.Replacement{}, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"the anchor carries a kind SpliceCell does not recognise",
			map[string]any{"anchor": a.Name, "kind": string(a.Kind)})
	}
}

// spliceDefinedName replaces a's own whole <c>...</c> (or self-closing <c/>)
// element — a.Span, exactly as anchor.discoverXLSX recorded it — with a
// freshly rendered cell carrying val, preserving the original cell's own r
// and s attributes untouched.
func spliceDefinedName(a anchor.Anchor, src []byte, val numfmt.Value) (xmlcopy.Replacement, error) {
	cell, found, err := elementAtSpanXLSX(src, nsSpreadsheet, "c", a.Span)
	if err != nil {
		return xmlcopy.Replacement{}, err
	}
	if !found {
		return xmlcopy.Replacement{}, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"a defined-name anchor's own span no longer matches a <c> element in its part",
			map[string]any{"anchor": a.Name, "part": a.Part})
	}
	ref, styleAttr := cellRefAndStyle(cell)
	return a.Span.Replace(renderCell(ref, styleAttr, val)), nil
}

// spliceTableColumn replaces one <c> cell within a's own shared row span —
// the one whose column matches a.Table.Column — with a freshly rendered cell
// carrying val, preserving that cell's own r and s attributes untouched. a's
// Span is the whole sample row, shared identically by every KindTableColumn
// anchor of that row (see anchor.Anchor.Span's own doc); a.Table.Column
// disambiguates which of the row's several cells is this one.
func spliceTableColumn(a anchor.Anchor, src []byte, val numfmt.Value) (xmlcopy.Replacement, error) {
	if a.Table == nil {
		return xmlcopy.Replacement{}, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"a table-column anchor carries no Table location info",
			map[string]any{"anchor": a.Name})
	}
	row, found, err := elementAtSpanXLSX(src, nsSpreadsheet, "row", a.Span)
	if err != nil {
		return xmlcopy.Replacement{}, err
	}
	if !found {
		return xmlcopy.Replacement{}, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"a table-column anchor's own row span no longer matches a <row> element in its part",
			map[string]any{"anchor": a.Name, "part": a.Part})
	}

	cell, found, err := findCellInRow(src, row, a.Table.Column)
	if err != nil {
		return xmlcopy.Replacement{}, err
	}
	if !found {
		// anchor.discoverXLSX already verified every declared column has a
		// placeholder cell in the sample row (VELLUM_ANCHOR_TABLE_UNSUPPORTED
		// otherwise), so this is reachable only if the row splice was given
		// bytes that no longer match what discovery saw.
		return xmlcopy.Replacement{}, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"the table's data row no longer carries a cell for this column",
			map[string]any{"anchor": a.Name, "column": a.Table.Column})
	}

	ref, styleAttr := cellRefAndStyle(cell)
	return cell.Span.Replace(renderCell(ref, styleAttr, val)), nil
}

// renderCell renders a whole <c>...</c> (or self-closing) cell element
// carrying val, preserving ref (the cell's own r attribute) and styleAttr
// (the cell's own already-formatted ` s="N"` attribute, or "" when the
// original cell carried none) untouched — reusing the template's own local
// formatting is the same principle this package's DOCX strategies already
// establish for w:rPr.
//
// Text is written as an inline string (t="inlineStr", <is><t>...</t></is>)
// rather than interned into xl/sharedStrings.xml, a deliberate fill-mode-only
// choice from what sheet's own compose-mode writer does: growing the shared
// string table surgically means also editing sharedStrings.xml's own count
// and uniqueCount attributes, a second part touched on every fill, for a
// dedup benefit that matters at whole-workbook scale and not for the handful
// of cells fill mode ever touches. Excel opens an inline-string cell exactly
// the same as a shared-string one.
//
// A number and a date both write as a live number with no t attribute —
// mirroring sheet's own writeCell exactly, including why: only the cell's own
// numFmt (its untouched s attribute) tells a reader which one it is looking
// at, because a date has never been a distinct SpreadsheetML storage type.
// [numfmt.Serial] converts a date's own time.Time to that same serial number.
//
// xml:space="preserve" on the inline string's own <t> is not optional:
// CLAUDE.md's own "Respect xml:space=preserve and normalise whitespace
// nowhere on this path" rule applies here exactly as it does to a DOCX run —
// a value carrying meaningful leading or trailing space must survive it.
func renderCell(ref, styleAttr string, val numfmt.Value) []byte {
	switch val.Kind {
	case numfmt.KindText:
		return []byte(`<c r="` + xmlcopy.EscapeAttr(ref) + `"` + styleAttr + ` t="inlineStr"><is><t xml:space="preserve">` +
			xmlcopy.EscapeText(val.Text) + `</t></is></c>`)
	case numfmt.KindBool:
		b := "0"
		if val.Bool {
			b = "1"
		}
		return []byte(`<c r="` + xmlcopy.EscapeAttr(ref) + `"` + styleAttr + ` t="b"><v>` + b + `</v></c>`)
	case numfmt.KindDate:
		return []byte(`<c r="` + xmlcopy.EscapeAttr(ref) + `"` + styleAttr + `><v>` + formatCellNumber(numfmt.Serial(val.Time)) + `</v></c>`)
	case numfmt.KindNumber:
		return []byte(`<c r="` + xmlcopy.EscapeAttr(ref) + `"` + styleAttr + `><v>` + formatCellNumber(val.Number) + `</v></c>`)
	default: // numfmt.KindEmpty
		return []byte(`<c r="` + xmlcopy.EscapeAttr(ref) + `"` + styleAttr + `/>`)
	}
}

// formatCellNumber mirrors sheet's own formatNumber exactly: strconv's
// shortest round-tripping form, deterministic for a given float64 across
// runs and platforms.
func formatCellNumber(n float64) string {
	return strconv.FormatFloat(n, 'g', -1, 64)
}

// cellRefAndStyle reads a cell element's own r attribute and, when present,
// its own s attribute rendered back as a ready-to-splice ` s="N"` fragment
// (or "" when the cell carried none).
func cellRefAndStyle(e xmlcopy.Element) (ref, styleAttr string) {
	for _, a := range e.Attr {
		if a.Name.Space != "" {
			continue
		}
		switch a.Name.Local {
		case "r":
			ref = a.Value
		case "s":
			styleAttr = ` s="` + xmlcopy.EscapeAttr(a.Value) + `"`
		}
	}
	return ref, styleAttr
}

// elementAtSpanXLSX walks src once looking for the element named local (in
// namespace) whose own Span exactly equals sp — this package's own copy of
// the same technique template/bind's repeat.go uses to relocate a
// KindTableColumn anchor's shared row span back to a real xmlcopy.Element.
func elementAtSpanXLSX(src []byte, namespace, local string, sp xmlcopy.Span) (xmlcopy.Element, bool, error) {
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

// findCellInRow walks src once looking for the <c> child of row whose own
// column (parsed from its r attribute) equals absCol, regardless of what row
// number that cell's own reference currently states — the row it is found
// nested inside is what matters, the same rule
// anchor.discoverXLSX's own rowHasCellForColumn already established at
// discovery time.
func findCellInRow(src []byte, row xmlcopy.Element, absCol int) (xmlcopy.Element, bool, error) {
	var result xmlcopy.Element
	found := false
	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if found || e.Name.Space != nsSpreadsheet || e.Name.Local != "c" {
			return nil
		}
		if e.Span.Start < row.Content.Start || e.Span.End > row.Content.End {
			return nil
		}
		ref, _ := cellRefAndStyle(e)
		colLetters, _, ok := anchor.ParseSimpleCellRef(ref)
		if ok && anchor.ColumnNumber(colLetters) == absCol {
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
