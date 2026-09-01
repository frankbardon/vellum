package deck

import (
	"strings"

	"github.com/frankbardon/vellum/asset"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/theme"
)

// Lower converts a resolved document into the PresentationML model.
//
// It takes a [fragment.Doc] rather than a specification, for the reason the
// resolve pass exists: theme application, font selection and asset resolution
// have already happened, once, in a place all four writers share. This
// function's only job is the mapping from a format-neutral shape onto a deck's
// idiom.
//
// # How a document becomes slides
//
// The mapping is stated here because a deck is the one target where it is not
// obvious, and because a rule discovered from the output is a rule nobody can
// rely on.
//
//   - A heading starts a slide and becomes its title. Every heading, at every
//     level: a deck's titles are flat. A deeper heading keeps the smaller size
//     the theme gave it rather than being promoted, because promoting it would
//     be overruling the author's own outline.
//   - Text accumulates into the slide's body.
//   - An asset gets a slide of its own, carrying the same title. A slide holds
//     text or a picture, never both — the alternative needs to know how tall
//     the text is, and Vellum does not lay out OOXML, so a measured answer
//     would not be reproducible.
//   - A page break ends the slide and continues under the same title.
//   - Notes become the speaker notes of the slide they follow.
//   - A spacer becomes an empty paragraph of the declared height.
//
// # What it does not do
//
// It does not invent a title slide. A document title is metadata, and turning
// it into a cover means deciding that the first thing in a deck is a cover and
// inventing a subtitle to sit under it — a section vocabulary by the back door.
// The title layout is there for a consumer driving this model directly.
func Lower(d *fragment.Doc) (*Deck, error) {
	design, err := designFrom(d)
	if err != nil {
		return nil, err
	}

	out, err := Author(design)
	if err != nil {
		return nil, err
	}
	out.Title = d.Title
	out.Media = mediaFrom(d)

	l := &lowering{doc: d, out: out, titleUsed: true}
	for si := range d.Sections {
		if err := l.section(si, &d.Sections[si]); err != nil {
			return nil, err
		}
	}
	l.emit()
	l.drainNotes()

	return out, nil
}

// lowering carries the state of one conversion.
//
// The slide under construction is held here rather than appended to as it is
// discovered, because a slide's layout depends on what ends up on it: a title
// with body text is a content slide and a title alone is a title-only slide,
// and which one it is not known until the next heading arrives.
type lowering struct {
	doc *fragment.Doc
	out *Deck

	// title is the heading the current slide sits under, or nil.
	title *fragment.Paragraph

	// titleUsed records whether the current title has already reached a slide,
	// so a heading followed by a picture does not also produce an empty text
	// slide before it.
	titleUsed bool

	// body is the text accumulated for the slide under construction.
	body []Paragraph

	// notes are the speaker notes waiting for the next slide.
	notes []string

	// mediaIndex maps a fragment asset index to a media index, built once.
	mediaIndex map[int]int
}

// section lowers one section.
//
// A section boundary ends the slide under construction and clears the title. A
// section is a division of the document, and carrying one section's heading
// onto the next section's slides would be joining two things the author kept
// apart.
func (l *lowering) section(index int, s *fragment.Section) error {
	l.emit()
	l.title, l.titleUsed = nil, true

	for bi := range s.Blocks {
		if err := l.block(&s.Blocks[bi], index, s.ID, bi); err != nil {
			return err
		}
	}
	return nil
}

// block lowers one block.
func (l *lowering) block(b *fragment.Block, sectionIndex int, sectionID string, blockIndex int) error {
	switch b.Kind {
	case spec.BlockHeading:
		l.emit()
		l.title, l.titleUsed = b.Paragraph, false

	case spec.BlockText:
		l.body = append(l.body, l.paragraph(b.Paragraph, l.bodyInherit()))

	case spec.BlockSpacer:
		// An empty paragraph sized to the space. The mark's own formatting is
		// what gives an empty paragraph its height, so the size goes on the end
		// style and nowhere else.
		l.body = append(l.body, Paragraph{
			Bullet:   Bullet{Kind: BulletNone},
			EndStyle: RunStyle{SizeEMU: b.Space.HeightEMU},
		})

	case spec.BlockPageBreak:
		// The declared degradation: a page break is a new slide. The title
		// carries over, because the content after a break is the same subject
		// continued and a slide with no title is one the audience cannot place.
		l.emit()

	case spec.BlockNotes:
		l.notes = append(l.notes, paragraphText(&b.Note.Body))

	case spec.BlockAsset:
		return l.assetSlide(b.Asset, sectionIndex, sectionID, blockIndex)

	case spec.BlockTable:
		return verr.NewCodedErrorWithDetails(verr.VELLUM_DECK_BLOCK_UNSUPPORTED,
			"the PPTX writer does not render tables yet",
			map[string]any{"kind": string(b.Kind), "section_index": sectionIndex,
				"section_id": sectionID, "block_index": blockIndex})

	default:
		return verr.NewCodedErrorWithDetails(verr.VELLUM_DECK_BLOCK_UNSUPPORTED,
			"the PPTX writer does not render this block kind",
			map[string]any{"kind": string(b.Kind), "section_index": sectionIndex,
				"section_id": sectionID, "block_index": blockIndex})
	}
	return nil
}

