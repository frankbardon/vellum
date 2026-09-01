// Package text breaks shaped text into lines and places them.
//
// # Where the arithmetic happens
//
// Advances arrive from [shape] in font design units, as integers. They stay
// integers here, and the conversion to text space happens once per styled run
// per line rather than once per glyph:
//
//	toTextSpace(advanceUnits, unitsPerEm, fontSize)
//
// The distinction that matters is per-glyph against per-run. Converting each
// glyph and summing rounds sixty times down a line of prose and accumulates,
// and a paragraph measured one way then re-measured the other breaks into
// different lines — so the same specification would paginate differently
// depending on which code path measured it. Converting once per run is one
// rounding for ordinary prose and at most a handful for a line carrying a bold
// word, it is bounded by the styling the author wrote rather than by the length
// of the text, and it is the same number every time the line is measured.
//
// A line cannot be measured with one division across the whole of it, because a
// styled run brings its own size and its own units per em and there is no
// single denominator to divide by. Per run is where the exactness ends and it
// is where the author's own structure puts the boundary.
//
// # Breaking
//
// Break opportunities come from the UAX#14 line breaking algorithm, via
// go-text's segmenter, over the paragraph's whole text — so an opportunity
// falling inside a styled run, or exactly at the boundary between two, is found
// the same way as any other. The fill is greedy: take the last opportunity that
// fits. Greedy rather than an optimal-fit paragraph shaper because the two
// produce different line breaks, and a document's pagination must not change
// because a later version got cleverer about ragged edges.
//
// Shaping is per run, because a change of face or size is a shaping boundary:
// no typesetter kerns across one, and asking harfbuzz to would mean shaping
// text in a face that only sets part of it.
//
// Text is never broken where Unicode does not permit it. A word wider than its
// box is [errors.VELLUM_PDF_TEXT_OVERFLOW], not a mid-word break: a break no
// reader would have made is worse than a diagnosable failure.
package text

import (
	"strings"
	"unicode"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/color"
	"github.com/frankbardon/vellum/pdf/font"
	"github.com/frankbardon/vellum/pdf/object"
	"github.com/frankbardon/vellum/pdf/shape"
	"github.com/go-text/typesetting/segmenter"
)

// Align is how a line is placed within its measure.
type Align string

const (
	// AlignLeft leaves the ragged edge on the right.
	AlignLeft Align = "left"

	// AlignRight leaves it on the left.
	AlignRight Align = "right"

	// AlignCenter splits the slack between both edges.
	AlignCenter Align = "center"

	// AlignJustify distributes the slack between the words instead.
	AlignJustify Align = "justify"
)

// AllAlignments returns the alignments, in declaration order.
func AllAlignments() []Align { return []Align{AlignLeft, AlignRight, AlignCenter, AlignJustify} }

// Style is everything about how a run of text is set and drawn.
//
// The face and the shaper travel together because they must describe the same
// font program: shaping with one and encoding with another produces glyph
// identifiers that address the wrong outlines, which draws as plausible
// nonsense rather than as an error.
type Style struct {
	// Face is the embedded font this run is drawn with.
	Face *font.Face

	// Shaper shapes this run. It must be over the same program as Face.
	Shaper *shape.Shaper

	// Size is the type size, in text space.
	Size object.Real

	// Color is the fill colour. The zero value is black.
	Color color.RGB
}

// Span is a run of text in one style.
type Span struct {
	Text  string
	Style Style
}

// Segment is the part of a line that came from one span.
type Segment struct {
	// Style is the span's style, copied so a laid-out line is self-contained.
	Style Style

	// Glyphs are this segment's glyphs, trailing whitespace included when the
	// segment ends the line.
	Glyphs []shape.Glyph

	// Visible is the number of leading glyphs that are not trailing whitespace
	// at the end of the line.
	Visible int

	// runes is the paragraph's runes. A glyph's cluster indexes it directly:
	// clusters are rebased onto the paragraph when the spans are flattened, so
	// a segment does not carry an offset of its own to get wrong.
	runes []rune
}

// Line is one laid-out line.
type Line struct {
	// Segments are the line's styled parts, in order. A line of ordinary prose
	// has one.
	Segments []Segment

	// Width is the width of the visible glyphs, in text space.
	Width object.Real

	// Gaps is how many inter-word spaces the visible glyphs contain, which is
	// how many places justification has to distribute slack into.
	Gaps int

	// Height is the largest type size on the line. It is what a leading is
	// computed from: a line carrying one word at twice the body size is twice
	// as tall, and spacing it as though it were not overlaps the line above.
	Height object.Real

	// Mandatory reports that the line ended at a break the text demanded — a
	// newline — rather than at one chosen to fit the measure.
	Mandatory bool

	// Last reports that this is the final line of its paragraph.
	Last bool

	// gaps locates the inter-word spaces as (segment, glyph) pairs, which is
	// what justification needs: the displacement after a space is written into
	// the TJ array of the segment that space belongs to, because the segments
	// are drawn with different fonts.
	gaps []gap
}

