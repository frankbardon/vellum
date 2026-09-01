package deck

import (
	verr "github.com/frankbardon/vellum/errors"
)

// Design is the resolved appearance a deck is authored from.
//
// It is the writer's input contract rather than a theme document: every
// measurement is already in EMU and every colour already a hex triplet, because
// resolving a theme is the resolve pass's job and doing it twice is how two
// writers come to disagree about what a heading looks like.
type Design struct {
	// Name is the theme's label in PowerPoint's theme gallery.
	Name string

	// SlideSize is the slide's dimensions. Zero selects [Widescreen].
	SlideSize Size

	// MarginTop, MarginRight, MarginBottom and MarginLeft are the insets that
	// bound where placeholders are laid out.
	MarginTop, MarginRight, MarginBottom, MarginLeft int64

	// Colors is the twelve-slot scheme, already mapped.
	//
	// The mapping from Vellum's colour roles onto DrawingML's slots is the
	// caller's, and it is not mechanical: DrawingML's vocabulary is dk1/lt1
	// and dk2/lt2 rather than text/background and heading/muted, and which of
	// a pair is text depends on whether the deck is light or dark. Doing it
	// here would be this package holding an opinion about a theme it has
	// already been handed the answer for.
	Colors ColorScheme

	// HeadingFamily and BodyFamily are the two scheme faces.
	HeadingFamily, BodyFamily string

	// TitleSize is the slide title's type size, in EMU.
	TitleSize int64

	// SubtitleSize is the title slide's subtitle size, in EMU. Zero derives it
	// from the outermost body size.
	SubtitleSize int64

	// TitleBold is whether slide titles are bold.
	//
	// A field rather than a constant, because it is the base a title run
	// inherits from and the lowering compares against it. Getting it wrong
	// makes every title carry an explicit weight, which is exactly the
	// override that stops a deck following its master.
	TitleBold bool

	// BodySizes are the outline level sizes, outermost first. At least one is
	// required: a body placeholder with no size renders invisible.
	BodySizes []int64

	// LineHeight is the line spacing as a multiple. Zero writes none, which
	// takes the reader's own single spacing.
	LineHeight float64

	// ParagraphSpace is the space above a body paragraph, in EMU.
	ParagraphSpace int64

	// TitleGap is the space between the title band and the body, in EMU.
	TitleGap int64
}

// Layout identifiers [Author] produces. A slide names one of these.
const (
	LayoutIDTitle     = "title"
	LayoutIDContent   = "content"
	LayoutIDTitleOnly = "title-only"
	LayoutIDBlank     = "blank"
)

// MasterID is the identifier of the master [Author] produces.
const MasterID = "master"

// Author builds the theme, master and layouts a deck needs from a design.
//
// This is the resolution of the question a .pptx forces and a .docx does not.
// A slide inherits from a layout, a layout from a master, and the master from a
// theme part naming its colours and fonts; a deck with none of those is not a
// deck with default styling but a file PowerPoint refuses. The alternatives
// were to ship a fixed .pptx and copy its parts through — which makes every
// consumer's deck look like Vellum's — or to author them. Authoring them is
// what makes the built-in theme produce a working deck with nothing wired.
//
// The returned deck has no slides. Append them.
//
// # Bullets
//
// The body list style carries no bullet at any level. A deck of bulleted prose
// is the obvious-looking default and it is wrong here: the block vocabulary has
// no list kind, so every paragraph Vellum lowers is prose, and bulleting prose
// invents structure the author did not write. A consumer driving this model
// directly sets [Paragraph.Bullet] where a list is meant.
func Author(d Design) (*Deck, error) {
	if len(d.BodySizes) == 0 {
		return nil, verr.NewCodedError(verr.VELLUM_THEME_INVALID,
			"the design declares no body type sizes, so a body placeholder would render invisible")
	}
	if d.TitleSize <= 0 {
		return nil, verr.NewCodedError(verr.VELLUM_THEME_INVALID,
			"the design declares no title type size")
	}

	size := d.SlideSize
	if size.IsZero() {
		size = Widescreen
	}

	g := geometryOf(d, size)

	deck := &Deck{
		SlideSize: size,
		NotesSize: NotesPortrait,
		Theme: Theme{
			Name:   d.Name,
			Colors: d.Colors,
			Major:  Typeface{Latin: d.HeadingFamily},
			Minor:  Typeface{Latin: d.BodyFamily},
		},
		Masters: []Master{{
			ID: MasterID,
			Shapes: []Shape{
				titlePlaceholder(PlaceholderTitle, 0, g.title, AlignLeft),
				bodyPlaceholder(1, g.body),
			},
			TextStyles: textStyles(d),
		}},
	}

	deck.Layouts = []Layout{
		{
			ID: LayoutIDTitle, MasterID: MasterID, Name: "Title Slide", Type: LayoutTitle,
			Shapes: []Shape{
				titlePlaceholder(PlaceholderCenterTitle, 0, g.centreTitle, AlignCenter),
				subtitlePlaceholder(1, g.subtitle, d),
			},
		},
		{
			ID: LayoutIDContent, MasterID: MasterID, Name: "Title and Content", Type: LayoutObject,
			Shapes: []Shape{
				titlePlaceholder(PlaceholderTitle, 0, g.title, AlignLeft),
				// No type attribute: a content placeholder is the one that
				// accepts text, a table or a picture, and it says so by
				// declaring no type at all.
				bodyPlaceholder(1, g.body),
			},
		},
		{
			ID: LayoutIDTitleOnly, MasterID: MasterID, Name: "Title Only", Type: LayoutTitleOnly,
			Shapes: []Shape{
				titlePlaceholder(PlaceholderTitle, 0, g.title, AlignLeft),
			},
		},
		{
			ID: LayoutIDBlank, MasterID: MasterID, Name: "Blank", Type: LayoutBlank,
		},
	}

	return deck, nil
}

