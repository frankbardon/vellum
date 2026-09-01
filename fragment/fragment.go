// Package fragment is the resolved, format-neutral intermediate.
//
// Everything a specification left open is closed here. Fonts are concrete
// families with a decided embedding plan, colours are literal values rather
// than roles, every length is EMU, every value carries its format code and its
// rendered text, and every asset carries bytes, a media type and a content
// hash.
//
// # Why this layer exists at all
//
// Theme application, font selection, number formatting, asset resolution and
// mark resolution are the same work for all four output formats. Done once here
// they are shared; done in each writer they are four chances to disagree, and
// the disagreement would show as the same specification rendering differently
// in a document and in the deck built beside it.
//
// It earns its place a second way: it has two genuinely different lowerings.
// A whole [Doc] becomes a format model, and a bounded [Sequence] is spliced into
// a template's existing idiom by fill mode. Fill never constructs a document
// model at all, so a layer above this one would serve only half the library.
//
// # What it does not know
//
// Pages, sheets, slides and XML. A [Section] carries the page geometry it was
// resolved against because that is a measurement, but nothing here paginates,
// positions or serialises. Those are the format models' work, and they are
// genuinely different from each other — a flow document, a workbook, a slide
// deck and a page tree forced into one representation produce a model that
// serves none of them.
package fragment

import (
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/theme"
)

// Doc is a whole resolved document.
type Doc struct {
	// Title is the document title, carried into format metadata.
	Title string

	// ThemeID names the theme this was resolved against, for provenance.
	ThemeID string

	// Fonts is the font manifest: every face the document uses, with the
	// embedding decision already made. Runs reference it by index rather than
	// by name, so a face is described once however many runs use it.
	Fonts []Face

	// Assets is every resolved asset, deduplicated by content hash and ordered
	// by it. Ordering by hash rather than by first use is what makes the media
	// part names in an OOXML package a function of content: two documents with
	// the same pictures in a different order produce the same media parts.
	Assets []Asset

	// Sections are the document's divisions, in order.
	Sections []Section
}

// Face is one resolved font, with its embedding decision.
type Face struct {
	// Role is the typographic slot this face fills.
	Role theme.FontRole

	// Family is the family name to write into the document. When Substituted
	// is true this is the substitute, not what the theme asked for.
	Family string

	// Requested is the family the theme named. Equal to Family unless
	// Substituted.
	Requested string

	// Substituted records that the theme declared this face non-embeddable and
	// named a replacement. Every substitution is reported as a warning as well,
	// because a silent one is how the same specification comes to render
	// differently on two machines.
	Substituted bool

	// Embed is the decision: whether to embed the face and how much of it.
	Embed EmbedPlan

	// AssetIndex points into [Doc.Assets] for the font program, or -1 when the
	// face is not embedded.
	AssetIndex int
}

// EmbedPlan is what a writer should do with a face.
type EmbedPlan string

const (
	// EmbedNone references the family by name and embeds nothing. This is what
	// a substituted face gets, and what every OOXML target gets in v1.
	EmbedNone EmbedPlan = "none"

	// EmbedSubset embeds a subset of the font program.
	EmbedSubset EmbedPlan = "subset"

	// EmbedWhole embeds the whole font program.
	EmbedWhole EmbedPlan = "whole"
)

// Asset is a resolved asset.
type Asset struct {
	// Handle is the host's identifier, retained for provenance and for errors.
	Handle string

	// MediaType is the verified IANA type.
	MediaType string

	// Hash is the content hash. It names the asset within the document and
	// participates in naming the artifact.
	Hash string

	// Bytes is the content.
	Bytes []byte

	// WidthPx and HeightPx are the intrinsic dimensions, zero when the asset
	// did not declare them.
	WidthPx  float64
	HeightPx float64
}

// Page is a resolved page geometry, in EMU.
//
// EMU throughout, as int64. A measurement that round-trips through float64
// accumulates error that shows up as a one-EMU disagreement between two runs of
// the same specification, which is a determinism failure wearing a rounding
// bug's clothes.
type Page struct {
	Width, Height                                    int64
	MarginTop, MarginRight, MarginBottom, MarginLeft int64
}

