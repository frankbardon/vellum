// Package doc is the WordprocessingML model and its writer.
//
// The model is public, and that is a decision rather than an accident. A
// consumer composing from the block model gets a document assembled by the
// lowering; a consumer needing format-specific reach the block vocabulary does
// not express — a list, a header, a table of contents field — builds a
// [Document] directly and writes it. Both paths converge on one writer, so
// there is no second serialiser able to drift.
//
// # The boundary between the two
//
// Some of this model is unreachable from the block vocabulary. Lists, headers,
// footers, hyperlinks, bookmarks and TOC fields have no block kind, because the
// block vocabulary is deliberately small and generic and adding one product's
// document furniture to it would make the next consumer fight it.
//
// That boundary is recorded rather than implied: [AdvancedOnly] names every
// feature reachable only by driving this model directly, and a test asserts
// the lowering does not produce any of them. Without it the two APIs drift —
// the lowering quietly grows a capability, and the block model acquires a
// feature nobody declared in the capability matrix.
package doc

import (
	"github.com/frankbardon/vellum/provenance"
)

// Document is a WordprocessingML document.
type Document struct {
	// Title is carried into the package's core properties.
	Title string

	// Sections are the document's page-geometry divisions, in order. A
	// document always has at least one; the last one's properties become the
	// body-level sectPr, which is where Word expects them.
	Sections []Section

	// Styles is the styles part. A document whose paragraphs name a style that
	// nothing defines opens with those paragraphs unstyled, so this is
	// assembled by the lowering rather than left to the caller.
	Styles StyleSheet

	// Numbering defines the list numbering. Empty when the document has no
	// lists, in which case the part is not written at all — Word tolerates a
	// numbering part with no definitions, but an absent one is what an authored
	// document looks like.
	Numbering Numbering

	// Media are the images the document embeds, in the order their parts are
	// named. See [Media].
	Media []Media

	// Footnotes are the document's footnotes. Referenced from a run by index.
	Footnotes []Footnote

	// Headers and Footers are the running heads and feet a section may
	// reference by ID.
	Headers []HeaderFooter
	Footers []HeaderFooter

	// Provenance, when set, is embedded in the package's custom document
	// properties. It is part of the bytes and therefore part of the
	// determinism guarantee, which is why it carries no machine identity.
	Provenance *provenance.Record
}

// Section is one page-geometry division.
type Section struct {
	// Page is the geometry.
	Page PageSetup

	// Type is how the section begins. The zero value is a next-page break,
	// which is what Word writes for an unqualified section.
	Type SectionType

	// HeaderID and FooterID reference a [HeaderFooter] by its ID, or are empty
	// for none.
	HeaderID, FooterID string

	// TitlePage suppresses the header and footer on the section's first page.
	TitlePage bool

	// Content is the section's block-level content, in order.
	Content []Content
}

// SectionType is how a section break behaves.
type SectionType string

const (
	// SectionNextPage starts the section on a new page. The zero value.
	SectionNextPage SectionType = ""

	// SectionContinuous starts the section without a page break, which is what
	// a change of geometry mid-page needs.
	SectionContinuous SectionType = "continuous"

	// SectionEvenPage and SectionOddPage start on the next even or odd page.
	SectionEvenPage SectionType = "evenPage"
	SectionOddPage  SectionType = "oddPage"
)

// Content is one block-level item: a paragraph or a table.
//
// A tagged struct rather than an interface, for the reason the specification's
// blocks are: the model must be inspectable by a consumer and walkable by a
// writer without a type switch over an open set.
type Content struct {
	// Paragraph is set when this item is a paragraph.
	Paragraph *Paragraph

	// Table is set when this item is a table.
	Table *Table
}

