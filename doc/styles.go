package doc

import (
	"strconv"

	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/theme"
)

// Style IDs. Fixed rather than generated, because a style ID appears in every
// paragraph that names it and a generated one would make two documents built
// from the same theme disagree in their styles part for no reason a reader
// could see.
//
// The names are Word's own where Word has one. A document whose heading style
// is called "Heading1" is a document whose navigation pane, table of contents
// and outline view all work; one whose heading style is called "VellumHeading1"
// is a document where none of them do, because Word keys those features off the
// built-in names.
const (
	StyleNormal        = "Normal"
	StyleCaption       = "Caption"
	StyleFootnoteText  = "FootnoteText"
	StyleFootnoteRef   = "FootnoteReference"
	StyleListParagraph = "ListParagraph"
	StyleTableGrid     = "VellumTable"
	StyleHeader        = "Header"
	StyleFooter        = "Footer"
)

// HeadingStyleID returns the style ID for an outline level.
func HeadingStyleID(level int) string {
	if level < 1 {
		level = 1
	}
	return "Heading" + strconv.Itoa(level)
}

// StyleSheet is the styles part.
type StyleSheet struct {
	// DefaultFont and DefaultSizeEMU are the document defaults, applied by
	// w:docDefaults and inherited by everything that does not override them.
	DefaultFont    string
	DefaultSizeEMU int64

	// DefaultLineHeight is the document's line spacing multiple.
	DefaultLineHeight float64

	// Paragraph and Character are the defined styles, in emission order.
	//
	// Ordered slices rather than maps: the styles part's bytes are output, and
	// a map ranged here would put a different order into every build.
	Paragraph []ParagraphStyle
	Character []CharacterStyle

	// Table is the single table style. One rather than a set, because a table
	// style's whole job here is to carry the theme's borders and header band,
	// and a document offering six of them would be offering a choice the block
	// model cannot express.
	Table TableStyle
}

// ParagraphStyle is one defined paragraph style.
type ParagraphStyle struct {
	// ID is the style ID paragraphs reference.
	ID string

	// Name is the display name Word shows in its gallery.
	Name string

	// BasedOn is the style this inherits from, or empty.
	BasedOn string

	// NextStyleID is the style Word applies to the paragraph typed after one in
	// this style. A heading whose next style is itself makes a document where
	// pressing Enter after a title gives another title, which is not what
	// anybody wants.
	NextStyleID string

	// OutlineLevel is the outline depth this style implies, 1-based. Zero is
	// body text.
	OutlineLevel int

	// SpaceBefore and SpaceAfter are the style's spacing, in EMU.
	SpaceBefore, SpaceAfter int64

	// LineHeight is the line spacing multiple. Zero inherits.
	LineHeight float64

	// KeepNext binds a paragraph in this style to the one after it.
	KeepNext bool

	// Run is the character formatting the style carries.
	Run RunProperties

	// Primary marks a style as one Word shows in its gallery by default.
	Primary bool
}

// CharacterStyle is one defined character style.
type CharacterStyle struct {
	ID   string
	Name string
	Run  RunProperties
}

// TableStyle is the document's table style.
type TableStyle struct {
	ID   string
	Name string

	// BorderColor is the hairline colour, an uppercase hex triplet.
	BorderColor string

	// HeaderFill and HeaderColor are the header band's fill and text colour.
	HeaderFill, HeaderColor string

	// StripeFill is the alternating body-row fill, or empty for none.
	StripeFill string

	// Run is the character formatting inside the table.
	Run RunProperties
}