// gap is one inter-word space, located within the line.
type gap struct{ segment, glyph int }

// Glyphs returns the line's glyphs, across every segment.
//
// A convenience for tests and for callers that need the shape of a line rather
// than its styling. Drawing goes through [Line.Show], which needs the segments.
func (l Line) Glyphs() []shape.Glyph {
	var out []shape.Glyph
	for i := range l.Segments {
		out = append(out, l.Segments[i].Glyphs...)
	}
	return out
}

// Visible returns the number of glyphs the line draws.
func (l Line) Visible() int {
	n := 0
	for i := range l.Segments {
		n += l.Segments[i].Visible
	}
	return n
}

// Text returns the characters the line covers, trailing whitespace included.
func (l Line) Text() string {
	var b strings.Builder
	for i := range l.Segments {
		b.WriteString(l.Segments[i].text())
	}
	return b.String()
}

// text returns the characters a segment's glyphs came from.
func (s Segment) text() string {
	if len(s.Glyphs) == 0 {
		return ""
	}
	from, to := len(s.runes), 0
	for _, g := range s.Glyphs {
		from = min(from, g.Cluster)
		to = max(to, g.Cluster+1)
	}
	if from > to {
		return ""
	}
	return string(s.runes[max(from, 0):min(to, len(s.runes))])
}

// WrapOptions configures a wrap.
type WrapOptions struct {
	// Width is the measure: the horizontal space a line may occupy.
	Width object.Real
}

// Wrap shapes a paragraph's spans and breaks them into lines.
//
// The spans are shaped individually and broken together: each is shaped in its
// own face at its own size, and the break opportunities are found over their
// concatenated text. That is the arrangement a change of style demands —
// shaping across a face boundary is not a thing harfbuzz can do — and it is
// also the only one that finds an opportunity sitting exactly on a boundary.
func Wrap(spans []Span, opts WrapOptions) ([]Line, error) {
	if opts.Width <= 0 {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"text cannot be laid out in a non-positive measure",
			map[string]any{"width": opts.Width.String()})
	}
	for i := range spans {
		if spans[i].Style.Size <= 0 {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
				"text cannot be laid out at a non-positive size",
				map[string]any{"span": i, "size": spans[i].Style.Size.String()})
		}
		if spans[i].Style.Shaper == nil || spans[i].Style.Face == nil {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
				"a span carries no face or no shaper",
				map[string]any{"span": i})
		}
	}

	// Nothing to lay out is not a blank line. A caller passing an empty run —
	// a paragraph whose only content was resolved away — asked for no lines,
	// and returning one would insert vertical space nobody wrote.
	empty := true
	for i := range spans {
		if spans[i].Text != "" {
			empty = false
			break
		}
	}
	if empty {
		return nil, nil
	}

	var out []Line
	for _, paragraph := range splitHardBreaks(spans) {
		lines, err := wrapParagraph(paragraph, opts)
		if err != nil {
			return nil, err
		}
		if len(lines) == 0 {
			// A hard break with nothing before it is a blank line, and a blank
			// line is content: two newlines in a row are a gap the author asked
			// for, and dropping it reflows the document.
			out = append(out, Line{Mandatory: true, Height: paragraph.height()})
			continue
		}
		// The paragraph's last line ended where the text said to, not where the
		// measure did, which is what keeps it from being justified.
		lines[len(lines)-1].Mandatory = true
		out = append(out, lines...)
	}

	if len(out) > 0 {
		out[len(out)-1].Last = true
	}
	return out, nil
}

// WrapText is the single-style convenience: one face, one size, one colour.
func WrapText(style Style, s string, opts WrapOptions) ([]Line, error) {
	if s == "" {
		return nil, nil
	}
	return Wrap([]Span{{Text: s, Style: style}}, opts)
}

// paragraph is a run of spans with no mandatory break inside it.
type paragraph []Span

// height is the largest size the paragraph is set at.
func (p paragraph) height() object.Real {
	var h object.Real
	for i := range p {
		h = max(h, p[i].Style.Size)
	}
	return h
}

