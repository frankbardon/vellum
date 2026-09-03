package template_test

// TestNonDestructiveCorpus_XLSX is E11-S3's xlsx counterpart to E9-S5's
// TestNonDestructiveCorpus: the same "every part outside the fill's own
// touched parts is byte-identical to source" property, proved against a
// richer, more realistic xlsx fixture than fill_xlsx_test.go's own minimal
// one. Where the DOCX corpus carries tracked changes, a comment, a custom
// XML part, a footnote and an OLE object, this one carries the xlsx-native
// equivalents that have no WordprocessingML counterpart: a second worksheet
// nothing in the binding ever names, a legacy cell comment paired with its
// own VML drawing (the two-part shape CLAUDE.md's own byte-layout
// invariants document for compose mode — a template authored by a real
// Excel is exactly the kind of file that carries one), a defined name the
// binding deliberately leaves alone via Binding.OptionalAnchors, and the
// same custom XML part the DOCX fixture uses, proving that guarantee
// generalises rather than being a WordprocessingML accident.

import (
	"bytes"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/template"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/template/bind"
)

const (
	xndRelComments       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/comments"
	xndRelVMLDrawing     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/vmlDrawing"
	xndRelCustomXML      = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/customXml"
	xndRelCustomXMLProps = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/customXmlProps"
	xndNSVML             = "urn:schemas-microsoft-com:vml"
	xndNSOffice          = "urn:schemas-microsoft-com:office:office"
	xndNSExcelVML        = "urn:schemas-microsoft-com:office:excel"
	xndNSCustomXML       = "http://schemas.openxmlformats.org/officeDocument/2006/customXml"

	xndCTComments       = "application/vnd.openxmlformats-officedocument.spreadsheetml.comments+xml"
	xndCTVMLDrawing     = "application/vnd.openxmlformats-officedocument.vmlDrawing"
	xndCTCustomXML      = "application/xml"
	xndCTCustomXMLProps = "application/vnd.openxmlformats-officedocument.customXmlProperties+xml"

	xndSheet2Part         = "/xl/worksheets/sheet2.xml"
	xndCommentsPart       = "/xl/comments1.xml"
	xndVMLDrawingPart     = "/xl/drawings/vmlDrawing1.vml"
	xndCustomXMLPart      = "/customXml/item1.xml"
	xndCustomXMLPropsPart = "/customXml/itemProps1.xml"

	// xndOptionalDefinedName is the defined name the richer fixture carries
	// but no Bind statement ever references — the anchor a binding
	// legitimately leaves alone rather than one that simply never existed.
	xndOptionalDefinedName = "ReportDate"
)

