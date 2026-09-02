package sheet_test

import (
	"strings"
	"testing"
	"time"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/provenance"
	"github.com/frankbardon/vellum/sheet"
)

// TestStylesXML_ReservedFillIndicesArePresentEvenWithNoCustomFills pins the
// one collection where getting an index wrong does not degrade a workbook, it
// breaks it: ECMA-376 reserves fill index 0 for "none" and index 1 for
// "gray125", present whether or not any cell selects them.
//
// Non-vacuous by construction: a workbook with zero custom fills is the case
// most likely to tempt an implementation into skipping the reserved pair
// altogether, so that is exactly the case this asserts against.
func TestStylesXML_ReservedFillIndicesArePresentEvenWithNoCustomFills(t *testing.T) {
	wb := &sheet.Workbook{Sheets: []sheet.Sheet{{Name: "Sheet1"}}}
	xml := part(t, write(t, wb), "xl/styles.xml")

	if !strings.Contains(xml, `<fill><patternFill patternType="none"/></fill>`) {
		t.Errorf("fill index 0 is not the reserved \"none\" pattern:\n%s", xml)
	}
	if !strings.Contains(xml, `<fill><patternFill patternType="gray125"/></fill>`) {
		t.Errorf("fill index 1 is not the reserved \"gray125\" pattern:\n%s", xml)
	}
	if !strings.Contains(xml, `<fills count="2">`) {
		t.Errorf("fills count is not 2 with no custom fills declared:\n%s", xml)
	}

	// And the reserved pair comes first even when custom fills exist,
	// pushing every custom index up by two — [xmlFillID]'s whole job.
	custom := &sheet.Workbook{
		Sheets: []sheet.Sheet{{Name: "Sheet1"}},
		Styles: sheet.StyleSheet{
			Fills:   []sheet.Fill{{Color: "112233"}},
			Formats: []sheet.CellFormat{{}, {FillIndex: 1}},
		},
	}
	xml = part(t, write(t, custom), "xl/styles.xml")
	if !strings.Contains(xml, `fillId="2"`) {
		t.Errorf("the one custom fill is not written at index 2, after the two reserved entries:\n%s", xml)
	}
	if !strings.Contains(xml, `<fills count="3">`) {
		t.Errorf("fills count is not 3 (2 reserved + 1 custom):\n%s", xml)
	}
}

// TestStylesXML_ElementOrderIsFixedByTheSchema pins the one property of
// styles.xml a caller cannot get right by construction: the element order
// ECMA-376 requires, which this writer's own [StyleSheet] fields do not
// enforce by themselves — only [Workbook.WriteTo] does, by writing them in
// this order regardless of what order a caller filled the struct in.
func TestStylesXML_ElementOrderIsFixedByTheSchema(t *testing.T) {
	wb := &sheet.Workbook{
		Sheets: []sheet.Sheet{{Name: "Sheet1"}},
		Styles: sheet.StyleSheet{
			NumFmts: []sheet.NumFmt{{Code: "0.0%"}},
			Fonts:   []sheet.Font{{Name: "Calibri", Bold: true}},
			Fills:   []sheet.Fill{{Color: "112233"}},
			Borders: []sheet.Border{{Color: "808080"}},
			Formats: []sheet.CellFormat{{}},
		},
	}
	xml := part(t, write(t, wb), "xl/styles.xml")

	order := []string{"<numFmts", "<fonts", "<fills", "<borders",
		"<cellStyleXfs", "<cellXfs", "<cellStyles"}
	last := -1
	for _, el := range order {
		i := strings.Index(xml, el)
		if i < 0 {
			t.Fatalf("styles.xml does not carry %s:\n%s", el, xml)
		}
		if i < last {
			t.Fatalf("%s appears before an earlier required element; order is not %v:\n%s", el, order, xml)
		}
		last = i
	}
}

// TestPackage_RequiresAtLeastOneSheet pins the workbook-level check: a
// package with zero sheets is one Excel refuses outright.
func TestPackage_RequiresAtLeastOneSheet(t *testing.T) {
	_, err := (&sheet.Workbook{}).Package(sheet.WriteOptions{})
	if !verr.HasCode(err, verr.VELLUM_SHEET_INVALID) {
		t.Fatalf("error = %v, want VELLUM_SHEET_INVALID", err)
	}
}

// TestPackage_RefusesADuplicateSheetName is checked here rather than left to
// Excel's own repair-on-open, which silently renames the second one — a
// consumer's fill binding referencing a sheet by name deserves a loud error,
// not a workbook whose tabs no longer match what was asked for.
func TestPackage_RefusesADuplicateSheetName(t *testing.T) {
	wb := &sheet.Workbook{Sheets: []sheet.Sheet{{Name: "Findings"}, {Name: "Findings"}}}
	_, err := wb.Package(sheet.WriteOptions{})
	if !verr.HasCode(err, verr.VELLUM_SHEET_INVALID) {
		t.Fatalf("error = %v, want VELLUM_SHEET_INVALID", err)
	}
}

