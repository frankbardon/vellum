package doc_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"io"
	stdstrings "strings"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/doc"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/provenance"
	"github.com/frankbardon/vellum/resolve"
	"github.com/frankbardon/vellum/spec"
)

var onePixelPNG = mustDecode(`iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==`)

func mustDecode(s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

const wideSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 360"/>`

func pngURI() string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(onePixelPNG)
}

func svgURI() string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(wideSVG))
}

// lower resolves a specification and lowers it, which is the path a caller
// composing from blocks actually takes.
func lower(t *testing.T, blocks ...spec.Block) *doc.Document {
	t.Helper()
	d := resolved(t, blocks...)
	out, err := doc.Lower(d)
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	return out
}

func resolved(t *testing.T, blocks ...spec.Block) *fragment.Doc {
	t.Helper()
	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Title:         "Report",
		Sections:      []spec.Section{{ID: "s1", Blocks: blocks}},
	}
	res, err := resolve.Resolve(context.Background(), s, resolve.Options{Format: artifact.FormatDOCX})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return res.Doc
}

func heading(level int, content string) spec.Block {
	return spec.Block{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: level, Content: content}}
}

func text(content string, marks ...string) spec.Block {
	return spec.Block{Kind: spec.BlockText, Marks: marks, Text: &spec.Text{Content: content}}
}

// write emits a document and returns the package bytes.
func write(t *testing.T, d *doc.Document) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := d.WriteTo(&buf, doc.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

// part returns one part's bytes from a written package.
func part(t *testing.T, raw []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(b)
	}
	t.Fatalf("the package has no part %q", name)
	return ""
}

func partNames(t *testing.T, raw []byte) []string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	out := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		out = append(out, f.Name)
	}
	return out
}

func TestLower_ProducesStylesNotDirectFormatting(t *testing.T) {
	d := lower(t, heading(1, "Title"), text("Body."))
	body := d.Sections[0].Content

	if got := body[0].Paragraph.StyleID; got != doc.HeadingStyleID(1) {
		t.Errorf("heading style = %q, want %q", got, doc.HeadingStyleID(1))
	}
	if got := body[1].Paragraph.StyleID; got != doc.StyleNormal {
		t.Errorf("text style = %q, want %q", got, doc.StyleNormal)
	}

	// The subtraction is the point: a run whose appearance is exactly what its
	// style already says carries no direct formatting at all. Emitting the full
	// set would produce a document that looks identical and cannot be restyled,
	// because direct formatting wins over a style.
	for i, c := range body {
		if !c.Paragraph.Runs[0].Properties.IsZero() {
			t.Errorf("paragraph %d carries direct formatting %+v that its style already provides",
				i, c.Paragraph.Runs[0].Properties)
		}
	}
}

// TestLower_MarksBecomeDirectFormatting is the other half of that rule. A mark
// is by definition something the theme styles individually, and minting a
// character style per mark combination would fill the styles part with names
// nobody chose.
func TestLower_MarksBecomeDirectFormatting(t *testing.T) {
	d := lower(t, text("moved", "flagged"))
	props := d.Sections[0].Content[0].Paragraph.Runs[0].Properties

	if props.IsZero() {
		t.Fatal("a marked run carries no direct formatting, so the mark is invisible")
	}
	if !props.Italic {
		t.Errorf("properties = %+v, want the mark's italic", props)
	}
}

func TestLower_HeadingStylesCoverEveryLevelUsed(t *testing.T) {
	d := lower(t, heading(1, "One"), heading(2, "Two"), heading(3, "Three"))

	for level := 1; level <= 3; level++ {
		id := doc.HeadingStyleID(level)
		found := false
		for _, s := range d.Styles.Paragraph {
			if s.ID == id {
				found = true
				if s.OutlineLevel != level {
					t.Errorf("%s has outline level %d, want %d", id, s.OutlineLevel, level)
				}
				if !s.KeepNext {
					t.Errorf("%s does not keep with the next paragraph; a heading stranded at the foot of a page is the commonest ugly artefact in a generated document", id)
				}
				if s.NextStyleID != doc.StyleNormal {
					t.Errorf("%s's next style is %q; typing after a heading should give body text", id, s.NextStyleID)
				}
			}
		}
		if !found {
			t.Errorf("the styles part declares no %s, so those paragraphs open unstyled", id)
		}
	}
}

