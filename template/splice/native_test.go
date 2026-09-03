package splice_test

import (
	"bytes"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/template/splice"
	"github.com/frankbardon/vellum/xmlcopy"
)

// nativeDoc wraps a block-level content control — a direct child of w:body,
// the shape a real "Rich Text content control" is, and the only shape whose
// sdtContent can legally hold block content (paragraphs, a table, a
// drawing). It is deliberately not nested inside a w:p: CT_SdtBlock's own
// content model requires that.
func nativeDoc(sdtContent string) []byte {
	return wordDoc(`<w:p><w:r><w:t>before</w:t></w:r></w:p>` +
		`<w:sdt><w:sdtPr><w:tag w:val="body"/><w:alias w:val="Body"/></w:sdtPr>` +
		`<w:sdtContent>` + sdtContent + `</w:sdtContent></w:sdt>` +
		`<w:p><w:r><w:t>after</w:t></w:r></w:p>`)
}

func nativeAnchor(t *testing.T, src []byte, name string) anchor.Anchor {
	t.Helper()
	return anchor.Anchor{
		Name: name,
		Kind: anchor.KindNative,
		Part: partDocument,
		Span: elementSpan(t, src, "sdt", 0),
	}
}

func run(text string, style fragment.TextStyle) fragment.Run {
	return fragment.Run{Text: text, Style: style}
}

func textBlock(runs ...fragment.Run) fragment.Block {
	return fragment.Block{Kind: spec.BlockText, Paragraph: &fragment.Paragraph{Runs: runs}}
}

func TestSpliceNative_FormattingBasisFromPlaceholderRun(t *testing.T) {
	src := nativeDoc(`<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>placeholder</w:t></w:r></w:p>`)
	pkg := buildPackage(t, src)
	a := nativeAnchor(t, src, "body")

	seq := fragment.Sequence{Blocks: []fragment.Block{
		textBlock(run("Hello, Acme.", fragment.TextStyle{})),
	}}

	repl, err := splice.Splice(pkg, a, seq)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	out := mustApply(t, src, []xmlcopy.Replacement{repl})

	// The new run inherited the placeholder's own bold rPr verbatim.
	if !bytes.Contains(out, []byte(`<w:rPr><w:b/></w:rPr><w:t>Hello, Acme.</w:t>`)) {
		t.Errorf("new run did not inherit the placeholder's own rPr: %s", out)
	}
	// The placeholder text itself is gone.
	if bytes.Contains(out, []byte("placeholder")) {
		t.Error("placeholder text survived the splice")
	}
	// Nothing outside the sdtContent's own Content span moved.
	if !bytes.Contains(out, []byte("<w:t>before</w:t>")) || !bytes.Contains(out, []byte("<w:t>after</w:t>")) {
		t.Errorf("surrounding paragraphs were disturbed: %s", out)
	}
}

