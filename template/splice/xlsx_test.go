package splice_test

// Unit coverage for E11-S1's xlsx splice strategies, exercised directly
// against splice.SpliceCell rather than through the whole fill stack — the
// end-to-end round trip (discovery -> reconcile -> execute -> apply,
// including a real table_row repeat) lives in template/fill_xlsx_test.go.

import (
	"bytes"
	"strconv"
	"testing"
	"time"

	"github.com/frankbardon/vellum/numfmt"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/template/splice"
	"github.com/frankbardon/vellum/xmlcopy"
)

const (
	nsSpreadsheetXT = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	ctWorksheetXT   = "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"
	partSheet1      = "/xl/worksheets/sheet1.xml"
)

func worksheetXMLXT(sheetDataXML string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<worksheet xmlns="` + nsSpreadsheetXT + `"><sheetData>` + sheetDataXML + `</sheetData></worksheet>`)
}

func buildSheetPackage(t *testing.T, sheetDataXML string) *opc.Package {
	t.Helper()
	p := opc.New()
	if err := p.Put(&opc.Part{Name: partSheet1, ContentType: ctWorksheetXT, Data: worksheetXMLXT(sheetDataXML)}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	return p
}

func cellSpanXT(t *testing.T, src []byte, ref string) xmlcopy.Span {
	t.Helper()
	var found xmlcopy.Span
	ok := false
	if err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if e.Name.Space != nsSpreadsheetXT || e.Name.Local != "c" {
			return nil
		}
		for _, a := range e.Attr {
			if a.Name.Space == "" && a.Name.Local == "r" && a.Value == ref {
				found = e.Span
				ok = true
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if !ok {
		t.Fatalf("cell %q not found", ref)
	}
	return found
}

func rowSpanXT(t *testing.T, src []byte, r string) xmlcopy.Span {
	t.Helper()
	var found xmlcopy.Span
	ok := false
	if err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if e.Name.Space != nsSpreadsheetXT || e.Name.Local != "row" {
			return nil
		}
		for _, a := range e.Attr {
			if a.Name.Space == "" && a.Name.Local == "r" && a.Value == r {
				found = e.Span
				ok = true
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if !ok {
		t.Fatalf("row %q not found", r)
	}
	return found
}

func mustApplyXT(t *testing.T, src []byte, repl xmlcopy.Replacement) []byte {
	t.Helper()
	out, err := xmlcopy.Apply(src, []xmlcopy.Replacement{repl})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := xmlcopy.Walk(out, func(xmlcopy.Element) error { return nil }); err != nil {
		t.Fatalf("output does not parse: %v\n%s", err, out)
	}
	return out
}

// --- KindDefinedName ---------------------------------------------------

func TestSpliceCell_DefinedNameText(t *testing.T) {
	src := worksheetXMLXT(`<row r="2"><c r="B2" s="3" t="inlineStr"><is><t>placeholder</t></is></c></row>`)
	pkg := buildSheetPackage(t, `<row r="2"><c r="B2" s="3" t="inlineStr"><is><t>placeholder</t></is></c></row>`)
	a := anchor.Anchor{Name: "CustomerName", Kind: anchor.KindDefinedName, Part: partSheet1, Span: cellSpanXT(t, src, "B2")}

	repl, err := splice.SpliceCell(pkg, a, numfmt.Value{Kind: numfmt.KindText, Text: "Acme & Co."})
	if err != nil {
		t.Fatalf("SpliceCell: %v", err)
	}
	out := mustApplyXT(t, src, repl)

	want := `<c r="B2" s="3" t="inlineStr"><is><t xml:space="preserve">Acme &amp; Co.</t></is></c>`
	if !contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
}

func TestSpliceCell_DefinedNameNumber(t *testing.T) {
	src := worksheetXMLXT(`<row r="2"><c r="B2" s="1"><v>0</v></c></row>`)
	pkg := buildSheetPackage(t, `<row r="2"><c r="B2" s="1"><v>0</v></c></row>`)
	a := anchor.Anchor{Name: "Total", Kind: anchor.KindDefinedName, Part: partSheet1, Span: cellSpanXT(t, src, "B2")}

	repl, err := splice.SpliceCell(pkg, a, numfmt.Value{Kind: numfmt.KindNumber, Number: 42.5})
	if err != nil {
		t.Fatalf("SpliceCell: %v", err)
	}
	out := mustApplyXT(t, src, repl)

	want := `<c r="B2" s="1"><v>42.5</v></c>`
	if !contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
	if contains(out, ` t="`) {
		t.Errorf("a number cell must carry no t attribute:\n%s", out)
	}
}