// TestLower_SectionsMergeUnlessTheGeometryChanges pins that a specification's
// sections are logical divisions rather than page breaks. Emitting a section
// break per division would put a page break between every heading and its
// prose.
func TestLower_SectionsMergeUnlessTheGeometryChanges(t *testing.T) {
	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Sections: []spec.Section{
			{ID: "a", Blocks: []spec.Block{heading(1, "A")}},
			{ID: "b", Blocks: []spec.Block{text("B")}},
			{ID: "c", Blocks: []spec.Block{text("C")}},
		},
	}
	res, err := resolve.Resolve(context.Background(), s, resolve.Options{Format: artifact.FormatDOCX})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	d, err := doc.Lower(res.Doc)
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}

	if len(d.Sections) != 1 {
		t.Fatalf("three specification sections with one geometry produced %d document sections", len(d.Sections))
	}
	if len(d.Sections[0].Content) != 3 {
		t.Errorf("the merged section holds %d items, want 3", len(d.Sections[0].Content))
	}
}

func TestLower_PageBreakIsAnExplicitBreak(t *testing.T) {
	d := lower(t, text("A"), spec.Block{Kind: spec.BlockPageBreak, PageBreak: &spec.PageBreak{}}, text("B"))
	runs := d.Sections[0].Content[1].Paragraph.Runs

	if len(runs) != 1 || runs[0].Break != doc.BreakPage {
		t.Errorf("the page-break block produced %+v, want a page break run", runs)
	}
}

func TestLower_NotesBecomeFootnotes(t *testing.T) {
	d := lower(t, text("Body."), spec.Block{Kind: spec.BlockNotes, Notes: &spec.Notes{Content: "A note."}})

	if len(d.Footnotes) != 1 {
		t.Fatalf("the notes block produced %d footnotes, want 1", len(d.Footnotes))
	}
	anchor := d.Sections[0].Content[1].Paragraph.Runs[0]
	if anchor.FootnoteRef != 1 {
		t.Errorf("the anchor references footnote %d, want 1", anchor.FootnoteRef)
	}
	if anchor.StyleID != doc.StyleFootnoteRef {
		t.Errorf("the anchor run has style %q, want %q", anchor.StyleID, doc.StyleFootnoteRef)
	}
}

func TestLower_AssetBecomesADrawing(t *testing.T) {
	d := lower(t, spec.Block{Kind: spec.BlockAsset,
		Asset: &spec.Asset{Handle: pngURI(), AltText: "a chart"}})

	drawing := d.Sections[0].Content[0].Paragraph.Runs[0].Drawing
	if drawing == nil {
		t.Fatal("the asset block produced no drawing")
	}
	if drawing.AltText != "a chart" {
		t.Errorf("AltText = %q, want %q", drawing.AltText, "a chart")
	}
	if drawing.WidthEMU <= 0 || drawing.HeightEMU <= 0 {
		t.Errorf("the drawing is %dx%d EMU; both must be concrete by this point",
			drawing.WidthEMU, drawing.HeightEMU)
	}
	if drawing.FallbackIndex != -1 {
		t.Errorf("a raster needs no fallback, got index %d", drawing.FallbackIndex)
	}
}

// TestLower_SVGNeedsARasterFallback is the capability matrix's declared
// degradation, enforced. Word draws an SVG only when a raster accompanies it,
// and Vellum will not rasterise — so a lone vector is a document that opens
// with a hole where the chart is, and that is refused rather than shipped.
func TestLower_SVGNeedsARasterFallback(t *testing.T) {
	t.Run("alone is refused", func(t *testing.T) {
		_, err := doc.Lower(resolved(t, spec.Block{Kind: spec.BlockAsset,
			Asset: &spec.Asset{Handle: svgURI()}}))
		if !verr.HasCode(err, verr.VELLUM_ASSET_MEDIA_UNSUPPORTED) {
			t.Fatalf("error = %v, want VELLUM_ASSET_MEDIA_UNSUPPORTED", err)
		}
	})

	t.Run("paired with a raster is embedded", func(t *testing.T) {
		d := lower(t,
			spec.Block{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: svgURI()}},
			spec.Block{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: pngURI()}},
		)
		var vector *doc.Drawing
		for _, c := range d.Sections[0].Content {
			dr := c.Paragraph.Runs[0].Drawing
			if d.Media[dr.MediaIndex].MediaType == "image/svg+xml" {
				vector = dr
			}
		}
		if vector == nil {
			t.Fatal("no vector drawing was produced")
		}
		if vector.FallbackIndex < 0 {
			t.Fatal("the vector has no raster fallback")
		}
		if d.Media[vector.FallbackIndex].MediaType != "image/png" {
			t.Errorf("the fallback is %q, want a raster", d.Media[vector.FallbackIndex].MediaType)
		}
	})
}

