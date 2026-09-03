package template_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/template"
)

// relStylesForFontTests mirrors the officeDocument relationship type a
// WordprocessingML main part uses to name its styles part. Duplicated from
// template/fonts.go's own unexported relStyles constant: this file lives in
// the external template_test package and cannot reach it.
const relStylesForFontTests = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles"

const ctStyles = "application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"

// buildDOCXTemplate assembles a two-part (or one-part, when stylesXML is
// empty) DOCX-shaped package: /word/document.xml carrying documentXML, and,
// when stylesXML is non-empty, /word/styles.xml carrying it and related to
// the document part via relStylesForFontTests.
func buildDOCXTemplate(t *testing.T, documentXML, stylesXML string) []byte {
	t.Helper()
	p := opc.New()
	if err := p.Put(&opc.Part{
		Name:        "/word/document.xml",
		ContentType: ctMainDocument,
		Data:        []byte(documentXML),
	}); err != nil {
		t.Fatalf("Put document: %v", err)
	}
	if _, err := p.Relationships("/").Add(relOfficeDocument, "word/document.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add root rel: %v", err)
	}

	if stylesXML != "" {
		if err := p.Put(&opc.Part{
			Name:        "/word/styles.xml",
			ContentType: ctStyles,
			Data:        []byte(stylesXML),
		}); err != nil {
			t.Fatalf("Put styles: %v", err)
		}
		if _, err := p.Relationships("/word/document.xml").Add(relStylesForFontTests, "styles.xml", opc.TargetInternal); err != nil {
			t.Fatalf("Add styles rel: %v", err)
		}
	}

	var buf bytes.Buffer
	if err := p.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

const wNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`

func TestInspect_FontsDedupAndSortAcrossCategories(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document ` + wNS + `>` +
		`<w:body>` +
		`<w:p>` +
		`<w:r><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri"/></w:rPr><w:t>Hello</w:t></w:r>` +
		`<w:r><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri"/></w:rPr><w:t> again</w:t></w:r>` +
		`<w:r><w:rPr><w:rFonts w:cs="Cambria Math"/></w:rPr><w:t>x</w:t></w:r>` +
		`<w:r><w:rPr><w:rFonts w:eastAsia="MS Mincho" w:ascii="Arial"/></w:rPr><w:t>y</w:t></w:r>` +
		`</w:p>` +
		`</w:body>` +
		`</w:document>`
	raw := buildDOCXTemplate(t, documentXML, "")

	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	report, err := template.Inspect(tpl)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	want := []template.FontRequirement{
		{Family: "Arial", Categories: []string{"ascii"}},
		{Family: "Calibri", Categories: []string{"ascii", "hAnsi"}},
		{Family: "Cambria Math", Categories: []string{"cs"}},
		{Family: "MS Mincho", Categories: []string{"eastAsia"}},
	}
	if !reflect.DeepEqual(report.Fonts, want) {
		t.Fatalf("Fonts = %#v, want %#v", report.Fonts, want)
	}
}

func TestInspect_FontsFromStylesDefaultsAreReported(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document ` + wNS + `>` +
		`<w:body><w:p><w:r><w:t>plain, no rFonts here</w:t></w:r></w:p></w:body>` +
		`</w:document>`
	stylesXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:styles ` + wNS + `>` +
		`<w:docDefaults>` +
		`<w:rPrDefault><w:rPr>` +
		`<w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/>` +
		`</w:rPr></w:rPrDefault>` +
		`</w:docDefaults>` +
		`</w:styles>`
	raw := buildDOCXTemplate(t, documentXML, stylesXML)

	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	report, err := template.Inspect(tpl)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	want := []template.FontRequirement{
		{Family: "Times New Roman", Categories: []string{"ascii", "hAnsi", "cs"}},
	}
	if !reflect.DeepEqual(report.Fonts, want) {
		t.Fatalf("Fonts = %#v, want %#v — scanning styles.xml's docDefaults is what closes the under-reporting gap", report.Fonts, want)
	}
}