// buildStyleSheet assembles the styles part from a resolved document.
//
// Every style's appearance comes from the theme, so a change of theme is a
// change of styles part rather than a change of every paragraph. That is the
// whole reason the lowering emits styles at all instead of direct formatting:
// direct formatting produces a document that looks right and cannot be
// restyled, which is worse than one that looks wrong.
func buildStyleSheet(d *fragment.Doc, th typeScale) StyleSheet {
	body := faceFamily(d, theme.FontBody)
	heading := faceFamily(d, theme.FontHeading)

	sheet := StyleSheet{
		DefaultFont:       body,
		DefaultSizeEMU:    th.BodySize,
		DefaultLineHeight: th.LineHeight,
		Table: TableStyle{
			ID:          StyleTableGrid,
			Name:        "Vellum Table",
			BorderColor: th.RuleColor,
			HeaderFill:  th.TableHeaderFill,
			HeaderColor: th.TableHeaderText,
			StripeFill:  th.TableStripeFill,
			Run:         RunProperties{Font: body, SizeEMU: th.TableBodySize},
		},
	}

	sheet.Paragraph = append(sheet.Paragraph, ParagraphStyle{
		ID:          StyleNormal,
		Name:        "Normal",
		SpaceBefore: th.ParagraphBefore,
		SpaceAfter:  th.ParagraphAfter,
		LineHeight:  th.LineHeight,
		Run:         RunProperties{Font: body, SizeEMU: th.BodySize, Color: th.TextColor},
		Primary:     true,
	})

	for level := 1; level <= len(th.HeadingSizes); level++ {
		sheet.Paragraph = append(sheet.Paragraph, ParagraphStyle{
			ID:           HeadingStyleID(level),
			Name:         "heading " + strconv.Itoa(level),
			BasedOn:      StyleNormal,
			NextStyleID:  StyleNormal,
			OutlineLevel: level,
			SpaceBefore:  th.HeadingBefore,
			SpaceAfter:   th.HeadingAfter,
			LineHeight:   th.LineHeight,
			// A heading at the foot of a page with its first paragraph
			// overleaf is the commonest ugly artefact in a generated document,
			// and it costs one attribute to prevent.
			KeepNext: true,
			Run: RunProperties{
				Font:    heading,
				SizeEMU: th.HeadingSizes[level-1],
				Color:   th.HeadingColor,
				Bold:    true,
			},
			Primary: true,
		})
	}

	sheet.Paragraph = append(sheet.Paragraph,
		ParagraphStyle{
			ID:         StyleCaption,
			Name:       "caption",
			BasedOn:    StyleNormal,
			SpaceAfter: th.ParagraphAfter,
			LineHeight: th.LineHeight,
			Run:        RunProperties{Font: body, SizeEMU: th.CaptionSize, Color: th.MutedColor, Italic: true},
			Primary:    true,
		},
		ParagraphStyle{
			ID:         StyleFootnoteText,
			Name:       "footnote text",
			BasedOn:    StyleNormal,
			LineHeight: 1,
			Run:        RunProperties{Font: body, SizeEMU: th.NotesSize, Color: th.MutedColor},
		},
		ParagraphStyle{
			ID:      StyleListParagraph,
			Name:    "List Paragraph",
			BasedOn: StyleNormal,
			Run:     RunProperties{Font: body, SizeEMU: th.BodySize, Color: th.TextColor},
		},
		ParagraphStyle{
			ID:      StyleHeader,
			Name:    "header",
			BasedOn: StyleNormal,
			Run:     RunProperties{Font: body, SizeEMU: th.NotesSize, Color: th.MutedColor},
		},
		ParagraphStyle{
			ID:      StyleFooter,
			Name:    "footer",
			BasedOn: StyleNormal,
			Run:     RunProperties{Font: body, SizeEMU: th.NotesSize, Color: th.MutedColor},
		},
	)

	sheet.Character = append(sheet.Character, CharacterStyle{
		ID:   StyleFootnoteRef,
		Name: "footnote reference",
		Run:  RunProperties{Superscript: true},
	})

	return sheet
}

// faceFamily returns the resolved family for a role.
//
// The face has already had the font policy applied, so this is the family that
// will actually appear — the substitute where one was declared, not what the
// theme originally asked for.
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

// Numbering is the list numbering part.
type Numbering struct {
	// Abstract are the abstract numbering definitions, in order. IDs are the
	// slice index, so they are derived from definition order rather than from a
	// counter that could depend on a code path.
	Abstract []AbstractNumbering

	// Instances map a concrete numbering ID to an abstract definition. A
	// document with two lists that restart independently needs two instances of
	// one abstract definition, which is why the indirection exists.
	Instances []NumberingInstance
}

// AbstractNumbering is one list shape.
type AbstractNumbering struct {
	// Levels are the nine levels WordprocessingML expects. Fewer is legal and
	// is what Vellum writes; a level a paragraph references and the definition
	// lacks is a paragraph Word renders unnumbered.
	Levels []NumberingLevel
}

// NumberingLevel is one indent level of a list.
type NumberingLevel struct {
	// Format is the numbering format: "bullet", "decimal", "lowerLetter".
	Format string

	// Text is the level text — "%1." for a numbered level, a glyph for a
	// bullet.
	Text string

	// IndentEMU and HangingEMU are the level's indentation.
	IndentEMU, HangingEMU int64

	// Font overrides the family for the number itself, which a bullet glyph
	// needs.
	Font string
}

// NumberingInstance binds a numbering ID to an abstract definition.
type NumberingInstance struct {
	// ID is the value a paragraph's NumberingID carries. One-based, because
	// zero means "no list".
	ID int

	// AbstractIndex points into [Numbering.Abstract].
	AbstractIndex int
}

// IsEmpty reports whether the numbering part should be written at all.
func (n Numbering) IsEmpty() bool { return len(n.Abstract) == 0 }

// typeScale is the subset of the theme the styles need, flattened to EMU so
// the style builder does not carry the theme's own types around.
type typeScale struct {
	BodySize      int64
	HeadingSizes  []int64
	CaptionSize   int64
	NotesSize     int64
	TableBodySize int64

	ParagraphBefore, ParagraphAfter int64
	HeadingBefore, HeadingAfter     int64
	LineHeight                      float64

	TextColor       string
	MutedColor      string
	HeadingColor    string
	RuleColor       string
	TableHeaderFill string
	TableHeaderText string
	TableStripeFill string
}