func TestLower_TableTilesItsGrid(t *testing.T) {
	table := &spec.Table{
		// A two-level banner: one node spanning two leaves.
		ColumnHeaders: spec.HeaderTree{
			{Label: "Region", Span: 2, Children: []spec.HeaderNode{
				{Label: "North", Span: 1}, {Label: "South", Span: 1},
			}},
			{Label: "Total", Span: 1},
		},
		Body: [][]spec.Cell{
			{{Text: "1"}, {Text: "2"}, {Text: "3"}},
			{{Text: "4"}, {Text: "5"}, {Text: "6"}},
		},
		Caption: "Table 1",
	}
	d := lower(t, spec.Block{Kind: spec.BlockTable, Table: table})

	tbl := d.Sections[0].Content[0].Table
	if tbl == nil {
		t.Fatal("the table block produced no table")
	}
	if len(tbl.Grid) != 3 {
		t.Fatalf("the grid has %d columns, want 3", len(tbl.Grid))
	}

	// Every row must tile the grid exactly. A row that does not opens as a
	// table with a hole in it, which looks deliberate.
	for i, row := range tbl.Rows {
		span := 0
		for _, c := range row.Cells {
			s := c.GridSpan
			if s < 1 {
				s = 1
			}
			span += s
		}
		if span != len(tbl.Grid) {
			t.Errorf("row %d spans %d columns, want %d", i, span, len(tbl.Grid))
		}
	}
	if tbl.HeaderRows != 2 {
		t.Errorf("HeaderRows = %d, want 2 for a two-level banner", tbl.HeaderRows)
	}
	// The banner rows repeat when the table breaks across pages. Without it a
	// multi-page table shows its headings once.
	for i := 0; i < tbl.HeaderRows; i++ {
		if !tbl.Rows[i].Header {
			t.Errorf("banner row %d is not marked as a repeating header", i)
		}
	}

	// The caption is real content in the Caption style, because that is what a
	// reader can restyle. w:tblCaption is an accessibility description Word
	// does not render.
	caption := d.Sections[0].Content[1].Paragraph
	if caption == nil || caption.StyleID != doc.StyleCaption {
		t.Errorf("the caption is not a paragraph in the Caption style")
	}
	if tbl.Caption != "Table 1" {
		t.Errorf("the table's accessibility caption = %q", tbl.Caption)
	}
}

// TestLower_TableGridSumsExactly pins that the columns add up to the declared
// table width. A grid that does not makes Word silently re-measure, and a table
// that re-measures on open is one whose appearance depends on the reader.
func TestLower_TableGridSumsExactly(t *testing.T) {
	// Seven columns, so the division has a remainder to distribute.
	headers := make(spec.HeaderTree, 7)
	row := make([]spec.Cell, 7)
	for i := range headers {
		headers[i] = spec.HeaderNode{Label: "c", Span: 1}
		row[i] = spec.Cell{Text: "x"}
	}
	d := lower(t, spec.Block{Kind: spec.BlockTable,
		Table: &spec.Table{ColumnHeaders: headers, Body: [][]spec.Cell{row}}})

	tbl := d.Sections[0].Content[0].Table
	var total int64
	for _, c := range tbl.Grid {
		total += c
	}
	want := d.Sections[0].Page.ContentWidth()
	if total != want {
		t.Errorf("the grid sums to %d EMU, want the content width %d", total, want)
	}
}

