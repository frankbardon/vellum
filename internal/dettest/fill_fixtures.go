package dettest

// This file is E11-S3's contribution: fill mode registered against the same
// determinism and golden-artifact harness every compose-mode writer already
// joins. CLAUDE.md's own package doc for this harness says it plainly —
// "every format epic registers cases here rather than growing determinism
// tests of its own" — and fill mode never had, across all of E9/E10/E11,
// until now.
//
// Each case below embeds a fixed template fixture (built inline, the same
// way template/fill_test.go, template/fill_xlsx_test.go and
// template/fill_pptx_test.go already build theirs — this file does not
// import those _test.go files, since a non-test package cannot, so the
// fixture shape is mirrored rather than shared), binds it with a fixed
// bind.Binding against fixed bind.Scope data, and writes the result exactly
// the way a real caller does: template.Open, template.Fill,
// Result.Package.WriteTo. Every case carries at least one plain bind and at
// least one repeat, so the golden pins a repeat's own output shape and not
// merely a trivial substitution.
//
// The fixture-construction step (opc.New/Put/Relationships/WriteTo) always
// uses the zero-value zipdet.WriteOptions{}, which selects zipdet.PinnedEpoch
// regardless of the epoch this case was asked to emit at — the input
// template's own bytes are therefore a pure function of the code here, never
// of the epoch parameter. Only the final Result.Package.WriteTo call, which
// produces the bytes this Case actually reports, is given the real epoch —
// exactly mirroring how every compose-mode case in fixtures.go threads epoch
// through to its own final WriteTo and nowhere else.

import (
	"bytes"
	"io"
	"time"

	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/template"
	"github.com/frankbardon/vellum/template/bind"
)

// fillXMLDecl is the declaration every OOXML part in this file's fixtures
// opens with, matching the exact bytes (including the CRLF) the writer
// packages and the existing fill tests already use.
const fillXMLDecl = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"

// --- DOCX ------------------------------------------------------------------

// fillDOCXCase is fill mode's docx path reaching the determinism harness: a
// template carrying an untouched opening paragraph, a marker bound directly,
// an if-driven marker, a table repeated by row, and an untouched closing
// paragraph — the same genuine mix template/fill_test.go's own end-to-end
// test exercises, pinned as a golden here.
func fillDOCXCase() Case {
	return Case{
		Name:  "fill-docx",
		Ext:   "docx",
		Write: writeFillDOCX,
	}
}

const (
	fillDOCXNSWord   = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	fillDOCXRelDoc   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	fillDOCXCTMain   = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	fillDOCXCTStyles = "application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"
)

// fillDOCXBody mirrors fill_test.go's own fillFixtureBody: an untouched
// opening paragraph, a marker bound directly, an if-driven marker, a table
// repeated by row, and an untouched closing paragraph.
const fillDOCXBody = `<w:p><w:r><w:t>Statement of account</w:t></w:r></w:p>` +
	`<w:p><w:r><w:t>Dear {{customer_name}},</w:t></w:r></w:p>` +
	`<w:p><w:r><w:t>{{vip_note}}</w:t></w:r></w:p>` +
	`<w:tbl><w:tblPr/><w:tblGrid><w:gridCol/><w:gridCol/></w:tblGrid>` +
	`<w:tr><w:tc><w:p><w:r><w:t>Item</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Qty</w:t></w:r></w:p></w:tc></w:tr>` +
	`<w:tr><w:tc><w:p><w:r><w:t>{{item_name}}</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>{{item_qty}}</w:t></w:r></w:p></w:tc></w:tr>` +
	`</w:tbl>` +
	`<w:p><w:r><w:t>Thank you for your business.</w:t></w:r></w:p>`

// fillDOCXFixture builds the raw bytes of the docx-shaped template. It is a
// pure function of no arguments: the same two parts, the same relationship,
// every time.
func fillDOCXFixture() ([]byte, error) {
	pkg := opc.New()

	doc := fillXMLDecl + `<w:document xmlns:w="` + fillDOCXNSWord + `"><w:body>` + fillDOCXBody + `</w:body></w:document>`
	if err := pkg.Put(&opc.Part{Name: "/word/document.xml", ContentType: fillDOCXCTMain, Data: []byte(doc)}); err != nil {
		return nil, err
	}
	styles := fillXMLDecl + `<w:styles xmlns:w="` + fillDOCXNSWord + `"/>`
	if err := pkg.Put(&opc.Part{Name: "/word/styles.xml", ContentType: fillDOCXCTStyles, Data: []byte(styles)}); err != nil {
		return nil, err
	}
	if _, err := pkg.Relationships("/").Add(fillDOCXRelDoc, "word/document.xml", opc.TargetInternal); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := pkg.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func fillDOCXBinding() *bind.Binding {
	return &bind.Binding{
		FormatVersion: bind.FormatVersion,
		Statements: []bind.Statement{
			{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "customer_name", Expr: "customer.name"}},
			{Kind: bind.StatementIf, If: &bind.If{
				When: "customer.vip",
				Then: []bind.Statement{{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "vip_note", Expr: `"VIP customer — priority handling."`}}},
				Else: []bind.Statement{{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "vip_note", Expr: `""`}}},
			}},
			{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
				Over: "items", As: "item", Target: bind.RepeatTargetRow,
				Body: []bind.Statement{
					{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_name", Expr: "item.name"}},
					{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_qty", Expr: "item.qty"}},
				},
			}},
		},
	}
}

