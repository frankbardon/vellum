package pdf_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/asset"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/pdf"
	"github.com/frankbardon/vellum/pdf/object"
	"github.com/frankbardon/vellum/resolve"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/theme"
	"golang.org/x/image/font/gofont/goregular"
)

// embeddableTheme is a theme a PDF can actually be composed against.
//
// The built-in theme cannot: its faces are declared non-embeddable and PDF/A-2b
// requires every font embedded. That is a matrix row, not an accident, and
// TestLower_RequiresAnEmbeddableTheme is where it is pinned.
func embeddableTheme(t *testing.T) theme.Provider {
	t.Helper()

	th, err := theme.Builtin()
	if err != nil {
		t.Fatalf("Builtin: %v", err)
	}
	th.ID = "embeddable"
	for i := range th.Fonts {
		th.Fonts[i].Family = "Go Regular"
		th.Fonts[i].Embeddable = true
		th.Fonts[i].Substitute = ""
		th.Fonts[i].Embed = theme.EmbedSubset
		th.Fonts[i].Handle = "font/go-regular"
	}
	p, err := theme.NewStaticProvider(th)
	if err != nil {
		t.Fatalf("NewStaticProvider: %v", err)
	}
	return p
}

func fontStore() asset.Resolver {
	return asset.NewMap(map[string]asset.Asset{
		"font/go-regular": {MediaType: "font/ttf", Bytes: goregular.TTF},
	})
}

// lower resolves a specification and lowers it, failing the test on either.
func lower(t *testing.T, blocks ...spec.Block) *pdf.Document {
	t.Helper()

	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Title:         "Test",
		Theme:         "embeddable",
		Sections:      []spec.Section{{ID: "s", Blocks: blocks}},
	}
	res, err := resolve.Resolve(context.Background(), s, resolve.Options{
		Format: artifact.FormatPDF, Themes: embeddableTheme(t), Assets: fontStore(),
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	d, err := pdf.Lower(res.Doc)
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	return d
}

func text(s string) spec.Block {
	return spec.Block{Kind: spec.BlockText, Text: &spec.Text{Content: s}}
}

// TestLower_RequiresAnEmbeddableTheme pins the answer that decides whether a
// theme can be used for PDF at all.
//
// The built-in theme names three families and carries no font programs, because
// Vellum ships none. For an OOXML target that is fine — the application
// resolves the family. For PDF/A it is fatal, and the failure has to arrive
// before any bytes exist rather than as a document that opens and renders in
// whatever happens to be installed.
func TestLower_RequiresAnEmbeddableTheme(t *testing.T) {
	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Sections:      []spec.Section{{ID: "s", Blocks: []spec.Block{text("hello")}}},
	}
	_, err := resolve.Resolve(context.Background(), s, resolve.Options{Format: artifact.FormatPDF})
	if !verr.HasCode(err, verr.VELLUM_FONT_EMBED_UNSUPPORTED) {
		t.Fatalf("error = %v, want VELLUM_FONT_EMBED_UNSUPPORTED", err)
	}
}

// TestLower_ProducesAPageForAnEmptyDocument checks the degenerate case.
//
// PDF cannot express zero pages: the tree's count would be zero and every
// reader treats that as damage. An empty page is the honest rendering of an
// empty specification.
func TestLower_ProducesAPageForAnEmptyDocument(t *testing.T) {
	// Built directly rather than resolved: a specification with no blocks is
	// refused by its own validation, so the only way to reach this is a caller
	// constructing a fragment — which the model is public for.
	d, err := pdf.Lower(&fragment.Doc{Title: "Empty"})
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	if len(d.Pages) != 1 {
		t.Fatalf("got %d pages for an empty document, want 1", len(d.Pages))
	}
	if len(d.Pages[0].Items) != 0 {
		t.Errorf("the empty page draws %d items", len(d.Pages[0].Items))
	}
}

// TestLower_Paginates is the property this story exists for.
//
// A PDF has no application behind it, so the page count is Vellum's decision.
// A specification that never mentions a page must still produce several when
// its content does not fit on one.
func TestLower_Paginates(t *testing.T) {
	var blocks []spec.Block
	for range 40 {
		blocks = append(blocks, text(strings.Repeat("The quick brown fox jumps over the lazy dog. ", 6)))
	}

	d := lower(t, blocks...)
	if len(d.Pages) < 3 {
		t.Fatalf("forty paragraphs produced %d pages; the content does not fit on that many", len(d.Pages))
	}
	for i, p := range d.Pages {
		if len(p.Items) == 0 {
			t.Errorf("page %d is empty, so the flow left a gap rather than filling it", i)
		}
	}
}

