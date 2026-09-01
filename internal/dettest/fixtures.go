package dettest

import (
	"context"
	"encoding/base64"
	"io"
	"strconv"
	"time"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/asset"
	"github.com/frankbardon/vellum/deck"
	"github.com/frankbardon/vellum/doc"
	"github.com/frankbardon/vellum/internal/imagetest"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/pdf"
	"github.com/frankbardon/vellum/pdf/content"
	pdffont "github.com/frankbardon/vellum/pdf/font"
	pdfimage "github.com/frankbardon/vellum/pdf/image"
	"github.com/frankbardon/vellum/pdf/object"
	"github.com/frankbardon/vellum/pdf/shape"
	"github.com/frankbardon/vellum/pdf/text"
	"github.com/frankbardon/vellum/pdf/xmp"
	"github.com/frankbardon/vellum/provenance"
	"github.com/frankbardon/vellum/resolve"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/theme"
	"golang.org/x/image/font/gofont/goregular"
)

// Cases returns every registered determinism and golden case.
//
// Adding a case is one entry here. The runners in the test files never change,
// which is the point: a format epic proves its determinism by registering,
// not by writing a suite of its own that may quietly assert something weaker.
func Cases() []Case {
	return []Case{
		substrateCase(),
		docxSkeletonCase(),
		docxProfileCase(),
		pptxMasterCase(),
		pptxComposeCase(),
		pptxTableCase(),
		pdfSpikeCase(),
		pdfPagesCase(),
		pdfProseCase(),
		pdfImageCase(),
		pdfComposeCase(),
	}
}

// pdfComposeCase is the block model reaching a PDF: spec in, document out.
//
// The other PDF cases build a page by hand, which proves the substrate and
// proves nothing about the writer above it. This one goes through the same door
// a consumer does — decode, resolve, lower, write — and it is the only case
// where the line breaks and the page breaks are Vellum's decisions rather than
// the fixture's.
//
// It carries more prose than fits on one page, deliberately. Pagination that is
// never exercised is pagination that is not known to work, and the page a
// paragraph splits across is where a flow layout goes wrong.
func pdfComposeCase() Case {
	return Case{
		Name:  "pdf-compose",
		Ext:   "pdf",
		Write: writePDFCompose,
	}
}

// composeTheme is a theme whose faces can actually be embedded.
//
// The built-in theme cannot be used for a PDF and that is deliberate rather
// than an oversight: its three faces are declared non-embeddable, because
// Vellum ships no font program, and PDF/A-2b requires every font embedded. The
// capability matrix says so at font.embed.none, and resolution refuses it
// before any bytes exist. A consumer targeting PDF supplies a theme like this
// one.
func composeTheme() (*theme.Theme, error) {
	th, err := theme.Builtin()
	if err != nil {
		return nil, err
	}
	th.ID = "compose-pdf"
	for i := range th.Fonts {
		th.Fonts[i].Family = "Go Regular"
		th.Fonts[i].Embeddable = true
		th.Fonts[i].Substitute = ""
		th.Fonts[i].Embed = theme.EmbedSubset
		th.Fonts[i].Handle = "font/go-regular"
	}
	return th, nil
}

// composeFonts serves the one font program the theme names.
func composeFonts() asset.Resolver {
	return asset.NewMap(map[string]asset.Asset{
		"font/go-regular": {MediaType: "font/ttf", Bytes: goregular.TTF},
	})
}

// writePDFCompose composes the block-model document.
func writePDFCompose(w io.Writer, epoch time.Time) error {
	th, err := composeTheme()
	if err != nil {
		return err
	}
	provider, err := theme.NewStaticProvider(th)
	if err != nil {
		return err
	}

	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Title:         "Composed to PDF",
		Theme:         th.ID,
		Sections: []spec.Section{{
			ID:     "prose",
			Blocks: composeBlocks(),
		}},
	}

	res, err := resolve.Resolve(context.Background(), s, resolve.Options{
		Format: artifact.FormatPDF,
		Themes: provider,
		Assets: composeFonts(),
	})
	if err != nil {
		return err
	}

	d, err := pdf.Lower(res.Doc)
	if err != nil {
		return err
	}
	d.Metadata.Creator = "Vellum determinism fixture"
	d.Metadata.Producer = "Vellum 0.0.0-golden"
	d.Metadata.Date = epoch

	return d.WriteTo(w, pdf.WriteOptions{SourceDateEpoch: epoch})
}