// emit closes the slide under construction and, when the title never reached a
// slide at all, emits the title-only slide that a heading with nothing under it
// means.
//
// Called where the title itself ends: a new heading, a section boundary, the
// end of the document.
func (l *lowering) emit() {
	if l.emitBody() {
		return
	}
	if l.title != nil && !l.titleUsed {
		l.append(Slide{LayoutID: LayoutIDTitleOnly, Shapes: l.titleShape()})
		l.titleUsed = true
	}
}

// emitBody closes the slide under construction without deciding anything about
// the title, and reports whether it emitted one.
//
// Separate from emit because a picture is about to become the slide this title
// belongs to. Flushing through emit there would produce an empty title-only
// slide in front of every picture — a deck twice as long as it should be, every
// other slide carrying nothing but a heading.
func (l *lowering) emitBody() bool {
	if len(l.body) == 0 {
		return false
	}
	l.append(Slide{
		LayoutID: LayoutIDContent,
		Shapes:   append(l.titleShape(), bodyShape(l.body)),
	})
	l.body = nil
	l.titleUsed = true
	return true
}

// assetSlide emits a slide holding one picture.
func (l *lowering) assetSlide(a *fragment.AssetRef, sectionIndex int, sectionID string, blockIndex int) error {
	l.emitBody()

	index, ok := l.media(a.AssetIndex)
	if !ok {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"an asset block references an asset that is not in the media manifest",
			map[string]any{"section_index": sectionIndex, "section_id": sectionID,
				"block_index": blockIndex, "asset_index": a.AssetIndex})
	}

	picture := &Picture{MediaIndex: index, AltText: a.AltText}

	// An SVG reaches PowerPoint only when a raster accompanies it: the vector
	// rides in an extension on the blip and the raster is the blip itself, so a
	// reader that understands SVG uses it and one that does not still shows
	// something. Vellum does not rasterise — it never renders — so the pair has
	// to be there already, and a lone SVG is a deck with a hole where the chart
	// should be.
	if l.out.Media[index].MediaType == asset.MediaSVG {
		fallback, ok := l.rasterFallback(index)
		if !ok {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_ASSET_MEDIA_UNSUPPORTED,
				"an SVG asset has no raster fallback, and PowerPoint draws an SVG only when one accompanies it",
				map[string]any{
					"section_index": sectionIndex, "section_id": sectionID,
					"block_index": blockIndex,
					"media_type":  asset.MediaSVG,
					"accepted":    []string{asset.MediaPNG, asset.MediaJPEG},
					"remedy":      "supply a raster alongside the vector; Vellum will not rasterise, because it never renders",
				})
		}
		picture.MediaIndex = fallback
		picture.SVGMediaIndex = index + 1
	}

	shapes := append(l.titleShape(), Shape{
		Name:    "Picture",
		Frame:   l.pictureFrame(a),
		Picture: picture,
	})
	l.append(Slide{LayoutID: LayoutIDTitleOnly, Shapes: shapes})
	l.titleUsed = true
	return nil
}

// pictureFrame centres the resolved asset size in the slide's content region.
//
// The size is the theme box's, applied during resolution. This decides only
// where the rectangle sits, and centring is the one placement that needs no
// measurement of anything: a picture wider than the region overhangs equally on
// both sides rather than running off one edge.
func (l *lowering) pictureFrame(a *fragment.AssetRef) Frame {
	g := l.out.contentRegion()
	return Frame{
		X:      g.X + (g.Width-a.WidthEMU)/2,
		Y:      g.Y + (g.Height-a.HeightEMU)/2,
		Width:  a.WidthEMU,
		Height: a.HeightEMU,
	}
}