// Paragraph is one block-level unit of prose.
type Paragraph struct {
	// StyleID names a style in the styles part. Empty means the default
	// paragraph style, which Word applies without needing it named.
	//
	// House rule: where a style exists, a paragraph names it rather than
	// carrying direct formatting. A document whose every paragraph carries its
	// own spacing is a document nobody can restyle.
	StyleID string

	// OutlineLevel is the heading depth, 1 being the most prominent. Zero is
	// body text. Set by the heading styles; a caller overriding it here is
	// declaring an outline position the style does not imply.
	OutlineLevel int

	// NumberingID and NumberingLevel place the paragraph in a list. Zero
	// NumberingID means no list.
	//
	// Advanced-only: no block kind produces a list. See [AdvancedOnly].
	NumberingID    int
	NumberingLevel int

	// SpaceBefore and SpaceAfter are direct spacing overrides in EMU, applied
	// only when non-zero. Normally the style carries them.
	SpaceBefore, SpaceAfter int64

	// KeepNext binds the paragraph to the one after it, so a heading does not
	// strand at the foot of a page.
	KeepNext bool

	// PageBreakBefore forces the paragraph to start a page.
	PageBreakBefore bool

	// Alignment is the horizontal alignment. Empty means the style's.
	Alignment Alignment

	// Runs are the paragraph's inline content, in order.
	Runs []Run
}

// Alignment is horizontal paragraph alignment.
type Alignment string

const (
	// AlignInherit takes the style's alignment. The zero value.
	AlignInherit Alignment = ""
	AlignLeft    Alignment = "left"
	AlignCenter  Alignment = "center"
	AlignRight   Alignment = "right"
	AlignJustify Alignment = "both"
)

// Run is a span of inline content sharing one set of properties.
//
// Exactly one of the content arms is set. A run with none is an empty run,
// which is legal and is what a bookmark or a field boundary sits in.
type Run struct {
	// StyleID names a character style, or is empty for none.
	StyleID string

	// Text is the run's text content.
	Text string

	// Properties is direct character formatting, applied over the style.
	//
	// Direct formatting here is not the placeholder it once was: a consumer's
	// mark is by definition something the theme styles individually, and
	// minting a character style per mark combination would fill the styles part
	// with names nobody chose. Marks resolve to properties; structure resolves
	// to styles.
	Properties RunProperties

	// Break, when set, makes this run a break rather than text.
	Break BreakType

	// Drawing, when set, makes this run an embedded image.
	Drawing *Drawing

	// FootnoteRef, when non-zero, makes this run a footnote reference. The
	// value is one-based into [Document.Footnotes].
	FootnoteRef int

	// Field, when set, makes this run part of a field — a page number, a TOC.
	//
	// Advanced-only for TOC and page numbers. See [AdvancedOnly].
	Field *Field

	// Tab, when true, emits a tab character.
	Tab bool
}

// RunProperties is direct character formatting.
//
// Every field's zero value means "inherit", which is what makes a
// zero-value RunProperties emit no rPr at all — the shape an authored document
// has for a run that simply takes its style.
type RunProperties struct {
	// Font is the family name. Empty inherits.
	Font string

	// SizeEMU is the type size. Zero inherits.
	SizeEMU int64

	// Color is an uppercase sRGB hex triplet with no leading hash. Empty
	// inherits.
	Color string

	// Highlight is a background fill, same form as Color. Empty is none.
	Highlight string

	// Bold, Italic and Underline are the character switches.
	Bold, Italic, Underline bool

	// Superscript and Subscript raise or lower the run. Both set is a caller
	// error and the writer prefers superscript, which is the one an annotation
	// asks for.
	Superscript, Subscript bool
}

// IsZero reports whether the properties carry nothing, so the writer can omit
// the rPr element entirely.
func (p RunProperties) IsZero() bool { return p == RunProperties{} }

// BreakType is a run-level break.
type BreakType string

const (
	// BreakNone is not a break. The zero value.
	BreakNone BreakType = ""

	// BreakLine is a soft line break within a paragraph.
	BreakLine BreakType = "textWrapping"

	// BreakPage is a hard page break.
	BreakPage BreakType = "page"

	// BreakColumn is a column break.
	BreakColumn BreakType = "column"
)