// buildRicherFixtureXLSX assembles a more realistic .xlsx-shaped package than
// fill_xlsx_test.go's own buildFillFixtureXLSX: everything that fixture
// carries (a workbook with one sheet and one defined name, a worksheet
// carrying the defined name's own target cell plus an Excel Table's header
// and one sample data row, the table's own part, and an unrelated styles
// part), plus:
//
//   - a second worksheet ("Sheet2"), entirely untouched by anything the
//     binding names;
//   - a legacy cell comment on Sheet1's own C1, paired with its own VML
//     drawing part and the worksheet's own <legacyDrawing r:id="..."/>
//     reference — the two-part shape a real Excel-authored comment always
//     carries;
//   - a second defined name ("ReportDate") resolving to Sheet1's own D1,
//     which the binding never references;
//   - a custom XML part and its own properties part, mirroring
//     nondestructive_fixture_test.go's own DOCX shape.
func buildRicherFixtureXLSX(t *testing.T) []byte {
	t.Helper()
	pkg := opc.New()

	// The workbook part carries two relationships of the same type (both
	// worksheets), and a built [opc.Relationships] set is renumbered by a
	// sorted (Type, Mode, Target) walk at serialisation time — not left in
	// insertion order — so the identifiers embedded in workbook.xml's own
	// text below must be read back with Freeze+IDFor rather than trusted
	// from Add's own return value, exactly as
	// nondestructive_fixture_test.go's DOCX fixture already does for its own
	// three same-owner relationships.
	wbRels := pkg.Relationships(xlsxWorkbookPart)
	if _, err := wbRels.Add(xlsxRelWorksheet, "worksheets/sheet1.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add sheet1 rel: %v", err)
	}
	if _, err := wbRels.Add(xlsxRelWorksheet, "worksheets/sheet2.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add sheet2 rel: %v", err)
	}
	wbRels.Freeze()
	sheet1RID, ok := wbRels.IDFor(xlsxRelWorksheet, "worksheets/sheet1.xml")
	if !ok {
		t.Fatal("sheet1 relationship not found after Freeze")
	}
	sheet2RID, ok := wbRels.IDFor(xlsxRelWorksheet, "worksheets/sheet2.xml")
	if !ok {
		t.Fatal("sheet2 relationship not found after Freeze")
	}

	wb := xmlDeclXLSX() + `<workbook xmlns="` + xlsxNSSpreadsheet + `" xmlns:r="` + xlsxNSRelationships + `">` +
		`<sheets>` +
		`<sheet name="Sheet1" sheetId="1" r:id="` + sheet1RID + `"/>` +
		`<sheet name="Sheet2" sheetId="2" r:id="` + sheet2RID + `"/>` +
		`</sheets>` +
		`<definedNames>` +
		`<definedName name="CustomerName">Sheet1!$B$1</definedName>` +
		`<definedName name="` + xndOptionalDefinedName + `">Sheet1!$D$1</definedName>` +
		`</definedNames>` +
		`</workbook>`
	if err := pkg.Put(&opc.Part{Name: xlsxWorkbookPart, ContentType: xlsxCTWorkbook, Data: []byte(wb)}); err != nil {
		t.Fatalf("Put workbook.xml: %v", err)
	}
	if _, err := pkg.Relationships("/").Add(xlsxRelOfficeDocument, "xl/workbook.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add officeDocument rel: %v", err)
	}
	if _, err := pkg.Relationships("/").Add(xndRelCustomXML, "customXml/item1.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add customXml rel: %v", err)
	}

	// sheet1's own relationships (table, comments, vmlDrawing) are three
	// distinct types, so the sorted-by-Type renumbering the comment above
	// describes really does reorder them relative to insertion order here —
	// Freeze+IDFor is not merely defensive in this case, it is required for
	// the r:id values embedded in sheet1.xml's own text below to resolve at
	// all.
	sheetRels := pkg.Relationships(xlsxSheetPart)
	if _, err := sheetRels.Add(xlsxRelTable, "../tables/table1.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add table rel: %v", err)
	}
	if _, err := sheetRels.Add(xndRelComments, "../comments1.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add comments rel: %v", err)
	}
	if _, err := sheetRels.Add(xndRelVMLDrawing, "../drawings/vmlDrawing1.vml", opc.TargetInternal); err != nil {
		t.Fatalf("Add vmlDrawing rel: %v", err)
	}
	sheetRels.Freeze()
	tableRID, ok := sheetRels.IDFor(xlsxRelTable, "../tables/table1.xml")
	if !ok {
		t.Fatal("table relationship not found after Freeze")
	}
	vmlRID, ok := sheetRels.IDFor(xndRelVMLDrawing, "../drawings/vmlDrawing1.vml")
	if !ok {
		t.Fatal("vmlDrawing relationship not found after Freeze")
	}

	table := xmlDeclXLSX() + `<table xmlns="` + xlsxNSSpreadsheet + `" id="1" name="CustomerTable" displayName="CustomerTable" ref="A3:B4">` +
		`<tableColumns count="2"><tableColumn id="1" name="Item"/><tableColumn id="2" name="Qty"/></tableColumns></table>`
	if err := pkg.Put(&opc.Part{Name: xlsxTablePart, ContentType: xlsxCTTable, Data: []byte(table)}); err != nil {
		t.Fatalf("Put table1.xml: %v", err)
	}

	// Row 1: the defined name's own target cell (B1), a commented cell (C1,
	// carrying no comment text of its own — the comment lives in the
	// comments part, addressed by cell reference), and the second defined
	// name's own target cell (D1). Row 3/4: the table's own header and one
	// sample data row, exactly as buildFillFixtureXLSX's own fixture.
	sheetData := `<row r="1"><c r="A1" t="inlineStr"><is><t>Customer</t></is></c>` +
		`<c r="B1" t="inlineStr"><is><t>placeholder</t></is></c>` +
		`<c r="C1" t="inlineStr"><is><t>See comment</t></is></c>` +
		`<c r="D1" t="inlineStr"><is><t>2024-01-01</t></is></c></row>` +
		`<row r="3"><c r="A3" t="inlineStr"><is><t>Item</t></is></c>` +
		`<c r="B3" t="inlineStr"><is><t>Qty</t></is></c></row>` +
		`<row r="4"><c r="A4" s="1" t="inlineStr"><is><t>placeholder</t></is></c>` +
		`<c r="B4" s="1"><v>0</v></c></row>`
	ws := xmlDeclXLSX() + `<worksheet xmlns="` + xlsxNSSpreadsheet + `" xmlns:r="` + xlsxNSRelationships + `">` +
		`<sheetData>` + sheetData + `</sheetData>` +
		`<legacyDrawing r:id="` + vmlRID + `"/>` +
		`<tableParts count="1"><tablePart r:id="` + tableRID + `"/></tableParts>` +
		`</worksheet>`
	if err := pkg.Put(&opc.Part{Name: xlsxSheetPart, ContentType: xlsxCTWorksheet, Data: []byte(ws)}); err != nil {
		t.Fatalf("Put sheet1.xml: %v", err)
	}

	// xl/comments1.xml -- the legacy comments part: text only, no shape.
	comments := xmlDeclXLSX() + `<comments xmlns="` + xlsxNSSpreadsheet + `">` +
		`<authors><author>Reviewer</author></authors>` +
		`<commentList><comment ref="C1" authorId="0">` +
		`<text><r><t>Please confirm this total before publishing.</t></r></text>` +
		`</comment></commentList></comments>`
	if err := pkg.Put(&opc.Part{Name: xndCommentsPart, ContentType: xndCTComments, Data: []byte(comments)}); err != nil {
		t.Fatalf("Put comments1.xml: %v", err)
	}

	// xl/drawings/vmlDrawing1.vml -- the shape that draws the comment's
	// indicator and flyout, mirroring sheet/write_comments.go's own
	// vmlDrawingXML shape. No XML declaration: a VML drawing is not
	// namespaced XML in the strict sense and every reader's VML parser
	// expects it exactly this way, per CLAUDE.md's own byte-layout
	// invariants.
	vml := `<xml xmlns:v="` + xndNSVML + `" xmlns:o="` + xndNSOffice + `" xmlns:x="` + xndNSExcelVML + `">` +
		`<o:shapelayout v:ext="edit"><o:idmap v:ext="edit" data="1"/></o:shapelayout>` +
		`<v:shapetype id="_x0000_t202" coordsize="21600,21600" o:spt="202" ` +
		`path="m,l,21600r21600,l21600,xe"><v:stroke joinstyle="miter"/>` +
		`<v:path gradientshapeok="t" o:connecttype="rect"/></v:shapetype>` +
		`<v:shape id="_x0000_s1025" type="#_x0000_t202" ` +
		`style='position:absolute;margin-left:59.25pt;margin-top:1.5pt;width:108pt;height:59.25pt;` +
		`z-index:1;visibility:hidden' fillcolor="#ffffe1" o:insetmode="auto">` +
		`<v:fill color2="#ffffe1"/><v:shadow on="t" color="black" obscured="t"/>` +
		`<v:path o:connecttype="none"/>` +
		`<v:textbox style='mso-direction-alt:auto'><div style='text-align:left'/></v:textbox>` +
		`<x:ClientData ObjectType="Note"><x:MoveWithCells/><x:SizeWithCells/>` +
		`<x:Row>0</x:Row><x:Column>2</x:Column></x:ClientData>` +
		`</v:shape></xml>`
	if err := pkg.Put(&opc.Part{Name: xndVMLDrawingPart, ContentType: xndCTVMLDrawing, Data: []byte(vml)}); err != nil {
		t.Fatalf("Put vmlDrawing1.vml: %v", err)
	}

	// xl/worksheets/sheet2.xml -- the second, entirely untouched worksheet.
	ws2 := xmlDeclXLSX() + `<worksheet xmlns="` + xlsxNSSpreadsheet + `">` +
		`<sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Untouched sheet content.</t></is></c></row></sheetData>` +
		`</worksheet>`
	if err := pkg.Put(&opc.Part{Name: xndSheet2Part, ContentType: xlsxCTWorksheet, Data: []byte(ws2)}); err != nil {
		t.Fatalf("Put sheet2.xml: %v", err)
	}

	// customXml/item1.xml + itemProps1.xml -- the same kind of consumer-owned
	// part nondestructive_fixture_test.go's DOCX fixture carries. Vellum
	// never looks inside it.
	itemXML := xmlDeclXLSX() + `<consumerData xmlns="http://example.com/consumer-schema"><field>value</field></consumerData>`
	if err := pkg.Put(&opc.Part{Name: xndCustomXMLPart, ContentType: xndCTCustomXML, Data: []byte(itemXML)}); err != nil {
		t.Fatalf("Put customXml/item1.xml: %v", err)
	}
	itemPropsXML := xmlDeclXLSX() +
		`<ds:datastoreItem ds:itemID="{12345678-1234-1234-1234-123456789012}" xmlns:ds="` + xndNSCustomXML + `">` +
		`<ds:schemaRefs><ds:schemaRef ds:uri="http://example.com/consumer-schema"/></ds:schemaRefs>` +
		`</ds:datastoreItem>`
	if err := pkg.Put(&opc.Part{Name: xndCustomXMLPropsPart, ContentType: xndCTCustomXMLProps, Data: []byte(itemPropsXML)}); err != nil {
		t.Fatalf("Put customXml/itemProps1.xml: %v", err)
	}
	if _, err := pkg.Relationships(xndCustomXMLPart).Add(xndRelCustomXMLProps, "itemProps1.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add customXml item rel: %v", err)
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

