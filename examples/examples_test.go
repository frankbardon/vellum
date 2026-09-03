package examples_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/frankbardon/vellum"
	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/examples"
	"github.com/frankbardon/vellum/internal/dettest"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/template/bind"
	"github.com/frankbardon/vellum/theme"
)

// allDocs is a small test helper, not a gate itself — every gate below reads
// the same parsed set, so a file that fails to load fails every gate at
// once rather than being silently skipped by one and caught by another.
func allDocs(t *testing.T) []examples.Doc {
	t.Helper()
	docs, err := examples.All()
	if err != nil {
		t.Fatalf("examples.All(): %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("examples.All() returned no documents")
	}
	return docs
}

func stems(docs []examples.Doc) map[string]examples.Doc {
	out := make(map[string]examples.Doc, len(docs))
	for _, d := range docs {
		out[d.Stem()] = d
	}
	return out
}

// blockKebab mirrors skills' own filename convention: a block kind's wire
// string (which may carry underscores, e.g. "page_break") becomes a
// filename with hyphens.
func blockKebab(k spec.BlockKind) string {
	return "block-" + strings.ReplaceAll(string(k), "_", "-")
}

// newVellum builds the facade every gate below composes and fills through —
// never a lower-level package directly — so this package proves examples
// run the way a real embedder would run them. The zero-value Options is the
// "wire nothing and it still works" default CLAUDE.md's "Extension Points"
// describes: the built-in theme, inline assets only.
func newVellum(t *testing.T) *vellum.Vellum {
	t.Helper()
	v, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("vellum.New(): %v", err)
	}
	return v
}

// newPDFVellum builds the facade format-pdf.json's gate composes through.
//
// The built-in theme's three faces are declared non-embeddable — Vellum
// ships no font program — and PDF/A-2b requires every font embedded, so
// composing the plain built-in theme to PDF is VELLUM_FONT_EMBED_UNSUPPORTED
// by design (see resolve/fonts.go). internal/dettest's own pdf-compose
// determinism case carries an embeddable-faced variant of the built-in
// theme for exactly this reason; reused here under [theme.BuiltinID] itself
// (rather than a fixture-specific id) so format-pdf.json's own "theme"
// field can stay empty, the same as every other format example's — a
// consumer supplying an embeddable theme through [vellum.Options.Themes]
// sees exactly this behaviour without changing the specification at all.
func newPDFVellum(t *testing.T) *vellum.Vellum {
	t.Helper()
	th, err := dettest.ComposeTheme()
	if err != nil {
		t.Fatalf("dettest.ComposeTheme(): %v", err)
	}
	th.ID = theme.BuiltinID
	provider, err := theme.NewStaticProvider(th)
	if err != nil {
		t.Fatalf("theme.NewStaticProvider(): %v", err)
	}
	v, err := vellum.New(vellum.Options{Themes: provider, Assets: dettest.ComposeFonts()})
	if err != nil {
		t.Fatalf("vellum.New(): %v", err)
	}
	return v
}

// TestExamplesCoverAllBlockKinds asserts every spec.AllBlockKinds() value
// has a matching block-<kind>.json file, that the file decodes as a
// [spec.Spec], and that it actually composes through the real facade
// against DOCX — the one format that renders or degrades every block kind
// without rejecting any of them (see capability/matrix.go), so this gate
// exercises real composition rather than only decode.
func TestExamplesCoverAllBlockKinds(t *testing.T) {
	docs := stems(allDocs(t))
	v := newVellum(t)
	ctx := context.Background()

	for _, k := range spec.AllBlockKinds() {
		want := blockKebab(k)
		doc, ok := docs[want]
		if !ok {
			t.Errorf("block kind %q has no %s.json", k, want)
			continue
		}
		t.Run(want, func(t *testing.T) {
			s, err := spec.Decode(doc.Raw)
			if err != nil {
				t.Fatalf("spec.Decode(%s): %v", doc.Filename, err)
			}
			if _, err := v.Compose(ctx, s, artifact.FormatDOCX, io.Discard); err != nil {
				t.Fatalf("Compose(%s, docx): %v", doc.Filename, err)
			}
		})
	}
}

// TestExamplesCoverAllFormats asserts every artifact.AllFormats() value has
// a matching format-<name>.json file, that it decodes as a [spec.Spec], and
// that it composes cleanly to that specific format through the real
// facade.
func TestExamplesCoverAllFormats(t *testing.T) {
	docs := stems(allDocs(t))
	v := newVellum(t)
	pdfV := newPDFVellum(t)
	ctx := context.Background()

	for _, f := range artifact.AllFormats() {
		want := "format-" + string(f)
		doc, ok := docs[want]
		if !ok {
			t.Errorf("format %q has no %s.json", f, want)
			continue
		}
		t.Run(want, func(t *testing.T) {
			s, err := spec.Decode(doc.Raw)
			if err != nil {
				t.Fatalf("spec.Decode(%s): %v", doc.Filename, err)
			}
			composer := v
			if f == artifact.FormatPDF {
				// See newPDFVellum's own doc comment: the built-in theme
				// cannot compose to PDF at all, by design, so this one
				// format's gate wires the same embeddable-faced theme
				// internal/dettest's own pdf-compose case uses.
				composer = pdfV
			}
			if _, err := composer.Compose(ctx, s, f, io.Discard); err != nil {
				t.Fatalf("Compose(%s, %s): %v", doc.Filename, f, err)
			}
		})
	}
}

