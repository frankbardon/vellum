package deck

import (
	"github.com/frankbardon/vellum/provenance"
)

// Deck is a PresentationML presentation.
type Deck struct {
	// Title is carried into the package's core properties.
	Title string

	// SlideSize is the slide's dimensions. Zero selects [Widescreen].
	SlideSize Size

	// NotesSize is the notes page's dimensions. Zero selects [NotesPortrait].
	NotesSize Size

	// Theme is the colour and font scheme every master, layout and slide
	// resolves its scheme references against. See [Author].
	Theme Theme

	// Masters are the slide masters, in order. A deck needs at least one:
	// PresentationML has no notion of an unmastered slide, so a deck without
	// one is a file PowerPoint refuses rather than a deck with default
	// styling.
	Masters []Master

	// Layouts are the slide layouts, in order. Each names the master it
	// belongs to; the master's layout list is derived from this rather than
	// stated twice, so the two cannot disagree.
	Layouts []Layout

	// Slides are the deck's slides, in order.
	Slides []Slide

	// Media are the images the deck embeds, in the order their parts are
	// named. See [Media].
	Media []Media

	// Provenance, when set, is embedded in the package's custom document
	// properties. It is part of the bytes and therefore part of the
	// determinism guarantee, which is why it carries no machine identity.
	Provenance *provenance.Record
}

// Size is a slide or notes page's dimensions, in EMU.
type Size struct {
	Width, Height int64
}

// IsZero reports whether the size is unset.
func (s Size) IsZero() bool { return s.Width == 0 && s.Height == 0 }

// Widescreen is the 16:9 slide, 13.333 by 7.5 inches. The default, because it
// has been PowerPoint's own default since 2013 and a 4:3 deck shown on modern
// hardware is pillarboxed.
var Widescreen = Size{Width: 12192000, Height: 6858000}

// Standard is the 4:3 slide, 10 by 7.5 inches.
var Standard = Size{Width: 9144000, Height: 6858000}

// NotesPortrait is the notes page, 7.5 by 10 inches.
var NotesPortrait = Size{Width: 6858000, Height: 9144000}

// Theme is the appearance scheme the theme part declares.
//
// Everything a master, layout or slide says about colour is a reference into
// this by name — schemeClr val="accent1" — rather than a literal. That is what
// makes a deck restylable at all: replacing the theme part restyles the deck,
// where replacing a literal would mean rewriting every slide.
type Theme struct {
	// Name is the theme's label in PowerPoint's own theme gallery.
	Name string

	// Colors is the twelve-slot colour scheme.
	Colors ColorScheme

	// Major is the heading typeface; Minor is body.
	//
	// Named for the scheme slots rather than for their use, because that is
	// what a slide references: a run asking for the heading face writes
	// latin typeface="+mj-lt".
	Major, Minor Typeface
}

// ColorScheme is DrawingML's fixed twelve-slot palette.
//
// Named fields rather than a keyed collection, because the vocabulary is closed
// and its order in the theme part is fixed by the schema. A map would put an
// iteration on the output path to express something that cannot vary.
//
// Every value is an sRGB hex triplet, uppercase, without a leading hash — the
// form OOXML carries natively.
type ColorScheme struct {
	// Dark1 and Light1 are the primary text and background pair; Dark2 and
	// Light2 the secondary pair.
	//
	// The names are the scheme's, and they are the reason a master carries a
	// colour map: dk1 is "the dark one", not "the text one", and which of the
	// pair is text depends on whether the deck is light or dark. The map is
	// what states that, once, in the master.
	Dark1, Light1 string
	Dark2, Light2 string

	Accent1, Accent2, Accent3 string
	Accent4, Accent5, Accent6 string

	Hyperlink, FollowedHyperlink string
}

// Typeface is one scheme font slot.
type Typeface struct {
	// Latin is the family name.
	Latin string
}

// Master is a slide master: the formatting every layout below it inherits.
type Master struct {
	// ID identifies the master to a [Layout]. Not written to the package.
	ID string

	// Background is a literal sRGB hex fill, or empty for the scheme's own
	// background. Empty is almost always right: a literal here is a colour the
	// theme cannot restyle.
	Background string

	// Shapes are the master's own shapes, which are its placeholders and any
	// standing furniture.
	Shapes []Shape

	// TextStyles are the three list styles a master declares: one for titles,
	// one for body placeholders and one for everything else.
	//
	// This is where a deck's typography lives. A run that names no size
	// resolves to the level style here, which is what makes changing the
	// master change the deck.
	TextStyles TextStyles
}

