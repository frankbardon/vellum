package bind_test

// E11-S1's own coverage for RepeatTargetTableRow: a real xlsx-shaped fixture
// (workbook + worksheet + table part, wired through actual [opc.Package]
// relationships) run through the full anchor.Discover -> bind.Execute ->
// xmlcopy.Apply pipeline, proving the row-insert repeat, the row/cell
// renumbering, the table's own ref update, and the bottom-of-sheet
// constraint all together. template/splice/xlsx_test.go covers SpliceCell's
// own cell-rendering shapes in isolation; this file is about the repeat
// mechanism generalized in repeat.go.

import (
	"bytes"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/template/bind"
	"github.com/frankbardon/vellum/xmlcopy"
)

const (
	nsSpreadsheetXB = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	nsRelXB         = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	relWorksheetXB  = nsRelXB + "/worksheet"
	relTableXB      = nsRelXB + "/table"

	ctWorkbookXB  = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"
	ctWorksheetXB = "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"
	ctTableXB     = "application/vnd.openxmlformats-officedocument.spreadsheetml.table+xml"

	xbWorkbookPart = "/xl/workbook.xml"
	xbSheetPart    = "/xl/worksheets/sheet1.xml"
	xbTablePart    = "/xl/tables/table1.xml"
)

// buildXLSXRepeatFixture builds a workbook with one sheet carrying a header
// row (row 1), a table's own one sample data row (row 2, "CustomerTable":
// Name/Email/Amount columns over A1:C2), and whatever extraRowsXML the
// caller supplies after it — used to exercise the bottom-of-sheet
// constraint. tableRef lets a caller construct a ref inconsistent with
// exactly one data row when a test wants that (discovery-time rejection is
// template/anchor's own job; this package's own tests always pass a valid
// "A1:C2" unless proving something about repeat execution itself).
func buildXLSXRepeatFixture(t *testing.T, extraRowsXML string) *opc.Package {
	t.Helper()
	pkg := opc.New()

	sheetRID, err := pkg.Relationships(xbWorkbookPart).Add(relWorksheetXB, "worksheets/sheet1.xml", opc.TargetInternal)
	if err != nil {
		t.Fatalf("Add worksheet rel: %v", err)
	}
	wb := xmlDecl() + `<workbook xmlns="` + nsSpreadsheetXB + `" xmlns:r="` + nsRelXB + `">` +
		`<sheets><sheet name="Sheet1" sheetId="1" r:id="` + sheetRID + `"/></sheets></workbook>`
	if err := pkg.Put(&opc.Part{Name: xbWorkbookPart, ContentType: ctWorkbookXB, Data: []byte(wb)}); err != nil {
		t.Fatalf("Put workbook.xml: %v", err)
	}

	tableRID, err := pkg.Relationships(xbSheetPart).Add(relTableXB, "../tables/table1.xml", opc.TargetInternal)
	if err != nil {
		t.Fatalf("Add table rel: %v", err)
	}
	table := xmlDecl() + `<table xmlns="` + nsSpreadsheetXB + `" id="1" name="CustomerTable" displayName="CustomerTable" ref="A1:C2">` +
		`<tableColumns count="3">` +
		`<tableColumn id="1" name="Name"/><tableColumn id="2" name="Email"/><tableColumn id="3" name="Amount"/>` +
		`</tableColumns></table>`
	if err := pkg.Put(&opc.Part{Name: xbTablePart, ContentType: ctTableXB, Data: []byte(table)}); err != nil {
		t.Fatalf("Put table1.xml: %v", err)
	}

	header := `<row r="1"><c r="A1" t="inlineStr"><is><t>Name</t></is></c>` +
		`<c r="B1" t="inlineStr"><is><t>Email</t></is></c>` +
		`<c r="C1" t="inlineStr"><is><t>Amount</t></is></c></row>`
	sample := `<row r="2"><c r="A2" s="1" t="inlineStr"><is><t>placeholder</t></is></c>` +
		`<c r="B2" s="1" t="inlineStr"><is><t>placeholder</t></is></c>` +
		`<c r="C2" s="1"><v>0</v></c></row>`
	sheetData := header + sample + extraRowsXML

	ws := xmlDecl() + `<worksheet xmlns="` + nsSpreadsheetXB + `" xmlns:r="` + nsRelXB + `">` +
		`<sheetData>` + sheetData + `</sheetData>` +
		`<tableParts count="1"><tablePart r:id="` + tableRID + `"/></tableParts>` +
		`</worksheet>`
	if err := pkg.Put(&opc.Part{Name: xbSheetPart, ContentType: ctWorksheetXB, Data: []byte(ws)}); err != nil {
		t.Fatalf("Put sheet1.xml: %v", err)
	}

	return pkg
}

