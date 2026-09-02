package sheet

import (
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/theme"
)

// StyleSheet is the styles part.
//
// SpreadsheetML's styles.xml is a fixed preamble around a caller's content
// rather than a free-form document: `numFmts`, `fonts`, `fills`, `borders`,
// `cellStyleXfs`, `cellXfs` and `cellStyles` appear in that order and no
// other, and two of the collections carry reserved entries a writer must not
// disturb.
//
//   - Fill indices 0 and 1 are reserved by the ECMA-376 schema itself: index 0
//     is the "none" pattern and index 1 is "gray125", and every reader expects
//     them there whether or not any cell uses them. [StyleSheet.Fills] carries
//     only the workbook's own custom fills; the writer prepends the two
//     reserved entries and a [Cell.StyleID] of zero for "no fill" — see
//     [xmlFillID] — never collides with them.
//   - `cellStyleXfs` must carry at least one entry, and `cellStyles` must
//     reference it by name "Normal" with `builtinId="0"`. Neither is a
//     modelling decision a caller makes, so neither appears here: the writer
//     emits both, fixed, every time.
//
// [Formats] is `cellXfs` — what a [Cell.StyleID] actually indexes — and index
// 0 is the workbook's ordinary appearance. A hand-built [Workbook] leaving
// [Formats] empty gets one synthesised at write time, because a styles part
// with no index 0 is one Excel refuses.
type StyleSheet struct {
	// DefaultFont is font index 0: the workbook's own default, which every
	// cell that names no other font takes. Every SpreadsheetML workbook has
	// one and it is always written first, so it is a field here rather than
	// element zero of Fonts — a slice a caller could accidentally leave empty
	// and lose the one font index 0 always resolves to.
	DefaultFont Font

	// Fonts are the fonts a [CellFormat.FontIndex] of 1 or more selects.
	// FontIndex 0 is [DefaultFont] and is not listed here — see [xmlFontID].
	Fonts []Font

	// Fills are the workbook's custom fills. See the type doc for the reserved
	// indices this collection does not carry.
	Fills []Fill

	// Borders are the workbook's custom borders. BorderIndex 0 is "no border"
	// and is not listed here — see [xmlBorderID].
	Borders []Border

	// NumFmts are the workbook's custom number-format codes. NumFmtIndex 0 is
	// "General" and is not listed here — see [xmlNumFmtID].
	NumFmts []NumFmt

	// Formats is `cellXfs`: every combination of font, fill, border, number
	// format and wrap setting a cell in this workbook actually uses. A
	// [Cell.StyleID] is an index into this slice.
	Formats []CellFormat
}

// Font is one entry of the font collection.
type Font struct {
	// Name is the family name.
	Name string

	// SizeEMU is the type size.
	SizeEMU int64

	// Color is an uppercase sRGB hex triplet with no leading hash. Empty
	// means the reader's own default, normally black.
	Color string

	Bold, Italic bool
}

// Fill is one entry of the custom fill collection: a solid pattern fill.
//
// SpreadsheetML's fill element can express gradients and other pattern types;
// this carries only a solid colour because that is the whole of what a table
// band or a header row needs, and a richer model here would be surface no
// caller in this library exercises.
type Fill struct {
	// Color is an uppercase sRGB hex triplet with no leading hash.
	Color string
}

// Border is one entry of the custom border collection: a single hairline
// weight and colour on all four edges.
//
// As with [Fill], SpreadsheetML can vary each edge independently and dash each
// one differently; this carries the one shape a table's rule actually is.
type Border struct {
	// Color is an uppercase sRGB hex triplet with no leading hash.
	Color string
}

// NumFmt is one custom number-format code.
type NumFmt struct {
	// Code is the format code, verbatim — the same vocabulary
	// [numfmt.Format.Code] carries, because it is the same vocabulary: xlsx's
	// own, shared by all four writers.
	Code string
}

// CellFormat is one `cellXfs` entry: a combination of formatting a cell may
// reference by [Cell.StyleID].
type CellFormat struct {
	// FontIndex selects [StyleSheet.Fonts]. Zero is the workbook default font.
	// Non-zero N selects Fonts[N-1].
	FontIndex int

	// FillIndex selects [StyleSheet.Fills]. Zero is no fill. Non-zero N
	// selects Fills[N-1].
	FillIndex int

	// BorderIndex selects [StyleSheet.Borders]. Zero is no border. Non-zero N
	// selects Borders[N-1].
	BorderIndex int

	// NumFmtIndex selects [StyleSheet.NumFmts]. Zero is General. Non-zero N
	// selects NumFmts[N-1].
	NumFmtIndex int

	// WrapText wraps the cell's content at the column width, which is the
	// shape [FeatureBlockText] degrades to: "a wrapped cell".
	WrapText bool

	// VerticalTop anchors the cell's content to the top rather than Excel's
	// own default of the bottom.
	//
	// The one place this matters is a row-header stub merged across several
	// body rows: a group label left at the reader's default sits at the
	// bottom of its own merge, which for a ten-row group is a label that is
	// not visible until the reader has already scrolled past every row it
	// names. Top anchoring puts it beside the first row of the group it
	// labels, which is where a reader looks for it.
	VerticalTop bool
}

// xmlFontID converts a model-level [CellFormat.FontIndex] to the position in
// the written `fonts` collection. Font 0 is [StyleSheet.DefaultFont], written
// first; index 0 is never present in [StyleSheet.Fonts] itself.
func xmlFontID(index int) int { return index }