func fillDOCXData() bind.Scope {
	return bind.Scope{
		"customer": map[string]any{"name": "Acme & Co.", "vip": true},
		"items": []any{
			map[string]any{"name": "Widget", "qty": 3.0},
			map[string]any{"name": "Gadget", "qty": 5.0},
			map[string]any{"name": "Sprocket", "qty": 9.0},
		},
	}
}

// FillDOCXFixture returns the raw bytes of this package's own fill-docx
// determinism case's template — a real, already-exercised docx carrying a
// plain marker anchor ("customer_name"), an if-driven marker
// ("vip_note"), and a table row repeated by ("item_name", "item_qty").
// Exported so examples/'s own fill-mode gate can bind its own, differently
// focused [bind.Binding] documents against a real template without
// fabricating a second one from scratch. See fillDOCXBody for the exact
// anchors this template discovers.
func FillDOCXFixture() ([]byte, error) { return fillDOCXFixture() }

// FillDOCXData returns the [bind.Scope] this package's own fill-docx case
// binds against — a customer (with a name and a vip flag) and a list of
// line items, matching [FillDOCXFixture]'s own anchors.
func FillDOCXData() bind.Scope { return fillDOCXData() }

func writeFillDOCX(w io.Writer, epoch time.Time) error {
	raw, err := fillDOCXFixture()
	if err != nil {
		return err
	}
	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	res, err := template.Fill(tpl, fillDOCXBinding(), fillDOCXData())
	if err != nil {
		return err
	}
	return res.Package.WriteTo(w, zipdet.WriteOptions{SourceDateEpoch: epoch})
}

// --- XLSX --------------------------------------------------------------

// fillXLSXCase is fill mode's xlsx path reaching the determinism harness: a
// workbook carrying a defined-name bind and a table_row repeat together, the
// same mix template/fill_xlsx_test.go's own end-to-end test exercises.
func fillXLSXCase() Case {
	return Case{
		Name:  "fill-xlsx",
		Ext:   "xlsx",
		Write: writeFillXLSX,
	}
}

const (
	fillXLSXRelOfficeDocument = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	fillXLSXRelWorksheet      = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet"
	fillXLSXRelTable          = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/table"
	fillXLSXNSSpreadsheet     = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	fillXLSXNSRelationships   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

	fillXLSXCTWorkbook  = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"
	fillXLSXCTWorksheet = "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"
	fillXLSXCTTable     = "application/vnd.openxmlformats-officedocument.spreadsheetml.table+xml"
	fillXLSXCTStyles    = "application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"

	fillXLSXWorkbookPart = "/xl/workbook.xml"
	fillXLSXSheetPart    = "/xl/worksheets/sheet1.xml"
	fillXLSXTablePart    = "/xl/tables/table1.xml"
	fillXLSXStylesPart   = "/xl/styles.xml"
)

