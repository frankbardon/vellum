package pdf

import (
	"strconv"

	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/pdf/color"
	"github.com/frankbardon/vellum/pdf/font"
	pdfimage "github.com/frankbardon/vellum/pdf/image"
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
	out.Overflow = l.splits
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

	// images are built on first use and shared afterwards, keyed by the index
	// into the document's asset manifest. Resource names come from that index,
	// so a document's /Im2 is its second asset however the pages use it.
	images map[int]*pdfimage.XObject

	// geometry is the current page's, and cursor the baseline the next line
	// would sit on.
	geometry fragment.Page
	cursor   object.Real
	open     bool

	// notes are the footnotes waiting to be laid out at the foot of the current
	// page, and reserve the vertical space they will need. The reserve raises
	// the floor the flow may write down to, which is why a note has to be
	// admitted before the body that follows it is placed rather than after.
	notes   []pendingNote
	reserve object.Real

	// noteNumber counts footnotes across the document, so the marks run
	// continuously rather than restarting on each page.
	noteNumber int

	// splits is the overflow report, accumulated as tables are placed.
	splits []TableSplit
}

// pendingNote is a footnote whose body has been wrapped but not yet placed.
type pendingNote struct {
	number  int
	lines   []text.Line
	leading object.Real
}

// height is the space the note occupies once placed.
func (n pendingNote) height() object.Real {
	return object.Real(len(n.lines)) * n.leading
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

	case spec.BlockAsset:
		return l.asset(b.Asset, sectionIndex, sectionID, blockIndex)

	case spec.BlockNotes:
		return l.note(b.Note)

	case spec.BlockTable:
		if b.Table == nil {
			return verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT,
				"a table block carries no table")
		}
		return l.table(b.Table, sectionIndex, sectionID, blockIndex)
	}

	// A kind the matrix declares and this writer does not draw is an invariant
	// rather than a capability rejection, and deliberately so: the matrix says
	// it renders, so the gap would be in this package rather than in what
	// Vellum promises. Saying otherwise here would turn the matrix into a
	// description of the code instead of a contract on it.
	return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
		"the PDF writer does not lower this block kind",
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

// bottom is the lowest baseline the flow may write down to.
//
// It rises as footnotes are admitted, because the space they will occupy is not
// available to the body above them. That is why a note is admitted before the
// text that follows it is placed: after the fact, the body would already be
// sitting where the note has to go.
func (l *lowering) bottom() object.Real {
	return points(l.geometry.MarginBottom) + l.reserve
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
	l.placeNotes()
	l.pages = append(l.pages, *l.current)
	l.current = nil
	l.notes, l.reserve = nil, 0
}

// asset places a resolved asset, breaking the page when it does not fit.
//
// The size is already concrete: resolution applied the theme box and, where the
// box declared an intrinsic height, the asset's own aspect ratio. Nothing here
// asks a picture how tall it is, and nothing here scales one — a picture placed
// at a size the layout did not choose is a picture in the wrong place.
func (l *lowering) asset(a *fragment.AssetRef, sectionIndex int, sectionID string, blockIndex int) error {
	if a == nil {
		return verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT,
			"an asset block carries no asset reference")
	}
	if a.AssetIndex < 0 || a.AssetIndex >= len(l.doc.Assets) {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"an asset block references an asset that is not in the manifest",
			map[string]any{"section_index": sectionIndex, "section_id": sectionID,
				"block_index": blockIndex, "asset_index": a.AssetIndex})
	}

	im, err := l.image(a.AssetIndex)
	if err != nil {
		return err
	}

	height := points(a.HeightEMU)
	l.ensurePage()
	if l.cursor-height < l.bottom() && !l.empty() {
		// It does not fit here and the page has content, so the picture moves
		// rather than being cropped by the page edge. A picture taller than the
		// whole text box stays on a page of its own and overflows visibly,
		// which is a geometry the theme chose and not something to silently
		// resize away.
		l.flush()
		l.newPage(l.geometry)
	}

	top := l.cursor
	l.current.Items = append(l.current.Items, Image(ImageItem{
		X:      points(l.geometry.MarginLeft),
		Y:      top - height,
		Width:  points(a.WidthEMU),
		Height: height,
		Image:  im,
	}))
	l.cursor = top - height
	return nil
}