// splitHardBreaks divides spans at the breaks Unicode makes mandatory.
//
// Done before shaping rather than during it, for two reasons. Shaping context
// does not cross a hard break — no ligature or kerning pair spans a line the
// author ended — so nothing is lost. And a line separator is not text to draw:
// shaping one asks the face for a glyph it does not have, which is correctly an
// error, so the character has to be gone before the shaper sees it.
//
// A single trailing break does not produce an empty final paragraph. It
// terminates the last line rather than starting another one, which is what every
// text editor means by it.
func splitHardBreaks(spans []Span) []paragraph {
	replacer := strings.NewReplacer(
		"\r\n", "\n",
		"\r", "\n",
		" ", "\n", // LINE SEPARATOR
		" ", "\n", // PARAGRAPH SEPARATOR
		"\v", "\n",
		"\f", "\n",
	)

	out := []paragraph{{}}
	for _, sp := range spans {
		parts := strings.Split(replacer.Replace(sp.Text), "\n")
		for i, part := range parts {
			if i > 0 {
				out = append(out, paragraph{})
			}
			if part == "" {
				continue
			}
			last := len(out) - 1
			out[last] = append(out[last], Span{Text: part, Style: sp.Style})
		}
	}

	// A break at the very end terminates the last line rather than starting an
	// empty one. Any earlier empty paragraph is a blank line the author asked
	// for and is kept.
	if n := len(out); n > 1 && len(out[n-1]) == 0 {
		out = out[:n-1]
	}
	// A paragraph carrying no spans at all still needs a style to be measured
	// against, which is the previous one's — the same face the blank line sits
	// between.
	for i := range out {
		if len(out[i]) == 0 && i > 0 && len(out[i-1]) > 0 {
			out[i] = paragraph{{Style: out[i-1][len(out[i-1])-1].Style}}
		}
	}
	return out
}

// placed is one glyph together with the span it came from.
//
// A flat list across the paragraph's spans, in reading order, which is what
// lets the break search treat a styled paragraph exactly like an unstyled one.
type placed struct {
	span  int
	glyph shape.Glyph
}

// wrapParagraph breaks one hard-break-free paragraph into lines.
func wrapParagraph(p paragraph, opts WrapOptions) ([]Line, error) {
	if len(p) == 0 {
		return nil, nil
	}

	var runes []rune
	starts := make([]int, len(p))
	for i := range p {
		starts[i] = len(runes)
		runes = append(runes, []rune(p[i].Text)...)
	}
	if len(runes) == 0 {
		return nil, nil
	}

	var flat []placed
	for i := range p {
		if p[i].Text == "" {
			continue
		}
		run, err := p[i].Style.Shaper.Shape(p[i].Text)
		if err != nil {
			return nil, err
		}
		for _, g := range run.Glyphs {
			// Clusters arrive relative to the span's own text and are rebased
			// onto the paragraph's, so every downstream lookup — whitespace,
			// break opportunities, the overflow message — indexes one string.
			g.Cluster += starts[i]
			flat = append(flat, placed{span: i, glyph: g})
		}
	}
	if len(flat) == 0 {
		return nil, nil
	}

	m := &measurer{p: p, runes: runes, starts: starts, flat: flat, width: opts.Width}
	stops := breakStops(runes)
	glyphAt := clusterIndexOf(flat, len(runes))

	var out []Line
	start := 0     // first glyph of the line being filled
	accepted := -1 // last opportunity that fitted, as a glyph index

	for i := 0; i < len(stops); {
		end := glyphAt(stops[i].offset)
		if end <= start {
			i++
			continue
		}

		if m.fits(start, end) {
			accepted = end
			i++
			continue
		}

		if accepted > start {
			// Take the last opportunity that fitted and reconsider this one
			// against the line that now begins after it. The index deliberately
			// does not advance: whether this opportunity fits on the new line is
			// a fresh question, and assuming it does is the off-by-one this loop
			// exists to avoid.
			out = append(out, m.line(start, accepted))
			start, accepted = accepted, -1
			continue
		}

		// Nothing shorter fitted, so this piece contains no break opportunity
		// and is wider than the measure on its own.
		return nil, m.overflow(start, end)
	}

	if start < len(flat) {
		out = append(out, m.line(start, len(flat)))
	}
	return out, nil
}

// measurer measures and cuts ranges of one paragraph's flattened glyphs.
type measurer struct {
	p      paragraph
	runes  []rune
	starts []int
	flat   []placed
	width  object.Real
}

// runsIn divides a glyph range at its span boundaries.
//
// The spans are laid out in order, so a range covers a contiguous stretch of
// them and the division is a scan.
func (m *measurer) runsIn(from, to int) [][2]int {
	var out [][2]int
	for i := from; i < to; {
		j := i + 1
		for j < to && m.flat[j].span == m.flat[i].span {
			j++
		}
		out = append(out, [2]int{i, j})
		i = j
	}
	return out
}

