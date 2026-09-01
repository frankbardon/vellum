package pdf

import (
	"strconv"

	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/pdf/color"
	"github.com/frankbardon/vellum/pdf/font"
	"github.com/frankbardon/vellum/pdf/object"
	"github.com/frankbardon/vellum/pdf/shape"
	"github.com/frankbardon/vellum/pdf/text"
	"github.com/frankbardon/vellum/pdf/xmp"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/theme"

	verr "github.com/frankbardon/vellum/errors"
)

// emuPerPoint is the EMU definition: 914400 per inch, 72 points per inch.
const emuPerPoint = 12700

// points converts EMU to the fixed-point points this writer measures in.
//
// Integer arithmetic, rounding half away from zero. A measurement that
// round-trips through float64 accumulates error that shows as a one-thousandth
// disagreement between two runs of the same specification — a determinism
// failure wearing a rounding bug's clothes.
func points(emu int64) object.Real {
	n := emu * object.RealScale
	const d = emuPerPoint
	if n < 0 {
		return object.Real((n - d/2) / d)
	}
	return object.Real((n + d/2) / d)
}

// Lower converts a resolved document into a paginated PDF model.
//
// It takes a [fragment.Doc] rather than a specification, which is the whole
// reason the resolve pass exists: theme application, font selection, number
// formatting and asset resolution have already happened, once, in a place all
// four writers share.
//
// What makes this writer different from the three OOXML ones is that it
// paginates. Word, Excel and PowerPoint lay out their own content and Vellum
// hands them a flow; a PDF has no application behind it, so every line break
// and every page break is decided here and is part of the bytes. That is why
// this package owns shaping, line breaking and measurement, and it is why those
// are integer arithmetic throughout.
//
// A block kind this writer cannot render is a hard error naming the kind and
// its position. Silently dropping content is the failure mode the library
// exists to prevent: a missing section a reader notices is far worse than an
// error the caller was told about.
func Lower(d *fragment.Doc) (*Document, error) {
	if d == nil {
		return nil, verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT,
			"the resolved document is nil")
	}

	l := &lowering{doc: d}
	if err := l.buildFaces(); err != nil {
		return nil, err
	}

	out := &Document{Metadata: xmp.Metadata{Title: d.Title}}
	for i := range d.Sections {
		if err := l.section(i, &d.Sections[i]); err != nil {
			return nil, err
		}
	}
	l.flush()

	if len(l.pages) == 0 {
		// A document with no content is still a document. PDF has no way to
		// express zero pages — the page tree's count would be zero and every
		// reader treats that as damage — so an empty page is the honest
		// rendering of an empty specification.
		l.newPage(defaultPage())
		l.flush()
	}
	out.Pages = l.pages
	return out, nil
}

// defaultPage is the geometry used when a section declares none: A4 portrait
// with a one-inch margin, in EMU.
func defaultPage() fragment.Page {
	const (
		a4Width   = 7560000  // 210mm
		a4Height  = 10692000 // 297mm
		oneInch   = 914400
		emuPerMil = 914400 / 1000
	)
	_ = emuPerMil
	return fragment.Page{
		Width: a4Width, Height: a4Height,
		MarginTop: oneInch, MarginRight: oneInch,
		MarginBottom: oneInch, MarginLeft: oneInch,
	}
}

// lowering carries the state of one conversion.
type lowering struct {
	doc *fragment.Doc

	// faces and shapers are indexed by fragment face index. A face is built
	// once for the document, so a paragraph in the body font on page forty
	// shares the subset with one on page one.
	faces   []*font.Face
	shapers []*shape.Shaper

	// pages are the pages already closed, and current the one being filled.
	pages   []Page
	current *Page

	// geometry is the current page's, and cursor the baseline the next line
	// would sit on.
	geometry fragment.Page
	cursor   object.Real
	open     bool
}