// TestExamplesCoverAllBindModes asserts every bind.AllStatementKinds()
// value has a matching fill-<kind>.json file, that it decodes as a
// [bind.Binding], and that it actually fills — through [vellum.Vellum.Fill]
// — against a real docx template: the same one
// internal/dettest.FillDOCXFixture already exercises in the determinism
// harness, reused here rather than fabricated a second time. Each example
// focuses on exactly one statement kind; the template's other anchors are
// marked [bind.Binding.OptionalAnchors] rather than bound, per that field's
// own documented purpose ("deliberately leaves unbound").
func TestExamplesCoverAllBindModes(t *testing.T) {
	docs := stems(allDocs(t))
	v := newVellum(t)
	ctx := context.Background()

	raw, err := dettest.FillDOCXFixture()
	if err != nil {
		t.Fatalf("dettest.FillDOCXFixture(): %v", err)
	}
	data := dettest.FillDOCXData()

	for _, k := range bind.AllStatementKinds() {
		want := "fill-" + string(k)
		doc, ok := docs[want]
		if !ok {
			t.Errorf("bind.StatementKind %q has no %s.json", k, want)
			continue
		}
		t.Run(want, func(t *testing.T) {
			b, err := bind.Decode(doc.Raw)
			if err != nil {
				t.Fatalf("bind.Decode(%s): %v", doc.Filename, err)
			}
			res, err := v.Fill(ctx, bytes.NewReader(raw), int64(len(raw)), b, data)
			if err != nil {
				t.Fatalf("Fill(%s): %v", doc.Filename, err)
			}
			// Filling is the behaviour under test; writing the result back
			// out is the same "runnable all the way through" proof the
			// block/format gates apply via Compose, exercising the
			// non-destructiveness receipt's own Package one step further
			// than Fill alone would.
			if err := res.Package.WriteTo(io.Discard, zipdet.WriteOptions{}); err != nil {
				t.Fatalf("Result.Package.WriteTo(%s): %v", doc.Filename, err)
			}
		})
	}
}

// TestExamplesHaveNoOrphans asserts every embedded file's own category
// belongs to a live registry entry — the inverse of the three coverage
// gates above, which check every registered thing has a file. Without this,
// a stale example (a block kind renamed, a bind statement kind retired)
// could sit in the pack forever, decoding and composing successfully while
// naming something no longer registered under that name, which is exactly
// the kind of drift this story exists to prevent from being invisible.
func TestExamplesHaveNoOrphans(t *testing.T) {
	docs := allDocs(t)

	blockWant := make(map[string]bool)
	for _, k := range spec.AllBlockKinds() {
		blockWant[blockKebab(k)] = true
	}
	formatWant := make(map[string]bool)
	for _, f := range artifact.AllFormats() {
		formatWant["format-"+string(f)] = true
	}
	fillWant := make(map[string]bool)
	for _, k := range bind.AllStatementKinds() {
		fillWant["fill-"+string(k)] = true
	}

	for _, d := range docs {
		switch d.Category() {
		case "block":
			if !blockWant[d.Stem()] {
				t.Errorf("%s: no live block kind named %q", d.Filename, d.Stem())
			}
		case "format":
			if !formatWant[d.Stem()] {
				t.Errorf("%s: no live format named %q", d.Filename, d.Stem())
			}
		case "fill":
			if !fillWant[d.Stem()] {
				t.Errorf("%s: no live bind.StatementKind named %q", d.Filename, d.Stem())
			}
		default:
			t.Errorf("%s: unrecognised category %q", d.Filename, d.Category())
		}
	}
}

// TestGet_FindsEveryDocumentByItsOwnStem is a sanity check on the lookup
// path, mirroring skills.Get's own test.
func TestGet_FindsEveryDocumentByItsOwnStem(t *testing.T) {
	docs := allDocs(t)
	for _, d := range docs {
		got, ok, err := examples.Get(d.Stem())
		if err != nil {
			t.Fatalf("examples.Get(%q): %v", d.Stem(), err)
		}
		if !ok {
			t.Errorf("examples.Get(%q) reported not found", d.Stem())
			continue
		}
		if got.Filename != d.Filename {
			t.Errorf("examples.Get(%q).Filename = %q, want %q", d.Stem(), got.Filename, d.Filename)
		}
	}
	if _, ok, err := examples.Get("does-not-exist"); err != nil || ok {
		t.Errorf("examples.Get(\"does-not-exist\") = (_, %v, %v), want (_, false, nil)", ok, err)
	}
}