// titleShape returns the slide's title shape, or nothing when it has no title.
//
// It carries no frame. The frame is the layout's, and a slide restating it is a
// slide that stops moving when the layout does — which is the whole reason a
// placeholder exists.
func (l *lowering) titleShape() []Shape {
	if l.title == nil {
		return nil
	}
	return []Shape{{
		Name:        "Title",
		Placeholder: &Placeholder{Type: PlaceholderTitle},
		Text: &TextBody{Paragraphs: []Paragraph{
			l.paragraph(l.title, l.inherit(l.out.Masters[0].TextStyles.Title, 0)),
		}},
	}}
}

// bodyShape returns the slide's content shape.
func bodyShape(paragraphs []Paragraph) Shape {
	return Shape{
		Name:        "Content Placeholder",
		Placeholder: &Placeholder{Type: PlaceholderContent, Index: 1},
		Text:        &TextBody{Paragraphs: paragraphs},
	}
}

// append adds a slide and gives it whatever notes were waiting.
func (l *lowering) append(s Slide) {
	if len(l.notes) > 0 {
		s.Notes = strings.Join(l.notes, "\n\n")
		l.notes = nil
	}
	l.out.Slides = append(l.out.Slides, s)
}

// drainNotes attaches notes that arrived after the last slide's content.
//
// A notes block at the very end of a document annotates the slide before it,
// which is the only reading available: there is no slide after it to annotate.
func (l *lowering) drainNotes() {
	if len(l.notes) == 0 || len(l.out.Slides) == 0 {
		return
	}
	last := &l.out.Slides[len(l.out.Slides)-1]
	if last.Notes != "" {
		last.Notes += "\n\n"
	}
	last.Notes += strings.Join(l.notes, "\n\n")
	l.notes = nil
}

// inherited is the formatting a run will take from the level style above it.
//
// Read back off the master this lowering just authored rather than recomputed
// from the design, so what a run is compared against is exactly what it will
// inherit. Two computations of one style is where a run comes to state
// something it already had.
type inherited struct {
	size   int64
	color  string
	font   string
	bold   bool
	italic bool
}

// inherit reads one level of a list style, resolving its scheme references back
// to the values a resolved run carries.
func (l *lowering) inherit(s ListStyle, level int) inherited {
	if level >= len(s.Levels) {
		if len(s.Levels) == 0 {
			return inherited{}
		}
		level = len(s.Levels) - 1
	}
	lvl := s.Levels[level]

	return inherited{
		size:   lvl.SizeEMU,
		color:  l.colorValue(lvl.Color),
		font:   l.fontFamily(lvl.Font),
		bold:   lvl.Bold,
		italic: lvl.Italic,
	}
}

// paragraph lowers one resolved paragraph, emitting only what differs from the
// level style it will inherit.
//
// A run whose size, family, colour and weight all match what it inherits
// carries no properties at all, which is what keeps the deck restylable: a
// slide that restates its master is a slide that stops following it.
func (l *lowering) paragraph(p *fragment.Paragraph, in inherited) Paragraph {
	out := Paragraph{}
	for i := range p.Runs {
		r := &p.Runs[i]
		out.Runs = append(out.Runs, Run{
			Text:  r.Text,
			Style: l.runStyle(&r.Style, in),
		})
	}
	return out
}

// runStyle reduces a resolved style to the difference from what it inherits.
func (l *lowering) runStyle(s *fragment.TextStyle, in inherited) RunStyle {
	out := RunStyle{
		Bold:   Set(s.Bold, in.bold),
		Italic: Set(s.Italic, in.italic),
	}

	if s.SizeEMU != 0 && s.SizeEMU != in.size {
		out.SizeEMU = s.SizeEMU
	}
	if s.Color != "" && s.Color != in.color {
		out.Color = l.schemeRef(s.Color)
	}
	if family := l.faceFamily(s.FaceIndex); family != "" && family != in.font {
		out.Font = l.fontRef(family)
	}
	return out
}

// colorValue resolves a colour as the model states it — a scheme reference or a
// literal — into the value a resolved run would carry.
func (l *lowering) colorValue(value string) string {
	slot, ok := strings.CutPrefix(value, schemePrefix)
	if !ok {
		return value
	}
	for _, s := range schemeSlots {
		if s.mapped == slot {
			return l.doc.Palette.Color(s.role, "")
		}
	}
	return ""
}

// fontFamily resolves a font as the model states it into a family name.
func (l *lowering) fontFamily(value string) string {
	switch value {
	case FontMajor:
		return l.out.Theme.Major.Latin
	case FontMinor:
		return l.out.Theme.Minor.Latin
	}
	return value
}

