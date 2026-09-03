package template_test

// TestFill_XLSX* exercise E11-S1's xlsx fill mode end to end through the
// same public surface template/fill_test.go already exercises for DOCX:
// template.Open then template.Fill, with the package a real caller writes
// re-opened through opc.Open and every part outside Result.Touched checked
// byte-for-byte against the source — the same non-destructiveness property
// TestNonDestructiveCorpus proves at the lower splice/xmlcopy layer, and
// TestFill_EndToEndMixOfBindIfAndRepeat proves for DOCX, proved again here
// for a workbook template carrying a defined-name bind and a table_row
// repeat together.

import (
	"bytes"
	"testing"

	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/template"
	"github.com/frankbardon/vellum/template/bind"
)

const (
	xlsxRelOfficeDocument = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	xlsxRelWorksheet      = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet"
	xlsxRelTable          = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/table"
	xlsxNSSpreadsheet     = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	xlsxNSRelationships   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

	xlsxCTWorkbook  = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"
	xlsxCTWorksheet = "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"
	xlsxCTTable     = "application/vnd.openxmlformats-officedocument.spreadsheetml.table+xml"
	xlsxCTStyles    = "application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"

	xlsxWorkbookPart = "/xl/workbook.xml"
	xlsxSheetPart    = "/xl/worksheets/sheet1.xml"
	xlsxTablePart    = "/xl/tables/table1.xml"
	xlsxStylesPart   = "/xl/styles.xml"
)