// xmlFillID converts a model-level [CellFormat.FillIndex] to the position in
// the written `fills` collection, after the two entries ECMA-376 reserves.
func xmlFillID(index int) int {
	if index == 0 {
		return 0
	}
	return index + 1
}

// xmlBorderID converts a model-level [CellFormat.BorderIndex] to the position
// in the written `borders` collection, after the implicit "no border" at 0.
func xmlBorderID(index int) int { return index }

// firstCustomNumFmtID is where a caller's own format codes begin. Ids below
// this are reserved by Excel for its built-in codes — General, "0.00%" and the
// rest — and reusing one of those numbers for a different code is exactly the
// kind of collision a reader silently resolves by its own built-in meaning
// rather than the one a workbook's own `numFmts` entry states.
const firstCustomNumFmtID = 164

// xmlNumFmtID converts a model-level [CellFormat.NumFmtIndex] to the written
// `numFmtId`. Zero is General, which needs no `numFmts` entry at all.
func xmlNumFmtID(index int) int {
	if index == 0 {
		return 0
	}
	return firstCustomNumFmtID + index - 1
}

// themeStyles is the subset of the theme the sheet writer needs, flattened to
// EMU so [buildStyleSheet] does not carry the theme's own types around. Mirrors
// [doc]'s typeScale.
type themeStyles struct {
	BodyFont    string
	BodySize    int64
	HeadingFont string
	HeadingSize int64

	TextColor       string
	MutedColor      string
	RuleColor       string
	TableHeaderFill string
	TableHeaderText string
	TableStripeFill string
}

// stylesFrom flattens the theme carried on a resolved document.
func stylesFrom(d *fragment.Doc) themeStyles {
	body := faceFamily(d, theme.FontBody)
	heading := faceFamily(d, theme.FontHeading)

	return themeStyles{
		BodyFont:    body,
		BodySize:    d.Scale.Body,
		HeadingFont: heading,
		HeadingSize: d.Scale.HeadingSize(1),

		TextColor:       d.Palette.Color(theme.ColorText, ""),
		MutedColor:      d.Palette.Color(theme.ColorTextMuted, ""),
		RuleColor:       d.Palette.Color(theme.ColorRule, ""),
		TableHeaderFill: d.Palette.Color(theme.ColorTableHeaderBackground, ""),
		TableHeaderText: d.Palette.Color(theme.ColorTableHeaderText, ""),
		TableStripeFill: d.Palette.Color(theme.ColorTableStripe, ""),
	}
}

// faceFamily returns the resolved family for a role. Mirrors [doc]'s helper of
// the same name.
func faceFamily(d *fragment.Doc, role theme.FontRole) string {
	for i := range d.Fonts {
		if d.Fonts[i].Role == role {
			return d.Fonts[i].Family
		}
	}
	for i := range d.Fonts {
		if d.Fonts[i].Role == theme.FontBody {
			return d.Fonts[i].Family
		}
	}
	return ""
}

// styleBuilder interns fonts, fills, borders and number formats by value and
// hands back the [CellFormat] index a cell should carry.
//
// Built once per lowering. Interning rather than appending unconditionally is
// what keeps a document with a hundred cells in the header band from writing a
// hundred identical `cellXfs` entries — and, more than tidiness, it is what
// keeps the styles part a function of the distinct appearances a workbook
// actually uses rather than of how many cells happen to use them.
type styleBuilder struct {
	defaultFont Font

	fonts   []Font
	fills   []Fill
	borders []Border
	numFmts []NumFmt
	formats []CellFormat
}

// newStyleBuilder seeds index 0 of every collection cellXfs can reference by
// index: the default format, referencing the default font, no fill, no
// border, General.
func newStyleBuilder(defaultFont Font) *styleBuilder {
	return &styleBuilder{defaultFont: defaultFont, formats: []CellFormat{{}}}
}

func (b *styleBuilder) font(f Font) int {
	for i, existing := range b.fonts {
		if existing == f {
			return i + 1
		}
	}
	b.fonts = append(b.fonts, f)
	return len(b.fonts)
}

func (b *styleBuilder) fill(f Fill) int {
	if f.Color == "" {
		return 0
	}
	for i, existing := range b.fills {
		if existing == f {
			return i + 1
		}
	}
	b.fills = append(b.fills, f)
	return len(b.fills)
}

func (b *styleBuilder) numFmt(code string) int {
	if code == "" {
		return 0
	}
	for i, existing := range b.numFmts {
		if existing.Code == code {
			return i + 1
		}
	}
	b.numFmts = append(b.numFmts, NumFmt{Code: code})
	return len(b.numFmts)
}

// format interns a complete cell appearance and returns its [Cell.StyleID].
func (b *styleBuilder) format(cf CellFormat) int {
	for i, existing := range b.formats {
		if existing == cf {
			return i
		}
	}
	b.formats = append(b.formats, cf)
	return len(b.formats) - 1
}

// sheet assembles the collected styles into a [StyleSheet].
func (b *styleBuilder) sheet() StyleSheet {
	return StyleSheet{
		DefaultFont: b.defaultFont,
		Fonts:       b.fonts,
		Fills:       b.fills,
		Borders:     b.borders,
		NumFmts:     b.numFmts,
		Formats:     b.formats,
	}
}