// richerFixtureBindingXLSX is fillFixtureBindingXLSX's own statements, plus
// OptionalAnchors naming the defined name the fixture carries but no Bind
// ever references.
func richerFixtureBindingXLSX() *bind.Binding {
	b := fillFixtureBindingXLSX()
	b.OptionalAnchors = []string{xndOptionalDefinedName}
	return b
}

// legacyDrawingRID extracts the r:id value from a worksheet's own
// <legacyDrawing r:id="..."/> element by a plain substring search — this
// file's fixture is built by hand with a known, single legacyDrawing
// element, so a full xmlcopy.Walk is more machinery than the assertion
// needs.
func legacyDrawingRID(t *testing.T, sheetXML []byte) string {
	t.Helper()
	const marker = `<legacyDrawing r:id="`
	i := bytes.Index(sheetXML, []byte(marker))
	if i < 0 {
		t.Fatalf("worksheet XML carries no <legacyDrawing> element: %s", sheetXML)
	}
	rest := sheetXML[i+len(marker):]
	j := bytes.IndexByte(rest, '"')
	if j < 0 {
		t.Fatalf("worksheet XML's legacyDrawing r:id attribute is not properly terminated: %s", sheetXML)
	}
	return string(rest[:j])
}

func TestNonDestructiveCorpus_XLSX(t *testing.T) {
	raw := buildRicherFixtureXLSX(t)

	srcPkg, err := opc.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("opc.Open on the fixture: %v", err)
	}

	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("template.Open: %v", err)
	}

	// --- The optional defined name really is discovered, not merely absent -
	// This is what makes the OptionalAnchors assertion below meaningful:
	// without this, "the binding never mentions ReportDate and Fill still
	// succeeds" could just as easily mean the fixture never produced that
	// anchor at all.
	inv, err := anchor.Discover(tpl.Package(), tpl.Format(), tpl.MainPart())
	if err != nil {
		t.Fatalf("anchor.Discover: %v", err)
	}
	foundOptional := false
	for _, a := range inv.Anchors {
		if a.Name == xndOptionalDefinedName {
			foundOptional = true
		}
	}
	if !foundOptional {
		t.Fatalf("the fixture's own %q defined name was not discovered as an anchor; "+
			"the OptionalAnchors assertion below would prove nothing", xndOptionalDefinedName)
	}

	// --- Without OptionalAnchors, reconciliation would fail -----------------
	// A negative control: the discovered-but-unreferenced anchor really would
	// break Fill if the binding did not name it optional, which is what
	// proves the fixture is exercising Binding.OptionalAnchors rather than
	// merely not tripping over an anchor it never had.
	tplForControl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("template.Open (control): %v", err)
	}
	withoutOptional := fillFixtureBindingXLSX()
	if _, err := template.Fill(tplForControl, withoutOptional, fillFixtureDataXLSX()); !verr.HasCode(err, verr.VELLUM_BIND_ANCHOR_UNRECONCILED) {
		t.Fatalf("Fill without OptionalAnchors err = %v, want VELLUM_BIND_ANCHOR_UNRECONCILED "+
			"(the fixture's %q defined name should be discovered but unreferenced)", err, xndOptionalDefinedName)
	}

	// --- The actual fill ----------------------------------------------------
	res, err := template.Fill(tpl, richerFixtureBindingXLSX(), fillFixtureDataXLSX())
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}

	wantTouched := map[string]bool{xlsxSheetPart: true, xlsxTablePart: true}
	if len(res.Touched) != len(wantTouched) {
		t.Fatalf("Touched = %v, want exactly %v", res.Touched, wantTouched)
	}
	touched := make(map[string]bool, len(res.Touched))
	for _, name := range res.Touched {
		touched[name] = true
		if !wantTouched[name] {
			t.Errorf("unexpected touched part %q", name)
		}
	}

	// --- The worksheet carries the filled content, and the untouched cells -
	// on the same part survive -----------------------------------------------
	sheet := mustPartBytes(t, res.Package, xlsxSheetPart)
	for _, want := range []string{"Acme &amp; Co.", "Widget", "Gadget", "Sprocket"} {
		if !bytes.Contains(sheet, []byte(want)) {
			t.Errorf("filled sheet missing %q: %s", want, sheet)
		}
	}
	if bytes.Contains(sheet, []byte("placeholder")) {
		t.Errorf("filled sheet still carries a raw placeholder: %s", sheet)
	}
	for _, want := range []string{"Customer", "Item", "Qty", "See comment", "2024-01-01", `<legacyDrawing r:id=`} {
		if !bytes.Contains(sheet, []byte(want)) {
			t.Errorf("filled sheet lost untouched content %q, which the fill had no anchor targeting: %s", want, sheet)
		}
	}

	// --- Every part outside Touched is byte-identical to the source --------
	srcNames := srcPkg.Names()
	filledNames := res.Package.Names()
	if len(srcNames) != len(filledNames) {
		t.Fatalf("part count changed: source has %d, filled has %d\nsource: %v\nfilled: %v",
			len(srcNames), len(filledNames), srcNames, filledNames)
	}
	for _, name := range srcNames {
		if touched[name] {
			continue // asserted above; expected to differ.
		}
		srcBytes := mustPartBytes(t, srcPkg, name)
		filledBytes := mustPartBytes(t, res.Package, name)
		if !bytes.Equal(srcBytes, filledBytes) {
			t.Errorf("part %q changed even though it was never in Touched; "+
				"every part outside the fill's own touched parts must survive untouched.\n"+
				"source (%d bytes):\n%s\nfilled (%d bytes):\n%s",
				name, len(srcBytes), srcBytes, len(filledBytes), filledBytes)
		}
	}

	// --- Named, specific checks for what "untouched" actually means here ---
	// The generic loop above already proves byte-identity; these name the
	// pieces explicitly so a regression reads as "the second worksheet
	// changed" rather than requiring a human to map a part name back to what
	// it represents.
	for _, part := range []string{
		xndSheet2Part, xndCommentsPart, xndVMLDrawingPart,
		xndCustomXMLPart, xndCustomXMLPropsPart,
		xlsxWorkbookPart, xlsxStylesPart,
	} {
		if touched[part] {
			t.Fatalf("part %q was touched by the fill; this fixture's whole point is that it must not be", part)
		}
		if !srcPkg.Has(part) {
			t.Fatalf("fixture is missing its own %q part", part)
		}
	}

	// --- The result round-trips through opc.Open, and the second sheet's --
	// own relationships still resolve ----------------------------------------
	var out bytes.Buffer
	if err := res.Package.WriteTo(&out, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	reopened, err := opc.Open(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatalf("the filled package does not round-trip through opc.Open: %v", err)
	}
	if !reopened.Has(xndSheet2Part) {
		t.Error("reopened package lost the second, untouched worksheet")
	}
	rels, ok := reopened.RelationshipsFor(xlsxSheetPart)
	if !ok {
		t.Fatal("reopened package carries no sheet1.xml relationships")
	}
	sawVML := false
	for _, rel := range rels.ByType(xndRelVMLDrawing) {
		sawVML = true
		target := "/xl/" + rel.Target[len("../"):]
		if !reopened.Has(target) {
			t.Errorf("the legacyDrawing relationship target %q does not resolve to a part in the reopened package", target)
		}
	}
	// The stronger form: not merely that a vmlDrawing relationship exists
	// somewhere on sheet1, but that the specific r:id sheet1.xml's own
	// <legacyDrawing r:id="..."/> names resolves to it — the same check
	// nondestructive_pptx_test.go's own blipEmbedID assertion makes for its
	// picture, for the same reason: a built [opc.Relationships] set is
	// renumbered by a sorted walk at Freeze/marshal time, so an r:id
	// captured before Freeze can point at the wrong relationship once the
	// package is actually serialised even though every part still "exists".
	legacyDrawingID := legacyDrawingRID(t, mustPartBytes(t, res.Package, xlsxSheetPart))
	rel, ok := rels.Resolve(legacyDrawingID)
	if !ok {
		t.Fatalf("sheet1.xml's own legacyDrawing r:id=%q does not resolve to any relationship sheet1 owns", legacyDrawingID)
	}
	if rel.Type != xndRelVMLDrawing {
		t.Errorf("sheet1.xml's own legacyDrawing r:id=%q resolves to a %q relationship, want %q",
			legacyDrawingID, rel.Type, xndRelVMLDrawing)
	}

	if !sawVML {
		t.Error("sheet1's own vmlDrawing relationship did not survive the fill")
	}

	// --- The originally opened Template is untouched ------------------------
	origSheet := mustPartBytes(t, tpl.Package(), xlsxSheetPart)
	if !bytes.Contains(origSheet, []byte("placeholder")) {
		t.Error("Fill mutated the *Template it was called against; the original placeholder text should still be there")
	}
}