// Layout is a slide layout.
type Layout struct {
	// ID identifies the layout to a [Slide]. Not written to the package.
	ID string

	// MasterID names the master this layout belongs to.
	MasterID string

	// Name is the label PowerPoint shows in its layout gallery.
	Name string

	// Type is the layout's declared kind. See [LayoutType].
	Type LayoutType

	// Shapes are the layout's placeholders and furniture.
	Shapes []Shape
}

// LayoutType is the value of a layout's type attribute.
//
// PowerPoint uses it to decide which layout to offer for a given action — "new
// slide" reaches for obj, "duplicate title" for title. The full vocabulary is
// large; these are the ones [Author] produces, and a consumer building layouts
// directly may use any string the schema allows.
type LayoutType string

const (
	// LayoutTitle is the title slide: a centred title and a subtitle.
	LayoutTitle LayoutType = "title"

	// LayoutObject is a title with one content placeholder. The workhorse.
	LayoutObject LayoutType = "obj"

	// LayoutTitleOnly is a title and nothing else.
	LayoutTitleOnly LayoutType = "titleOnly"

	// LayoutBlank has no placeholders at all, for a slide that positions
	// everything itself.
	LayoutBlank LayoutType = "blank"
)

// Slide is one slide.
type Slide struct {
	// LayoutID names the layout this slide inherits from. Required: a slide
	// with no layout is not a slide with default formatting, it is a package
	// with a dangling relationship.
	LayoutID string

	// Shapes are the slide's content, in order.
	Shapes []Shape

	// Notes is the speaker note. Empty writes no notes slide at all, which is
	// what an authored deck looks like.
	Notes string
}

// Shape is one item on a slide, layout or master.
//
// A tagged struct rather than an interface, for the reason the specification's
// blocks are: the model must be inspectable by a consumer and walkable by a
// writer without a type switch over an open set.
type Shape struct {
	// Name is the shape's name in the selection pane. Empty takes a name
	// derived from the shape's kind and position, because PowerPoint shows the
	// name and an empty one is unhelpful to whoever opens the deck.
	Name string

	// Frame is the position and size. The zero value inherits the frame from
	// the placeholder this shape fills, which is the whole point of a
	// placeholder — a slide restating its layout's geometry is a slide that
	// stops moving when the layout does.
	Frame Frame

	// Placeholder ties the shape to a layout or master placeholder. Nil is a
	// free-standing shape, which must carry its own frame.
	Placeholder *Placeholder

	// Exactly one of the following is set.

	// Text is a text box or a text-bearing placeholder.
	Text *TextBody

	// Picture is an embedded image.
	Picture *Picture
}

// Frame is a shape's position and extent, in EMU.
type Frame struct {
	X, Y          int64
	Width, Height int64
}

// IsZero reports whether the frame is unset, which means inherit.
func (f Frame) IsZero() bool {
	return f.X == 0 && f.Y == 0 && f.Width == 0 && f.Height == 0
}

// Placeholder ties a shape to the one it inherits from.
type Placeholder struct {
	// Type is the placeholder's kind.
	Type PlaceholderType

	// Index matches a shape to its counterpart in the layout above it.
	//
	// The rule is not intuitive and the failure is silent: a title matches by
	// type alone and carries no index, while every other placeholder matches
	// by index. Two body placeholders sharing an index inherit from the same
	// layout shape and land on top of each other.
	Index int
}

// PlaceholderType is a placeholder's kind.
type PlaceholderType string

const (
	// PlaceholderTitle is the slide title.
	PlaceholderTitle PlaceholderType = "title"

	// PlaceholderCenterTitle is the title on a title slide, which is a
	// different type rather than a differently positioned title.
	PlaceholderCenterTitle PlaceholderType = "ctrTitle"

	// PlaceholderSubTitle is the title slide's subtitle.
	PlaceholderSubTitle PlaceholderType = "subTitle"

	// PlaceholderBody is running content.
	PlaceholderBody PlaceholderType = "body"

	// PlaceholderContent accepts text, a table or a picture. The type "new
	// slide" fills.
	PlaceholderContent PlaceholderType = ""

	// PlaceholderDate, PlaceholderFooter and PlaceholderSlideNumber are the
	// three furniture slots a master declares along its foot.
	PlaceholderDate        PlaceholderType = "dt"
	PlaceholderFooter      PlaceholderType = "ftr"
	PlaceholderSlideNumber PlaceholderType = "sldNum"
)

// TextBody is a shape's text.
type TextBody struct {
	// Anchor is the vertical alignment within the shape. The zero value is
	// top, which is what a text box does without being told.
	Anchor Anchor

	// Wrap reports whether text wraps at the shape's width. The zero value
	// wraps; the field is named so, because a shape that does not wrap grows
	// off the slide and that should be the deliberate reading.
	NoWrap bool

	// Paragraphs are the body's paragraphs, in order.
	Paragraphs []Paragraph
}