// fontRef renders a family as a scheme reference when it is one of the theme's
// two faces, and as a literal when it is not.
func (l *lowering) fontRef(family string) string {
	switch family {
	case l.out.Theme.Major.Latin:
		return FontMajor
	case l.out.Theme.Minor.Latin:
		return FontMinor
	}
	return family
}

// faceFamily returns the family of a face in the document's manifest.
func (l *lowering) faceFamily(index int) string {
	if index < 0 || index >= len(l.doc.Fonts) {
		return ""
	}
	return l.doc.Fonts[index].Family
}

// schemeRef renders a colour as a scheme reference when the theme declares it,
// and as a literal when it does not.
//
// A literal is a colour the theme cannot restyle, so it is the answer of last
// resort. It can only arise from a colour that reached the fragment without
// coming from a theme role, which the block model has no way to express — a
// consumer driving the fragment directly can.
//
// Two roles holding the same value resolve to whichever slot comes first. Both
// references render identically and both follow the theme, so the ambiguity is
// only visible if the theme later changes one of the two and not the other.
func (l *lowering) schemeRef(value string) string {
	for _, slot := range schemeSlots {
		if l.doc.Palette.Color(slot.role, "") == value {
			return SchemeColor(slot.mapped)
		}
	}
	return value
}

// bodyInherit is the formatting a body paragraph takes from the master.
func (l *lowering) bodyInherit() inherited {
	return l.inherit(l.out.Masters[0].TextStyles.Body, 0)
}

// contentRegion is the area a layout leaves below its title.
//
// Read off the authored content layout's own body placeholder rather than
// recomputed, so a picture and a body of text occupy the same rectangle. Two
// computations of one region is where a picture comes to sit a hair off the
// text beside it on every slide.
func (d *Deck) contentRegion() Frame {
	for _, l := range d.Layouts {
		if l.ID != LayoutIDContent {
			continue
		}
		for _, s := range l.Shapes {
			if s.Placeholder != nil && s.Placeholder.Type == PlaceholderContent {
				return s.Frame
			}
		}
	}
	return Frame{Width: d.SlideSize.Width, Height: d.SlideSize.Height}
}

// schemeSlots maps DrawingML's twelve colour slots onto Vellum's theme roles.
//
// The mapping is not mechanical and it is not derivable, which is why it is a
// table. DrawingML says dk1/lt1 and dk2/lt2 where the theme says text,
// background, heading and muted; which member of a pair is text depends on
// whether the deck is light or dark, and the master's colour map is what states
// that.
//
// Accents three through six are the theme's remaining roles rather than a
// designed ramp. Vellum's theme declares ten colour roles and DrawingML wants
// twelve slots filled, so the choice is between filling them from roles that
// exist and inventing five colours the theme never mentioned. A theme wanting a
// real accent ramp needs a place to declare one, and that is a theme-schema
// question rather than a deck-writer one.
//
// mapped is the name a shape refers to, which is the colour map's output rather
// than the theme's own slot: a shape naming lt1 directly would bypass the map
// and read a dark deck as a light one.
var schemeSlots = []struct {
	slot   string
	mapped string
	role   theme.ColorRole
}{
	{"dk1", SchemeText1, theme.ColorText},
	{"lt1", SchemeBackground1, theme.ColorBackground},
	{"dk2", SchemeText2, theme.ColorHeading},
	{"lt2", SchemeBackground2, theme.ColorTableStripe},
	{"accent1", SchemeAccent1, theme.ColorAccent},
	{"accent2", SchemeAccent2, theme.ColorTableHeaderBackground},
	{"accent3", SchemeAccent3, theme.ColorTextMuted},
	{"accent4", SchemeAccent4, theme.ColorRule},
	{"accent5", SchemeAccent5, theme.ColorAccentText},
	{"accent6", SchemeAccent6, theme.ColorTableHeaderText},
	{"hlink", SchemeHyperlink, theme.ColorAccent},
	{"folHlink", SchemeFollowed, theme.ColorTextMuted},
}

// schemeDefaults is what a slot takes when the theme declares no such role.
//
// Black and white for the two pairs, and the pairs' own members for the rest. A
// slot left empty is a colour scheme a reader fills with its own idea of an
// accent, which is how one theme comes to render differently in two
// applications.
var schemeDefaults = map[string]string{
	"dk1": "000000", "lt1": "FFFFFF",
	"dk2": "000000", "lt2": "FFFFFF",
}