// TestLower_RowHeaderStubMergesVertically pins the analytical stub: a grouping
// variable's label appears once against the block of rows it covers.
func TestLower_RowHeaderStubMergesVertically(t *testing.T) {
	table := &spec.Table{
		ColumnHeaders: spec.HeaderTree{{Label: "Value", Span: 1}},
		RowHeaders: spec.HeaderTree{
			{Label: "Group A", Span: 2, Children: []spec.HeaderNode{
				{Label: "a1", Span: 1}, {Label: "a2", Span: 1},
			}},
		},
		Body: [][]spec.Cell{{{Text: "1"}}, {{Text: "2"}}},
	}
	d := lower(t, spec.Block{Kind: spec.BlockTable, Table: table})
	tbl := d.Sections[0].Content[0].Table

	// Two stub columns plus one value column.
	if len(tbl.Grid) != 3 {
		t.Fatalf("the grid has %d columns, want 3", len(tbl.Grid))
	}
	bodyRows := tbl.Rows[tbl.HeaderRows:]
	if len(bodyRows) != 2 {
		t.Fatalf("got %d body rows, want 2", len(bodyRows))
	}
	if bodyRows[0].Cells[0].VerticalMerge != doc.MergeRestart {
		t.Errorf("the group label does not begin a vertical merge")
	}
	if bodyRows[1].Cells[0].VerticalMerge != doc.MergeContinue {
		t.Errorf("the second row does not continue the merge, so the label repeats")
	}
}

func TestLower_AnnotationsSitBesideTheValue(t *testing.T) {
	table := &spec.Table{
		ColumnHeaders: spec.HeaderTree{{Label: "Share", Span: 1}},
		Body: [][]spec.Cell{{{
			Value:  &spec.Value{Kind: spec.ValueNumber, Number: 0.457},
			Format: "0.0%",
			Annotations: []spec.Annotation{
				{Text: "a"},
				{Text: "*", Position: spec.AnnotationPrefix},
			},
		}}},
	}
	d := lower(t, spec.Block{Kind: spec.BlockTable, Table: table})
	tbl := d.Sections[0].Content[0].Table
	cell := tbl.Rows[tbl.HeaderRows].Cells[0]
	runs := cell.Content[0].Paragraph.Runs

	if len(runs) != 3 {
		t.Fatalf("got %d runs, want prefix, value and superscript: %+v", len(runs), runs)
	}
	if runs[0].Text != "*" {
		t.Errorf("run 0 = %q, want the prefix annotation", runs[0].Text)
	}
	if runs[1].Text != "45.7%" {
		t.Errorf("run 1 = %q, want the formatted value", runs[1].Text)
	}
	// An annotation attaches to the value rather than replacing it, and a
	// significance letter belongs raised.
	if runs[2].Text != "a" || !runs[2].Properties.Superscript {
		t.Errorf("run 2 = %+v, want a superscript annotation", runs[2])
	}
}