// Anchor is vertical alignment inside a text body.
type Anchor string

const (
	// AnchorTop is the zero value.
	AnchorTop Anchor = ""

	AnchorCenter Anchor = "ctr"
	AnchorBottom Anchor = "b"
)

// Paragraph is one paragraph of a text body.
type Paragraph struct {
	// Level is the outline depth, zero being the outermost. It selects the
	// level style from the master's list style, which is where the size,
	// indent and bullet come from.
	Level int

	// Align overrides the level style's alignment. The zero value inherits.
	Align Align

	// Bullet overrides the level style's bullet. The zero value inherits,
	// which is what a paragraph in a body placeholder wants.
	Bullet Bullet

	// SpaceBefore is a spacing override in EMU. Zero inherits.
	SpaceBefore int64

	// Runs are the paragraph's runs, in order. An empty paragraph is a blank
	// line, which is legal and occasionally meant.
	Runs []Run

	// EndStyle is the formatting of the paragraph mark itself, which is what
	// PowerPoint uses to size an empty paragraph. Zero inherits.
	EndStyle RunStyle
}

// Align is horizontal alignment.
type Align string

const (
	// AlignInherit is the zero value: take the level style's alignment.
	AlignInherit Align = ""

	AlignLeft    Align = "l"
	AlignCenter  Align = "ctr"
	AlignRight   Align = "r"
	AlignJustify Align = "just"
)

// Run is a span of text in one style.
type Run struct {
	// Text is the run's characters.
	Text string

	// Style is the character formatting. Every zero field inherits.
	Style RunStyle
}

// RunStyle is character formatting.
//
// Every field's zero value means inherit, and there is no way to say "inherit"
// other than by leaving it zero. That is deliberate: a model that could state
// both "inherit" and "the same value the layout gives" would let a lowering
// produce a deck that looks correct and cannot be restyled.
type RunStyle struct {
	// Font names the family. Empty inherits. The two scheme references
	// [FontMajor] and [FontMinor] are the usual values — a literal family name
	// here is a family the theme cannot change.
	Font string

	// SizeEMU is the type size. Zero inherits.
	SizeEMU int64

	// Bold and Italic are three-valued: inherit, on, or off.
	//
	// A plain bool cannot express the case that matters. A title style that is
	// bold and a run inside it that is not needs the run to say so — b="0" —
	// and a two-valued field can only say "bold" or "say nothing", which for
	// that run means staying bold. The zero value is inherit, so the honest
	// answer is also the default one.
	Bold, Italic Toggle

	// Color is a literal sRGB hex triplet, or one of the scheme references
	// below. Empty inherits.
	Color string
}

// IsZero reports whether the style states nothing, and therefore whether the
// writer emits any run properties at all.
func (s RunStyle) IsZero() bool {
	return s.Font == "" && s.SizeEMU == 0 && s.Color == "" &&
		s.Bold == ToggleInherit && s.Italic == ToggleInherit
}

// Toggle is a three-valued character switch.
type Toggle uint8

const (
	// ToggleInherit takes the value from the style above. The zero value.
	ToggleInherit Toggle = iota

	// ToggleOn turns the switch on.
	ToggleOn

	// ToggleOff turns it off, against a style above that has it on.
	ToggleOff
)

// Set returns the toggle stating b, or inherit when b already matches what is
// inherited.
//
// The one place the overrides-only rule is arithmetic rather than judgement, so
// it lives here rather than being written out at each call site.
func Set(want, inherited bool) Toggle {
	switch {
	case want == inherited:
		return ToggleInherit
	case want:
		return ToggleOn
	default:
		return ToggleOff
	}
}

// attr renders the toggle as an OOXML boolean attribute value, and reports
// whether it should be written at all.
func (t Toggle) attr() (string, bool) {
	switch t {
	case ToggleOn:
		return "1", true
	case ToggleOff:
		return "0", true
	}
	return "", false
}

// Scheme font references. A run naming one of these follows the theme.
const (
	// FontMajor is the theme's heading face.
	FontMajor = "+mj-lt"

	// FontMinor is the theme's body face.
	FontMinor = "+mn-lt"
)

// SchemeColor returns the reference for a theme colour slot, for use in
// [RunStyle.Color] and the fill fields.
//
// A colour written this way follows the theme; a literal does not. The
// distinction is invisible in the rendered deck and total when the theme
// changes, which is why the model carries the reference rather than resolving
// it.
func SchemeColor(slot string) string { return schemePrefix + slot }

// schemePrefix marks a colour value as a scheme reference rather than a literal.
//
// A hex triplet cannot begin with a plus, so the two are distinguishable
// without a second field saying which kind this is — and a second field is
// exactly where a value that is neither would come from.
const schemePrefix = "+"

