// Package text breaks shaped text into lines and places them.
//
// # Where the arithmetic happens
//
// Advances arrive from [shape] in font design units, as integers. They stay
// integers here. A line's fit is decided by comparing two products rather than
// by dividing:
//
//	advanceUnits * fontSize   against   availableWidth * unitsPerEm
//
// which is exact, where converting each advance to points and adding them up is
// a rounding per glyph and an accumulation across the line. The difference is
// not cosmetic: a paragraph measured one way and re-measured the other breaks
// into different lines, so the same specification would paginate differently
// depending on which code path measured it.
//
// The conversion to text space happens once per line, at the end.
//
// # Breaking
//
// Break opportunities come from the UAX#14 line breaking algorithm, via
// go-text's segmenter. The fill is greedy: take the last opportunity that fits.
// Greedy rather than an optimal-fit paragraph shaper because the two produce
// different line breaks, and a document's pagination must not change because a
// later version got cleverer about ragged edges.
//
// Text is never broken where Unicode does not permit it. A word wider than its
// box is [errors.VELLUM_PDF_TEXT_OVERFLOW], not a mid-word break: a break no
// reader would have made is worse than a diagnosable failure.
package text

import (
	"strings"
	"unicode"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/object"
	"github.com/frankbardon/vellum/pdf/shape"
	"github.com/go-text/typesetting/segmenter"
)

// Align is how a line is placed within its measure.
type Align string

const (
	// AlignLeft leaves the ragged edge on the right. The default.
	AlignLeft Align = "left"

	// AlignRight leaves the ragged edge on the left.
	AlignRight Align = "right"

	// AlignCenter splits the slack evenly.
	AlignCenter Align = "center"

	// AlignJustify spreads the slack between words, leaving the last line of a
	// paragraph set flush left. Justifying the last line too is the classic
	// error that produces a final line of three words spanning the measure.
	AlignJustify Align = "justify"
)

// AllAlignments returns the alignments, in declaration order.
func AllAlignments() []Align { return []Align{AlignLeft, AlignRight, AlignCenter, AlignJustify} }

// Line is one laid-out line.
type Line struct {
	// Glyphs are the line's glyphs, trailing whitespace included. Whitespace at
	// a break is retained because it belongs to the text; it is excluded from
	// [Line.Width] because it hangs into the margin rather than occupying the
	// measure, which is what every typesetter does and what makes a ragged
	// right edge look straight.
	Glyphs []shape.Glyph

	// Visible is the number of leading glyphs that are not trailing whitespace.
	Visible int

	// Width is the width of those visible glyphs, in text space.
	Width object.Real

	// Gaps is how many inter-word spaces the visible glyphs contain, which is
	// how many places justification has to distribute slack into.
	Gaps int

	// Mandatory reports that the line ended at a break the text demanded — a
	// newline — rather than at one chosen to fit the measure.
	Mandatory bool

	// Last reports that this is the final line of its paragraph.
	Last bool

	// gaps holds the glyph indices of the inter-word spaces, and size the font
	// size the line was measured at.
	//
	// Unexported because a Line is produced by [Wrap] and not assembled: both
	// are facts about how it was measured, and a Line carrying a size that
	// disagrees with the one it is drawn at would justify to the wrong width.
	gaps []int
	size object.Real
}

// WrapOptions configures a wrap.
type WrapOptions struct {
	// Size is the font size, in text space.
	Size object.Real

	// Width is the measure: the horizontal space a line may occupy.
	Width object.Real
}

