package splice_test

import (
	"bytes"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/template/splice"
	"github.com/frankbardon/vellum/xmlcopy"
)

// TestSplice_EndToEndBothAnchorKindsInOnePart is the round trip the story
// asks for: a real minimal .docx-shaped package carrying one native and one
// marker anchor in the same part, both spliced, both replacements applied
// together in a single xmlcopy.Apply pass — the shape E10's own orchestration
// is expected to use, since a single part can carry more than one anchor and
// Apply requires every replacement for a part in one ascending, non-
// overlapping call — and a second, wholly untouched part in the package
// proving non-destructiveness holds at the package level too.
func TestSplice_EndToEndBothAnchorKindsInOnePart(t *testing.T) {
	src := wordDoc(
		`<w:p><w:r><w:t>Dear {{customer_name}}, thanks for your order.</w:t></w:r></w:p>` +
			`<w:sdt><w:sdtPr><w:tag w:val="notes"/></w:sdtPr>` +
			`<w:sdtContent><w:p><w:r><w:t>placeholder notes</w:t></w:r></w:p></w:sdtContent>` +
			`</w:sdt>`)

	pkg := opc.New()
	if err := pkg.Put(&opc.Part{Name: partDocument, ContentType: ctMainDocument, Data: src}); err != nil {
		t.Fatalf("Put document: %v", err)
	}
	const otherPart = "/docProps/core.xml"
	otherBytes := []byte(`<?xml version="1.0" encoding="UTF-8"?><cp:coreProperties xmlns:cp="x"/>`)
	if err := pkg.Put(&opc.Part{Name: otherPart, ContentType: "application/octet-stream", Data: otherBytes}); err != nil {
		t.Fatalf("Put other part: %v", err)
	}

	markerA := anchor.Anchor{Name: "customer_name", Kind: anchor.KindMarker, Part: partDocument,
		Span: elementSpan(t, src, "p", 0)}
	nativeA := anchor.Anchor{Name: "notes", Kind: anchor.KindNative, Part: partDocument,
		Span: elementSpan(t, src, "sdt", 0)}

	markerRepl, err := splice.Splice(pkg, markerA, fragment.Sequence{
		Blocks: []fragment.Block{textBlock(run("Acme Corp", fragment.TextStyle{}))},
	})
	if err != nil {
		t.Fatalf("Splice marker: %v", err)
	}
	nativeRepl, err := splice.Splice(pkg, nativeA, fragment.Sequence{
		Blocks: []fragment.Block{
			textBlock(run("First note.", fragment.TextStyle{})),
			textBlock(run("Second note.", fragment.TextStyle{})),
		},
	})
	if err != nil {
		t.Fatalf("Splice native: %v", err)
	}

	// Both replacements target the same part's pristine source bytes, so
	// they are applied together in one pass, in ascending span order — the
	// marker's paragraph comes before the sdt in this fixture.
	replacements := []xmlcopy.Replacement{markerRepl, nativeRepl}
	if replacements[0].Start > replacements[1].Start {
		replacements[0], replacements[1] = replacements[1], replacements[0]
	}
	out := mustApply(t, src, replacements)

	if err := pkg.Put(&opc.Part{Name: partDocument, ContentType: ctMainDocument, Data: out}); err != nil {
		t.Fatalf("Put spliced document: %v", err)
	}

	// The whole package still parses part by part.
	pkg.Walk(func(p *opc.Part) error {
		b, err := p.Bytes()
		if err != nil {
			t.Fatalf("%s: Bytes: %v", p.Name, err)
		}
		if p.Name == partDocument {
			if err := xmlcopy.Walk(b, func(xmlcopy.Element) error { return nil }); err != nil {
				t.Fatalf("%s does not parse: %v", p.Name, err)
			}
		}
		return nil
	})

	if !bytes.Contains(out, []byte("Dear ")) || !bytes.Contains(out, []byte("Acme Corp")) || !bytes.Contains(out, []byte(", thanks for your order.")) {
		t.Errorf("marker substitution missing: %s", out)
	}
	if !bytes.Contains(out, []byte("First note.")) || !bytes.Contains(out, []byte("Second note.")) {
		t.Errorf("native substitution missing: %s", out)
	}
	if bytes.Contains(out, []byte("placeholder notes")) {
		t.Error("placeholder content survived")
	}

	// The untouched part is byte-identical.
	other, ok := pkg.Get(otherPart)
	if !ok {
		t.Fatal("other part missing")
	}
	ob, err := other.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if !bytes.Equal(ob, otherBytes) {
		t.Error("the untouched part changed")
	}
}

func TestSplice_NilPackageIsRejected(t *testing.T) {
	_, err := splice.Splice(nil, anchor.Anchor{Part: partDocument}, fragment.Sequence{})
	if !verr.HasCode(err, verr.VELLUM_INTERNAL_INVARIANT) {
		t.Fatalf("err = %v, want VELLUM_INTERNAL_INVARIANT", err)
	}
}

func TestSplice_UnknownPartIsRejected(t *testing.T) {
	pkg := opc.New()
	_, err := splice.Splice(pkg, anchor.Anchor{Part: "/word/missing.xml"}, fragment.Sequence{})
	if !verr.HasCode(err, verr.VELLUM_INTERNAL_INVARIANT) {
		t.Fatalf("err = %v, want VELLUM_INTERNAL_INVARIANT", err)
	}
}

func TestSplice_UnknownAnchorKindIsRejected(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>hi</w:t></w:r></w:p>`)
	pkg := buildPackage(t, src)
	_, err := splice.Splice(pkg, anchor.Anchor{Kind: "bogus", Part: partDocument, Span: elementSpan(t, src, "p", 0)}, fragment.Sequence{})
	if !verr.HasCode(err, verr.VELLUM_INTERNAL_INVARIANT) {
		t.Fatalf("err = %v, want VELLUM_INTERNAL_INVARIANT", err)
	}
}