// image builds an image XObject for an asset, once per document.
func (l *lowering) image(index int) (*pdfimage.XObject, error) {
	if im, ok := l.images[index]; ok {
		return im, nil
	}

	a := &l.doc.Assets[index]
	im, err := pdfimage.New(pdfimage.Options{
		Resource:  object.Name("Im" + strconv.Itoa(index+1)),
		Handle:    a.Handle,
		MediaType: a.MediaType,
		Bytes:     a.Bytes,
	})
	if err != nil {
		return nil, err
	}
	if l.images == nil {
		l.images = map[int]*pdfimage.XObject{}
	}
	l.images[index] = im
	return im, nil
}

// note lowers a notes block into a footnote.
//
// The matrix declares this degradation and names what it becomes. A PDF
// annotation would be closer in spirit to a note and is not guaranteed visible
// in every reader, and PDF/A restricts the annotation types a conforming file
// may carry; a footnote is legible everywhere and needs nothing but text.
//
// The mark goes where the block sat in the flow and the body goes to the foot
// of the page, which is what makes it a footnote rather than small print in the
// middle of the document.
func (l *lowering) note(n *fragment.Note) error {
	if n == nil {
		return verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT,
			"a notes block carries no note")
	}

	spans, err := l.spans(&n.Body)
	if err != nil {
		return err
	}
	lines, err := text.Wrap(spans, text.WrapOptions{Width: l.measure()})
	if err != nil {
		return err
	}

	l.ensurePage()
	note := pendingNote{
		number:  l.noteNumber + 1,
		lines:   lines,
		leading: leadingFor(lines, n.Body.LineHeight),
	}

	// Admitting the note raises the page's floor, and the body already placed
	// above may now be sitting in the space the note needs. When it is, the
	// note and its mark move to the next page together — splitting them would
	// put a mark on one page and its text on another, which is the one thing a
	// footnote may not do.
	grown := note.height()
	if len(l.notes) == 0 {
		grown += noteSeparatorGap
	}
	if l.cursor-noteMarkHeight < l.bottom()+grown && !l.empty() {
		l.flush()
		l.newPage(l.geometry)
		return l.note(n)
	}

	l.noteNumber = note.number
	l.notes = append(l.notes, note)
	l.reserve += grown

	// The mark: the note's number, set small, on the flow's own baseline.
	mark := l.markSpans(note.number)
	if len(mark) > 0 {
		markLines, err := text.Wrap(mark, text.WrapOptions{Width: l.measure()})
		if err != nil {
			return err
		}
		leading := leadingFor(markLines, 1)
		l.place(markLines, leading)
	}
	return nil
}

// noteSeparatorGap is the space between the body text and the first footnote,
// which the separator rule sits in.
const noteSeparatorGap = object.Real(10 * object.RealScale)

// noteMarkHeight is the space a footnote's in-flow mark occupies.
const noteMarkHeight = object.Real(12 * object.RealScale)

// markSpans builds the in-flow reference mark for a footnote.
func (l *lowering) markSpans(number int) []text.Span {
	if len(l.faces) == 0 {
		return nil
	}
	return []text.Span{{
		Text: strconv.Itoa(number),
		Style: text.Style{
			Face:   l.faces[0],
			Shaper: l.shapers[0],
			Size:   noteMarkSize,
			Color:  color.Black,
		},
	}}
}

// noteMarkSize is the type size of a reference mark.
const noteMarkSize = object.Real(8 * object.RealScale)

// placeNotes lays the pending footnotes at the foot of the page.
//
// Called from flush, so a page's notes are placed after everything above them
// is known — which is the whole reason the reserve exists rather than the notes
// being emitted where they were encountered.
func (l *lowering) placeNotes() {
	if len(l.notes) == 0 {
		return
	}

	// Stacked upward from the bottom margin, so the last note ends exactly at
	// the margin however many there are.
	y := points(l.geometry.MarginBottom)
	for i := len(l.notes) - 1; i >= 0; i-- {
		n := l.notes[i]
		y += n.height()
		l.current.Items = append(l.current.Items, Text(TextItem{
			X:       points(l.geometry.MarginLeft),
			Y:       y - n.leading,
			Width:   l.measure(),
			Align:   text.AlignLeft,
			Leading: n.leading,
			Lines:   n.lines,
		}))
	}

	// The separator, a third of the measure wide, above the first note. Its
	// only job is to say where the body stops and the notes start.
	l.current.Items = append(l.current.Items, Rule(RuleItem{
		X:      points(l.geometry.MarginLeft),
		Y:      y + noteSeparatorGap/2,
		Width:  l.measure() / 3,
		Height: noteRuleHeight,
		Color:  color.RGB{R: 0x80, G: 0x80, B: 0x80},
	}))
}

// noteRuleHeight is the thickness of the footnote separator.
const noteRuleHeight = object.Real(object.RealScale / 2)