// Wrap shapes text and breaks it into lines.
//
// The whole paragraph is shaped once and then divided, rather than each
// candidate line being shaped separately. Shaping is contextual — a ligature or
// a kerning pair spans the boundary a naive implementation would shape across —
// so measuring a fragment in isolation gives a width the assembled line does not
// have.
func Wrap(s *shape.Shaper, text string, opts WrapOptions) ([]Line, error) {
	if opts.Size <= 0 {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"text cannot be laid out at a non-positive size",
			map[string]any{"size": opts.Size.String()})
	}
	if opts.Width <= 0 {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"text cannot be laid out in a non-positive measure",
			map[string]any{"width": opts.Width.String()})
	}
	if text == "" {
		return nil, nil
	}

	var out []Line
	for _, paragraph := range splitHardBreaks(text) {
		lines, err := wrapParagraph(s, paragraph, opts)
		if err != nil {
			return nil, err
		}
		if len(lines) == 0 {
			// A hard break with nothing before it is a blank line, and a blank
			// line is content: two newlines in a row are a gap the author asked
			// for, and dropping it reflows the document.
			out = append(out, Line{Mandatory: true, size: opts.Size})
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

// splitHardBreaks divides text at the breaks Unicode makes mandatory.
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
func splitHardBreaks(text string) []string {
	replacer := strings.NewReplacer(
		"\r\n", "\n",
		"\r", "\n",
		"\u2028", "\n", // LINE SEPARATOR
		"\u2029", "\n", // PARAGRAPH SEPARATOR
		"\v", "\n",
		"\f", "\n",
	)
	out := strings.Split(replacer.Replace(text), "\n")
	if n := len(out); n > 1 && out[n-1] == "" {
		out = out[:n-1]
	}
	return out
}

// wrapParagraph breaks one hard-break-free paragraph into lines.
func wrapParagraph(s *shape.Shaper, text string, opts WrapOptions) ([]Line, error) {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil, nil
	}

	run, err := s.Shape(text)
	if err != nil {
		return nil, err
	}

	upem := int64(s.UnitsPerEm())
	stops := breakStops(runes)
	glyphAt := clusterIndex(run.Glyphs, len(runes))

	var out []Line
	start := 0     // first glyph of the line being filled
	accepted := -1 // last opportunity that fitted, as a glyph index

	for i := 0; i < len(stops); {
		end := glyphAt(stops[i].offset)
		if end <= start {
			i++
			continue
		}

		if fits(run.Glyphs[start:end], runes, upem, opts) {
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
			out = append(out, measure(run.Glyphs[start:accepted], runes, upem, opts, false))
			start, accepted = accepted, -1
			continue
		}

		// Nothing shorter fitted, so this piece contains no break opportunity
		// and is wider than the measure on its own.
		return nil, overflow(runes, run.Glyphs[start:end], opts)
	}

	if start < len(run.Glyphs) {
		out = append(out, measure(run.Glyphs[start:], runes, upem, opts, false))
	}
	return out, nil
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

// clusterIndex returns a function mapping a rune offset to the first glyph at
// or after it.
//
// Clusters ascend for left-to-right text, so this is a scan rather than a
// search, and it is memoised into a table because a paragraph is queried once
// per break opportunity.
func clusterIndex(glyphs []shape.Glyph, runeCount int) func(offset int) int {
	table := make([]int, runeCount+1)
	g := 0
	for r := 0; r <= runeCount; r++ {
		for g < len(glyphs) && glyphs[g].Cluster < r {
			g++
		}
		table[r] = g
	}
	return func(offset int) int {
		if offset < 0 {
			return 0
		}
		if offset >= len(table) {
			return len(glyphs)
		}
		return table[offset]
	}
}

// visibleAdvance returns the advance of a glyph range excluding trailing
// whitespace, and the glyph indices of the inter-word gaps within it.
func visibleAdvance(glyphs []shape.Glyph, runes []rune) (advance int64, visible int, gaps []int) {
	visible = len(glyphs)
	for visible > 0 && isSpaceGlyph(glyphs[visible-1], runes) {
		visible--
	}
	for i := range visible {
		advance += int64(glyphs[i].Advance)
		if isSpaceGlyph(glyphs[i], runes) {
			gaps = append(gaps, i)
		}
	}
	return advance, visible, gaps
}

// isSpaceGlyph reports whether a glyph came from a space.
func isSpaceGlyph(g shape.Glyph, runes []rune) bool {
	if g.Cluster < 0 || g.Cluster >= len(runes) {
		return false
	}
	return unicode.IsSpace(runes[g.Cluster])
}

// fits reports whether a glyph range is no wider than the measure.
//
// Two products, never a division: converting the advance to text space first
// would round, and the rounding decides the break for a line that lands within
// half a unit of the measure.
func fits(glyphs []shape.Glyph, runes []rune, upem int64, opts WrapOptions) bool {
	advance, _, _ := visibleAdvance(glyphs, runes)
	return advance*int64(opts.Size) <= int64(opts.Width)*upem
}

// measure converts a glyph range into a Line.
func measure(glyphs []shape.Glyph, runes []rune, upem int64, opts WrapOptions, mandatory bool) Line {
	advance, visible, gaps := visibleAdvance(glyphs, runes)
	return Line{
		Glyphs:    glyphs,
		Visible:   visible,
		Width:     toTextSpace(advance, upem, opts.Size),
		Gaps:      len(gaps),
		Mandatory: mandatory,
		gaps:      gaps,
		size:      opts.Size,
	}
}

// toTextSpace converts an advance in font units to text space at a size.
//
// Once per line, at the end, rounding half away from zero. Doing it per glyph
// and summing would accumulate up to one unit of error per glyph, which over a
// line of sixty characters is a visible difference in where the right edge
// falls.
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

// overflow builds the error for a piece with no break opportunity inside it.
func overflow(runes []rune, glyphs []shape.Glyph, opts WrapOptions) error {
	from, to := len(runes), 0
	for _, g := range glyphs {
		from = min(from, g.Cluster)
		to = max(to, g.Cluster+1)
	}
	if from > to {
		from, to = 0, 0
	}
	to = min(to, len(runes))

	return verr.NewCodedErrorWithDetails(verr.VELLUM_PDF_TEXT_OVERFLOW,
		"a piece of text with no break opportunity is wider than the space available",
		map[string]any{
			"text":       string(runes[from:to]),
			"width":      opts.Width.String(),
			"font_size":  opts.Size.String(),
			"rune_start": from,
		})
}
