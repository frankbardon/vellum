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

func markerAnchor(t *testing.T, src []byte, name string) anchor.Anchor {
	t.Helper()
	return anchor.Anchor{
		Name: name,
		Kind: anchor.KindMarker,
		Part: partDocument,
		Span: elementSpan(t, src, "p", 0),
	}
}

func TestSpliceMarker_EntirelyWithinOneRun(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Dear {{customer_name}}, thanks.</w:t></w:r></w:p>`)
	pkg := buildPackage(t, src)
	a := markerAnchor(t, src, "customer_name")

	seq := fragment.Sequence{Blocks: []fragment.Block{textBlock(run("Acme & Co.", fragment.TextStyle{}))}}
	repl, err := splice.Splice(pkg, a, seq)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	out := mustApply(t, src, []xmlcopy.Replacement{repl})

	// The marker is entirely consumed within one run: Prefix and Suffix are
	// both nil, so the only formatting source is Site.RunRPr — the run's own
	// bold rPr — and the new text must still come out bold.
	if !bytes.Contains(out, []byte(`<w:rPr><w:b/></w:rPr><w:t>Acme &amp; Co.</w:t>`)) {
		t.Errorf("expected the marker's own bold formatting to survive the splice: %s", out)
	}
	if !bytes.Contains(out, []byte(`<w:t xml:space="preserve">Dear </w:t>`)) {
		t.Errorf("expected the prefix piece to survive, preserved: %s", out)
	}
	if !bytes.Contains(out, []byte(`<w:t>, thanks.</w:t>`)) {
		t.Errorf("expected the suffix piece to survive: %s", out)
	}
}

func TestSpliceMarker_FragmentedAcrossThreeRunsFormattingBasisFromFirstTouchedRun(t *testing.T) {
	// A marker split by Word's own editing (spell-check, revision boundary)
	// across three runs, none of which the match leaves any text in — the
	// exact "Prefix and Suffix both nil" gap the story exists to close.
	src := wordDoc(`<w:p>` +
		`<w:r><w:rPr><w:color w:val="FF0000"/></w:rPr><w:t>{{cust</w:t></w:r>` +
		`<w:r><w:rPr><w:i/></w:rPr><w:t>omer_na</w:t></w:r>` +
		`<w:r><w:rPr><w:u w:val="single"/></w:rPr><w:t>me}}</w:t></w:r>` +
		`</w:p>`)
	pkg := buildPackage(t, src)
	a := markerAnchor(t, src, "customer_name")

	seq := fragment.Sequence{Blocks: []fragment.Block{textBlock(run("Acme", fragment.TextStyle{}))}}
	repl, err := splice.Splice(pkg, a, seq)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	out := mustApply(t, src, []xmlcopy.Replacement{repl})

	// The formatting basis is the *first* touched run's own rPr (color), not
	// the second or third run's — even though none of the three survives as
	// a Prefix or Suffix Piece.
	if !bytes.Contains(out, []byte(`<w:rPr><w:color w:val="FF0000"/></w:rPr><w:t>Acme</w:t>`)) {
		t.Errorf("expected the first touched run's own rPr as the formatting basis: %s", out)
	}
	if bytes.Contains(out, []byte("<w:i/>")) || bytes.Contains(out, []byte(`<w:u w:val="single"/>`)) {
		t.Errorf("the discarded runs' own formatting must not leak in: %s", out)
	}
}

func TestSpliceMarker_MultiRunSplitPreservesSurroundingText(t *testing.T) {
	src := wordDoc(`<w:p>` +
		`<w:r><w:t>Dear {{cust</w:t></w:r>` +
		`<w:r><w:t>omer_name}}, thanks.</w:t></w:r>` +
		`</w:p>`)
	pkg := buildPackage(t, src)
	a := markerAnchor(t, src, "customer_name")

	seq := fragment.Sequence{Blocks: []fragment.Block{textBlock(run("Acme", fragment.TextStyle{}))}}
	repl, err := splice.Splice(pkg, a, seq)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	out := mustApply(t, src, []xmlcopy.Replacement{repl})

	if !bytes.Contains(out, []byte("Dear ")) || !bytes.Contains(out, []byte(", thanks.")) {
		t.Errorf("surrounding text must survive: %s", out)
	}
	if !bytes.Contains(out, []byte("Acme")) {
		t.Errorf("new value missing: %s", out)
	}
}

func TestSpliceMarker_BoldOverrideDoesNotDuplicateAnAlreadyBoldBase(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>{{name}}</w:t></w:r></w:p>`)
	pkg := buildPackage(t, src)
	a := markerAnchor(t, src, "name")

	seq := fragment.Sequence{Blocks: []fragment.Block{
		textBlock(run("Acme", fragment.TextStyle{Bold: true})),
	}}
	repl, err := splice.Splice(pkg, a, seq)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	out := mustApply(t, src, []xmlcopy.Replacement{repl})

	if bytes.Count(out, []byte("<w:b/>")) != 1 {
		t.Errorf("expected exactly one <w:b/>, got: %s", out)
	}
}