// TestLower_ProducesNothingAdvancedOnly is the boundary between the two public
// APIs, asserted rather than assumed.
//
// Without it the lowering quietly grows a capability and the block model
// acquires a feature the capability matrix never declared. A consumer needing
// one of these builds a Document directly, which is why the model is exported.
func TestLower_ProducesNothingAdvancedOnly(t *testing.T) {
	d := lower(t,
		heading(1, "Title"),
		text("Body.", "flagged"),
		spec.Block{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: pngURI()}},
		spec.Block{Kind: spec.BlockNotes, Notes: &spec.Notes{Content: "note"}},
		spec.Block{Kind: spec.BlockSpacer, Spacer: &spec.Spacer{Height: spec.Points(12)}},
		spec.Block{Kind: spec.BlockPageBreak, PageBreak: &spec.PageBreak{}},
		spec.Block{Kind: spec.BlockTable, Table: &spec.Table{
			ColumnHeaders: spec.HeaderTree{{Label: "x", Span: 1}},
			Body:          [][]spec.Cell{{{Text: "1"}}},
		}},
	)

	if len(doc.AdvancedOnly) == 0 {
		t.Fatal("the advanced-only list is empty, so this test asserts nothing")
	}
	if !d.Numbering.IsEmpty() {
		t.Error("the lowering produced list numbering, which no block kind should reach")
	}
	if len(d.Headers) > 0 || len(d.Footers) > 0 {
		t.Error("the lowering produced headers or footers, which no block kind should reach")
	}
	for _, s := range d.Sections {
		if s.Type != doc.SectionNextPage {
			t.Errorf("the lowering produced a %q section break", s.Type)
		}
		if s.TitlePage {
			t.Error("the lowering suppressed a title-page header")
		}
		if s.HeaderID != "" || s.FooterID != "" {
			t.Error("the lowering referenced a running head or foot")
		}
		walkParagraphs(s.Content, func(p *doc.Paragraph) {
			if p.NumberingID != 0 {
				t.Error("the lowering numbered a paragraph")
			}
			for _, r := range p.Runs {
				if r.Field != nil {
					t.Errorf("the lowering produced a field: %q", r.Field.Instruction)
				}
				if r.Tab {
					t.Error("the lowering produced a tab")
				}
				if r.Break == doc.BreakColumn || r.Break == doc.BreakLine {
					t.Errorf("the lowering produced a %q break", r.Break)
				}
				// The footnote reference style is the one character style the
				// lowering is allowed to name, because a footnote anchor has to
				// be raised and that is a character-level property.
				if r.StyleID != "" && r.StyleID != doc.StyleFootnoteRef {
					t.Errorf("the lowering named the character style %q", r.StyleID)
				}
			}
		})
	}
}

func walkParagraphs(content []doc.Content, visit func(*doc.Paragraph)) {
	for i := range content {
		c := &content[i]
		if c.Paragraph != nil {
			visit(c.Paragraph)
		}
		if c.Table != nil {
			for j := range c.Table.Rows {
				for k := range c.Table.Rows[j].Cells {
					walkParagraphs(c.Table.Rows[j].Cells[k].Content, visit)
				}
			}
		}
	}
}

func TestWrite_PackageShape(t *testing.T) {
	raw := write(t, lower(t,
		heading(1, "Title"),
		text("Body."),
		spec.Block{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: pngURI()}},
		spec.Block{Kind: spec.BlockNotes, Notes: &spec.Notes{Content: "note"}},
	))

	names := partNames(t, raw)
	if names[0] != "[Content_Types].xml" {
		t.Errorf("first entry is %q, want [Content_Types].xml", names[0])
	}
	for _, want := range []string{
		"_rels/.rels", "word/document.xml", "word/_rels/document.xml.rels",
		"word/styles.xml", "word/settings.xml", "word/footnotes.xml",
		"word/media/image1.png", "docProps/core.xml", "docProps/app.xml",
	} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Errorf("the package has no %s\ngot: %v", want, names)
		}
	}
	// Numbering is absent because the document has no lists, which is what an
	// authored document looks like.
	for _, n := range names {
		if n == "word/numbering.xml" {
			t.Error("the package carries a numbering part for a document with no lists")
		}
	}
}

// TestWrite_ImageRelationshipsResolve is the check that would have caught a
// whole class of silent breakage: document.xml references an image by
// relationship ID, and the relationships part is written afterwards, so the two
// must agree.
func TestWrite_ImageRelationshipsResolve(t *testing.T) {
	raw := write(t, lower(t,
		spec.Block{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: pngURI()}},
	))
	document := part(t, raw, "word/document.xml")
	rels := part(t, raw, "word/_rels/document.xml.rels")

	id := between(t, document, `<a:blip r:embed="`, `"`)
	if !stdstrings.Contains(rels, `Id="`+id+`"`) {
		t.Fatalf("document.xml references %q, which the relationships part does not declare:\n%s", id, rels)
	}
	if !stdstrings.Contains(rels, `Id="`+id+`" Type="`+
		"http://schemas.openxmlformats.org/officeDocument/2006/relationships/image") {
		t.Errorf("%q is declared but is not an image relationship:\n%s", id, rels)
	}
}

func between(t *testing.T, s, open, close string) string {
	t.Helper()
	i := stdstrings.Index(s, open)
	if i < 0 {
		t.Fatalf("no %q in the part", open)
	}
	rest := s[i+len(open):]
	j := stdstrings.Index(rest, close)
	if j < 0 {
		t.Fatalf("unterminated %q", open)
	}
	return rest[:j]
}