// composeBlocks is enough prose to paginate, with a marked run in it so the
// styled-span path is exercised by an artifact rather than only by a unit test.
func composeBlocks() []spec.Block {
	out := []spec.Block{
		{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 1, Content: "Composed to PDF"}},
		{Kind: spec.BlockText, Text: &spec.Text{
			Content: "This document was not assembled by hand. A specification was decoded, " +
				"resolved against a theme, lowered into a page tree, and written — the same " +
				"path a consumer takes.",
		}},
		{Kind: spec.BlockText, Marks: []string{"flagged"}, Text: &spec.Text{
			Content: "A marked paragraph, whose mark the theme styles.",
		}},
		{Kind: spec.BlockSpacer, Spacer: &spec.Spacer{Height: spec.Points(12)}},
	}

	// Repeated bodies rather than one long string, so the page break falls
	// between paragraphs on one page and inside a paragraph on another. Both
	// are pagination and only one of them is easy.
	for i := 1; i <= 9; i++ {
		out = append(out,
			spec.Block{Kind: spec.BlockHeading, Heading: &spec.Heading{
				Level: 2, Content: "Section " + strconv.Itoa(i),
			}},
			spec.Block{Kind: spec.BlockText, Text: &spec.Text{Content: composeBody}},
		)
	}
	png := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imagetest.RGB())

	out = append(out,
		spec.Block{Kind: spec.BlockPageBreak, PageBreak: &spec.PageBreak{}},
		spec.Block{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 1, Content: "After the break"}},
		spec.Block{Kind: spec.BlockText, Text: &spec.Text{
			Content: "An explicit page break starts a page even when the one before it had room.",
		}},
		// The asset and the note, together on the last page, so one reading of
		// the artifact shows both declared behaviours: a picture embedded at
		// the size the theme box chose, and a notes block that became a
		// footnote at the foot of the page rather than small print in the flow.
		spec.Block{Kind: spec.BlockAsset, Asset: &spec.Asset{
			Handle:  png,
			Role:    "asset.full",
			AltText: "a placeholder square, which PDF has nowhere to put",
		}},
		spec.Block{Kind: spec.BlockNotes, Notes: &spec.Notes{
			Content: "Base: every respondent. This note is a footnote, which is what the capability matrix says a note becomes here.",
		}},
	)
	return out
}

// composeBody is deliberately ordinary. Its job is to wrap and to run past the
// bottom of a page, not to be interesting.
const composeBody = "Vellum decides every line break and every page break in this file. " +
	"There is no application behind a PDF to lay the text out afterwards, so the " +
	"positions in the content stream are the positions a reader sees. The breaking " +
	"is greedy over the opportunities Unicode permits, and the measurement is integer " +
	"arithmetic from the font's own design units, so the same specification paginates " +
	"the same way on every machine that renders it."

// pdfImageCase exercises the asset path: the three embeddings that differ.
//
// A JPEG passing through as DCTDecode, an opaque PNG passing through as
// FlateDecode with its own predictor, and a PNG whose alpha channel had to be
// separated into a soft mask. The third is the one worth a golden — it is the
// only image form whose bytes Vellum rearranges, and the only one where a
// mistake produces a document that opens and is wrong.
func pdfImageCase() Case {
	return Case{
		Name:  "pdf-image",
		Ext:   "pdf",
		Write: writePDFImage,
	}
}

// writePDFImage composes the image document.
func writePDFImage(w io.Writer, epoch time.Time) error {
	const title = "Assets, embedded rather than redrawn"

	face, err := pdffont.New(pdffont.Options{
		Resource: "F1",
		BaseName: "GoRegular",
		Program:  goregular.TTF,
		Plan:     pdffont.PlanSubset,
	})
	if err != nil {
		return err
	}

	// Named for what each proves rather than for what it depicts, because the
	// picture is eight pixels of gradient and the path is the point.
	plates := []struct {
		resource object.Name
		media    string
		bytes    []byte
		caption  string
	}{
		{"ImJpeg", asset.MediaJPEG, imagetest.JPEGColor(),
			"JPEG: DCTDecode, the file's own bytes"},
		{"ImPng", asset.MediaPNG, imagetest.RGB(),
			"PNG: FlateDecode with the PNG predictor"},
		{"ImAlpha", asset.MediaPNG, imagetest.RGBA(),
			"PNG with alpha: colour and mask separated"},
	}

	var c content.Builder
	images := make([]*pdfimage.XObject, 0, len(plates))

	titleGlyphs, err := face.Encode(title)
	if err != nil {
		return err
	}
	c.BeginText().
		SetFont("F1", object.Points(18)).
		MoveText(object.Points(72), object.Points(700)).
		ShowGlyphs(titleGlyphs).
		EndText()

	// A filled rule behind the plates, so a soft mask that failed to apply is
	// visible as a solid square rather than as a slightly different white.
	c.Save().
		SetFillRGB(object.Ratio(240, 255), object.Ratio(240, 255), object.Ratio(230, 255)).
		Rect(object.Points(60), object.Points(430), object.Points(492), object.Points(220)).
		Fill().
		Restore()

	for i, p := range plates {
		im, err := pdfimage.New(pdfimage.Options{
			Resource:  p.resource,
			Handle:    string(p.resource),
			MediaType: p.media,
			Bytes:     p.bytes,
		})
		if err != nil {
			return err
		}
		images = append(images, im)

		x := object.Points(int64(80 + i*160))
		c.DrawImage(p.resource, x, object.Points(480), object.Points(120), object.Points(120))

		glyphs, err := face.Encode(p.caption)
		if err != nil {
			return err
		}
		c.BeginText().
			SetFont("F1", object.Points(8)).
			MoveText(x, object.Points(462)).
			ShowGlyphs(glyphs).
			EndText()
	}

	d := pdf.Document{
		Metadata: xmp.Metadata{
			Title:    title,
			Creator:  "Vellum determinism fixture",
			Producer: "Vellum 0.0.0-golden",
			Date:     epoch,
		},
		Pages: []pdf.Page{{
			Width:  object.Points(612),
			Height: object.Points(792),
			Items: []pdf.Item{pdf.Raw(pdf.RawItem{
				Content: c.Bytes(),
				Fonts:   []*pdffont.Face{face},
				Images:  images,
			})},
		}},
	}
	return d.WriteTo(w, pdf.WriteOptions{SourceDateEpoch: epoch})
}