// buildFaces turns the resolved font manifest into embeddable faces.
//
// Resource names come from the manifest index rather than from a counter, so a
// document's /F2 is its second declared face however the pages use them.
func (l *lowering) buildFaces() error {
	l.faces = make([]*font.Face, len(l.doc.Fonts))
	l.shapers = make([]*shape.Shaper, len(l.doc.Fonts))

	for i := range l.doc.Fonts {
		f := &l.doc.Fonts[i]
		where := map[string]any{
			"font_role": string(f.Role),
			"family":    f.Family,
		}
		if f.AssetIndex < 0 || f.AssetIndex >= len(l.doc.Assets) {
			// Resolution refuses this for PDF — font.embed.none rejects — so
			// reaching it means a caller built a fragment by hand. The message
			// says what the theme has to provide rather than what the index was.
			return verr.NewCodedErrorWithDetails(verr.VELLUM_FONT_EMBED_UNSUPPORTED,
				"PDF/A requires every font embedded and this face carries no font program",
				where)
		}

		program := l.doc.Assets[f.AssetIndex].Bytes
		plan := font.PlanSubset
		if f.Embed == fragment.EmbedWhole {
			plan = font.PlanWhole
		}

		face, err := font.New(font.Options{
			Resource:   object.Name("F" + strconv.Itoa(i+1)),
			BaseName:   f.Family,
			Program:    program,
			Plan:       plan,
			Serif:      f.Role != theme.FontMono,
			FixedPitch: f.Role == theme.FontMono,
		})
		if err != nil {
			return verr.Annotate(err, where)
		}
		sh, err := shape.New(program)
		if err != nil {
			return verr.Annotate(err, where)
		}
		l.faces[i], l.shapers[i] = face, sh
	}
	return nil
}

// section lays out one resolved section.
//
// A section does not start a page. A specification's sections are logical
// divisions, not page breaks: breaking between every heading and its prose is
// not what the author asked for, and an explicit page_break block is what
// produces a page break. A change of page geometry does force one, because
// there is nowhere else to put it.
func (l *lowering) section(index int, s *fragment.Section) error {
	if !l.open || s.Page != l.geometry {
		l.flush()
		l.newPage(s.Page)
	}

	for i := range s.Blocks {
		if err := l.block(index, s.ID, i, &s.Blocks[i]); err != nil {
			return err
		}
	}
	return nil
}

func (l *lowering) block(sectionIndex int, sectionID string, blockIndex int, b *fragment.Block) error {
	switch b.Kind {
	case spec.BlockHeading, spec.BlockText:
		return l.paragraph(b.Paragraph)

	case spec.BlockSpacer:
		// A spacer is vertical space, not blank lines. Blank lines are a height
		// that changes with the font; space is the height that was asked for.
		l.advance(points(b.Space.HeightEMU))
		return nil

	case spec.BlockPageBreak:
		l.flush()
		l.newPage(l.geometry)
		return nil
	}

	// Every remaining kind is declared as rendering on PDF in the capability
	// matrix and is not built yet. It is an invariant rather than a capability
	// rejection precisely because the matrix says it renders: the gap is in
	// this package, not in what Vellum promises, and saying otherwise here
	// would make the matrix a description of the code instead of a contract on
	// it.
	return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
		"the PDF writer does not yet lower this block kind",
		map[string]any{
			"kind":          string(b.Kind),
			"section_index": sectionIndex,
			"section_id":    sectionID,
			"block_index":   blockIndex,
		})
}

// paragraph wraps a resolved paragraph and places its lines, breaking the page
// as many times as the paragraph needs.
func (l *lowering) paragraph(p *fragment.Paragraph) error {
	if p == nil {
		return verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT,
			"a paragraph block carries no paragraph")
	}

	spans, err := l.spans(p)
	if err != nil {
		return err
	}

	l.advance(points(p.SpaceBefore))

	lines, err := text.Wrap(spans, text.WrapOptions{Width: l.measure()})
	if err != nil {
		return err
	}
	if len(lines) > 0 {
		leading := leadingFor(lines, p.LineHeight)
		for len(lines) > 0 {
			fit := l.fit(lines, leading)
			if fit == 0 {
				// Nothing fits on this page. On a fresh page that would loop
				// forever, so the first line is placed regardless: a line taller
				// than the whole text box is a geometry the theme chose, and
				// overflowing it visibly beats not terminating.
				if l.empty() {
					fit = 1
				} else {
					l.flush()
					l.newPage(l.geometry)
					continue
				}
			}
			l.place(lines[:fit], leading)
			lines = lines[fit:]
		}
	}

	l.advance(points(p.SpaceAfter))
	return nil
}

