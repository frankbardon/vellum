package splice_test

import (
	"bytes"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/template/splice"
	"github.com/frankbardon/vellum/xmlcopy"
)

const (
	nsPresentationS = "http://schemas.openxmlformats.org/presentationml/2006/main"
	nsDrawingMainS  = "http://schemas.openxmlformats.org/drawingml/2006/main"

	ctSlideS  = "application/vnd.openxmlformats-officedocument.presentationml.slide+xml"
	partSlide = "/ppt/slides/slide1.xml"
)

// slideDoc wraps one shape (built by the caller) inside a realistic
// PresentationML slide root, mirroring wordDoc's own role for DOCX fixtures.
func slideDoc(shapeXML string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<p:sld xmlns:a="` + nsDrawingMainS + `" xmlns:r="` + nsRelationships +
		`" xmlns:p="` + nsPresentationS + `">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/>` +
		shapeXML +
		`</p:spTree></p:cSld></p:sld>`)
}

func buildPackagePPTX(t *testing.T, slideXML []byte) *opc.Package {
	t.Helper()
	p := opc.New()
	if err := p.Put(&opc.Part{Name: partSlide, ContentType: ctSlideS, Data: slideXML}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	return p
}

func elementSpanPPTX(t *testing.T, src []byte, local string, n int) xmlcopy.Span {
	t.Helper()
	var spans []xmlcopy.Span
	if err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if e.Name.Space == nsPresentationS && e.Name.Local == local {
			spans = append(spans, e.Span)
		}
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if n >= len(spans) {
		t.Fatalf("%s %d not found; only %d found", local, n, len(spans))
	}
	return spans[n]
}

func shapeAnchor(t *testing.T, src []byte, name string) anchor.Anchor {
	t.Helper()
	return anchor.Anchor{
		Name: name,
		Kind: anchor.KindShape,
		Part: partSlide,
		Span: elementSpanPPTX(t, src, "sp", 0),
	}
}

func TestSpliceShape_FormattingBasisFromPlaceholderRun(t *testing.T) {
	shape := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="headline"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr/>` +
		`<p:txBody><a:bodyPr/><a:p><a:r><a:rPr b="1"/><a:t>placeholder</a:t></a:r></a:p></p:txBody>` +
		`</p:sp>`
	src := slideDoc(shape)
	pkg := buildPackagePPTX(t, src)
	a := shapeAnchor(t, src, "headline")

	seq := fragment.Sequence{Blocks: []fragment.Block{
		{Kind: spec.BlockText, Paragraph: &fragment.Paragraph{Runs: []fragment.Run{{Text: "Q3 Results"}}}},
	}}

	repl, err := splice.Splice(pkg, a, seq)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	out := mustApply(t, src, []xmlcopy.Replacement{repl})

	if !bytes.Contains(out, []byte(`<a:rPr b="1"/><a:t>Q3 Results</a:t>`)) {
		t.Errorf("new run did not inherit the placeholder's own rPr: %s", out)
	}
	if bytes.Contains(out, []byte("placeholder")) {
		t.Error("placeholder text survived the splice")
	}
	// bodyPr survives untouched — the metadata-preservation behaviour this
	// strategy exists to get right, unlike a naive whole-Content replace.
	if !bytes.Contains(out, []byte(`<p:txBody><a:bodyPr/><a:p>`)) {
		t.Errorf("bodyPr was not preserved ahead of the new paragraph: %s", out)
	}
}

func TestSpliceShape_BodyPrAndLstStyleSurviveAMultiParagraphSplice(t *testing.T) {
	shape := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="body"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr/>` +
		`<p:txBody><a:bodyPr anchor="t"/><a:lstStyle><a:lvl1pPr/></a:lstStyle>` +
		`<a:p><a:r><a:t>placeholder</a:t></a:r></a:p></p:txBody>` +
		`</p:sp>`
	src := slideDoc(shape)
	pkg := buildPackagePPTX(t, src)
	a := shapeAnchor(t, src, "body")

	seq := fragment.Sequence{Blocks: []fragment.Block{
		{Kind: spec.BlockText, Paragraph: &fragment.Paragraph{Runs: []fragment.Run{{Text: "First."}}}},
		{Kind: spec.BlockText, Paragraph: &fragment.Paragraph{Runs: []fragment.Run{{Text: "Second."}}}},
	}}

	repl, err := splice.Splice(pkg, a, seq)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	out := mustApply(t, src, []xmlcopy.Replacement{repl})

	if !bytes.Contains(out, []byte(`<a:bodyPr anchor="t"/><a:lstStyle><a:lvl1pPr/></a:lstStyle><a:p>`)) {
		t.Errorf("bodyPr/lstStyle were not preserved verbatim ahead of the new paragraphs: %s", out)
	}
	if !bytes.Contains(out, []byte(`<a:t>First.</a:t>`)) || !bytes.Contains(out, []byte(`<a:t>Second.</a:t>`)) {
		t.Errorf("both paragraphs did not render: %s", out)
	}
	if bytes.Contains(out, []byte("placeholder")) {
		t.Error("placeholder text survived the splice")
	}
}

func TestSpliceShape_TableBlockIsRejected(t *testing.T) {
	shape := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="body"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr/>` +
		`<p:txBody><a:bodyPr/><a:p><a:r><a:t>placeholder</a:t></a:r></a:p></p:txBody>` +
		`</p:sp>`
	src := slideDoc(shape)
	pkg := buildPackagePPTX(t, src)
	a := shapeAnchor(t, src, "body")

	seq := fragment.Sequence{Blocks: []fragment.Block{
		{Kind: spec.BlockTable, Table: &fragment.Table{}},
	}}

	_, err := splice.Splice(pkg, a, seq)
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_SHAPE_BLOCK_UNSUPPORTED) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_SHAPE_BLOCK_UNSUPPORTED", err)
	}
}

func TestSpliceShape_ZeroBlocksProducesAnEmptyParagraph(t *testing.T) {
	shape := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="body"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr/>` +
		`<p:txBody><a:bodyPr/><a:p><a:r><a:t>placeholder</a:t></a:r></a:p></p:txBody>` +
		`</p:sp>`
	src := slideDoc(shape)
	pkg := buildPackagePPTX(t, src)
	a := shapeAnchor(t, src, "body")

	repl, err := splice.Splice(pkg, a, fragment.Sequence{})
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	out := mustApply(t, src, []xmlcopy.Replacement{repl})
	if !bytes.Contains(out, []byte(`<p:txBody><a:bodyPr/><a:p/></p:txBody>`)) {
		t.Errorf("zero blocks did not produce a minimal empty paragraph: %s", out)
	}
}

func TestSpliceShape_NoTxBodyIsCodedError(t *testing.T) {
	shape := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="decor"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr/>` +
		`</p:sp>`
	src := slideDoc(shape)
	pkg := buildPackagePPTX(t, src)
	a := shapeAnchor(t, src, "decor")

	seq := fragment.Sequence{Blocks: []fragment.Block{
		{Kind: spec.BlockText, Paragraph: &fragment.Paragraph{Runs: []fragment.Run{{Text: "x"}}}},
	}}
	_, err := splice.Splice(pkg, a, seq)
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_SHAPE_TXBODY_MISSING) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_SHAPE_TXBODY_MISSING", err)
	}
}
