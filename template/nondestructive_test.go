package template_test

// This file is E9-S5's non-destructiveness gate: TestNonDestructiveCorpus,
// named in CLAUDE.md's Non-Skippable CI Gates list.
//
// There is no public template.Fill entry point yet — binding evaluation, the
// repeat/if/with control layer and the Result/Touched receipt type are all
// E10's job. fillFixture below is a test-local stand-in that drives the
// pieces E9 already built (anchor.Discover, defrag.Flatten/Locate,
// splice.Splice, xmlcopy.Apply, opc.Package.Put) directly, the same way a
// real orchestrator eventually will. It is deliberately not exported and
// deliberately not named Fill, so nobody mistakes it for the E10 entry point.

import (
	"bytes"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/template"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/template/splice"
	"github.com/frankbardon/vellum/xmlcopy"
)

// TestNonDestructiveCorpus builds a .docx-shaped fixture carrying tracked
// changes, a comment, a custom XML part, a footnote and an embedded OLE
// object, fills its two anchors (one native, one marker), and asserts:
//
//  1. word/document.xml changed — the anchors' own content is visibly
//     different — but every byte outside the anchors' own spans is
//     unchanged, checked directly against xmlcopy.Apply's own contract
//     rather than by re-deriving offsets by hand.
//  2. Every other part in the package is byte-identical to the source,
//     part for part, compared with bytes.Equal rather than any semantic
//     "looks equivalent" check.
//  3. The result still opens: the touched part re-parses with xmlcopy.Walk,
//     and the written bytes round-trip through opc.Open.
func TestNonDestructiveCorpus(t *testing.T) {
	raw := buildNonDestructiveFixture(t)

	srcPkg, err := opc.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("opc.Open on the fixture: %v", err)
	}
	srcDoc := mustPart(t, srcPkg, "/word/document.xml")

	tmpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("template.Open: %v", err)
	}
	if tmpl.Format() != artifact.FormatDOCX {
		t.Fatalf("format = %v, want DOCX", tmpl.Format())
	}

	inv, err := anchor.Discover(tmpl.Package(), tmpl.Format(), tmpl.MainPart())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Anchors) != 2 {
		t.Fatalf("got %d anchors, want 2 (one native, one marker): %+v", len(inv.Anchors), inv.Anchors)
	}

	seqs := map[string]fragment.Sequence{
		"customer_name": {Blocks: []fragment.Block{
			{Kind: spec.BlockText, Paragraph: &fragment.Paragraph{
				Runs: []fragment.Run{{Text: "Acme & Co."}},
			}},
		}},
		"body": {Blocks: []fragment.Block{
			{Kind: spec.BlockText, Paragraph: &fragment.Paragraph{
				Runs: []fragment.Run{{Text: "Filled body content."}},
			}},
		}},
	}

	filledPkg := fillFixture(t, tmpl.Package(), inv, seqs)

	// --- Assertion 1: word/document.xml -----------------------------------

	filledDocPart, ok := filledPkg.Get("/word/document.xml")
	if !ok {
		t.Fatal("filled package lost /word/document.xml")
	}
	filledDoc, err := filledDocPart.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	if bytes.Equal(srcDoc, filledDoc) {
		t.Fatal("document.xml did not change at all; the fill did nothing")
	}
	if !bytes.Contains(filledDoc, []byte("Acme &amp; Co.")) {
		t.Errorf("marker anchor's new content missing: %s", filledDoc)
	}
	if !bytes.Contains(filledDoc, []byte("Filled body content.")) {
		t.Errorf("native anchor's new content missing: %s", filledDoc)
	}
	if bytes.Contains(filledDoc, []byte("{{customer_name}}")) {
		t.Error("the raw marker text survived the splice")
	}
	if bytes.Contains(filledDoc, []byte(">placeholder<")) {
		t.Error("the native anchor's placeholder text survived the splice")
	}

	// The two anchors are, by construction, the last two top-level elements
	// in the body — everything else in the document sits strictly before
	// them in byte order. That is exactly what makes the untouched-region
	// assertion below independent of exactly how defrag/splice sized their
	// own replacements: xmlcopy.Apply's own contract is "every byte before
	// the first replacement's Start, and every byte after the last
	// replacement's End, is copied through unchanged" — so this checks that
	// contract directly rather than re-deriving offsets by hand.
	firstStart := inv.Anchors[0].Span.Start
	for i := 1; i < len(inv.Anchors); i++ {
		if inv.Anchors[i].Span.Start < firstStart {
			firstStart = inv.Anchors[i].Span.Start
		}
	}
	lastEnd := inv.Anchors[0].Span.End
	for i := 1; i < len(inv.Anchors); i++ {
		if inv.Anchors[i].Span.End > lastEnd {
			lastEnd = inv.Anchors[i].Span.End
		}
	}

	prefix := srcDoc[:firstStart]
	if !bytes.HasPrefix(filledDoc, prefix) {
		t.Fatalf("bytes before the first anchor changed; want the tracked-change, comment, footnote and OLE structure preserved verbatim.\nwant prefix:\n%s\ngot document:\n%s", prefix, filledDoc)
	}
	suffix := srcDoc[lastEnd:]
	if !bytes.HasSuffix(filledDoc, suffix) {
		t.Fatalf("bytes after the last anchor changed; want the closing paragraph preserved verbatim.\nwant suffix:\n%s\ngot document:\n%s", suffix, filledDoc)
	}

	// Name what the preserved prefix actually contains, so a failure above
	// reads as "the tracked-change structure broke" rather than requiring a
	// human to eyeball an XML diff to find out what regressed.
	for _, want := range []struct {
		label string
		frag  string
	}{
		{"tracked insertion", `<w:ins w:id="100"`},
		{"tracked deletion", `<w:delText>deleted text</w:delText>`},
		{"comment range", `<w:commentRangeStart w:id="0"/>`},
		{"comment reference", `<w:commentReference w:id="0"/>`},
		{"footnote reference", `<w:footnoteReference w:id="1"/>`},
		{"OLE object", `<o:OLEObject`},
	} {
		if !bytes.Contains(prefix, []byte(want.frag)) {
			t.Errorf("preserved prefix lost the %s structure (%q); this fixture is not exercising what it claims to", want.label, want.frag)
		}
	}

	// --- Assertion 2: every other part is byte-identical -------------------

	srcNames := srcPkg.Names()
	filledNames := filledPkg.Names()
	if len(srcNames) != len(filledNames) {
		t.Fatalf("part count changed: source has %d, filled has %d\nsource: %v\nfilled: %v",
			len(srcNames), len(filledNames), srcNames, filledNames)
	}
	for _, name := range srcNames {
		srcPart, ok := srcPkg.Get(name)
		if !ok {
			t.Fatalf("source package lost its own part %q", name)
		}
		filledPart, ok := filledPkg.Get(name)
		if !ok {
			t.Errorf("part %q present in the source is missing from the filled package", name)
			continue
		}
		srcBytes, err := srcPart.Bytes()
		if err != nil {
			t.Fatalf("reading source part %q: %v", name, err)
		}
		filledBytes, err := filledPart.Bytes()
		if err != nil {
			t.Fatalf("reading filled part %q: %v", name, err)
		}
		if name == "/word/document.xml" {
			// Asserted in detail above; the anchors' own content is
			// *expected* to differ here.
			continue
		}
		if !bytes.Equal(srcBytes, filledBytes) {
			t.Errorf("part %q is not byte-identical after fill; every part outside the anchors' own part must survive untouched.\nsource (%d bytes):\n%s\nfilled (%d bytes):\n%s",
				name, len(srcBytes), srcBytes, len(filledBytes), filledBytes)
		}
	}

	// --- Assertion 3: the result still opens --------------------------------

	if err := xmlcopy.Walk(filledDoc, func(xmlcopy.Element) error { return nil }); err != nil {
		t.Fatalf("filled document.xml does not parse: %v", err)
	}

	var out bytes.Buffer
	if err := filledPkg.WriteTo(&out, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if _, err := opc.Open(bytes.NewReader(out.Bytes()), int64(out.Len())); err != nil {
		t.Fatalf("the filled package does not round-trip through opc.Open: %v", err)
	}
}

