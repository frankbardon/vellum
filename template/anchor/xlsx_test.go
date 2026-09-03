package anchor_test

// Real-XLSX-shaped fixtures for E11-S1's discoverXLSX: a genuine
// xl/workbook.xml plus xl/worksheets/sheetN.xml plus (for the table cases)
// xl/tables/table1.xml, wired together through actual [opc.Package]
// relationships rather than hardcoded part-name guesses — the same
// resolve-through-the-relationship-graph discipline template.Open itself
// uses, exercised here at the level anchor.Discover actually reads it.

import (
	"testing"

	"github.com/frankbardon/vellum/artifact"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/template/anchor"
)

const (
	nsSpreadsheetT   = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	nsRelationshipsT = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	relWorksheetT    = nsRelationshipsT + "/worksheet"
	relTableT        = nsRelationshipsT + "/table"

	ctWorkbook  = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"
	ctWorksheet = "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"
	ctTable     = "application/vnd.openxmlformats-officedocument.spreadsheetml.table+xml"
)

func xmlDeclT() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"
}

func workbookXML(sheetsXML, definedNamesXML string) string {
	return xmlDeclT() +
		`<workbook xmlns="` + nsSpreadsheetT + `" xmlns:r="` + nsRelationshipsT + `">` +
		`<sheets>` + sheetsXML + `</sheets>` +
		definedNamesXML +
		`</workbook>`
}

func worksheetXML(sheetDataXML, tablePartsXML string) string {
	return xmlDeclT() +
		`<worksheet xmlns="` + nsSpreadsheetT + `" xmlns:r="` + nsRelationshipsT + `">` +
		`<sheetData>` + sheetDataXML + `</sheetData>` +
		tablePartsXML +
		`</worksheet>`
}

func tableXML(displayName, ref, columnsXML string) string {
	return xmlDeclT() +
		`<table xmlns="` + nsSpreadsheetT + `" id="1" name="` + displayName +
		`" displayName="` + displayName + `" ref="` + ref + `">` +
		`<tableColumns count="1">` + columnsXML + `</tableColumns>` +
		`</table>`
}

// xlsxFixture builds a minimal but realistic .xlsx-shaped package: a
// workbook part (with the given sheets/defined-names bodies) plus one
// worksheet part, wired with real relationships. It returns the package and
// the workbook part's own name, the pair anchor.Discover needs.
type xlsxFixture struct {
	t             *testing.T
	pkg           *opc.Package
	sheetRelID    string
	tableRelID    string
	workbookPart  string
	worksheetPart string
	tablePart     string
}

func newXLSXFixture(t *testing.T) *xlsxFixture {
	t.Helper()
	return &xlsxFixture{
		t:             t,
		pkg:           opc.New(),
		workbookPart:  "/xl/workbook.xml",
		worksheetPart: "/xl/worksheets/sheet1.xml",
		tablePart:     "/xl/tables/table1.xml",
	}
}

// build assembles the fixture: sheetDataXML is the worksheet's own
// <sheetData> content, definedNamesXML is the workbook's own <definedNames>
// element (or "" for none), and withTable, when true, wires a <tableParts>
// reference from the worksheet to a table part built from tableDisplayName,
// tableRef and tableColumnsXML.
func (f *xlsxFixture) build(sheetDataXML, definedNamesXML string, withTable bool, tableDisplayName, tableRef, tableColumnsXML string) (*opc.Package, string) {
	f.t.Helper()

	sheetRelID, err := f.pkg.Relationships(f.workbookPart).Add(relWorksheetT, "worksheets/sheet1.xml", opc.TargetInternal)
	if err != nil {
		f.t.Fatalf("Add worksheet relationship: %v", err)
	}
	f.sheetRelID = sheetRelID

	sheetsXML := `<sheet name="Sheet1" sheetId="1" r:id="` + sheetRelID + `"/>`
	if err := f.pkg.Put(&opc.Part{
		Name: f.workbookPart, ContentType: ctWorkbook,
		Data: []byte(workbookXML(sheetsXML, definedNamesXML)),
	}); err != nil {
		f.t.Fatalf("Put workbook.xml: %v", err)
	}

	tablePartsXML := ""
	if withTable {
		tableRelID, err := f.pkg.Relationships(f.worksheetPart).Add(relTableT, "../tables/table1.xml", opc.TargetInternal)
		if err != nil {
			f.t.Fatalf("Add table relationship: %v", err)
		}
		f.tableRelID = tableRelID
		tablePartsXML = `<tableParts count="1"><tablePart r:id="` + tableRelID + `"/></tableParts>`

		if err := f.pkg.Put(&opc.Part{
			Name: f.tablePart, ContentType: ctTable,
			Data: []byte(tableXML(tableDisplayName, tableRef, tableColumnsXML)),
		}); err != nil {
			f.t.Fatalf("Put table1.xml: %v", err)
		}
	}

	if err := f.pkg.Put(&opc.Part{
		Name: f.worksheetPart, ContentType: ctWorksheet,
		Data: []byte(worksheetXML(sheetDataXML, tablePartsXML)),
	}); err != nil {
		f.t.Fatalf("Put sheet1.xml: %v", err)
	}

	return f.pkg, f.workbookPart
}

