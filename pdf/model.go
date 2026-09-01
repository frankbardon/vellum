package pdf

import (
	"github.com/frankbardon/vellum/pdf/color"
	"github.com/frankbardon/vellum/pdf/font"
	pdfimage "github.com/frankbardon/vellum/pdf/image"
	"github.com/frankbardon/vellum/pdf/object"
	"github.com/frankbardon/vellum/pdf/text"
)

// ItemKind names which arm of an [Item] carries content.
type ItemKind string

const (
	ItemText  ItemKind = "text"
	ItemImage ItemKind = "image"
	ItemRule  ItemKind = "rule"
	ItemRaw   ItemKind = "raw"
)

// Item is one thing drawn on a page. A tagged union, as elsewhere in this
// library and for the same reason: a writer that switches on a kind cannot
// forget an arm, and a reader can see the whole vocabulary in one place.
//
// Every coordinate is in points from the page's bottom-left corner, which is
// PDF's own origin. Converting at the model boundary rather than at each draw
// would mean two coordinate systems in one file and a sign error that renders.
type Item struct {
	Kind  ItemKind
	Text  *TextItem
	Image *ImageItem
	Rule  *RuleItem
	Raw   *RawItem
}

// TextItem is one paragraph, already broken into lines.
//
// The lines arrive laid out: breaking has happened, and so has the decision
// about which lines fit on this page. What remains is placement, which is why
// this carries a first baseline and a leading rather than a box.
type TextItem struct {
	// X is the left edge of the measure and Y the first baseline.
	X, Y object.Real

	// Width is the measure the lines were broken to, which alignment and
	// justification are both computed against.
	Width object.Real

	// Align is how each line sits within the measure.
	Align text.Align

	// Leading is the baseline-to-baseline distance. One value for the whole
	// paragraph rather than one per line: a paragraph set at a consistent
	// rhythm is what a reader expects, and varying the leading inside one
	// because a single word is larger produces a visibly uneven block.
	Leading object.Real

	// Lines are the paragraph's lines that belong on this page.
	Lines []text.Line
}

// Height is the vertical space the paragraph occupies from its first baseline.
func (t *TextItem) Height() object.Real {
	if t == nil || len(t.Lines) == 0 {
		return 0
	}
	return object.Real(len(t.Lines)) * t.Leading
}

// ImageItem places a raster asset in a rectangle.
type ImageItem struct {
	// X and Y are the rectangle's bottom-left corner.
	X, Y object.Real

	// Width and Height are its size in points. Both are concrete: the aspect
	// ratio was applied during resolution, because a writer has no business
	// asking an asset how tall it is.
	Width, Height object.Real

	Image *pdfimage.XObject
}

// RuleItem is a filled rectangle: a rule, a cell shade, a table border.
type RuleItem struct {
	X, Y          object.Real
	Width, Height object.Real
	Color         color.RGB
}

// RawItem is a prebuilt content stream.
//
// The escape hatch for a consumer needing PDF operators this model does not
// express. It declares the resources it uses, because nothing can read them out
// of the bytes — and a page whose resource dictionary is missing a font the
// stream selects draws nothing at all, with no error anywhere.
type RawItem struct {
	Content []byte
	Fonts   []*font.Face
	Images  []*pdfimage.XObject
}

// Text returns a text item.
func Text(t TextItem) Item { return Item{Kind: ItemText, Text: &t} }

// Image returns an image item.
func Image(i ImageItem) Item { return Item{Kind: ItemImage, Image: &i} }

// Rule returns a filled-rectangle item.
func Rule(r RuleItem) Item { return Item{Kind: ItemRule, Rule: &r} }

// Raw returns a prebuilt-content item.
func Raw(r RawItem) Item { return Item{Kind: ItemRaw, Raw: &r} }

// faces returns the font faces an item selects, in the order it selects them.
func (i Item) faces() []*font.Face {
	switch i.Kind {
	case ItemText:
		var out []*font.Face
		for _, l := range i.Text.Lines {
			for _, s := range l.Segments {
				// Only segments that draw. A blank line carries a style so it
				// can be measured, and selects no font at all — naming its face
				// in the resource dictionary would embed a subset with no
				// glyphs in it, which the font writer correctly refuses.
				if s.Visible > 0 {
					out = append(out, s.Style.Face)
				}
			}
		}
		return out
	case ItemRaw:
		return i.Raw.Fonts
	}
	return nil
}

// images returns the image XObjects an item draws.
func (i Item) images() []*pdfimage.XObject {
	switch i.Kind {
	case ItemImage:
		return []*pdfimage.XObject{i.Image.Image}
	case ItemRaw:
		return i.Raw.Images
	}
	return nil
}