// fillXLSXStylesXML is an unrelated part Fill has no reason to touch,
// mirroring xlsxStylesXML's own role in fill_xlsx_test.go.
const fillXLSXStylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<styleSheet xmlns="` + fillXLSXNSSpreadsheet + `"/>`

// fillXLSXFixture builds the raw bytes of the xlsx-shaped template: a
// workbook with one sheet and one defined name ("CustomerName"), a worksheet
// carrying the defined name's own target cell plus an Excel Table's header
// and one sample data row, the table's own part, and an unrelated styles
// part.
func fillXLSXFixture() ([]byte, error) {
	pkg := opc.New()

	sheetRID, err := pkg.Relationships(fillXLSXWorkbookPart).Add(fillXLSXRelWorksheet, "worksheets/sheet1.xml", opc.TargetInternal)
	if err != nil {
		return nil, err
	}
	wb := fillXMLDecl + `<workbook xmlns="` + fillXLSXNSSpreadsheet + `" xmlns:r="` + fillXLSXNSRelationships + `">` +
		`<sheets><sheet name="Sheet1" sheetId="1" r:id="` + sheetRID + `"/></sheets>` +
		`<definedNames><definedName name="CustomerName">Sheet1!$B$1</definedName></definedNames>` +
		`</workbook>`
	if err := pkg.Put(&opc.Part{Name: fillXLSXWorkbookPart, ContentType: fillXLSXCTWorkbook, Data: []byte(wb)}); err != nil {
		return nil, err
	}
	if _, err := pkg.Relationships("/").Add(fillXLSXRelOfficeDocument, "xl/workbook.xml", opc.TargetInternal); err != nil {
		return nil, err
	}

	tableRID, err := pkg.Relationships(fillXLSXSheetPart).Add(fillXLSXRelTable, "../tables/table1.xml", opc.TargetInternal)
	if err != nil {
		return nil, err
	}
	table := fillXMLDecl + `<table xmlns="` + fillXLSXNSSpreadsheet + `" id="1" name="CustomerTable" displayName="CustomerTable" ref="A3:B4">` +
		`<tableColumns count="2"><tableColumn id="1" name="Item"/><tableColumn id="2" name="Qty"/></tableColumns></table>`
	if err := pkg.Put(&opc.Part{Name: fillXLSXTablePart, ContentType: fillXLSXCTTable, Data: []byte(table)}); err != nil {
		return nil, err
	}

	sheetData := `<row r="1"><c r="A1" t="inlineStr"><is><t>Customer</t></is></c>` +
		`<c r="B1" t="inlineStr"><is><t>placeholder</t></is></c></row>` +
		`<row r="3"><c r="A3" t="inlineStr"><is><t>Item</t></is></c>` +
		`<c r="B3" t="inlineStr"><is><t>Qty</t></is></c></row>` +
		`<row r="4"><c r="A4" s="1" t="inlineStr"><is><t>placeholder</t></is></c>` +
		`<c r="B4" s="1"><v>0</v></c></row>`
	ws := fillXMLDecl + `<worksheet xmlns="` + fillXLSXNSSpreadsheet + `" xmlns:r="` + fillXLSXNSRelationships + `">` +
		`<sheetData>` + sheetData + `</sheetData>` +
		`<tableParts count="1"><tablePart r:id="` + tableRID + `"/></tableParts>` +
		`</worksheet>`
	if err := pkg.Put(&opc.Part{Name: fillXLSXSheetPart, ContentType: fillXLSXCTWorksheet, Data: []byte(ws)}); err != nil {
		return nil, err
	}

	if err := pkg.Put(&opc.Part{Name: fillXLSXStylesPart, ContentType: fillXLSXCTStyles, Data: []byte(fillXLSXStylesXML)}); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := pkg.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func fillXLSXBinding() *bind.Binding {
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

func fillXLSXData() bind.Scope {
	return bind.Scope{
		"customer": map[string]any{"name": "Acme & Co."},
		"items": []any{
			map[string]any{"name": "Widget", "qty": 3.0},
			map[string]any{"name": "Gadget", "qty": 5.0},
			map[string]any{"name": "Sprocket", "qty": 9.0},
		},
	}
}

func writeFillXLSX(w io.Writer, epoch time.Time) error {
	raw, err := fillXLSXFixture()
	if err != nil {
		return err
	}
	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	res, err := template.Fill(tpl, fillXLSXBinding(), fillXLSXData())
	if err != nil {
		return err
	}
	return res.Package.WriteTo(w, zipdet.WriteOptions{SourceDateEpoch: epoch})
}

// --- PPTX ----------------------------------------------------------------

// fillPPTXCase is fill mode's pptx path reaching the determinism harness: a
// deck carrying a plain shape bind on one slide and a slide-clone repeat over
// another, the same mix template/fill_pptx_test.go's own end-to-end test
// exercises.
func fillPPTXCase() Case {
	return Case{
		Name:  "fill-pptx",
		Ext:   "pptx",
		Write: writeFillPPTX,
	}
}

const (
	fillPPTXRelOfficeDocument = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	fillPPTXRelSlide          = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide"
	fillPPTXNSPresentation    = "http://schemas.openxmlformats.org/presentationml/2006/main"
	fillPPTXNSDrawingMain     = "http://schemas.openxmlformats.org/drawingml/2006/main"
	fillPPTXNSRelationships   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

	fillPPTXCTPresentation = "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
	fillPPTXCTSlide        = "application/vnd.openxmlformats-officedocument.presentationml.slide+xml"
	fillPPTXCTPresProps    = "application/vnd.openxmlformats-officedocument.presentationml.presProps+xml"

	fillPPTXPresentationPart = "/ppt/presentation.xml"
	fillPPTXSlide1Part       = "/ppt/slides/slide1.xml"
	fillPPTXSlide2Part       = "/ppt/slides/slide2.xml"
	fillPPTXPresPropsPart    = "/ppt/presProps.xml"
)

const fillPPTXPresPropsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<p:presentationPr xmlns:p="` + fillPPTXNSPresentation + `"/>`