// TestLower_NoLineFallsBelowTheBottomMargin is the pagination invariant.
//
// A flow that overruns does not report anything: the text is simply drawn off
// the page, below the media box, and a reader shows a page that stops
// mid-sentence.
func TestLower_NoLineFallsBelowTheBottomMargin(t *testing.T) {
	var blocks []spec.Block
	for range 25 {
		blocks = append(blocks, text(strings.Repeat("Filler prose that wraps and wraps. ", 8)))
	}

	d := lower(t, blocks...)
	for i, p := range d.Pages {
		for _, it := range p.Items {
			if it.Kind != pdf.ItemText {
				continue
			}
			last := it.Text.Y - it.Text.Leading*object.Real(len(it.Text.Lines)-1)
			if last < 0 {
				t.Errorf("page %d draws a baseline at %s, below the page itself", i, last)
			}
		}
	}
}

// TestLower_APageBreakStartsAPage pins that an explicit break is honoured even
// when the page it leaves had room.
func TestLower_APageBreakStartsAPage(t *testing.T) {
	d := lower(t,
		text("before"),
		spec.Block{Kind: spec.BlockPageBreak, PageBreak: &spec.PageBreak{}},
		text("after"),
	)
	if len(d.Pages) != 2 {
		t.Fatalf("got %d pages, want the break to start a second", len(d.Pages))
	}
}

// TestLower_IsDeterministic lowers the same document twice and compares bytes.
//
// The layout is a function of the input, so two lowerings must agree — and the
// place that would break it is a map reached on the way to a resource name or a
// page's item order.
func TestLower_IsDeterministic(t *testing.T) {
	blocks := []spec.Block{
		{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 1, Content: "Heading"}},
		text(strings.Repeat("Body text that wraps several times over. ", 20)),
	}

	var first []byte
	for range 5 {
		d := lower(t, blocks...)
		var buf bytes.Buffer
		if err := d.WriteTo(&buf, pdf.WriteOptions{}); err != nil {
			t.Fatalf("WriteTo: %v", err)
		}
		if first == nil {
			first = buf.Bytes()
			continue
		}
		if !bytes.Equal(first, buf.Bytes()) {
			t.Fatal("lowering the same document twice produced different bytes")
		}
	}
}

// blockOf builds a minimal block of a kind, for the pending-kinds check.
func blockOf(kind spec.BlockKind) spec.Block {
	switch kind {
	case spec.BlockTable:
		return spec.Block{Kind: kind, Table: &spec.Table{
			ColumnHeaders: spec.HeaderTree{{Label: "h"}},
			Body:          [][]spec.Cell{{{Text: "1"}}},
		}}
	case spec.BlockAsset:
		return spec.Block{Kind: kind, Asset: &spec.Asset{
			Handle: "data:image/png;base64,iVBORw0KGgo=",
		}}
	case spec.BlockNotes:
		return spec.Block{Kind: kind, Notes: &spec.Notes{Content: "a note"}}
	}
	return spec.Block{Kind: kind}
}

// TestLower_RefusesABlockKindItCannotDraw pins that nothing is dropped.
//
// Tables, assets and notes are declared as rendering on PDF in the capability
// matrix and are not built yet. Until they are, a specification containing one
// is a loud failure rather than a document missing a section — which is the
// failure the whole library is arranged to prevent.
func TestLower_RefusesABlockKindItCannotDraw(t *testing.T) {
	pending := []spec.BlockKind{spec.BlockTable, spec.BlockAsset, spec.BlockNotes}

	for _, kind := range pending {
		t.Run(string(kind), func(t *testing.T) {
			s := &spec.Spec{
				FormatVersion: spec.FormatVersion,
				Theme:         "embeddable",
				Sections:      []spec.Section{{ID: "s", Blocks: []spec.Block{blockOf(kind)}}},
			}
			res, err := resolve.Resolve(context.Background(), s, resolve.Options{
				Format: artifact.FormatPDF, Themes: embeddableTheme(t), Assets: fontStore(),
			})
			if err != nil {
				t.Skipf("resolution refuses this kind before lowering sees it: %v", err)
			}
			if _, err := pdf.Lower(res.Doc); err == nil {
				t.Fatal("the block was lowered silently; a kind this writer cannot draw must fail loudly")
			}
		})
	}
}