// visible returns the index one past the last glyph that is not trailing
// whitespace, within a range.
func (m *measurer) visible(from, to int) int {
	v := to
	for v > from && m.isSpace(v-1) {
		v--
	}
	return v
}

// isSpace reports whether a flattened glyph came from a space character.
func (m *measurer) isSpace(i int) bool {
	c := m.flat[i].glyph.Cluster
	if c < 0 || c >= len(m.runes) {
		return false
	}
	return unicode.IsSpace(m.runes[c])
}

// width of a range, in text space, converted once per styled run.
func (m *measurer) advance(from, to int) object.Real {
	var total object.Real
	for _, r := range m.runsIn(from, to) {
		st := m.p[m.flat[r[0]].span].Style
		var units int64
		for i := r[0]; i < r[1]; i++ {
			units += int64(m.flat[i].glyph.Advance)
		}
		total += toTextSpace(units, int64(st.Shaper.UnitsPerEm()), st.Size)
	}
	return total
}

// fits reports whether a glyph range is no wider than the measure.
func (m *measurer) fits(from, to int) bool {
	return m.advance(from, m.visible(from, to)) <= m.width
}

// line cuts a range into a Line.
func (m *measurer) line(from, to int) Line {
	visible := m.visible(from, to)

	l := Line{Width: m.advance(from, visible)}
	for n, r := range m.runsIn(from, to) {
		span := m.flat[r[0]].span
		glyphs := make([]shape.Glyph, 0, r[1]-r[0])
		for i := r[0]; i < r[1]; i++ {
			glyphs = append(glyphs, m.flat[i].glyph)
			if i < visible && m.isSpace(i) {
				l.gaps = append(l.gaps, gap{segment: n, glyph: i - r[0]})
			}
		}
		l.Segments = append(l.Segments, Segment{
			Style:   m.p[span].Style,
			Glyphs:  glyphs,
			Visible: max(0, min(visible, r[1])-r[0]),
			runes:   m.runes,
		})
		l.Height = max(l.Height, m.p[span].Style.Size)
	}
	l.Gaps = len(l.gaps)
	return l
}

// overflow builds the error for a piece with no break opportunity inside it.
func (m *measurer) overflow(from, to int) error {
	lo, hi := len(m.runes), 0
	for i := from; i < to; i++ {
		c := m.flat[i].glyph.Cluster
		lo = min(lo, c)
		hi = max(hi, c+1)
	}
	if lo > hi {
		lo, hi = 0, 0
	}
	hi = min(hi, len(m.runes))

	st := m.p[m.flat[from].span].Style
	return verr.NewCodedErrorWithDetails(verr.VELLUM_PDF_TEXT_OVERFLOW,
		"a piece of text with no break opportunity is wider than the space available",
		map[string]any{
			"text":       string(m.runes[lo:hi]),
			"width":      m.width.String(),
			"font_size":  st.Size.String(),
			"rune_start": lo,
		})
}

// breakStop is one UAX#14 opportunity.
type breakStop struct {
	offset    int // rune offset at which a line may end
	mandatory bool
}

// breakStops returns every position the text may be broken at.
func breakStops(runes []rune) []breakStop {
	var seg segmenter.Segmenter
	seg.Init(runes)

	var out []breakStop
	iter := seg.LineIterator()
	for iter.Next() {
		line := iter.Line()
		out = append(out, breakStop{
			offset:    line.Offset + len(line.Text),
			mandatory: line.IsMandatoryBreak,
		})
	}
	return out
}

// clusterIndexOf returns a function mapping a rune offset to the first
// flattened glyph at or after it.
//
// Clusters ascend for left-to-right text, so this is a scan rather than a
// search, and it is memoised into a table because a paragraph is queried once
// per break opportunity.
func clusterIndexOf(flat []placed, runeCount int) func(offset int) int {
	table := make([]int, runeCount+1)
	g := 0
	for r := 0; r <= runeCount; r++ {
		for g < len(flat) && flat[g].glyph.Cluster < r {
			g++
		}
		table[r] = g
	}
	return func(offset int) int {
		if offset < 0 {
			return 0
		}
		if offset >= len(table) {
			return len(flat)
		}
		return table[offset]
	}
}

// toTextSpace converts an advance in font units to text space at a size.
//
// Once per styled run per line, rounding half away from zero. Doing it per
// glyph and summing would accumulate up to one unit of error per glyph, which
// over a line of sixty characters is a visible difference in where the right
// edge falls.
func toTextSpace(advance, upem int64, size object.Real) object.Real {
	if upem == 0 {
		return 0
	}
	n := advance * int64(size)
	if (n < 0) != (upem < 0) {
		return object.Real((n - upem/2) / upem)
	}
	return object.Real((n + upem/2) / upem)
}