// pdfProseCase exercises shaping, line breaking and justification.
//
// A paragraph long enough to wrap several times, set justified, beside the same
// text set flush left so the difference is visible in the artifact rather than
// only in the assertions. It is the case that would catch a sign error in the
// TJ adjustments, which draws as a line that tightens rather than spreads and
// looks like a kerning fault.
func pdfProseCase() Case {
	return Case{
		Name:  "pdf-prose",
		Ext:   "pdf",
		Write: writePDFProse,
	}
}

// proseText is deliberately ordinary. Its job is to wrap, not to be interesting.
const proseText = "Vellum lays this paragraph out itself: it shapes the text with " +
	"harfbuzz, finds break opportunities with the Unicode line breaking algorithm, " +
	"and fills each line greedily in integer font units. Justification distributes " +
	"the remaining space between words through TJ adjustments, because word spacing " +
	"does nothing for a two-byte encoding."

func writePDFProse(w io.Writer, epoch time.Time) error {
	shaper, err := shape.New(goregular.TTF)
	if err != nil {
		return err
	}
	face, err := pdffont.New(pdffont.Options{
		Resource: "F1",
		BaseName: "GoRegular",
		Program:  goregular.TTF,
		Plan:     pdffont.PlanSubset,
	})
	if err != nil {
		return err
	}

	const (
		size    = object.Real(11 * object.RealScale)
		leading = object.Real(15 * object.RealScale)
		measure = object.Real(300 * object.RealScale)
		left    = object.Real(72 * object.RealScale)
	)

	body := text.Style{Face: face, Shaper: shaper, Size: size}
	lines, err := text.WrapText(body, proseText, text.WrapOptions{Width: measure})
	if err != nil {
		return err
	}

	var c content.Builder
	top := object.Points(700)
	for _, block := range []struct {
		label string
		align text.Align
	}{
		{"Justified", text.AlignJustify},
		{"Flush left", text.AlignLeft},
	} {
		heading, err := face.Encode(block.label)
		if err != nil {
			return err
		}
		c.BeginText().
			SetFont("F1", object.Points(9)).
			MoveText(left, top).
			ShowGlyphs(heading).
			EndText()
		top -= object.Points(18)

		for _, l := range lines {
			c.BeginText().
				MoveText(left+l.Offset(block.align, measure), top)
			// Show emits the Tf itself now: the size and the face travel with
			// the line's own segments, so a line cannot be drawn in a font it
			// was not measured in.
			if err := l.Show(&c, block.align, measure); err != nil {
				return err
			}
			c.EndText()
			top -= leading
		}
		top -= object.Points(24)
	}

	d := pdf.Document{
		Metadata: xmp.Metadata{
			Title:    "Justified Prose",
			Creator:  "Vellum determinism fixture",
			Producer: "Vellum 0.0.0-golden",
			Date:     epoch,
		},
		Pages: []pdf.Page{{
			Width:  object.Points(612),
			Height: object.Points(792),
			Items: []pdf.Item{pdf.Raw(pdf.RawItem{
				Content: c.Bytes(),
				Fonts:   []*pdffont.Face{face},
			})},
		}},
	}
	return d.WriteTo(w, pdf.WriteOptions{SourceDateEpoch: epoch})
}