// fillFixture drives discovery's own inventory through defrag/splice for
// every anchor named in seqs, applies every part's accumulated replacements
// in one xmlcopy.Apply pass per part, and returns a package carrying the
// result. See the file doc comment: this is a story-local test helper, not
// the E10 Fill entry point.
func fillFixture(t *testing.T, pkg *opc.Package, inv *anchor.Inventory, seqs map[string]fragment.Sequence) *opc.Package {
	t.Helper()

	byPart := make(map[string][]xmlcopy.Replacement)
	for _, a := range inv.Anchors {
		seq, ok := seqs[a.Name]
		if !ok {
			t.Fatalf("fixture anchor %q has no fragment.Sequence to splice; add one", a.Name)
		}
		repl, err := splice.Splice(pkg, a, seq)
		if err != nil {
			t.Fatalf("Splice(%q): %v", a.Name, err)
		}
		byPart[a.Part] = append(byPart[a.Part], repl)
	}

	out := pkg.Clone()
	for part, repls := range byPart {
		// Replacements must be supplied to xmlcopy.Apply in ascending,
		// non-overlapping order; anchor.Discover already returns anchors in
		// document order, so the replacements collected above are already
		// sorted per part.
		p, ok := out.Get(part)
		if !ok {
			t.Fatalf("part %q named by a discovered anchor is missing from the package", part)
		}
		src, err := p.Bytes()
		if err != nil {
			t.Fatalf("reading %q: %v", part, err)
		}
		applied, err := xmlcopy.Apply(src, repls)
		if err != nil {
			t.Fatalf("Apply(%q): %v", part, err)
		}
		if err := out.Put(&opc.Part{Name: part, ContentType: p.ContentType, Data: applied}); err != nil {
			t.Fatalf("Put(%q): %v", part, err)
		}
	}
	return out
}

func mustPart(t *testing.T, pkg *opc.Package, name string) []byte {
	t.Helper()
	p, ok := pkg.Get(name)
	if !ok {
		t.Fatalf("fixture is missing part %q", name)
	}
	b, err := p.Bytes()
	if err != nil {
		t.Fatalf("reading part %q: %v", name, err)
	}
	return b
}