func xmlDecl() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"
}

func discoverXLSXFrame(t *testing.T, pkg *opc.Package) bind.Frame {
	t.Helper()
	inv, err := anchor.Discover(pkg, artifact.FormatXLSX, xbWorkbookPart)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	anchors := make(map[string]anchor.Anchor, len(inv.Anchors))
	for _, a := range inv.Anchors {
		anchors[a.Name] = a
	}
	return bind.Frame{SrcPkg: pkg, Anchors: anchors}
}

func tableRowRepeatStatement() bind.Statement {
	return bind.Statement{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
		Over: "items", As: "item", Target: bind.RepeatTargetTableRow,
		Body: []bind.Statement{
			{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "CustomerTable.Name", Expr: "item.name"}},
			{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "CustomerTable.Email", Expr: "item.email"}},
			{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "CustomerTable.Amount", Expr: "item.amount"}},
		},
	}}
}

func threeCustomerItems() []any {
	return []any{
		map[string]any{"name": "Alice", "email": "alice@example.com", "amount": 10.0},
		map[string]any{"name": "Bob", "email": "bob@example.com", "amount": 20.0},
		map[string]any{"name": "Cara", "email": "cara@example.com", "amount": 30.0},
	}
}

func runXLSXRepeat(t *testing.T, pkg *opc.Package, items []any) (sheet, table []byte, err error) {
	t.Helper()
	frame := discoverXLSXFrame(t, pkg)
	repls := bind.NewReplacementSet()
	ev := bind.NewFEELEvaluator()
	stmts := []bind.Statement{tableRowRepeatStatement()}
	data := bind.Scope{"items": items}
	if execErr := bind.Execute(stmts, data, ev, frame, pkg, repls); execErr != nil {
		return nil, nil, execErr
	}

	sheetSrc, ok := pkg.Get(xbSheetPart)
	if !ok {
		t.Fatalf("package missing %s", xbSheetPart)
	}
	sheetBytes, e := sheetSrc.Bytes()
	if e != nil {
		t.Fatalf("Bytes: %v", e)
	}
	sheet, e = xmlcopy.Apply(sheetBytes, repls.For(xbSheetPart))
	if e != nil {
		t.Fatalf("Apply(sheet): %v", e)
	}

	tableSrc, ok := pkg.Get(xbTablePart)
	if !ok {
		t.Fatalf("package missing %s", xbTablePart)
	}
	tableBytes, e := tableSrc.Bytes()
	if e != nil {
		t.Fatalf("Bytes: %v", e)
	}
	table, e = xmlcopy.Apply(tableBytes, repls.For(xbTablePart))
	if e != nil {
		t.Fatalf("Apply(table): %v", e)
	}
	return sheet, table, nil
}