// pdfPagesCase exercises the page tree at a size where its shape matters.
//
// Twenty pages against a branching factor of eight, so the tree is three levels
// deep and neither level is full — which is the arrangement that catches an
// off-by-one in the fold, where a full tree would not. Every page shares its
// media box and resources, so the case also proves the inheritable attributes
// are lifted onto the root rather than repeated twenty times.
func pdfPagesCase() Case {
	return Case{
		Name:  "pdf-pages",
		Ext:   "pdf",
		Write: writePDFPages,
	}
}

// pageCount is the length of the multi-page fixture.
//
// Twenty rather than a round power of the branching factor, deliberately: a
// count that divides evenly hides a remainder bug in the last group of every
// level, and the last group is the one the fold gets wrong.
const pageCount = 20

func writePDFPages(w io.Writer, epoch time.Time) error {
	face, err := pdffont.New(pdffont.Options{
		Resource: "F1",
		BaseName: "GoRegular",
		Program:  goregular.TTF,
		Plan:     pdffont.PlanSubset,
	})
	if err != nil {
		return err
	}

	pages := make([]pdf.Page, pageCount)
	for i := range pages {
		label := "Page " + strconv.Itoa(i+1) + " of " + strconv.Itoa(pageCount)
		gids, err := face.Encode(label)
		if err != nil {
			return err
		}

		var c content.Builder
		c.BeginText().
			SetFont("F1", object.Points(18)).
			MoveText(object.Points(72), object.Points(700)).
			ShowGlyphs(gids).
			EndText()

		pages[i] = pdf.Page{
			Width:  object.Points(612),
			Height: object.Points(792),
			Items: []pdf.Item{pdf.Raw(pdf.RawItem{
				Content: c.Bytes(),
				Fonts:   []*pdffont.Face{face},
			})},
		}
	}

	d := pdf.Document{
		Metadata: xmp.Metadata{
			Title:    "Twenty Pages",
			Creator:  "Vellum determinism fixture",
			Producer: "Vellum 0.0.0-golden",
			Date:     epoch,
		},
		Pages: pages,
	}
	return d.WriteTo(w, pdf.WriteOptions{SourceDateEpoch: epoch})
}

// pdfSpikeCase is the PDF substrate proved end to end on one page.
//
// One page, one line, one subsetted face, an sRGB output intent and XMP that
// agrees with the information dictionary. Small, and it touches every hard part
// of the PDF work at once — the object writer, the cross-reference table, the
// subsetter, the composite font encoding and the conformance metadata — so a
// wrong assumption in any of them surfaces here rather than after the rest is
// built on top of it.
//
// The face is Go Regular. It is BSD-3, it arrives as a Go package rather than
// as a binary in testdata, and it carries composite glyphs, which is the part
// of subsetting a naive implementation gets wrong.
func pdfSpikeCase() Case {
	return Case{
		Name:  "pdf-spike",
		Ext:   "pdf",
		Write: writePDFSpike,
	}
}

// writePDFSpike composes the spike document.
func writePDFSpike(w io.Writer, epoch time.Time) error {
	const (
		heading = "Hello, PDF/A"
		body    = "Composed by Vellum. Every glyph here is embedded as a subset."
	)

	face, err := pdffont.New(pdffont.Options{
		Resource: "F1",
		BaseName: "GoRegular",
		Program:  goregular.TTF,
		Plan:     pdffont.PlanSubset,
	})
	if err != nil {
		return err
	}
	headingGlyphs, err := face.Encode(heading)
	if err != nil {
		return err
	}
	bodyGlyphs, err := face.Encode(body)
	if err != nil {
		return err
	}

	var c content.Builder
	// A filled rule above the heading, which puts a DeviceRGB colour in the
	// file. Without one the output intent is present but unexercised, and the
	// spike would not have proved the part of conformance most likely to be
	// wrong.
	c.Save().
		SetFillRGB(object.Ratio(37, 255), object.Ratio(99, 235), object.Ratio(235, 255)).
		Rect(object.Points(72), object.Points(724), object.Points(180), object.Points(4)).
		Fill().
		Restore()

	c.BeginText().
		SetFont("F1", object.Points(24)).
		MoveText(object.Points(72), object.Points(684)).
		ShowGlyphs(headingGlyphs).
		EndText()

	c.BeginText().
		SetFont("F1", object.Points(11)).
		MoveText(object.Points(72), object.Points(652)).
		ShowGlyphs(bodyGlyphs).
		EndText()

	d := pdf.Document{
		Metadata: xmp.Metadata{
			Title:    heading,
			Creator:  "Vellum determinism fixture",
			Producer: "Vellum 0.0.0-golden",
			Date:     epoch,
		},
		Pages: []pdf.Page{{
			Width:  object.Points(612),
			Height: object.Points(792),
			Items: []pdf.Item{pdf.Raw(pdf.RawItem{
				Content: c.Bytes(),
				Fonts:   []*pdffont.Face{face},
			})},
		}},
	}
	return d.WriteTo(w, pdf.WriteOptions{SourceDateEpoch: epoch})
}