// fillPPTXSlideXML builds one slide part carrying a single shape anchor
// named shapeName, mirroring fill_pptx_test.go's own pptxSlideXMLFill.
func fillPPTXSlideXML(shapeName string) []byte {
	return []byte(fillXMLDecl +
		`<p:sld xmlns:a="` + fillPPTXNSDrawingMain + `" xmlns:r="` + fillPPTXNSRelationships +
		`" xmlns:p="` + fillPPTXNSPresentation + `">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="` + shapeName + `"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>placeholder</a:t></a:r></a:p></p:txBody></p:sp>` +
		`</p:spTree></p:cSld>` +
		`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
		`</p:sld>`)
}

// fillPPTXFixture builds the raw bytes of the pptx-shaped template: two
// slides — slide1 carries a plain shape anchor ("customer_name") and slide2
// carries a shape anchor ("item_name") that a RepeatTargetSlide statement
// clones — plus presProps.xml, an unrelated part Fill has no reason to
// touch.
func fillPPTXFixture() ([]byte, error) {
	pkg := opc.New()

	slide1RID, err := pkg.Relationships(fillPPTXPresentationPart).Add(fillPPTXRelSlide, "slides/slide1.xml", opc.TargetInternal)
	if err != nil {
		return nil, err
	}
	slide2RID, err := pkg.Relationships(fillPPTXPresentationPart).Add(fillPPTXRelSlide, "slides/slide2.xml", opc.TargetInternal)
	if err != nil {
		return nil, err
	}

	// p:sldSz/p:notesSz are carried because every real PowerPoint-authored
	// deck writes them (deck/write_presentation.go does too, for the
	// compose-mode goldens), and their absence is exactly the kind of gap
	// this story's office-reader oracle exists to catch: LibreOffice opens
	// and reads a presentation part with no slide size just fine, but its
	// PDF export path fails outright without one — a defect this harness's
	// own bytes-against-bytes determinism assertions cannot see at all,
	// discovered by running TestOfficeReaderOpensGoldens against this exact
	// fixture before this comment existed.
	pres := fillXMLDecl +
		`<p:presentation xmlns:a="` + fillPPTXNSDrawingMain + `" xmlns:r="` + fillPPTXNSRelationships +
		`" xmlns:p="` + fillPPTXNSPresentation + `">` +
		`<p:sldIdLst>` +
		`<p:sldId id="256" r:id="` + slide1RID + `"/>` +
		`<p:sldId id="257" r:id="` + slide2RID + `"/>` +
		`</p:sldIdLst>` +
		`<p:sldSz cx="12192000" cy="6858000"/>` +
		`<p:notesSz cx="6858000" cy="9144000"/>` +
		`</p:presentation>`
	if err := pkg.Put(&opc.Part{Name: fillPPTXPresentationPart, ContentType: fillPPTXCTPresentation, Data: []byte(pres)}); err != nil {
		return nil, err
	}
	if _, err := pkg.Relationships("/").Add(fillPPTXRelOfficeDocument, "ppt/presentation.xml", opc.TargetInternal); err != nil {
		return nil, err
	}

	if err := pkg.Put(&opc.Part{Name: fillPPTXSlide1Part, ContentType: fillPPTXCTSlide, Data: fillPPTXSlideXML("customer_name")}); err != nil {
		return nil, err
	}
	if err := pkg.Put(&opc.Part{Name: fillPPTXSlide2Part, ContentType: fillPPTXCTSlide, Data: fillPPTXSlideXML("item_name")}); err != nil {
		return nil, err
	}
	if err := pkg.Put(&opc.Part{Name: fillPPTXPresPropsPart, ContentType: fillPPTXCTPresProps, Data: []byte(fillPPTXPresPropsXML)}); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := pkg.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func fillPPTXBinding() *bind.Binding {
	return &bind.Binding{
		FormatVersion: bind.FormatVersion,
		Statements: []bind.Statement{
			{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "customer_name", Expr: "customer.name"}},
			{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
				Over: "items", As: "item", Target: bind.RepeatTargetSlide,
				Body: []bind.Statement{
					{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_name", Expr: "item.name"}},
				},
			}},
		},
	}
}

func fillPPTXData() bind.Scope {
	return bind.Scope{
		"customer": map[string]any{"name": "Acme & Co."},
		"items": []any{
			map[string]any{"name": "Widget"},
			map[string]any{"name": "Gadget"},
			map[string]any{"name": "Sprocket"},
		},
	}
}

func writeFillPPTX(w io.Writer, epoch time.Time) error {
	raw, err := fillPPTXFixture()
	if err != nil {
		return err
	}
	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	res, err := template.Fill(tpl, fillPPTXBinding(), fillPPTXData())
	if err != nil {
		return err
	}
	return res.Package.WriteTo(w, zipdet.WriteOptions{SourceDateEpoch: epoch})
}