// TestPackage_RefusesAForbiddenSheetNameCharacter pins the other half of
// Excel's own tab-name rule.
func TestPackage_RefusesAForbiddenSheetNameCharacter(t *testing.T) {
	wb := &sheet.Workbook{Sheets: []sheet.Sheet{{Name: "North/South"}}}
	_, err := wb.Package(sheet.WriteOptions{})
	if !verr.HasCode(err, verr.VELLUM_SHEET_INVALID) {
		t.Fatalf("error = %v, want VELLUM_SHEET_INVALID", err)
	}
}

// TestSharedStrings_CountIsUsesNotUniqueCount checks the shared string
// table's own `count`/`uniqueCount` distinction: a string referenced twice
// counts twice toward `count` and once toward `uniqueCount`.
func TestSharedStrings_CountIsUsesNotUniqueCount(t *testing.T) {
	wb := &sheet.Workbook{Sheets: []sheet.Sheet{{
		Name: "Sheet1",
		Rows: []sheet.Row{
			{Index: 1, Cells: []sheet.Cell{{Column: 1, Value: sheet.Text("Region")}}},
			{Index: 2, Cells: []sheet.Cell{{Column: 1, Value: sheet.Text("Region")}}},
			{Index: 3, Cells: []sheet.Cell{{Column: 1, Value: sheet.Text("Share")}}},
		},
	}}}
	xml := part(t, write(t, wb), "xl/sharedStrings.xml")
	if !strings.Contains(xml, `count="3" uniqueCount="2"`) {
		t.Errorf("sharedStrings.xml does not state count=3 uniqueCount=2:\n%s", xml)
	}
}

// TestComments_PairsWithALegacyVMLDrawing checks that a comment produces both
// halves the mechanism needs: the comments part carrying the text, and the
// legacy VML drawing carrying the shape a reader draws the note's flag and
// flyout from. A comments part with no matching drawing is text a reader
// never shows.
func TestComments_PairsWithALegacyVMLDrawing(t *testing.T) {
	wb := &sheet.Workbook{Sheets: []sheet.Sheet{{
		Name:     "Sheet1",
		Rows:     []sheet.Row{{Index: 1, Cells: []sheet.Cell{{Column: 1, Value: sheet.Text("x")}}}},
		Comments: []sheet.Comment{{Row: 1, Col: 1, Author: "Vellum", Text: "a note"}},
	}}}
	raw := write(t, wb)

	comments := part(t, raw, "xl/comments1.xml")
	if !strings.Contains(comments, "a note") || !strings.Contains(comments, "Vellum") {
		t.Errorf("comments1.xml does not carry the text and author:\n%s", comments)
	}

	vml := part(t, raw, "xl/drawings/vmlDrawing1.vml")
	if !strings.Contains(vml, `<x:Row>0</x:Row>`) || !strings.Contains(vml, `<x:Column>0</x:Column>`) {
		t.Errorf("vmlDrawing1.vml does not anchor a shape at row 0, column 0:\n%s", vml)
	}

	sheetXML := sheetPart(t, raw, 0)
	if !strings.Contains(sheetXML, "<legacyDrawing") {
		t.Errorf("sheet1.xml does not reference the legacy drawing:\n%s", sheetXML)
	}
}

// TestProvenance_ReachesCustomProperties checks the same embedding mechanism
// [doc] and [deck] already carry: a consumer reads it in File > Info >
// Properties > Advanced without knowing anything about Vellum.
func TestProvenance_ReachesCustomProperties(t *testing.T) {
	wb := &sheet.Workbook{
		Sheets: []sheet.Sheet{{Name: "Sheet1"}},
		Provenance: &provenance.Record{
			VellumVersion:   "0.0.0-test",
			SourceDateEpoch: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC),
			SpecHash:        "abc123",
		},
	}
	raw := write(t, wb)
	custom := part(t, raw, "docProps/custom.xml")
	if !strings.Contains(custom, "VellumVersion") || !strings.Contains(custom, "0.0.0-test") {
		t.Errorf("docProps/custom.xml does not carry the provenance record:\n%s", custom)
	}
	if !strings.Contains(custom, "abc123") {
		t.Errorf("docProps/custom.xml does not carry the spec hash:\n%s", custom)
	}
}

// TestPackage_OmitsCustomPropertiesWithNoProvenance checks the absence half:
// a workbook nobody asked to carry provenance should not grow a
// docProps/custom.xml with nothing meaningful in it.
func TestPackage_OmitsCustomPropertiesWithNoProvenance(t *testing.T) {
	wb := &sheet.Workbook{Sheets: []sheet.Sheet{{Name: "Sheet1"}}}
	p, err := wb.Package(sheet.WriteOptions{})
	if err != nil {
		t.Fatalf("Package: %v", err)
	}
	if p.Has("/docProps/custom.xml") {
		t.Error("docProps/custom.xml was written with no provenance record set")
	}
}