// docxSkeletonCase is the first real artifact: a heading and a paragraph
// rendered to a .docx.
//
// It is registered rather than tested privately in the doc package, because a
// format epic should prove its determinism by joining this suite rather than
// by writing one of its own that may quietly assert something weaker.
// pptxMasterCase is the authored master, layout and theme set reaching a
// package.
//
// Built from the deck model directly rather than lowered from a specification,
// because there is no lowering yet and the thing under test is not the mapping
// — it is that a .pptx Vellum authored from a theme, with no shipped template
// anywhere in it, is a package a reader opens and finds its content in.
//
// It carries notes on one slide, deliberately. A notes slide drags in a notes
// master and a second theme part, which is where the relationship graph gets
// wide enough to go wrong.
func pptxMasterCase() Case {
	return Case{
		Name:  "pptx-master",
		Ext:   "pptx",
		Write: writePPTXMaster,
	}
}

// pptxDesign is the design the PPTX golden is authored from.
//
// Its colours are the built-in theme's, mapped onto DrawingML's twelve slots by
// hand. The mapping is not mechanical — DrawingML says dk1/lt1 where the theme
// says text/background — and doing it here rather than in the deck package is
// the same boundary the design struct exists to draw.
func pptxDesign() deck.Design {
	const pt = 12700
	return deck.Design{
		Name:           "Vellum",
		MarginTop:      457200,
		MarginRight:    457200,
		MarginBottom:   457200,
		MarginLeft:     457200,
		HeadingFamily:  "Helvetica Neue",
		BodyFamily:     "Helvetica",
		TitleSize:      40 * pt,
		BodySizes:      []int64{20 * pt, 18 * pt, 16 * pt},
		LineHeight:     1.2,
		ParagraphSpace: 6 * pt,
		TitleGap:       6 * pt,
		Colors: deck.ColorScheme{
			Dark1: "1A1A1A", Light1: "FFFFFF",
			Dark2: "0B3D91", Light2: "F2F2F2",
			Accent1: "0B3D91", Accent2: "C8102E", Accent3: "007A5E",
			Accent4: "F2A900", Accent5: "6B4E9B", Accent6: "5B6770",
			Hyperlink: "0B3D91", FollowedHyperlink: "6B4E9B",
		},
	}
}

// writePPTXMaster emits the authored deck.
func writePPTXMaster(w io.Writer, epoch time.Time) error {
	d, err := deck.Author(pptxDesign())
	if err != nil {
		return err
	}
	d.Title = "Authored From The Theme"

	d.Slides = []deck.Slide{
		{
			LayoutID: deck.LayoutIDTitle,
			Shapes: []deck.Shape{
				{
					Placeholder: &deck.Placeholder{Type: deck.PlaceholderCenterTitle},
					Text:        deckBody("Authored From The Theme"),
				},
				{
					Placeholder: &deck.Placeholder{Type: deck.PlaceholderSubTitle, Index: 1},
					Text:        deckBody("No template ships with this deck"),
				},
			},
		},
		{
			LayoutID: deck.LayoutIDContent,
			Notes:    "The master, the layouts and the theme part were all written from the theme document.\n\nNothing here was copied from a shipped package.",
			Shapes: []deck.Shape{
				{
					Placeholder: &deck.Placeholder{Type: deck.PlaceholderTitle},
					Text:        deckBody("Three content models"),
				},
				{
					Placeholder: &deck.Placeholder{Type: deck.PlaceholderContent, Index: 1},
					Text: &deck.TextBody{Paragraphs: []deck.Paragraph{
						{Runs: []deck.Run{{Text: "spec is unresolved and hashable"}}},
						{Level: 1, Runs: []deck.Run{{Text: "theme by reference, values untyped"}}},
						{Runs: []deck.Run{{Text: "fragment is resolved and format neutral"}}},
						{Level: 1, Runs: []deck.Run{{Text: "every length in EMU"}}},
						{Runs: []deck.Run{{Text: "doc, sheet, deck and pdf are laid out"}}},
					}},
				},
			},
		},
		{
			LayoutID: deck.LayoutIDTitleOnly,
			Shapes: []deck.Shape{
				{
					Placeholder: &deck.Placeholder{Type: deck.PlaceholderTitle},
					Text:        deckBody("Declared, not emergent"),
				},
			},
		},
	}

	return d.WriteTo(w, deck.WriteOptions{SourceDateEpoch: epoch})
}

