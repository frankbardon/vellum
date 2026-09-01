package text

import (
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/content"
	"github.com/frankbardon/vellum/pdf/font"
	"github.com/frankbardon/vellum/pdf/font/sfnt"
	"github.com/frankbardon/vellum/pdf/object"
	"github.com/frankbardon/vellum/pdf/shape"
)

// Offset returns the horizontal displacement that places the line within a
// measure.
//
// Justified lines take no offset: their slack goes between the words instead,
// which [Line.Show] handles.
func (l Line) Offset(align Align, width object.Real) object.Real {
	slack := width - l.Width
	if slack <= 0 {
		return 0
	}
	switch align {
	case AlignRight:
		return slack
	case AlignCenter:
		// Half, rounded down. Rounding up would turn a one-unit slack into a
		// one-unit offset, which is a thousandth of a point and not worth the
		// asymmetry between two lines that differ by one unit.
		return slack / 2
	default:
		return 0
	}
}

// Justifiable reports whether the line should have its slack distributed
// between words.
//
// The last line of a paragraph is never justified, and neither is a line ending
// at a break the text demanded. Justifying either produces the classic defect: a
// final line of three words stretched across the whole measure.
func (l Line) Justifiable(align Align) bool {
	return align == AlignJustify && !l.Last && !l.Mandatory && len(l.gaps) > 0
}

// Show emits the line's glyphs into a content stream.
//
// The face records the glyphs as used, which is what keeps the subset and the
// content stream in agreement about which glyphs exist.
//
// Trailing whitespace is not drawn. It stays on the line because it is part of
// the text, and drawing it would push the pen past the measure and, on a
// justified line, would earn a share of the slack.
func (l Line) Show(b *content.Builder, f *font.Face, align Align, width object.Real) error {
	glyphs := l.Glyphs[:l.Visible]
	if len(glyphs) == 0 {
		return nil
	}

	ids := make([]uint16, len(glyphs))
	for i, g := range glyphs {
		ids[i] = uint16(g.ID)
	}
	if _, err := f.EncodeGlyphs(glyphIDs(glyphs)); err != nil {
		return err
	}

	slack := width - l.Width
	if !l.Justifiable(align) || slack <= 0 {
		b.ShowGlyphs(ids)
		return nil
	}

	items, err := l.justify(ids, slack)
	if err != nil {
		return err
	}
	b.ShowAdjusted(items)
	return nil
}

// justify splits the line at its spaces and distributes the slack between them.
//
// The remainder of the division is spread over the first gaps rather than
// dropped, so the line ends exactly at the measure. Dropping it leaves every
// justified line short by up to one unit per gap, which down a column reads as a
// wobbling right edge rather than as a straight one.
func (l Line) justify(ids []uint16, slack object.Real) ([]content.Adjusted, error) {
	if len(l.gaps) == 0 {
		return nil, verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT,
			"a line with no inter-word gaps was asked to justify")
	}
	if l.size <= 0 {
		return nil, verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT,
			"the line carries no font size, so no displacement could be computed")
	}

	per := int64(slack) / int64(len(l.gaps))
	remainder := int64(slack) % int64(len(l.gaps))

	items := make([]content.Adjusted, 0, len(l.gaps)+1)
	from := 0
	for n, at := range l.gaps {
		extra := per
		if int64(n) < remainder {
			extra++
		}
		items = append(items, content.Adjusted{
			Glyphs: ids[from : at+1],
			Adjust: tjFor(object.Real(extra), l.size),
		})
		from = at + 1
	}
	if from < len(ids) {
		items = append(items, content.Adjusted{Glyphs: ids[from:]})
	}
	return items, nil
}

// glyphIDs projects shaped glyphs to the identifiers the face records.
func glyphIDs(glyphs []shape.Glyph) []sfnt.GlyphID {
	out := make([]sfnt.GlyphID, len(glyphs))
	for i, g := range glyphs {
		out[i] = g.ID
	}
	return out
}

// tjFor converts an extra displacement into the number a TJ array carries.
//
// A TJ number is subtracted from the pen's displacement, in thousandths of a
// unit of text space, and is scaled by the font size. So inserting d of space
// needs a number of -1000*d/size — negative, which is the part that is easy to
// get backwards and produces a line that gets tighter as it is justified.
//
// Computed in integers and rounded half away from zero, so the same line
// justifies identically however many times it is laid out.
func tjFor(extra, size object.Real) object.Real {
	if size == 0 {
		return 0
	}
	n := -int64(extra) * 1_000_000
	d := int64(size)
	if (n < 0) != (d < 0) {
		return object.Real((n - d/2) / d)
	}
	return object.Real((n + d/2) / d)
}