// Scheme colour slots, as [SchemeColor] takes them. These are the mapped names
// a shape uses, not the theme's own dk1/lt1: the master's colour map is what
// turns bg1 into lt1, and a shape referring to lt1 directly would bypass it.
const (
	SchemeBackground1 = "bg1"
	SchemeText1       = "tx1"
	SchemeBackground2 = "bg2"
	SchemeText2       = "tx2"
	SchemeAccent1     = "accent1"
	SchemeAccent2     = "accent2"
	SchemeAccent3     = "accent3"
	SchemeAccent4     = "accent4"
	SchemeAccent5     = "accent5"
	SchemeAccent6     = "accent6"
	SchemeHyperlink   = "hlink"
	SchemeFollowed    = "folHlink"
)

// Bullet is a paragraph's bullet.
type Bullet struct {
	// Kind selects the bullet's form.
	Kind BulletKind

	// Char is the bullet glyph for [BulletChar].
	Char string

	// Font is the family the glyph is drawn from. A bullet character from a
	// family that does not have it renders as a missing glyph, so the two
	// travel together.
	Font string

	// Format is the numbering scheme for [BulletNumber], as DrawingML names
	// them: arabicPeriod, alphaLcParenR, romanUcPeriod and so on.
	Format string
}

// BulletKind is a bullet's form.
type BulletKind string

const (
	// BulletInherit is the zero value: take the level style's bullet.
	BulletInherit BulletKind = ""

	// BulletNone suppresses the bullet a level style would give.
	BulletNone BulletKind = "none"

	// BulletChar is a literal glyph.
	BulletChar BulletKind = "char"

	// BulletNumber is an automatic number.
	BulletNumber BulletKind = "number"
)

// TextStyles are the three list styles a master declares.
type TextStyles struct {
	// Title styles title placeholders.
	Title ListStyle

	// Body styles body placeholders, which is where the outline levels matter.
	Body ListStyle

	// Other styles everything else: a free-standing text box takes its
	// defaults from here.
	Other ListStyle
}

// ListStyle is up to nine levels of paragraph formatting.
//
// Nine because DrawingML declares exactly nine and PowerPoint's outline
// demoting stops there. A style with fewer levels is legal and means the deeper
// ones fall to the reader's defaults rather than to anything the theme said,
// which is why [Author] fills all nine by clamping to the last size the design
// declared.
type ListStyle struct {
	Levels []LevelStyle
}

// listStyleLevels is the number of outline levels DrawingML declares.
const listStyleLevels = 9

// LevelStyle is one outline level's formatting.
type LevelStyle struct {
	// MarginLeft is the level's left inset, in EMU.
	MarginLeft int64

	// Indent is the first-line indent relative to MarginLeft, in EMU. A
	// bulleted level wants it negative, which is what hangs the text off the
	// bullet rather than under it.
	Indent int64

	// Align is the level's alignment.
	Align Align

	// SizeEMU is the type size.
	SizeEMU int64

	// LineHeight is the line spacing as a multiple. Zero writes none.
	LineHeight float64

	// SpaceBefore is the space above a paragraph at this level, in EMU.
	SpaceBefore int64

	// Font names the family, usually a scheme reference.
	Font string

	// Color is a literal hex triplet or a scheme reference.
	Color string

	// Bold and Italic are the level's weight and slope. A level style states
	// them absolutely rather than as a difference: it is the base a run
	// inherits from, so there is nothing above it to differ from.
	Bold, Italic bool

	// Bullet is the level's bullet.
	Bullet Bullet
}

// Picture is an embedded image.
type Picture struct {
	// MediaIndex is the index into [Deck.Media].
	MediaIndex int

	// AltText is the accessible description. Carried into the shape's
	// descr attribute, which is where a screen reader looks.
	AltText string

	// SVGMediaIndex, when positive, names an SVG carried alongside the raster
	// as the preferred rendition, in the extension PowerPoint 2016 reads. The
	// raster remains the fallback for every other reader.
	SVGMediaIndex int
}

// Media is one embedded image.
type Media struct {
	// Hash is the content hash. Part names are derived from the sorted set of
	// distinct hashes, so a deck's media parts are a function of what is in
	// them rather than of the order they were mentioned.
	Hash string

	// MediaType is the IANA type.
	MediaType string

	// Bytes is the content.
	Bytes []byte
}

// LayoutsFor returns the layouts belonging to a master, in deck order.
//
// Derived rather than stored on the master, so the two cannot disagree. A
// layout naming a master that does not exist is not silently dropped — see
// [Deck.Validate].
func (d *Deck) LayoutsFor(masterID string) []Layout {
	var out []Layout
	for _, l := range d.Layouts {
		if l.MasterID == masterID {
			out = append(out, l)
		}
	}
	return out
}