// geometry is where the authored placeholders sit.
type geometry struct {
	title       Frame
	body        Frame
	centreTitle Frame
	subtitle    Frame
}

// geometryOf places the placeholders inside the design's margins.
//
// The title band is two lines of the title size, so a title that wraps once
// still fits the box it was given rather than overflowing into the body. Every
// number here is derived from the design; none is a constant chosen to look
// right at one slide size.
func geometryOf(d Design, size Size) geometry {
	line := d.LineHeight
	if line <= 0 {
		line = 1.2
	}

	left := d.MarginLeft
	top := d.MarginTop
	width := size.Width - d.MarginLeft - d.MarginRight
	height := size.Height - d.MarginTop - d.MarginBottom

	titleHeight := roundDiv(int64(line*2*100000)*d.TitleSize, 100000)
	gap := d.TitleGap

	bodyTop := top + titleHeight + gap
	bodyHeight := height - titleHeight - gap

	// The title slide centres its pair on the slide rather than filling it.
	// Half the content height for the title, a third for the subtitle beneath.
	centreHeight := height / 3
	subHeight := height / 4
	centreTop := top + (height-centreHeight-subHeight)/2

	return geometry{
		title: Frame{X: left, Y: top, Width: width, Height: titleHeight},
		body:  Frame{X: left, Y: bodyTop, Width: width, Height: bodyHeight},
		centreTitle: Frame{X: left, Y: centreTop,
			Width: width, Height: centreHeight},
		subtitle: Frame{X: left, Y: centreTop + centreHeight,
			Width: width, Height: subHeight},
	}
}

// titlePlaceholder builds a title shape.
func titlePlaceholder(kind PlaceholderType, index int, frame Frame, align Align) Shape {
	anchor := AnchorBottom
	if kind == PlaceholderCenterTitle {
		anchor = AnchorCenter
	}
	return Shape{
		Name:        "Title",
		Frame:       frame,
		Placeholder: &Placeholder{Type: kind, Index: index},
		Text: &TextBody{
			Anchor:     anchor,
			Paragraphs: []Paragraph{{Align: align}},
		},
	}
}

// bodyPlaceholder builds a content shape.
func bodyPlaceholder(index int, frame Frame) Shape {
	return Shape{
		Name:        "Content Placeholder",
		Frame:       frame,
		Placeholder: &Placeholder{Type: PlaceholderContent, Index: index},
		Text:        &TextBody{Paragraphs: []Paragraph{{}}},
	}
}

// subtitlePlaceholder builds the title slide's subtitle.
func subtitlePlaceholder(index int, frame Frame, d Design) Shape {
	size := d.SubtitleSize
	if size == 0 {
		size = d.BodySizes[0]
	}
	return Shape{
		Name:        "Subtitle",
		Frame:       frame,
		Placeholder: &Placeholder{Type: PlaceholderSubTitle, Index: index},
		Text: &TextBody{
			Anchor: AnchorTop,
			Paragraphs: []Paragraph{{
				Align:    AlignCenter,
				Bullet:   Bullet{Kind: BulletNone},
				EndStyle: RunStyle{SizeEMU: size, Color: SchemeColor(SchemeText1)},
			}},
		},
	}
}

// textStyles builds the master's three list styles.
//
// The colours are scheme references rather than literals, so replacing the
// theme part restyles the deck. Headings take tx2 and body tx1, which is what
// makes the design's mapping of its heading colour onto Dark2 reach the slide.
func textStyles(d Design) TextStyles {
	title := ListStyle{Levels: []LevelStyle{{
		Align:      AlignLeft,
		SizeEMU:    d.TitleSize,
		LineHeight: d.LineHeight,
		Font:       FontMajor,
		Color:      SchemeColor(SchemeText2),
		Bold:       d.TitleBold,
		Bullet:     Bullet{Kind: BulletNone},
	}}}

	// All nine levels, not only the ones the design sized. A level past the end
	// takes the last declared size, which is the rule the theme's own heading
	// scale follows for the same reason: an outline deeper than the theme
	// anticipated is a document that should still render, and a level the
	// master does not declare falls to the reader's defaults rather than to
	// anything the theme said.
	var body ListStyle
	for i := 0; i < listStyleLevels; i++ {
		size := d.BodySizes[len(d.BodySizes)-1]
		if i < len(d.BodySizes) {
			size = d.BodySizes[i]
		}
		body.Levels = append(body.Levels, LevelStyle{
			// Each level steps in by its own type size, so the indent scales
			// with the type rather than with a constant that is right at one
			// size and wrong at every other.
			MarginLeft:  int64(i) * size,
			Align:       AlignLeft,
			SizeEMU:     size,
			LineHeight:  d.LineHeight,
			SpaceBefore: d.ParagraphSpace,
			Font:        FontMinor,
			Color:       SchemeColor(SchemeText1),
			Bullet:      Bullet{Kind: BulletNone},
		})
	}

	// The style a free-standing text box inherits. One level, body size: a text
	// box is not an outline.
	other := ListStyle{Levels: []LevelStyle{{
		Align:      AlignLeft,
		SizeEMU:    d.BodySizes[0],
		LineHeight: d.LineHeight,
		Font:       FontMinor,
		Color:      SchemeColor(SchemeText1),
		Bullet:     Bullet{Kind: BulletNone},
	}}}

	return TextStyles{Title: title, Body: body, Other: other}
}