// Drawing is an embedded image.
type Drawing struct {
	// MediaIndex points into [Document.Media].
	MediaIndex int

	// FallbackIndex points into [Document.Media] at a raster to display when
	// the primary is a vector the reader cannot draw, or is -1.
	//
	// This is how an SVG reaches Word: the vector goes in an extension on the
	// blip and the raster goes in the blip itself, so a reader that understands
	// SVG uses it and one that does not still shows something. Vellum does not
	// rasterise — it never renders — so the caller supplies the pair.
	FallbackIndex int

	// WidthEMU and HeightEMU are the placed size. Both concrete: the aspect
	// ratio was applied during resolution, because a writer has no business
	// asking an asset how tall it is.
	WidthEMU, HeightEMU int64

	// Name and AltText are the drawing's identity and its accessible
	// description.
	Name, AltText string
}

// Field is a Word field: a page number, a TOC, a cross-reference.
type Field struct {
	// Instruction is the field code, verbatim — "PAGE", "TOC \\o \"1-3\" \\h".
	Instruction string

	// Result is the cached result Word shows before it recalculates. A TOC's
	// cached result is deliberately a prompt rather than a computed table; see
	// [Field.Dirty].
	Result string

	// Dirty marks the field as needing recalculation when the document opens.
	//
	// A TOC is always dirty. Vellum does not paginate WordprocessingML — Word
	// does — so a page-number table Vellum computed would be a table of numbers
	// that disagree with the document they index. Asking Word to build it is
	// the only answer that is right.
	Dirty bool
}

// Footnote is one footnote.
type Footnote struct {
	// Content is the footnote's block content.
	Content []Content
}

// HeaderFooter is a running head or foot.
type HeaderFooter struct {
	// ID identifies it to a section. Assigned by the caller; the writer derives
	// part names from position, not from this.
	ID string

	// Content is the block content.
	Content []Content
}

// Media is one embedded binary part.
type Media struct {
	// Hash is the content hash. Part names are derived from the sorted set of
	// distinct hashes, so a document's media parts are a function of what is in
	// them rather than of the order they were mentioned.
	Hash string

	// MediaType is the IANA type.
	MediaType string

	// Bytes is the content.
	Bytes []byte
}

// PageSetup is a section's page geometry, in EMU.
//
// EMU rather than twips, though WordprocessingML writes twips: the conversion
// belongs at the boundary, and a model in the wire unit would make every
// consumer do the arithmetic Vellum already does.
type PageSetup struct {
	Width, Height                                    int64
	MarginTop, MarginRight, MarginBottom, MarginLeft int64
	MarginHeader, MarginFooter                       int64
	Landscape                                        bool
}

// A4Portrait is A4 with one-inch margins, for a caller building a Document by
// hand who wants a sane starting geometry.
func A4Portrait() PageSetup {
	const (
		mm    = emuPerInch / 25.4
		inch  = emuPerInch
		halfI = emuPerInch / 2
	)
	return PageSetup{
		Width:        int64(210 * mm),
		Height:       int64(297 * mm),
		MarginTop:    inch,
		MarginRight:  inch,
		MarginBottom: inch,
		MarginLeft:   inch,
		MarginHeader: halfI,
		MarginFooter: halfI,
	}
}

// ContentWidth is the page width less its horizontal margins — the column a
// table or an image is fitted to.
func (p PageSetup) ContentWidth() int64 { return p.Width - p.MarginLeft - p.MarginRight }

// AdvancedOnly names every feature of this model that the block vocabulary
// cannot reach.
//
// It exists so the boundary between the two public APIs is a declaration rather
// than an emergent property of whatever the lowering happens to do.
// TestLower_ProducesNothingAdvancedOnly walks a lowered document and fails if
// any of these appear — which is what stops the block model from quietly
// acquiring a capability the capability matrix never declared.
//
// A consumer needing one of these builds a [Document] directly. That is the
// point of exporting the model.
var AdvancedOnly = []string{
	"list numbering",
	"headers and footers",
	"fields, including TOC and page numbers",
	"hyperlinks",
	"bookmarks",
	"character styles",
	"tab stops",
	"column and line breaks",
	"title-page header suppression",
	"continuous, even-page and odd-page section breaks",
}

// blockContent is a convenience for the common case of one paragraph.
func para(p Paragraph) Content { return Content{Paragraph: &p} }