func TestSpliceNative_MultiParagraphSequence(t *testing.T) {
	src := nativeDoc(`<w:p><w:r><w:t>placeholder</w:t></w:r></w:p>`)
	pkg := buildPackage(t, src)
	a := nativeAnchor(t, src, "body")

	seq := fragment.Sequence{Blocks: []fragment.Block{
		textBlock(run("First paragraph.", fragment.TextStyle{})),
		textBlock(run("Second paragraph.", fragment.TextStyle{})),
	}}

	repl, err := splice.Splice(pkg, a, seq)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	out := mustApply(t, src, []xmlcopy.Replacement{repl})

	var paragraphs int
	if err := xmlcopy.Walk(out, func(e xmlcopy.Element) error {
		if e.Name.Space == nsWordprocessing && e.Name.Local == "p" {
			paragraphs++
		}
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	// before, after, and the two new paragraphs.
	if paragraphs != 4 {
		t.Errorf("got %d w:p elements, want 4", paragraphs)
	}
	if !bytes.Contains(out, []byte("First paragraph.")) || !bytes.Contains(out, []byte("Second paragraph.")) {
		t.Errorf("both new paragraphs must be present: %s", out)
	}
}

func TestSpliceNative_EmptyPlaceholderFallsBackToNoRPr(t *testing.T) {
	src := nativeDoc(`<w:p/>`)
	pkg := buildPackage(t, src)
	a := nativeAnchor(t, src, "body")

	seq := fragment.Sequence{Blocks: []fragment.Block{
		textBlock(run("New content.", fragment.TextStyle{})),
	}}

	repl, err := splice.Splice(pkg, a, seq)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	out := mustApply(t, src, []xmlcopy.Replacement{repl})

	if !bytes.Contains(out, []byte(`<w:r><w:t>New content.</w:t></w:r>`)) {
		t.Errorf("expected a plain, unformatted run: %s", out)
	}
	if bytes.Contains(out, []byte("<w:rPr")) {
		t.Errorf("no rPr should have been emitted at all: %s", out)
	}
}

func TestSpliceNative_EmptySequenceProducesEmptyParagraph(t *testing.T) {
	src := nativeDoc(`<w:p><w:r><w:t>placeholder</w:t></w:r></w:p>`)
	pkg := buildPackage(t, src)
	a := nativeAnchor(t, src, "body")

	repl, err := splice.Splice(pkg, a, fragment.Sequence{})
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	out := mustApply(t, src, []xmlcopy.Replacement{repl})
	if !bytes.Contains(out, []byte(`<w:sdtContent><w:p/></w:sdtContent>`)) {
		t.Errorf("expected an empty paragraph fallback: %s", out)
	}
}

func TestSpliceNative_TableSequence(t *testing.T) {
	src := nativeDoc(`<w:p><w:r><w:t>placeholder</w:t></w:r></w:p>`)
	pkg := buildPackage(t, src)
	a := nativeAnchor(t, src, "body")

	tbl := &fragment.Table{
		ColumnHeaders: fragment.HeaderTree{{Label: "Q1", Span: 1}, {Label: "Q2", Span: 1}},
		Body: [][]fragment.Cell{
			{{Text: "10"}, {Text: "20"}},
			{{Text: "30"}, {Text: "40"}},
		},
	}
	seq := fragment.Sequence{Blocks: []fragment.Block{
		{Kind: spec.BlockTable, Table: tbl},
	}}

	repl, err := splice.Splice(pkg, a, seq)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	out := mustApply(t, src, []xmlcopy.Replacement{repl})

	if !bytes.Contains(out, []byte("<w:tbl>")) {
		t.Fatalf("expected a w:tbl: %s", out)
	}
	for _, want := range []string{"Q1", "Q2", "10", "20", "30", "40"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("table output missing %q: %s", want, out)
		}
	}
	// Header cell text is bold.
	if !bytes.Contains(out, []byte(`<w:rPr><w:b/><w:bCs/></w:rPr><w:t>Q1</w:t>`)) {
		t.Errorf("header cell should be bold: %s", out)
	}

	var gridCols int
	if err := xmlcopy.Walk(out, func(e xmlcopy.Element) error {
		if e.Name.Space == nsWordprocessing && e.Name.Local == "gridCol" {
			gridCols++
		}
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if gridCols != 2 {
		t.Errorf("got %d gridCol elements, want 2", gridCols)
	}
}

func TestSpliceNative_ImageSequence(t *testing.T) {
	src := nativeDoc(`<w:p><w:r><w:t>placeholder</w:t></w:r></w:p>`)
	pkg := buildPackage(t, src)
	a := nativeAnchor(t, src, "logo")

	seq := fragment.Sequence{
		Assets: []fragment.Asset{{MediaType: "image/png", Hash: "abc123", Bytes: onePixelPNG}},
		Blocks: []fragment.Block{
			{Kind: spec.BlockAsset, Asset: &fragment.AssetRef{
				AssetIndex: 0, WidthEMU: 914400, HeightEMU: 914400, AltText: "Acme logo",
			}},
		},
	}

	repl, err := splice.Splice(pkg, a, seq)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	out := mustApply(t, src, []xmlcopy.Replacement{repl})

	if !bytes.Contains(out, []byte("<w:drawing")) {
		t.Fatalf("expected a w:drawing: %s", out)
	}
	if !bytes.Contains(out, []byte(`descr="Acme logo"`)) {
		t.Errorf("alt text missing: %s", out)
	}

	// The media part was added with a content-hash-derived name.
	mediaPart := "/word/media/imgabc123.png"
	part, ok := pkg.Get(mediaPart)
	if !ok {
		t.Fatalf("media part %s was not added; package has: %v", mediaPart, pkg.Names())
	}
	if part.ContentType != "image/png" {
		t.Errorf("content type = %q, want image/png", part.ContentType)
	}
	b, err := part.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if !bytes.Equal(b, onePixelPNG) {
		t.Error("media part bytes do not match the source asset")
	}

	// A relationship was registered and the drawing embeds it.
	rels, ok := pkg.RelationshipsFor(partDocument)
	if !ok || rels.Len() == 0 {
		t.Fatal("no relationships were registered on the document part")
	}
	rID, ok := rels.IDFor("http://schemas.openxmlformats.org/officeDocument/2006/relationships/image", "media/imgabc123.png")
	if !ok {
		t.Fatalf("no image relationship found for the media target; have: %+v", rels.All())
	}
	if !bytes.Contains(out, []byte(`r:embed="`+rID+`"`)) {
		t.Errorf("drawing does not embed the registered relationship id %s: %s", rID, out)
	}
}

func TestSpliceNative_UnsupportedMediaTypeIsRejected(t *testing.T) {
	src := nativeDoc(`<w:p><w:r><w:t>placeholder</w:t></w:r></w:p>`)
	pkg := buildPackage(t, src)
	a := nativeAnchor(t, src, "logo")

	seq := fragment.Sequence{
		Assets: []fragment.Asset{{MediaType: "image/gif", Hash: "deadbeef", Bytes: []byte("GIF89a")}},
		Blocks: []fragment.Block{
			{Kind: spec.BlockAsset, Asset: &fragment.AssetRef{AssetIndex: 0, WidthEMU: 100, HeightEMU: 100}},
		},
	}

	_, err := splice.Splice(pkg, a, seq)
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_ASSET_MEDIA_UNSUPPORTED) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_ASSET_MEDIA_UNSUPPORTED", err)
	}
}

func TestSpliceNative_UnsupportedBlockKindIsRejected(t *testing.T) {
	src := nativeDoc(`<w:p><w:r><w:t>placeholder</w:t></w:r></w:p>`)
	pkg := buildPackage(t, src)
	a := nativeAnchor(t, src, "body")

	seq := fragment.Sequence{Blocks: []fragment.Block{
		{Kind: spec.BlockPageBreak, Break: &fragment.Break{}},
	}}

	_, err := splice.Splice(pkg, a, seq)
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_BLOCK_UNSUPPORTED) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_BLOCK_UNSUPPORTED", err)
	}
}