// deckBody builds a single-paragraph text body.
func deckBody(text string) *deck.TextBody {
	return &deck.TextBody{Paragraphs: []deck.Paragraph{{Runs: []deck.Run{{Text: text}}}}}
}

// pptxComposeCase is the block model reaching a deck: spec in, slides out.
//
// The other PPTX case builds its slides by hand, which proves the writer and
// proves nothing about the mapping above it. This one goes through the door a
// consumer uses — decode, resolve, lower, write — and it is the only case where
// which slide a paragraph lands on is Vellum's decision rather than the
// fixture's.
//
// It carries a heading with nothing under it, a picture, a page break and a
// note, because those are the four places the mapping decides something.
func pptxComposeCase() Case {
	return Case{
		Name:  "pptx-compose",
		Ext:   "pptx",
		Write: writePPTXCompose,
	}
}

// writePPTXCompose composes the block-model deck.
func writePPTXCompose(w io.Writer, epoch time.Time) error {
	png := "data:image/png;base64," + base64.StdEncoding.EncodeToString(fixturePNG())

	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Title:         "Composed to a Deck",
		Sections: []spec.Section{
			{
				ID: "opening",
				Blocks: []spec.Block{
					{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 1, Content: "Composed to a Deck"}},
				},
			},
			{
				ID: "models",
				Blocks: []spec.Block{
					{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 1, Content: "Three content models"}},
					{Kind: spec.BlockText, Text: &spec.Text{Content: "spec is unresolved and hashable."}},
					{Kind: spec.BlockText, Marks: []string{"flagged"},
						Text: &spec.Text{Content: "fragment is resolved and format neutral."}},
					{Kind: spec.BlockNotes, Notes: &spec.Notes{
						Content: "Do not read the slide. Say why the fragment earns its place."}},
					{Kind: spec.BlockPageBreak, PageBreak: &spec.PageBreak{}},
					{Kind: spec.BlockText, Text: &spec.Text{Content: "doc, sheet, deck and pdf are laid out."}},
					{Kind: spec.BlockSpacer, Spacer: &spec.Spacer{Height: spec.Points(18)}},
					{Kind: spec.BlockText, Text: &spec.Text{Content: "Each is public, so format-specific reach exists."}},
				},
			},
			{
				ID: "evidence",
				Blocks: []spec.Block{
					{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 2, Content: "Evidence"}},
					{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: png, AltText: "a placeholder raster"}},
				},
			},
		},
	}

	res, err := resolve.Resolve(context.Background(), s, resolve.Options{Format: artifact.FormatPPTX})
	if err != nil {
		return err
	}

	d, err := deck.Lower(res.Doc)
	if err != nil {
		return err
	}
	return d.WriteTo(w, deck.WriteOptions{SourceDateEpoch: epoch})
}

// pptxTableCase is a table longer than a slide, split by the declared policy.
//
// The case exists because a table is where "the file opens" and "the file is
// correct" come apart most quietly in this format. A row that does not tile the
// grid, a merged cell whose covered cells are absent, a style identifier
// nothing defines — every one of those produces a deck that opens, with a table
// in it that is wrong in a way no reader reports.
//
// It carries a two-level banner, a merged row-header stub, a margin row and an
// annotated cell, and it is long enough to continue onto a second slide.
func pptxTableCase() Case {
	return Case{
		Name:  "pptx-table",
		Ext:   "pptx",
		Write: writePPTXTable,
	}
}

const pptxTableRows = 26

// writePPTXTable composes the overflowing crosstab.
func writePPTXTable(w io.Writer, epoch time.Time) error {
	ages := make(spec.HeaderTree, 0, pptxTableRows)
	body := make([][]spec.Cell, 0, pptxTableRows)
	for i := 0; i < pptxTableRows; i++ {
		ages = append(ages, spec.HeaderNode{Label: "Band " + strconv.Itoa(i+1)})

		row := []spec.Cell{
			{Text: strconv.Itoa(40 + i)},
			{Text: strconv.Itoa(60 - i)},
			{Text: strconv.Itoa(100)},
		}
		if i == pptxTableRows-1 {
			for j := range row {
				row[j].Class = spec.CellTotal
			}
		}
		if i == 0 {
			row[0].Annotations = []spec.Annotation{{Text: "a"}}
		}
		body = append(body, row)
	}

	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Title:         "A Table Longer Than A Slide",
		Sections: []spec.Section{{
			ID: "crosstab",
			Blocks: []spec.Block{
				{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 1, Content: "Awareness by band"}},
				{Kind: spec.BlockTable, Table: &spec.Table{
					ColumnHeaders: spec.HeaderTree{
						{Label: "Region", Span: 2, Children: spec.HeaderTree{
							{Label: "North"}, {Label: "South"},
						}},
						{Label: "Total"},
					},
					RowHeaders: spec.HeaderTree{{Label: "Age", Children: ages}},
					Body:       body,
					Caption:    "Percentages. Base: all adults.",
				}},
			},
		}},
	}

	res, err := resolve.Resolve(context.Background(), s, resolve.Options{Format: artifact.FormatPPTX})
	if err != nil {
		return err
	}

	d, err := deck.Lower(res.Doc)
	if err != nil {
		return err
	}
	return d.WriteTo(w, deck.WriteOptions{SourceDateEpoch: epoch})
}