func discoverXLSXT(t *testing.T, sheetDataXML, definedNamesXML string) (*anchor.Inventory, error) {
	t.Helper()
	f := newXLSXFixture(t)
	pkg, mainPart := f.build(sheetDataXML, definedNamesXML, false, "", "", "")
	return anchor.Discover(pkg, artifact.FormatXLSX, mainPart)
}

func definedNames(entries ...string) string {
	if len(entries) == 0 {
		return ""
	}
	out := "<definedNames>"
	for _, e := range entries {
		out += e
	}
	return out + "</definedNames>"
}

func definedName(name, formula string) string {
	return `<definedName name="` + name + `">` + formula + `</definedName>`
}

// --- defined name discovery -------------------------------------------------

func TestDiscoverXLSX_DefinedNameResolvesToCellAnchor(t *testing.T) {
	inv, err := discoverXLSXT(t,
		`<row r="2"><c r="B2" s="3" t="inlineStr"><is><t>placeholder</t></is></c></row>`,
		definedNames(definedName("CustomerName", "Sheet1!$B$2")))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Anchors) != 1 {
		t.Fatalf("got %d anchors, want 1: %+v", len(inv.Anchors), inv.Anchors)
	}
	a := inv.Anchors[0]
	if a.Kind != anchor.KindDefinedName {
		t.Errorf("kind = %q, want defined_name", a.Kind)
	}
	if a.Name != "CustomerName" {
		t.Errorf("name = %q, want CustomerName", a.Name)
	}
	if a.Part != "/xl/worksheets/sheet1.xml" {
		t.Errorf("part = %q", a.Part)
	}
	if a.Span.Start == 0 && a.Span.End == 0 {
		t.Error("span is zero-valued")
	}
	if a.Table != nil {
		t.Errorf("Table = %+v, want nil for a defined-name anchor", a.Table)
	}
}

func TestDiscoverXLSX_DefinedNameQuotedSheetName(t *testing.T) {
	f := newXLSXFixture(t)
	f.workbookPart = "/xl/workbook.xml"
	// Rebuild manually since the sheet name itself needs a space, which the
	// shared fixture helper's <sheet name="Sheet1"...> does not carry.
	pkg := opc.New()
	rid, err := pkg.Relationships("/xl/workbook.xml").Add(relWorksheetT, "worksheets/sheet1.xml", opc.TargetInternal)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	wb := xmlDeclT() + `<workbook xmlns="` + nsSpreadsheetT + `" xmlns:r="` + nsRelationshipsT + `">` +
		`<sheets><sheet name="My Sheet" sheetId="1" r:id="` + rid + `"/></sheets>` +
		definedNames(definedName("Total", "'My Sheet'!$C$5")) +
		`</workbook>`
	if err := pkg.Put(&opc.Part{Name: "/xl/workbook.xml", ContentType: ctWorkbook, Data: []byte(wb)}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	ws := worksheetXML(`<row r="5"><c r="C5"><v>0</v></c></row>`, "")
	if err := pkg.Put(&opc.Part{Name: "/xl/worksheets/sheet1.xml", ContentType: ctWorksheet, Data: []byte(ws)}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	inv, err := anchor.Discover(pkg, artifact.FormatXLSX, "/xl/workbook.xml")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Anchors) != 1 || inv.Anchors[0].Name != "Total" {
		t.Fatalf("Anchors = %+v, want one named Total", inv.Anchors)
	}
}

func TestDiscoverXLSX_DefinedNameUnsupportedShapeRejected(t *testing.T) {
	cases := []struct {
		name    string
		formula string
	}{
		{"relative reference", "Sheet1!B2"},
		{"multi-cell range", "Sheet1!$B$2:$B$4"},
		{"reference to another name", "OtherName"},
		{"no sheet qualifier", "$B$2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := discoverXLSXT(t,
				`<row r="2"><c r="B2"><v>0</v></c></row>`,
				definedNames(definedName("X", c.formula)))
			if !verr.HasCode(err, verr.VELLUM_ANCHOR_DEFINED_NAME_UNSUPPORTED) {
				t.Fatalf("formula %q: err = %v, want VELLUM_ANCHOR_DEFINED_NAME_UNSUPPORTED", c.formula, err)
			}
		})
	}
}