// designFrom reads the deck's design off the resolved document.
func designFrom(d *fragment.Doc) (Design, error) {
	if len(d.Sections) == 0 {
		return Design{}, verr.NewCodedError(verr.VELLUM_SPEC_INVALID,
			"the document has no sections, so there is no slide geometry to author from")
	}

	page := d.Sections[0].Page
	scale := d.Scale

	body := scale.Body
	if body == 0 {
		return Design{}, verr.NewCodedError(verr.VELLUM_THEME_INVALID,
			"the theme declares no body type size, so a slide's text would render invisible")
	}

	colors := ColorScheme{}
	fields := []*string{
		&colors.Dark1, &colors.Light1, &colors.Dark2, &colors.Light2,
		&colors.Accent1, &colors.Accent2, &colors.Accent3,
		&colors.Accent4, &colors.Accent5, &colors.Accent6,
		&colors.Hyperlink, &colors.FollowedHyperlink,
	}
	for i, slot := range schemeSlots {
		*fields[i] = d.Palette.Color(slot.role, schemeDefaults[slot.slot])
	}

	return Design{
		Name:          d.ThemeID,
		SlideSize:     Size{Width: page.Width, Height: page.Height},
		MarginTop:     page.MarginTop,
		MarginRight:   page.MarginRight,
		MarginBottom:  page.MarginBottom,
		MarginLeft:    page.MarginLeft,
		Colors:        colors,
		HeadingFamily: faceFamily(d, theme.FontHeading),
		BodyFamily:    faceFamily(d, theme.FontBody),
		TitleSize:     scale.HeadingSize(1),
		// Resolution makes every heading bold, unconditionally, so the title
		// style says so once and no title run has to. If heading weight ever
		// becomes something the theme declares, this reads it from there.
		SubtitleSize:   body,
		BodySizes:      []int64{body},
		TitleBold:      true,
		LineHeight:     scale.LineHeight,
		ParagraphSpace: scale.ParagraphAfter,
		TitleGap:       scale.HeadingAfter,
	}, nil
}

// faceFamily returns the family resolved for a font role.
func faceFamily(d *fragment.Doc, role theme.FontRole) string {
	for i := range d.Fonts {
		if d.Fonts[i].Role == role {
			return d.Fonts[i].Family
		}
	}
	return ""
}

// paragraphText joins a paragraph's runs.
func paragraphText(p *fragment.Paragraph) string {
	var b strings.Builder
	for i := range p.Runs {
		b.WriteString(p.Runs[i].Text)
	}
	return b.String()
}

// mediaFrom collects the embeddable assets, ordered by content hash.
//
// Ordered by hash rather than by first use, because media part names are
// derived from position and a part name that depended on mention order would
// make two decks with the same pictures in a different order produce different
// packages. Font programs are excluded: a face is referenced by name in OOXML,
// so a font in the manifest is provenance rather than a part.
func mediaFrom(d *fragment.Doc) []Media {
	out := make([]Media, 0, len(d.Assets))
	for i := range d.Assets {
		a := &d.Assets[i]
		switch a.MediaType {
		case asset.MediaPNG, asset.MediaJPEG, asset.MediaSVG:
		default:
			continue
		}
		out = append(out, Media{Hash: a.Hash, MediaType: a.MediaType, Bytes: a.Bytes})
	}
	return out
}

// media maps a fragment asset index to a media index.
func (l *lowering) media(assetIndex int) (int, bool) {
	if l.mediaIndex == nil {
		l.mediaIndex = make(map[int]int, len(l.doc.Assets))
		for i := range l.doc.Assets {
			for j := range l.out.Media {
				if l.out.Media[j].Hash == l.doc.Assets[i].Hash {
					l.mediaIndex[i] = j
					break
				}
			}
		}
	}
	index, ok := l.mediaIndex[assetIndex]
	return index, ok
}

// rasterFallback finds the one raster a vector can fall back to.
//
// One, unambiguously, or none. A deck carrying two rasters gives no way to say
// which belongs with the vector, and guessing would put the wrong picture on
// the slide in the readers that need the fallback and the right one everywhere
// else — a defect visible only to the readers least likely to report it.
func (l *lowering) rasterFallback(vector int) (int, bool) {
	found, count := -1, 0
	for i := range l.out.Media {
		if i == vector {
			continue
		}
		switch l.out.Media[i].MediaType {
		case asset.MediaPNG, asset.MediaJPEG:
			found = i
			count++
		}
	}
	if count == 1 {
		return found, true
	}
	return -1, false
}