func docxSkeletonCase() Case {
	return Case{
		Name: "docx-skeleton",
		Ext:  "docx",
		Write: writeDOCX(&spec.Spec{
			FormatVersion: spec.FormatVersion,
			Title:         "Walking Skeleton",
			Sections: []spec.Section{{
				ID: "intro",
				Blocks: []spec.Block{
					{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 1, Content: "Walking Skeleton"}},
					{Kind: spec.BlockText, Text: &spec.Text{Content: "The substrate carries a real artifact end to end."}},
					{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 2, Content: "Why this exists"}},
					{Kind: spec.BlockText, Text: &spec.Text{Content: "Breadth is not the goal; structural correctness and byte-identical output are."}},
				},
			}},
		}, nil),
	}
}

// docxProfileCase exercises the DOCX conformance profile at its full breadth in
// one document.
//
// One broad case rather than one per feature, deliberately. The features
// interact — a table inside a section whose geometry differs, an annotation
// inside a merged cell, a footnote anchored after a picture — and a suite of
// narrow fixtures proves each in isolation while proving nothing about the
// document a consumer actually produces.
func docxProfileCase() Case {
	png := "data:image/png;base64," + base64.StdEncoding.EncodeToString(fixturePNG())

	return Case{
		Name: "docx-profile",
		Ext:  "docx",
		Write: writeDOCX(&spec.Spec{
			FormatVersion: spec.FormatVersion,
			Title:         "Conformance Profile",
			Sections: []spec.Section{
				{
					ID: "prose",
					Blocks: []spec.Block{
						{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 1, Content: "Findings"}},
						{Kind: spec.BlockText, Text: &spec.Text{Content: "Unmarked prose, which takes its style and carries no direct formatting."}},
						{Kind: spec.BlockText, Marks: []string{"flagged"},
							Text: &spec.Text{Content: "Marked prose, whose mark the theme styles."}},
						{Kind: spec.BlockText, Marks: []string{"unstyled-mark"},
							Text: &spec.Text{Content: "A mark the theme does not style, which warns and renders plain."}},
						{Kind: spec.BlockSpacer, Spacer: &spec.Spacer{Height: spec.Points(18)}},
					},
				},
				{
					ID: "table",
					Blocks: []spec.Block{
						{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 2, Content: "Crosstab"}},
						{Kind: spec.BlockTable, Table: profileTable()},
						{Kind: spec.BlockNotes, Notes: &spec.Notes{Content: "Base: all respondents."}},
					},
				},
				{
					ID: "figure",
					Blocks: []spec.Block{
						{Kind: spec.BlockPageBreak, PageBreak: &spec.PageBreak{}},
						{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 2, Content: "Figure"}},
						{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: png, Role: "asset.full", AltText: "a four-by-three placeholder"}},
					},
				},
			},
		}, &provenance.Record{
			VellumVersion:   "0.0.0-golden",
			SourceDateEpoch: zipdet.PinnedEpoch,
			SpecHash:        "00000000000000000000000000000000",
			ThemeHash:       "11111111111111111111111111111111",
			Fonts: []provenance.FontRef{
				{Family: "Georgia", SubstitutedWith: "Times New Roman"},
			},
			Sources: []provenance.Source{{Kind: "fixture", ID: "docx-profile"}},
		}),
	}
}