func TestInspect_NoFontReferencesIsAnEmptyListNotAnError(t *testing.T) {
	raw := buildDOCXTemplate(t, minimalDocumentXML, "")

	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	report, err := template.Inspect(tpl)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(report.Fonts) != 0 {
		t.Errorf("Fonts = %#v, want empty", report.Fonts)
	}
}

func TestInspect_ReportRoundTripsThroughJSON(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document ` + wNS + `>` +
		`<w:body>` +
		`<w:sdt><w:sdtPr><w:tag w:val="client_name"/><w:alias w:val="Client Name"/></w:sdtPr>` +
		`<w:sdtContent><w:p><w:r><w:t>placeholder</w:t></w:r></w:p></w:sdtContent></w:sdt>` +
		`<w:p><w:r><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri"/></w:rPr><w:t>{{ report_date }}</w:t></w:r></w:p>` +
		`</w:body>` +
		`</w:document>`
	raw := buildDOCXTemplate(t, documentXML, "")

	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	report, err := template.Inspect(tpl)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(report.Anchors) != 2 {
		t.Fatalf("got %d anchors, want 2 (one native, one marker)", len(report.Anchors))
	}
	if len(report.Fonts) != 1 || report.Fonts[0].Family != "Calibri" {
		t.Fatalf("Fonts = %#v, want [Calibri]", report.Fonts)
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var round template.InspectReport
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(*report, round) {
		t.Fatalf("round-tripped report differs:\n  got  %#v\n  want %#v", round, *report)
	}
}

func TestInspectReport_TableShapes(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document ` + wNS + `>` +
		`<w:body>` +
		`<w:sdt><w:sdtPr><w:tag w:val="client_name"/><w:alias w:val="Client Name"/></w:sdtPr>` +
		`<w:sdtContent><w:p><w:r><w:t>placeholder</w:t></w:r></w:p></w:sdtContent></w:sdt>` +
		`<w:p><w:r><w:rPr><w:rFonts w:ascii="Calibri" w:cs="Cambria Math"/></w:rPr><w:t>x</w:t></w:r></w:p>` +
		`</w:body>` +
		`</w:document>`
	raw := buildDOCXTemplate(t, documentXML, "")

	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	report, err := template.Inspect(tpl)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	anchorRows := report.AnchorsTable()
	if len(anchorRows) != 2 { // header + one native anchor
		t.Fatalf("AnchorsTable() has %d rows, want 2", len(anchorRows))
	}
	if !reflect.DeepEqual(anchorRows[0], []string{"Name", "Kind", "Alias", "Part"}) {
		t.Errorf("AnchorsTable() header = %#v", anchorRows[0])
	}
	if got, want := anchorRows[1], []string{"client_name", "native", "Client Name", "/word/document.xml"}; !reflect.DeepEqual(got, want) {
		t.Errorf("AnchorsTable() row = %#v, want %#v", got, want)
	}

	fontRows := report.FontsTable()
	if len(fontRows) != 3 { // header + Calibri + Cambria Math
		t.Fatalf("FontsTable() has %d rows, want 3: %#v", len(fontRows), fontRows)
	}
	if !reflect.DeepEqual(fontRows[0], []string{"Family", "Categories"}) {
		t.Errorf("FontsTable() header = %#v", fontRows[0])
	}
	if got, want := fontRows[1], []string{"Calibri", "ascii"}; !reflect.DeepEqual(got, want) {
		t.Errorf("FontsTable() row = %#v, want %#v", got, want)
	}
	if got, want := fontRows[2], []string{"Cambria Math", "cs"}; !reflect.DeepEqual(got, want) {
		t.Errorf("FontsTable() row = %#v, want %#v", got, want)
	}
}

func TestInspectReport_NilTablesReturnJustTheHeader(t *testing.T) {
	var r *template.InspectReport
	if got := r.AnchorsTable(); len(got) != 1 {
		t.Errorf("AnchorsTable() on nil = %#v, want a single header row", got)
	}
	if got := r.FontsTable(); len(got) != 1 {
		t.Errorf("FontsTable() on nil = %#v, want a single header row", got)
	}
}