// ContentWidth is the page width less its horizontal margins.
func (p Page) ContentWidth() int64 { return p.Width - p.MarginLeft - p.MarginRight }

// ContentHeight is the page height less its vertical margins.
func (p Page) ContentHeight() int64 { return p.Height - p.MarginTop - p.MarginBottom }

// Section is a resolved division.
type Section struct {
	// ID is the consumer's identifier, uninterpreted.
	ID string

	// LayoutID names the theme layout this was resolved against.
	LayoutID string

	// Page is the geometry that layout declared.
	Page Page

	// Blocks are the section's content, in order.
	Blocks []Block
}

// Block is a resolved block. A tagged union, as in the specification, and for
// the same reasons.
type Block struct {
	// Kind names which arm carries content.
	Kind spec.BlockKind

	// Paragraph is set for a heading or a text block. The two collapse here
	// because once resolved they differ only in outline level and style, and
	// keeping them apart would make every writer branch on a distinction that
	// no longer carries information.
	Paragraph *Paragraph

	// Table is set for a table block.
	Table *Table

	// Asset is set for an asset block.
	Asset *AssetRef

	// Break is set for a page-break block.
	Break *Break

	// Note is set for a notes block.
	Note *Note

	// Space is set for a spacer block.
	Space *Space
}

// Paragraph is resolved prose.
type Paragraph struct {
	// OutlineLevel is the heading depth, 1 being the most prominent. Zero is
	// body text.
	OutlineLevel int

	// Runs are the styled spans, in order.
	Runs []Run

	// SpaceBefore and SpaceAfter are the vertical rhythm, in EMU.
	SpaceBefore, SpaceAfter int64

	// LineHeight is the multiple of the type size a line occupies.
	LineHeight float64
}

// Text returns the paragraph's runs concatenated, for a target that cannot
// carry per-run styling.
func (p *Paragraph) Text() string {
	if p == nil {
		return ""
	}
	var out string
	for i := range p.Runs {
		out += p.Runs[i].Text
	}
	return out
}

// Run is a styled span.
type Run struct {
	// Text is the content.
	Text string

	// Style is the resolved appearance.
	Style TextStyle
}

// TextStyle is fully resolved character appearance.
type TextStyle struct {
	// FaceIndex points into [Doc.Fonts].
	FaceIndex int

	// SizeEMU is the type size.
	SizeEMU int64

	// Color is a literal sRGB hex triplet, uppercase, no leading hash — the
	// form OOXML carries natively. A role has already been looked up; nothing
	// downstream needs the theme again.
	Color string

	// Background is a literal fill, or empty for none.
	Background string

	// Bold, Italic and Underline are the character switches.
	Bold, Italic, Underline bool
}

// AssetRef places a resolved asset at a size.
type AssetRef struct {
	// AssetIndex points into [Doc.Assets].
	AssetIndex int

	// Role names the theme box this filled.
	Role theme.BoxRole

	// WidthEMU and HeightEMU are the placed size. Both are always concrete:
	// a box with an intrinsic height has had the asset's aspect ratio applied
	// by this point, because a writer has no business asking an asset how tall
	// it is.
	WidthEMU, HeightEMU int64

	// AltText is the accessible description.
	AltText string
}

// Break starts a new page, slide or sheet, depending on the target.
type Break struct{}

// Note is resolved annotation content.
type Note struct {
	// Body is the note's prose.
	Body Paragraph
}

// Space is resolved vertical space.
type Space struct {
	// HeightEMU is the space to insert.
	HeightEMU int64
}

// Sequence is a bounded ordered run of blocks.
//
// The second lowering this layer exists for: fill mode evaluates a binding to a
// Sequence and splices it into a template's existing idiom, never constructing
// a document model. A Sequence carries the assets and faces its blocks
// reference, because it is spliced into a package that must then be told about
// them.
type Sequence struct {
	// Blocks are the content, in order.
	Blocks []Block

	// Fonts and Assets are the manifests the blocks reference by index, with
	// the same meaning as on [Doc].
	Fonts  []Face
	Assets []Asset
}