func TestExecute_TableRowRepeatThreeItems(t *testing.T) {
	pkg := buildXLSXRepeatFixture(t, "")
	sheet, table, err := runXLSXRepeat(t, pkg, threeCustomerItems())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if err := xmlcopy.Walk(sheet, func(xmlcopy.Element) error { return nil }); err != nil {
		t.Fatalf("filled sheet does not parse: %v\n%s", err, sheet)
	}
	if err := xmlcopy.Walk(table, func(xmlcopy.Element) error { return nil }); err != nil {
		t.Fatalf("filled table does not parse: %v\n%s", err, table)
	}

	rows := collectRowsXB(t, sheet)
	// header (row 1) + 3 data rows (2, 3, 4).
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4: %q", len(rows), rowRefsXB(rows))
	}
	wantRefs := []string{"1", "2", "3", "4"}
	for i, r := range rows {
		if ref, _ := attrOf(r, "r"); ref != wantRefs[i] {
			t.Errorf("row %d: r = %q, want %q", i, ref, wantRefs[i])
		}
	}

	wantCells := map[string]string{
		"A2": "Alice", "B2": "alice@example.com",
		"A3": "Bob", "B3": "bob@example.com",
		"A4": "Cara", "B4": "cara@example.com",
	}
	for ref, want := range wantCells {
		if got := cellTextXB(t, sheet, ref); got != want {
			t.Errorf("cell %s = %q, want %q", ref, got, want)
		}
	}
	for _, ref := range []string{"C2", "C3", "C4"} {
		if !containsXB(sheet, `<c r="`+ref+`" s="1"><v>`) {
			t.Errorf("cell %s missing its own numeric value or lost its style: %s", ref, sheet)
		}
	}

	if !containsXB(table, `ref="A1:C4"`) {
		t.Errorf("table ref not updated to A1:C4: %s", table)
	}
}

func TestExecute_TableRowRepeatZeroItems(t *testing.T) {
	pkg := buildXLSXRepeatFixture(t, "")
	sheet, table, err := runXLSXRepeat(t, pkg, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	rows := collectRowsXB(t, sheet)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1 (header only): %q", len(rows), rowRefsXB(rows))
	}
	if !containsXB(table, `ref="A1:C1"`) {
		t.Errorf("table ref not shrunk to header-only A1:C1: %s", table)
	}
}

func TestExecute_TableRowRepeatBottomOfSheetSatisfied(t *testing.T) {
	// Nothing below the sample row: succeeds.
	pkg := buildXLSXRepeatFixture(t, "")
	if _, _, err := runXLSXRepeat(t, pkg, threeCustomerItems()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestExecute_TableRowRepeatBottomOfSheetViolationRejected(t *testing.T) {
	extra := `<row r="3"><c r="A3" t="inlineStr"><is><t>something else</t></is></c></row>`
	pkg := buildXLSXRepeatFixture(t, extra)

	_, _, err := runXLSXRepeat(t, pkg, threeCustomerItems())
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_TABLE_NOT_AT_SHEET_BOTTOM) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_TABLE_NOT_AT_SHEET_BOTTOM", err)
	}
}

// --- small xlsx-flavoured helpers, local to this file --------------------

func collectRowsXB(t *testing.T, src []byte) []xmlcopy.Element {
	t.Helper()
	var rows []xmlcopy.Element
	if err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if e.Name.Space == nsSpreadsheetXB && e.Name.Local == "row" {
			rows = append(rows, e)
		}
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	return rows
}

func rowRefsXB(rows []xmlcopy.Element) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		v, _ := attrOf(r, "r")
		out[i] = v
	}
	return out
}

func attrOf(e xmlcopy.Element, local string) (string, bool) {
	for _, a := range e.Attr {
		if a.Name.Space == "" && a.Name.Local == local {
			return a.Value, true
		}
	}
	return "", false
}

// cellTextXB returns the plain-text content of an inline-string cell's own
// <is><t>...</t></is>, or "" if ref is not found or is not an inline string.
func cellTextXB(t *testing.T, src []byte, ref string) string {
	t.Helper()
	var cell xmlcopy.Element
	found := false
	if err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if found || e.Name.Space != nsSpreadsheetXB || e.Name.Local != "c" {
			return nil
		}
		if v, ok := attrOf(e, "r"); ok && v == ref {
			cell = e
			found = true
		}
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if !found {
		return ""
	}
	// Walking a sub-slice starting mid-document would lose the ancestor
	// xmlns declaration that resolves <t>'s own default namespace (the same
	// hazard template/defrag's own Flatten doc comment names), so this walks
	// the whole of src again and filters by position instead.
	var text string
	if err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if e.Name.Space != nsSpreadsheetXB || e.Name.Local != "t" {
			return nil
		}
		if e.Content.Start >= cell.Content.Start && e.Content.End <= cell.Content.End {
			text = string(src[e.Content.Start:e.Content.End])
		}
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	return text
}

func containsXB(src []byte, needle string) bool {
	return bytes.Contains(src, []byte(needle))
}