// profileTable is a crosstab exercising every table feature the profile
// declares: a two-level column banner, a row-header stub that merges
// vertically, cell annotations, a margin row, and a per-cell mark.
func profileTable() *spec.Table {
	return &spec.Table{
		ColumnHeaders: spec.HeaderTree{
			{Label: "Region", Span: 2, Children: []spec.HeaderNode{
				{Label: "North", Span: 1},
				{Label: "South", Span: 1},
			}},
			{Label: "Total", Span: 1},
		},
		RowHeaders: spec.HeaderTree{
			{Label: "Age", Span: 2, Children: []spec.HeaderNode{
				{Label: "18-34", Span: 1},
				{Label: "35+", Span: 1},
			}},
			{Label: "All", Span: 1},
		},
		Body: [][]spec.Cell{
			{
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 0.412}, Format: "0.0%",
					Annotations: []spec.Annotation{{Text: "a"}}},
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 0.388}, Format: "0.0%"},
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 0.400}, Format: "0.0%"},
			},
			{
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 0.213}, Format: "0.0%",
					Marks: []string{"muted"}},
				{Text: "*"},
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 0.207}, Format: "0.0%"},
			},
			{
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 1200}, Format: "#,##0", Class: spec.CellMargin},
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 1100}, Format: "#,##0", Class: spec.CellMargin},
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 2300}, Format: "#,##0", Class: spec.CellTotal},
			},
		},
		Caption: "Table 1. Awareness by region and age.",
	}
}

// writeDOCX returns a Case writer that resolves and lowers a specification.
//
// Resolution happens inside the writer rather than once outside it, so every
// repetition of a determinism run exercises the whole pipeline. A fixture that
// resolved once and lowered many times would prove the writer deterministic
// while saying nothing about the resolver.
func writeDOCX(s *spec.Spec, rec *provenance.Record) func(io.Writer, time.Time) error {
	return func(w io.Writer, epoch time.Time) error {
		res, err := resolve.Resolve(context.Background(), s, resolve.Options{Format: artifact.FormatDOCX})
		if err != nil {
			return err
		}
		d, err := doc.Lower(res.Doc)
		if err != nil {
			return err
		}
		d.Provenance = rec
		return d.WriteTo(w, doc.WriteOptions{SourceDateEpoch: epoch})
	}
}

const (
	ctXML     = "application/xml"
	ctPNG     = "image/png"
	relCustom = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/customXml"
	relImage  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"
)

// substrateCase exercises the packaging layer with no writer above it.
//
// It exists so the harness is proven against a real artifact before any format
// package exists — which is the whole reason this story is sequenced before
// the first writer rather than after it. It deliberately includes a stored
// media part and two relationships, because those are the two places
// nondeterminism enters a package: compression choice and identifier
// assignment.
func substrateCase() Case {
	return Case{
		Name: "substrate-package",
		Ext:  "zip",
		Write: func(w io.Writer, epoch time.Time) error {
			p := opc.New()

			if err := p.Put(&opc.Part{
				Name:        "/content/document.xml",
				ContentType: ctXML,
				Data:        []byte(`<?xml version="1.0"?><document><body>substrate</body></document>`),
			}); err != nil {
				return err
			}
			if err := p.Put(&opc.Part{
				Name:        "/content/notes.xml",
				ContentType: ctXML,
				Data:        []byte(`<?xml version="1.0"?><notes/>`),
			}); err != nil {
				return err
			}
			if err := p.Put(&opc.Part{
				Name:        "/media/image1.png",
				ContentType: ctPNG,
				Data:        fixturePNG(),
			}); err != nil {
				return err
			}

			if _, err := p.Relationships("/").Add(relCustom, "content/document.xml", opc.TargetInternal); err != nil {
				return err
			}
			if _, err := p.Relationships("/content/document.xml").Add(relImage, "../media/image1.png", opc.TargetInternal); err != nil {
				return err
			}
			if _, err := p.Relationships("/content/document.xml").Add(relCustom, "notes.xml", opc.TargetInternal); err != nil {
				return err
			}

			return p.WriteTo(w, zipdet.WriteOptions{SourceDateEpoch: epoch})
		},
	}
}

// fixturePNG is a real 4x3 PNG: two flat bands, produced by image/png and
// round-tripped through its decoder before being encoded here.
//
// It is real because the first version was not. That one was hand-assembled
// from a signature, an IHDR and an IEND — no IDAT, no chunk CRCs. Vellum's
// sniffer recognised the signature, its probe read 4x3 out of the IHDR, the
// package assembled correctly, and the determinism harness was entirely
// satisfied, because every stage was comparing our bytes against our bytes.
// Word drew "the picture can't be displayed".
//
// A fixture no reader accepts proves the packaging and proves nothing about the
// embedding, which is most of what a golden containing an image is for.
// TestGoldenMediaDecodes now fails the build for it.
func fixturePNG() []byte {
	const encoded = "iVBORw0KGgoAAAANSUhEUgAAAAQAAAADCAIAAAA7ljmRAAAAHElEQVR42mKR96tk" +
		"gAEmBiTAeOHCBTgHEAAA//88uANeGlr4UwAAAABJRU5ErkJggg=="
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		panic("dettest: the fixture PNG does not decode: " + err.Error())
	}
	return raw
}