func TestWrite_SectionPropertiesAreOnTheBody(t *testing.T) {
	raw := write(t, lower(t, text("Body.")))
	document := part(t, raw, "word/document.xml")

	// The final section's properties belong on the body. Putting them in the
	// last paragraph instead produces a document that opens with every page the
	// same size, silently.
	if !stdstrings.Contains(document, `</w:p><w:sectPr>`) {
		t.Errorf("the body carries no trailing sectPr:\n%s", document)
	}
}

func TestWrite_FootnotesCarryTheReservedSeparators(t *testing.T) {
	raw := write(t, lower(t, spec.Block{Kind: spec.BlockNotes, Notes: &spec.Notes{Content: "note"}}))
	footnotes := part(t, raw, "word/footnotes.xml")

	// Ids -1 and 0 are reserved for the separator and continuation separator,
	// which every document carries. Omitting them makes Word draw a footnote
	// with no rule above it, or repair the file.
	for _, want := range []string{`w:type="separator" w:id="-1"`, `w:type="continuationSeparator" w:id="0"`, `w:id="1"`} {
		if !stdstrings.Contains(footnotes, want) {
			t.Errorf("footnotes.xml has no %s:\n%s", want, footnotes)
		}
	}
}

func TestWrite_ProvenanceReachesCustomProperties(t *testing.T) {
	d := lower(t, text("Body."))
	rec := provenanceFixture()
	d.Provenance = &rec

	raw := write(t, d)
	custom := part(t, raw, "docProps/custom.xml")

	for _, want := range []string{"VellumVersion", "VellumSpecHash", "VellumProvenanceHash"} {
		if !stdstrings.Contains(custom, want) {
			t.Errorf("custom.xml has no %s:\n%s", want, custom)
		}
	}
	// pid starts at 2: 0 and 1 are reserved, and a document numbering from 0 is
	// one Word refuses to open.
	if !stdstrings.Contains(custom, `pid="2"`) {
		t.Errorf("custom.xml does not number its properties from 2:\n%s", custom)
	}
	if stdstrings.Contains(custom, `pid="0"`) || stdstrings.Contains(custom, `pid="1"`) {
		t.Errorf("custom.xml uses a reserved pid:\n%s", custom)
	}

	names := partNames(t, raw)
	found := false
	for _, n := range names {
		if n == "docProps/custom.xml" {
			found = true
		}
	}
	if !found {
		t.Error("the custom properties part is not in the package")
	}
}

// TestWrite_NoProvenanceNoPart pins that provenance is opt-in. A document
// carrying an empty custom-properties part would be claiming a record it does
// not have.
func TestWrite_NoProvenanceNoPart(t *testing.T) {
	raw := write(t, lower(t, text("Body.")))
	for _, n := range partNames(t, raw) {
		if n == "docProps/custom.xml" {
			t.Error("a document with no provenance carries a custom properties part")
		}
	}
}

func TestWrite_IsDeterministic(t *testing.T) {
	build := func() []byte {
		return write(t, lower(t,
			heading(1, "Title"),
			text("Body.", "flagged"),
			spec.Block{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: pngURI()}},
			spec.Block{Kind: spec.BlockNotes, Notes: &spec.Notes{Content: "note"}},
			spec.Block{Kind: spec.BlockTable, Table: &spec.Table{
				ColumnHeaders: spec.HeaderTree{{Label: "x", Span: 1}},
				Body:          [][]spec.Cell{{{Text: "1"}}},
			}},
		))
	}
	first := build()
	for range 50 {
		if !bytes.Equal(first, build()) {
			t.Fatal("two identical composes produced different bytes")
		}
	}
}

func provenanceFixture() provenance.Record {
	return provenance.Record{
		VellumVersion:   "0.1.0-test",
		SourceDateEpoch: zipdet.PinnedEpoch,
		SpecHash:        "0123456789abcdef0123456789abcdef",
		ThemeHash:       "fedcba9876543210fedcba9876543210",
		Fonts: []provenance.FontRef{
			{Family: "Georgia", SubstitutedWith: "Times New Roman"},
		},
	}
}
