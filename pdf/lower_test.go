package pdf_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"encoding/base64"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/asset"
	"github.com/frankbardon/vellum/capability"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/internal/imagetest"
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

// TestLower_PlacesAnAsset checks a picture reaches a page at the size
// resolution chose.
func TestLower_PlacesAnAsset(t *testing.T) {
	d := lower(t, text("before"), assetBlock(""), text("after"))

	var found *pdf.ImageItem
	for _, p := range d.Pages {
		for _, it := range p.Items {
			if it.Kind == pdf.ItemImage {
				found = it.Image
			}
		}
	}
	if found == nil {
		t.Fatal("the asset block produced no image item")
	}
	if found.Width <= 0 || found.Height <= 0 {
		t.Errorf("the image was placed at %sx%s", found.Width, found.Height)
	}
	if found.Image == nil {
		t.Fatal("the image item carries no XObject")
	}
	// The bytes have to reach the file, not just the model.
	var buf bytes.Buffer
	if err := d.WriteTo(&buf, pdf.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("/Subtype /Image")) {
		t.Error("no image XObject reached the document")
	}
}

// TestLower_TheSameAssetIsEmbeddedOnce pins that a picture used twice costs one
// copy of its bytes.
func TestLower_TheSameAssetIsEmbeddedOnce(t *testing.T) {
	d := lower(t, assetBlock(""), assetBlock(""))

	var buf bytes.Buffer
	if err := d.WriteTo(&buf, pdf.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if n := bytes.Count(buf.Bytes(), []byte("/Subtype /Image")); n != 1 {
		t.Errorf("the same asset was embedded %d times", n)
	}
}

// TestLower_AltTextIsDegradedRatherThanDropped pins the matrix row.
//
// PDF/A-2b carries no structure tree, so an accessible description has nowhere
// to attach. It is content the consumer wrote, so the consumer is told.
func TestLower_AltTextIsDegradedRatherThanDropped(t *testing.T) {
	res := resolveFor(t, assetBlock("a placeholder square"))

	var found bool
	for _, w := range res.Warnings {
		if v, ok := w.Detail("feature"); ok && v == string(capability.FeatureAssetAltText) {
			found = true
		}
	}
	if !found {
		t.Fatalf("no warning names the dropped description; got %v", res.Warnings)
	}
}

// TestLower_ANoteBecomesAFootnote pins the declared degradation.
//
// The matrix says a notes block becomes a footnote on PDF, which means two
// things a test can check: a mark where the block sat, and the body at the foot
// of the page rather than in the middle of it.
func TestLower_ANoteBecomesAFootnote(t *testing.T) {
	d := lower(t,
		text(strings.Repeat("Body prose that fills some of the page. ", 10)),
		spec.Block{Kind: spec.BlockNotes, Notes: &spec.Notes{Content: "Base: all respondents."}},
	)
	if len(d.Pages) != 1 {
		t.Fatalf("got %d pages, want the note to fit on the first", len(d.Pages))
	}

	page := d.Pages[0]
	var lowest object.Real = 1 << 40
	var noteY object.Real = -1
	var rule bool
	for _, it := range page.Items {
		switch it.Kind {
		case pdf.ItemText:
			if strings.Contains(lineTextOf(it.Text), "Base: all respondents.") {
				noteY = it.Text.Y
				continue
			}
			lowest = min(lowest, it.Text.Y)
		case pdf.ItemRule:
			rule = true
		}
	}
	if noteY < 0 {
		t.Fatal("the note's body is not on the page")
	}
	if !rule {
		t.Error("no separator rule divides the body from the notes")
	}
	if noteY >= lowest {
		t.Errorf("the note sits at %s and the lowest body line at %s; a footnote goes below the body",
			noteY, lowest)
	}
}

// TestLower_FootnotesRaiseTheFloorForTheBody is the invariant that makes the
// footnote a footnote rather than an overlap.
//
// The space a note will occupy is not available to the text above it. Without
// the reserve, the body fills to the bottom margin and the notes are drawn on
// top of it — which renders, and is wrong.
func TestLower_FootnotesRaiseTheFloorForTheBody(t *testing.T) {
	blocks := []spec.Block{
		{Kind: spec.BlockNotes, Notes: &spec.Notes{
			Content: strings.Repeat("A long note that wraps over several lines. ", 6),
		}},
	}
	for range 12 {
		blocks = append(blocks, text(strings.Repeat("Body prose that wraps and wraps. ", 8)))
	}

	d := lower(t, blocks...)
	checked := 0
	for i, p := range d.Pages {
		var noteTop object.Real = -1
		var bodyBottom object.Real = 1 << 40
		for _, it := range p.Items {
			if it.Kind != pdf.ItemText {
				continue
			}
			if strings.Contains(lineTextOf(it.Text), "A long note that wraps") {
				noteTop = it.Text.Y
				continue
			}
			last := it.Text.Y - it.Text.Leading*object.Real(len(it.Text.Lines)-1)
			bodyBottom = min(bodyBottom, last)
		}
		if noteTop < 0 {
			continue
		}
		checked++
		if bodyBottom <= noteTop {
			t.Errorf("page %d: body reaches %s and the note starts at %s; they overlap",
				i, bodyBottom, noteTop)
		}
	}
	if checked == 0 {
		t.Fatal("no page carried a footnote, so the overlap check ran on nothing")
	}
}

// lineTextOf reassembles a text item's characters.
func lineTextOf(t *pdf.TextItem) string {
	var b strings.Builder
	for _, l := range t.Lines {
		b.WriteString(l.Text())
	}
	return b.String()
}

// assetBlock is a real PNG placed through the asset seam.
func assetBlock(alt string) spec.Block {
	handle := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imagetest.RGB())
	return spec.Block{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: handle, AltText: alt}}
}