func TestDiscoverXLSX_DefinedNameMissingTargetCellRejected(t *testing.T) {
	// No <c r="B2"> anywhere in the worksheet.
	_, err := discoverXLSXT(t,
		`<row r="2"><c r="A2"><v>0</v></c></row>`,
		definedNames(definedName("CustomerName", "Sheet1!$B$2")))
	if !verr.HasCode(err, verr.VELLUM_ANCHOR_DEFINED_NAME_UNSUPPORTED) {
		t.Fatalf("err = %v, want VELLUM_ANCHOR_DEFINED_NAME_UNSUPPORTED", err)
	}
}

// --- table column discovery -------------------------------------------------

func discoverXLSXTableT(t *testing.T, sheetDataXML, displayName, ref, columnsXML string) (*anchor.Inventory, error) {
	t.Helper()
	f := newXLSXFixture(t)
	pkg, mainPart := f.build(sheetDataXML, "", true, displayName, ref, columnsXML)
	return anchor.Discover(pkg, artifact.FormatXLSX, mainPart)
}

func tableColumnsXML(names ...string) string {
	out := ""
	for i, n := range names {
		out += `<tableColumn id="` + itoaT(i+1) + `" name="` + n + `"/>`
	}
	return out
}

func itoaT(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func TestDiscoverXLSX_TableColumnsDiscovered(t *testing.T) {
	sheetData := `<row r="1"><c r="A1" t="inlineStr"><is><t>Name</t></is></c>` +
		`<c r="B1" t="inlineStr"><is><t>Email</t></is></c>` +
		`<c r="C1" t="inlineStr"><is><t>Amount</t></is></c></row>` +
		`<row r="2"><c r="A2" s="1" t="inlineStr"><is><t>placeholder</t></is></c>` +
		`<c r="B2" s="1" t="inlineStr"><is><t>placeholder</t></is></c>` +
		`<c r="C2" s="1"><v>0</v></c></row>`

	inv, err := discoverXLSXTableT(t, sheetData, "CustomerTable", "A1:C2",
		tableColumnsXML("Name", "Email", "Amount"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Anchors) != 3 {
		t.Fatalf("got %d anchors, want 3: %+v", len(inv.Anchors), inv.Anchors)
	}

	want := map[string]int{
		"CustomerTable.Name":   1,
		"CustomerTable.Email":  2,
		"CustomerTable.Amount": 3,
	}
	for _, a := range inv.Anchors {
		if a.Kind != anchor.KindTableColumn {
			t.Errorf("%s: kind = %q, want table_column", a.Name, a.Kind)
		}
		wantCol, ok := want[a.Name]
		if !ok {
			t.Fatalf("unexpected anchor name %q", a.Name)
		}
		delete(want, a.Name)
		if a.Table == nil {
			t.Fatalf("%s: Table is nil", a.Name)
		}
		if a.Table.Column != wantCol {
			t.Errorf("%s: Table.Column = %d, want %d", a.Name, a.Table.Column, wantCol)
		}
		if a.Table.DisplayName != "CustomerTable" {
			t.Errorf("%s: Table.DisplayName = %q", a.Name, a.Table.DisplayName)
		}
		if a.Table.TablePart != "/xl/tables/table1.xml" {
			t.Errorf("%s: Table.TablePart = %q", a.Name, a.Table.TablePart)
		}
		if a.Table.HeaderRow != 1 || a.Table.FromColumn != 1 || a.Table.ToColumn != 3 {
			t.Errorf("%s: Table = %+v, want HeaderRow=1 FromColumn=1 ToColumn=3", a.Name, a.Table)
		}
	}
	if len(want) != 0 {
		t.Errorf("missing anchors: %v", want)
	}

	// All three columns' anchors share the same row Span (their common
	// container for a table_row repeat).
	if inv.Anchors[0].Span != inv.Anchors[1].Span || inv.Anchors[1].Span != inv.Anchors[2].Span {
		t.Errorf("table column anchors do not share one Span: %+v", inv.Anchors)
	}
}

func TestDiscoverXLSX_TableMoreThanOneDataRowRejected(t *testing.T) {
	sheetData := `<row r="1"><c r="A1" t="inlineStr"><is><t>Name</t></is></c></row>` +
		`<row r="2"><c r="A2" t="inlineStr"><is><t>x</t></is></c></row>` +
		`<row r="3"><c r="A3" t="inlineStr"><is><t>y</t></is></c></row>`

	_, err := discoverXLSXTableT(t, sheetData, "CustomerTable", "A1:A3", tableColumnsXML("Name"))
	if !verr.HasCode(err, verr.VELLUM_ANCHOR_TABLE_UNSUPPORTED) {
		t.Fatalf("err = %v, want VELLUM_ANCHOR_TABLE_UNSUPPORTED", err)
	}
}

func TestDiscoverXLSX_TableZeroDataRowsRejected(t *testing.T) {
	sheetData := `<row r="1"><c r="A1" t="inlineStr"><is><t>Name</t></is></c></row>`

	_, err := discoverXLSXTableT(t, sheetData, "CustomerTable", "A1:A1", tableColumnsXML("Name"))
	if !verr.HasCode(err, verr.VELLUM_ANCHOR_TABLE_UNSUPPORTED) {
		t.Fatalf("err = %v, want VELLUM_ANCHOR_TABLE_UNSUPPORTED", err)
	}
}

func TestDiscoverXLSX_TableMissingPlaceholderCellRejected(t *testing.T) {
	sheetData := `<row r="1"><c r="A1" t="inlineStr"><is><t>Name</t></is></c>` +
		`<c r="B1" t="inlineStr"><is><t>Email</t></is></c></row>` +
		// Data row is missing a B2 cell entirely.
		`<row r="2"><c r="A2" t="inlineStr"><is><t>placeholder</t></is></c></row>`

	_, err := discoverXLSXTableT(t, sheetData, "CustomerTable", "A1:B2", tableColumnsXML("Name", "Email"))
	if !verr.HasCode(err, verr.VELLUM_ANCHOR_TABLE_UNSUPPORTED) {
		t.Fatalf("err = %v, want VELLUM_ANCHOR_TABLE_UNSUPPORTED", err)
	}
}

func TestDiscoverXLSX_DuplicateAnchorNameAcrossKindsRejected(t *testing.T) {
	f := newXLSXFixture(t)
	sheetData := `<row r="1"><c r="A1" t="inlineStr"><is><t>Name</t></is></c></row>` +
		`<row r="2"><c r="A2" t="inlineStr"><is><t>placeholder</t></is></c>` +
		`<c r="B2"><v>0</v></c></row>`
	pkg, mainPart := f.build(sheetData,
		definedNames(definedName("CustomerTable.Name", "Sheet1!$B$2")),
		true, "CustomerTable", "A1:A2", tableColumnsXML("Name"))

	_, err := anchor.Discover(pkg, artifact.FormatXLSX, mainPart)
	if !verr.HasCode(err, verr.VELLUM_ANCHOR_DUPLICATE) {
		t.Fatalf("err = %v, want VELLUM_ANCHOR_DUPLICATE", err)
	}
}