func TestSpliceNative_MissingSdtContentIsRejected(t *testing.T) {
	src := wordDoc(`<w:sdt><w:sdtPr><w:tag w:val="body"/></w:sdtPr></w:sdt>`)
	pkg := buildPackage(t, src)
	a := nativeAnchor(t, src, "body")

	_, err := splice.Splice(pkg, a, fragment.Sequence{Blocks: []fragment.Block{textBlock(run("x", fragment.TextStyle{}))}})
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_SDT_CONTENT_MISSING) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_SDT_CONTENT_MISSING", err)
	}
}

func TestSpliceNative_NestedContentControlPlaceholderRunStillCountsAsFirst(t *testing.T) {
	// The outer control's own direct sdtContent has no run of its own — its
	// only child is a nested content control — but firstRunRPrIn recurses
	// into descendants, the same "direct child or descendant" reading
	// template/defrag's own Flatten already documents for exactly this
	// situation (a run inside a nested content control within the same
	// container still contributes). So the nested run's own rPr is still
	// picked up as the outer splice's formatting basis.
	src := nativeDoc(
		`<w:sdt><w:sdtPr><w:tag w:val="inner"/></w:sdtPr>` +
			`<w:sdtContent><w:p><w:r><w:rPr><w:i/></w:rPr><w:t>inner placeholder</w:t></w:r></w:p></w:sdtContent>` +
			`</w:sdt>`)
	pkg := buildPackage(t, src)
	// xmlcopy.Walk visits post-order, so among the two w:sdt elements — the
	// outer "body" control and the inner "inner" control nested inside its
	// own sdtContent — the inner one closes, and is therefore found, first:
	// index 1 is the outer control this test means to splice.
	a := anchor.Anchor{Name: "body", Kind: anchor.KindNative, Part: partDocument, Span: elementSpan(t, src, "sdt", 1)}

	seq := fragment.Sequence{Blocks: []fragment.Block{textBlock(run("outer text", fragment.TextStyle{}))}}
	repl, err := splice.Splice(pkg, a, seq)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	out := mustApply(t, src, []xmlcopy.Replacement{repl})

	if !bytes.Contains(out, []byte(`<w:rPr><w:i/></w:rPr><w:t>outer text</w:t>`)) {
		t.Errorf("expected the nested control's placeholder rPr to be used as the basis: %s", out)
	}
}