// spans projects a paragraph's resolved runs onto styled spans.
func (l *lowering) spans(p *fragment.Paragraph) ([]text.Span, error) {
	out := make([]text.Span, 0, len(p.Runs))
	for i := range p.Runs {
		r := &p.Runs[i]
		if r.Style.FaceIndex < 0 || r.Style.FaceIndex >= len(l.faces) {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
				"a run names a face outside the document's font manifest",
				map[string]any{"face_index": r.Style.FaceIndex, "faces": len(l.faces)})
		}
		rgb, err := color.ParseHex(r.Style.Color)
		if err != nil {
			return nil, err
		}
		out = append(out, text.Span{
			Text: r.Text,
			Style: text.Style{
				Face:   l.faces[r.Style.FaceIndex],
				Shaper: l.shapers[r.Style.FaceIndex],
				Size:   points(r.Style.SizeEMU),
				Color:  rgb,
			},
		})
	}
	return out, nil
}

// leadingFor is the baseline-to-baseline distance for a paragraph.
//
// Derived from the tallest line rather than per line, so a paragraph sets at
// one rhythm. The multiple arrives from the theme as a ratio and is applied
// once, here, rather than at each line: applying it repeatedly is where a
// paragraph's lines start drifting apart from the ones beside it.
func leadingFor(lines []text.Line, multiple float64) object.Real {
	var tallest object.Real
	for _, l := range lines {
		tallest = max(tallest, l.Height)
	}
	if multiple <= 0 {
		multiple = 1.2
	}
	return object.Real(int64(float64(tallest)*multiple + 0.5))
}

// measure is the width available to text on the current page.
func (l *lowering) measure() object.Real {
	return points(l.geometry.ContentWidth())
}

// bottom is the lowest baseline a line may sit on.
func (l *lowering) bottom() object.Real {
	return points(l.geometry.MarginBottom)
}

// empty reports whether nothing has been placed on the current page.
func (l *lowering) empty() bool { return l.current == nil || len(l.current.Items) == 0 }

// fit returns how many of the lines will sit above the bottom margin.
func (l *lowering) fit(lines []text.Line, leading object.Real) int {
	n := 0
	y := l.cursor
	for n < len(lines) {
		if y-leading < l.bottom() {
			break
		}
		y -= leading
		n++
	}
	return n
}

// place appends a paragraph fragment and moves the cursor below it.
func (l *lowering) place(lines []text.Line, leading object.Real) {
	l.ensurePage()
	first := l.cursor - leading
	l.current.Items = append(l.current.Items, Text(TextItem{
		X:       points(l.geometry.MarginLeft),
		Y:       first,
		Width:   l.measure(),
		Align:   text.AlignLeft,
		Leading: leading,
		Lines:   lines,
	}))
	l.cursor = first - leading*object.Real(len(lines)-1)
}

// advance moves the cursor down, breaking the page when it runs out.
//
// Space that would fall past the bottom of a page is absorbed by the break
// rather than carried onto the next one, which is what every flow layout does:
// a paragraph's trailing space is a separation from what follows it, and a page
// boundary already separates them.
func (l *lowering) advance(by object.Real) {
	if by <= 0 {
		return
	}
	l.ensurePage()
	l.cursor -= by
	if l.cursor < l.bottom() {
		l.flush()
		l.newPage(l.geometry)
	}
}

// ensurePage opens a page if none is open.
func (l *lowering) ensurePage() {
	if l.current == nil {
		l.newPage(l.geometry)
	}
}

// newPage starts a page with a geometry.
func (l *lowering) newPage(g fragment.Page) {
	if g.Width <= 0 || g.Height <= 0 {
		g = defaultPage()
	}
	l.geometry = g
	l.open = true
	l.current = &Page{Width: points(g.Width), Height: points(g.Height)}
	l.cursor = points(g.Height - g.MarginTop)
}

// flush closes the current page.
func (l *lowering) flush() {
	if l.current == nil {
		return
	}
	l.pages = append(l.pages, *l.current)
	l.current = nil
}