// xlsxStylesXML is an unrelated part Fill has no reason to touch — the
// non-destructiveness target, mirroring fillStylesXML's own role in
// fill_test.go.
const xlsxStylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<styleSheet xmlns="` + xlsxNSSpreadsheet + `"/>`

// buildFillFixtureXLSX assembles a realistic minimal .xlsx-shaped package:
// a workbook with one sheet and one defined name ("CustomerName"), a
// worksheet carrying the defined name's own target cell plus an Excel
// Table's header and one sample data row, the table's own part, and an
// unrelated styles part.
func buildFillFixtureXLSX(t *testing.T) []byte {
	t.Helper()
	pkg := opc.New()

	sheetRID, err := pkg.Relationships(xlsxWorkbookPart).Add(xlsxRelWorksheet, "worksheets/sheet1.xml", opc.TargetInternal)
	if err != nil {
		t.Fatalf("Add worksheet rel: %v", err)
	}
	wb := xmlDeclXLSX() + `<workbook xmlns="` + xlsxNSSpreadsheet + `" xmlns:r="` + xlsxNSRelationships + `">` +
		`<sheets><sheet name="Sheet1" sheetId="1" r:id="` + sheetRID + `"/></sheets>` +
		`<definedNames><definedName name="CustomerName">Sheet1!$B$1</definedName></definedNames>` +
		`</workbook>`
	if err := pkg.Put(&opc.Part{Name: xlsxWorkbookPart, ContentType: xlsxCTWorkbook, Data: []byte(wb)}); err != nil {
		t.Fatalf("Put workbook.xml: %v", err)
	}
	if _, err := pkg.Relationships("/").Add(xlsxRelOfficeDocument, "xl/workbook.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add officeDocument rel: %v", err)
	}

	tableRID, err := pkg.Relationships(xlsxSheetPart).Add(xlsxRelTable, "../tables/table1.xml", opc.TargetInternal)
	if err != nil {
		t.Fatalf("Add table rel: %v", err)
	}
	table := xmlDeclXLSX() + `<table xmlns="` + xlsxNSSpreadsheet + `" id="1" name="CustomerTable" displayName="CustomerTable" ref="A3:B4">` +
		`<tableColumns count="2"><tableColumn id="1" name="Item"/><tableColumn id="2" name="Qty"/></tableColumns></table>`
	if err := pkg.Put(&opc.Part{Name: xlsxTablePart, ContentType: xlsxCTTable, Data: []byte(table)}); err != nil {
		t.Fatalf("Put table1.xml: %v", err)
	}

	// Row 1: the defined name's own target cell, A1 labelled and B1 the
	// value cell. Row 3/4: the table's own header and one sample data row —
	// placed below the defined-name cell but the table's own sample row (4)
	// is still the sheet's last content, satisfying the bottom-of-sheet
	// constraint.
	sheetData := `<row r="1"><c r="A1" t="inlineStr"><is><t>Customer</t></is></c>` +
		`<c r="B1" t="inlineStr"><is><t>placeholder</t></is></c></row>` +
		`<row r="3"><c r="A3" t="inlineStr"><is><t>Item</t></is></c>` +
		`<c r="B3" t="inlineStr"><is><t>Qty</t></is></c></row>` +
		`<row r="4"><c r="A4" s="1" t="inlineStr"><is><t>placeholder</t></is></c>` +
		`<c r="B4" s="1"><v>0</v></c></row>`
	ws := xmlDeclXLSX() + `<worksheet xmlns="` + xlsxNSSpreadsheet + `" xmlns:r="` + xlsxNSRelationships + `">` +
		`<sheetData>` + sheetData + `</sheetData>` +
		`<tableParts count="1"><tablePart r:id="` + tableRID + `"/></tableParts>` +
		`</worksheet>`
	if err := pkg.Put(&opc.Part{Name: xlsxSheetPart, ContentType: xlsxCTWorksheet, Data: []byte(ws)}); err != nil {
		t.Fatalf("Put sheet1.xml: %v", err)
	}

	if err := pkg.Put(&opc.Part{Name: xlsxStylesPart, ContentType: xlsxCTStyles, Data: []byte(xlsxStylesXML)}); err != nil {
		t.Fatalf("Put styles.xml: %v", err)
	}

	var buf bytes.Buffer
	if err := pkg.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

func xmlDeclXLSX() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"
}

func fillFixtureBindingXLSX() *bind.Binding {
	return &bind.Binding{
		FormatVersion: bind.FormatVersion,
		Statements: []bind.Statement{
			{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "CustomerName", Expr: "customer.name"}},
			{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
				Over: "items", As: "item", Target: bind.RepeatTargetTableRow,
				Body: []bind.Statement{
					{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "CustomerTable.Item", Expr: "item.name"}},
					{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "CustomerTable.Qty", Expr: "item.qty"}},
				},
			}},
		},
	}
}

func fillFixtureDataXLSX() bind.Scope {
	return bind.Scope{
		"customer": map[string]any{"name": "Acme & Co."},
		"items": []any{
			map[string]any{"name": "Widget", "qty": 3.0},
			map[string]any{"name": "Gadget", "qty": 5.0},
			map[string]any{"name": "Sprocket", "qty": 9.0},
		},
	}
}

func TestFill_XLSXEndToEndDefinedNameAndTableRowRepeat(t *testing.T) {
	raw := buildFillFixtureXLSX(t)
	srcPkg, err := opc.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("opc.Open on the fixture: %v", err)
	}
	srcStyles := mustPartBytes(t, srcPkg, xlsxStylesPart)
	srcTable := mustPartBytes(t, srcPkg, xlsxTablePart)

	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	res, err := template.Fill(tpl, fillFixtureBindingXLSX(), fillFixtureDataXLSX())
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}

	// --- Touched receipt: the worksheet and the table part, nothing else --
	wantTouched := map[string]bool{xlsxSheetPart: true, xlsxTablePart: true}
	if len(res.Touched) != len(wantTouched) {
		t.Fatalf("Touched = %v, want exactly %v", res.Touched, wantTouched)
	}
	for _, name := range res.Touched {
		if !wantTouched[name] {
			t.Errorf("unexpected touched part %q", name)
		}
	}

	// --- The worksheet carries the filled content -------------------------
	sheet := mustPartBytes(t, res.Package, xlsxSheetPart)
	for _, want := range []string{"Acme &amp; Co.", "Widget", "Gadget", "Sprocket"} {
		if !bytes.Contains(sheet, []byte(want)) {
			t.Errorf("filled sheet missing %q: %s", want, sheet)
		}
	}
	if bytes.Contains(sheet, []byte("placeholder")) {
		t.Errorf("filled sheet still carries a raw placeholder: %s", sheet)
	}
	// Untouched surrounding content survives.
	for _, want := range []string{"Customer", "Item", "Qty"} {
		if !bytes.Contains(sheet, []byte(want)) {
			t.Errorf("filled sheet lost untouched header content %q: %s", want, sheet)
		}
	}

	// --- The table part's own ref grew to cover all three rows ------------
	table := mustPartBytes(t, res.Package, xlsxTablePart)
	if !bytes.Contains(table, []byte(`ref="A3:B6"`)) {
		t.Errorf("table's own ref not updated to A3:B6: %s", table)
	}
	if bytes.Equal(table, srcTable) {
		t.Error("table part reported touched but is byte-identical to the source")
	}

	// --- Non-destructiveness: styles.xml is byte-identical ----------------
	filledStyles := mustPartBytes(t, res.Package, xlsxStylesPart)
	if !bytes.Equal(srcStyles, filledStyles) {
		t.Errorf("styles.xml changed even though it was never in Touched:\nsource: %s\nfilled: %s", srcStyles, filledStyles)
	}

	// --- The result round-trips through opc.Open ---------------------------
	var out bytes.Buffer
	if err := res.Package.WriteTo(&out, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if _, err := opc.Open(bytes.NewReader(out.Bytes()), int64(out.Len())); err != nil {
		t.Fatalf("the filled package does not round-trip through opc.Open: %v", err)
	}

	// --- The originally opened Template is untouched -----------------------
	origSheet := mustPartBytes(t, tpl.Package(), xlsxSheetPart)
	if !bytes.Contains(origSheet, []byte("placeholder")) {
		t.Error("Fill mutated the *Template it was called against; the original placeholder text should still be there")
	}
}

func TestFill_XLSXTableRowRepeatBottomOfSheetViolationIsCodedError(t *testing.T) {
	pkg := opc.New()

	sheetRID, err := pkg.Relationships(xlsxWorkbookPart).Add(xlsxRelWorksheet, "worksheets/sheet1.xml", opc.TargetInternal)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	wb := xmlDeclXLSX() + `<workbook xmlns="` + xlsxNSSpreadsheet + `" xmlns:r="` + xlsxNSRelationships + `">` +
		`<sheets><sheet name="Sheet1" sheetId="1" r:id="` + sheetRID + `"/></sheets></workbook>`
	if err := pkg.Put(&opc.Part{Name: xlsxWorkbookPart, ContentType: xlsxCTWorkbook, Data: []byte(wb)}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := pkg.Relationships("/").Add(xlsxRelOfficeDocument, "xl/workbook.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add: %v", err)
	}

	tableRID, err := pkg.Relationships(xlsxSheetPart).Add(xlsxRelTable, "../tables/table1.xml", opc.TargetInternal)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	table := xmlDeclXLSX() + `<table xmlns="` + xlsxNSSpreadsheet + `" id="1" name="CustomerTable" displayName="CustomerTable" ref="A1:B2">` +
		`<tableColumns count="2"><tableColumn id="1" name="Item"/><tableColumn id="2" name="Qty"/></tableColumns></table>`
	if err := pkg.Put(&opc.Part{Name: xlsxTablePart, ContentType: xlsxCTTable, Data: []byte(table)}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Row 3 sits below the table's own sample row (2): the constraint this
	// story exists to enforce.
	sheetData := `<row r="1"><c r="A1" t="inlineStr"><is><t>Item</t></is></c>` +
		`<c r="B1" t="inlineStr"><is><t>Qty</t></is></c></row>` +
		`<row r="2"><c r="A2" t="inlineStr"><is><t>placeholder</t></is></c>` +
		`<c r="B2"><v>0</v></c></row>` +
		`<row r="3"><c r="A3" t="inlineStr"><is><t>a totals formula lives here</t></is></c></row>`
	ws := xmlDeclXLSX() + `<worksheet xmlns="` + xlsxNSSpreadsheet + `" xmlns:r="` + xlsxNSRelationships + `">` +
		`<sheetData>` + sheetData + `</sheetData>` +
		`<tableParts count="1"><tablePart r:id="` + tableRID + `"/></tableParts>` +
		`</worksheet>`
	if err := pkg.Put(&opc.Part{Name: xlsxSheetPart, ContentType: xlsxCTWorksheet, Data: []byte(ws)}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	var buf bytes.Buffer
	if err := pkg.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	raw := buf.Bytes()

	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	b := &bind.Binding{FormatVersion: bind.FormatVersion, Statements: []bind.Statement{
		{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
			Over: "items", As: "item", Target: bind.RepeatTargetTableRow,
			Body: []bind.Statement{
				{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "CustomerTable.Item", Expr: "item.name"}},
				{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "CustomerTable.Qty", Expr: "item.qty"}},
			},
		}},
	}}
	data := bind.Scope{"items": []any{map[string]any{"name": "Widget", "qty": 1.0}}}

	_, err = template.Fill(tpl, b, data)
	if err == nil {
		t.Fatal("Fill succeeded despite content below the table's own sample row")
	}
}