func TestSpliceMarker_ItalicOverrideAddedOnTopOfPlainBase(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>{{name}}</w:t></w:r></w:p>`)
	pkg := buildPackage(t, src)
	a := markerAnchor(t, src, "name")

	seq := fragment.Sequence{Blocks: []fragment.Block{
		textBlock(run("Acme", fragment.TextStyle{Italic: true, Underline: true})),
	}}
	repl, err := splice.Splice(pkg, a, seq)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	out := mustApply(t, src, []xmlcopy.Replacement{repl})

	if !bytes.Contains(out, []byte(`<w:rPr><w:i/><w:iCs/><w:u w:val="single"/></w:rPr>`)) {
		t.Errorf("expected italic and underline layered on: %s", out)
	}
}

func TestSpliceMarker_RejectsZeroBlocks(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>{{name}}</w:t></w:r></w:p>`)
	pkg := buildPackage(t, src)
	a := markerAnchor(t, src, "name")

	_, err := splice.Splice(pkg, a, fragment.Sequence{})
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED", err)
	}
}

func TestSpliceMarker_RejectsMultipleBlocks(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>{{name}}</w:t></w:r></w:p>`)
	pkg := buildPackage(t, src)
	a := markerAnchor(t, src, "name")

	seq := fragment.Sequence{Blocks: []fragment.Block{
		textBlock(run("one", fragment.TextStyle{})),
		textBlock(run("two", fragment.TextStyle{})),
	}}
	_, err := splice.Splice(pkg, a, seq)
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED", err)
	}
}

func TestSpliceMarker_RejectsTableBlock(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>{{name}}</w:t></w:r></w:p>`)
	pkg := buildPackage(t, src)
	a := markerAnchor(t, src, "name")

	seq := fragment.Sequence{Blocks: []fragment.Block{
		{Kind: spec.BlockTable, Table: &fragment.Table{Body: [][]fragment.Cell{{{Text: "x"}}}}},
	}}
	_, err := splice.Splice(pkg, a, seq)
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED", err)
	}
}

func TestSpliceMarker_RejectsAssetBlock(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>{{name}}</w:t></w:r></w:p>`)
	pkg := buildPackage(t, src)
	a := markerAnchor(t, src, "name")

	seq := fragment.Sequence{
		Assets: []fragment.Asset{{MediaType: "image/png", Hash: "abc", Bytes: onePixelPNG}},
		Blocks: []fragment.Block{
			{Kind: spec.BlockAsset, Asset: &fragment.AssetRef{AssetIndex: 0, WidthEMU: 100, HeightEMU: 100}},
		},
	}
	_, err := splice.Splice(pkg, a, seq)
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED", err)
	}
}