func TestSpliceCell_DefinedNameBool(t *testing.T) {
	src := worksheetXMLXT(`<row r="2"><c r="B2"><v>0</v></c></row>`)
	pkg := buildSheetPackage(t, `<row r="2"><c r="B2"><v>0</v></c></row>`)
	a := anchor.Anchor{Name: "IsActive", Kind: anchor.KindDefinedName, Part: partSheet1, Span: cellSpanXT(t, src, "B2")}

	repl, err := splice.SpliceCell(pkg, a, numfmt.Value{Kind: numfmt.KindBool, Bool: true})
	if err != nil {
		t.Fatalf("SpliceCell: %v", err)
	}
	out := mustApplyXT(t, src, repl)

	want := `<c r="B2" t="b"><v>1</v></c>`
	if !contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
}

func TestSpliceCell_DefinedNameDate(t *testing.T) {
	src := worksheetXMLXT(`<row r="2"><c r="B2" s="5"><v>0</v></c></row>`)
	pkg := buildSheetPackage(t, `<row r="2"><c r="B2" s="5"><v>0</v></c></row>`)
	a := anchor.Anchor{Name: "AsOf", Kind: anchor.KindDefinedName, Part: partSheet1, Span: cellSpanXT(t, src, "B2")}

	when := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	repl, err := splice.SpliceCell(pkg, a, numfmt.Value{Kind: numfmt.KindDate, Time: when})
	if err != nil {
		t.Fatalf("SpliceCell: %v", err)
	}
	out := mustApplyXT(t, src, repl)

	wantSerial := numfmt.Serial(when)
	want := `<c r="B2" s="5"><v>` + formatFloat(wantSerial) + `</v></c>`
	if !contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
}

func TestSpliceCell_DefinedNameNoStyleAttrPreserved(t *testing.T) {
	src := worksheetXMLXT(`<row r="2"><c r="B2"/></row>`)
	pkg := buildSheetPackage(t, `<row r="2"><c r="B2"/></row>`)
	a := anchor.Anchor{Name: "X", Kind: anchor.KindDefinedName, Part: partSheet1, Span: cellSpanXT(t, src, "B2")}

	repl, err := splice.SpliceCell(pkg, a, numfmt.Value{Kind: numfmt.KindText, Text: "hi"})
	if err != nil {
		t.Fatalf("SpliceCell: %v", err)
	}
	out := mustApplyXT(t, src, repl)
	if contains(out, ` s="`) {
		t.Errorf("no s attribute should have been introduced:\n%s", out)
	}
}

// --- KindTableColumn -----------------------------------------------------

func TestSpliceCell_TableColumnLocatesRightCell(t *testing.T) {
	sheetData := `<row r="2"><c r="A2" s="1" t="inlineStr"><is><t>placeholder</t></is></c>` +
		`<c r="B2" s="1" t="inlineStr"><is><t>placeholder</t></is></c>` +
		`<c r="C2" s="1"><v>0</v></c></row>`
	src := worksheetXMLXT(sheetData)
	pkg := buildSheetPackage(t, sheetData)
	rowSpan := rowSpanXT(t, src, "2")

	a := anchor.Anchor{
		Name: "CustomerTable.Email", Kind: anchor.KindTableColumn, Part: partSheet1, Span: rowSpan,
		Table: &anchor.TableColumn{DisplayName: "CustomerTable", Column: 2, TablePart: "/xl/tables/table1.xml", HeaderRow: 1, FromColumn: 1, ToColumn: 3},
	}

	repl, err := splice.SpliceCell(pkg, a, numfmt.Value{Kind: numfmt.KindText, Text: "jane@example.com"})
	if err != nil {
		t.Fatalf("SpliceCell: %v", err)
	}
	out := mustApplyXT(t, src, repl)

	if !contains(out, `<c r="B2" s="1" t="inlineStr"><is><t xml:space="preserve">jane@example.com</t></is></c>`) {
		t.Errorf("B2 not spliced correctly:\n%s", out)
	}
	// A2 and C2 are untouched.
	if !contains(out, `<c r="A2" s="1" t="inlineStr"><is><t>placeholder</t></is></c>`) {
		t.Errorf("A2 was unexpectedly touched:\n%s", out)
	}
	if !contains(out, `<c r="C2" s="1"><v>0</v></c>`) {
		t.Errorf("C2 was unexpectedly touched:\n%s", out)
	}
}

func contains(haystack []byte, needle string) bool {
	return bytes.Contains(haystack, []byte(needle))
}

// formatFloat mirrors splice's own unexported formatCellNumber exactly:
// strconv's shortest round-tripping form.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}