// resolveFor resolves one block and returns the whole result, warnings included.
func resolveFor(t *testing.T, blocks ...spec.Block) *resolve.Result {
	t.Helper()

	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Theme:         "embeddable",
		Sections:      []spec.Section{{ID: "s", Blocks: blocks}},
	}
	res, err := resolve.Resolve(context.Background(), s, resolve.Options{
		Format: artifact.FormatPDF, Themes: embeddableTheme(t), Assets: fontStore(),
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return res
}

// blockOf builds the smallest valid block of a kind.
//
// Smallest, and valid: a block the decoder would reject proves nothing about
// the writer, because resolution never hands it over.
func blockOf(kind spec.BlockKind) spec.Block {
	switch kind {
	case spec.BlockHeading:
		return spec.Block{Kind: kind, Heading: &spec.Heading{Level: 1, Content: "a heading"}}
	case spec.BlockText:
		return spec.Block{Kind: kind, Text: &spec.Text{Content: "some prose"}}
	case spec.BlockTable:
		return spec.Block{Kind: kind, Table: &spec.Table{
			ColumnHeaders: spec.HeaderTree{{Label: "h"}},
			Body:          [][]spec.Cell{{{Text: "1"}}},
		}}
	case spec.BlockAsset:
		return spec.Block{Kind: kind, Asset: &spec.Asset{
			Handle: "data:image/png;base64," +
				base64.StdEncoding.EncodeToString(imagetest.RGB()),
		}}
	case spec.BlockNotes:
		return spec.Block{Kind: kind, Notes: &spec.Notes{Content: "a note"}}
	case spec.BlockPageBreak:
		return spec.Block{Kind: kind, PageBreak: &spec.PageBreak{}}
	case spec.BlockSpacer:
		return spec.Block{Kind: kind, Spacer: &spec.Spacer{Height: spec.Points(12)}}
	}
	return spec.Block{Kind: kind}
}

// TestLower_DrawsEveryDeclaredBlockKind pins that nothing is dropped.
//
// The capability matrix declares every block kind as rendering on PDF, and this
// is where that declaration is checked against the code rather than against
// itself. It walks the live registry, so a kind added to the model and not to
// this writer fails here — with the message the writer itself would have
// produced — rather than reaching a consumer as a document missing a section.
//
// It replaced a list of kinds not yet built, which is the shape this check had
// while the writer was incomplete. A list is a thing that goes stale silently;
// the registry cannot.
func TestLower_DrawsEveryDeclaredBlockKind(t *testing.T) {
	for _, kind := range spec.AllBlockKinds() {
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
				t.Fatalf("resolution refused a kind the matrix declares: %v", err)
			}
			if _, err := pdf.Lower(res.Doc); err != nil {
				t.Fatalf("Lower: %v", err)
			}
		})
	}
}
