package doc

import (
	"sort"
	"strconv"

	"github.com/frankbardon/vellum/asset"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/theme"
)

// Lower converts a resolved document into the WordprocessingML model.
//
// It takes a [fragment.Doc] rather than a specification, which is the whole
// reason the resolve pass exists: theme application, font selection, number
// formatting and asset resolution have already happened, once, in a place all
// four writers share. This function's only job is the mapping from a
// format-neutral shape onto WordprocessingML's idiom.
//
// A block kind this writer cannot render is a hard error naming the kind and
// its position. Silently dropping content is the failure mode the library
// exists to prevent: a missing section a reader notices is far worse than an
// error the caller was told about.
func Lower(d *fragment.Doc) (*Document, error) {
	if d == nil {
		return nil, verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT,
			"the resolved document is nil")
	}

	th := scaleFrom(d)
	out := &Document{
		Title:  d.Title,
		Styles: buildStyleSheet(d, th),
	}
	out.Media = mediaFrom(d)

	l := &lowering{doc: d, out: out, scale: th}
	for i := range d.Sections {
		if err := l.section(i, &d.Sections[i]); err != nil {
			return nil, err
		}
	}
	if len(out.Sections) == 0 {
		// A document with no content still needs a section, because
		// WordprocessingML has nowhere else to put the page geometry.
		out.Sections = []Section{{Page: A4Portrait()}}
	}
	return out, nil
}

// lowering carries the state of one conversion.
type lowering struct {
	doc   *fragment.Doc
	out   *Document
	scale typeScale

	// mediaIndex maps a fragment asset index to a Document media index. They
	// differ because media parts are ordered by content hash while the fragment
	// orders assets by first use, and because font programs in the fragment's
	// asset manifest are not media at all.
	mediaIndex map[int]int

	// drawings counts the drawings emitted so far, which is where a docPr id
	// comes from. A counter is acceptable here — and would not be anywhere on
	// this path that mattered — because the walk it counts is itself
	// deterministic: sections in order, blocks in order.
	drawings int
}

// section lowers one resolved section.
//
// Adjacent sections sharing a page geometry are merged into one
// WordprocessingML section. A specification's sections are logical divisions,
// not page breaks: emitting a section break per division would put a page break
// between every heading and its prose, which is not what the author asked for.
// An explicit page_break block is what produces a page break.
func (l *lowering) section(index int, s *fragment.Section) error {
	page := pageFrom(s.Page)

	var target *Section
	if n := len(l.out.Sections); n > 0 && l.out.Sections[n-1].Page == page {
		target = &l.out.Sections[n-1]
	} else {
		l.out.Sections = append(l.out.Sections, Section{Page: page})
		target = &l.out.Sections[len(l.out.Sections)-1]
	}

	for i := range s.Blocks {
		content, err := l.block(index, s.ID, i, &s.Blocks[i])
		if err != nil {
			return err
		}
		target.Content = append(target.Content, content...)
	}
	return nil
}

func (l *lowering) block(sectionIndex int, sectionID string, blockIndex int, b *fragment.Block) ([]Content, error) {
	switch b.Kind {
	case spec.BlockHeading:
		return []Content{para(l.paragraph(b.Paragraph, HeadingStyleID(b.Paragraph.OutlineLevel)))}, nil

	case spec.BlockText:
		return []Content{para(l.paragraph(b.Paragraph, StyleNormal))}, nil

	case spec.BlockSpacer:
		// A spacer is an empty paragraph carrying its height as spacing rather
		// than a paragraph containing blank lines. Blank lines are a height
		// that changes with the font; spacing is the height that was asked for.
		return []Content{para(Paragraph{StyleID: StyleNormal, SpaceAfter: b.Space.HeightEMU})}, nil

	case spec.BlockPageBreak:
		return []Content{para(Paragraph{
			StyleID: StyleNormal,
			Runs:    []Run{{Break: BreakPage}},
		})}, nil

	case spec.BlockNotes:
		return l.note(b.Note), nil

	case spec.BlockAsset:
		content, err := l.asset(b.Asset, sectionIndex, sectionID, blockIndex)
		if err != nil {
			return nil, err
		}
		return content, nil

	case spec.BlockTable:
		content, err := l.table(b.Table, sectionIndex, sectionID, blockIndex)
		if err != nil {
			return nil, err
		}
		return content, nil

	default:
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_DOC_BLOCK_UNSUPPORTED,
			"the DOCX writer does not render this block kind",
			map[string]any{
				"kind":          string(b.Kind),
				"section_index": sectionIndex,
				"section_id":    sectionID,
				"block_index":   blockIndex,
			})
	}
}

// paragraph lowers resolved prose, naming a style and emitting direct
// formatting only where a run actually departs from it.
//
// That subtraction is the point. A run whose appearance is exactly what the
// style already says emits no rPr, so the document reads as authored rather
// than as generated, and restyling it in Word actually works.
func (l *lowering) paragraph(p *fragment.Paragraph, styleID string) Paragraph {
	out := Paragraph{StyleID: styleID, OutlineLevel: p.OutlineLevel}

	base := l.styleRun(styleID)
	for i := range p.Runs {
		r := &p.Runs[i]
		out.Runs = append(out.Runs, Run{
			Text:       r.Text,
			Properties: subtract(runProperties(l.doc, r.Style), base),
		})
	}
	return out
}

// note lowers a notes block.
//
// A flowing document has no speaker-note channel, so a note becomes a footnote
// anchored where the block sat — which is what the capability matrix declares
// and what the resolve pass has already warned about. The anchor is an empty
// paragraph rather than nothing, because a footnote reference has to live in
// a run and a run has to live in a paragraph.
func (l *lowering) note(n *fragment.Note) []Content {
	body := l.paragraph(&n.Body, StyleFootnoteText)
	l.out.Footnotes = append(l.out.Footnotes, Footnote{Content: []Content{para(body)}})

	return []Content{para(Paragraph{
		StyleID: StyleNormal,
		Runs: []Run{{
			StyleID:     StyleFootnoteRef,
			FootnoteRef: len(l.out.Footnotes),
		}},
	})}
}

// asset lowers an asset block into a drawing wrapped in its own paragraph.
func (l *lowering) asset(a *fragment.AssetRef, sectionIndex int, sectionID string, blockIndex int) ([]Content, error) {
	idx, ok := l.media(a.AssetIndex)
	if !ok {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"an asset block references an asset that is not in the media manifest",
			map[string]any{"section_index": sectionIndex, "section_id": sectionID,
				"block_index": blockIndex, "asset_index": a.AssetIndex})
	}

	l.drawings++
	drawing := &Drawing{
		ID:            l.drawings,
		MediaIndex:    idx,
		FallbackIndex: -1,
		WidthEMU:      a.WidthEMU,
		HeightEMU:     a.HeightEMU,
		Name:          "Picture " + strconv.Itoa(l.drawings),
		AltText:       a.AltText,
	}

	// An SVG reaches Word only when a raster accompanies it: the vector goes in
	// an extension on the blip and the raster goes in the blip itself, so a
	// reader that understands SVG uses it and one that does not still shows
	// something. Vellum does not rasterise, so the pair must already be present
	// — and a lone SVG is a document that opens with a hole where the chart is.
	if l.out.Media[idx].MediaType == asset.MediaSVG {
		fallback, ok := l.rasterFallback(idx)
		if !ok {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_ASSET_MEDIA_UNSUPPORTED,
				"an SVG asset has no raster fallback, and Word draws an SVG only when one accompanies it",
				map[string]any{
					"section_index": sectionIndex, "section_id": sectionID,
					"block_index": blockIndex,
					"media_type":  asset.MediaSVG,
					"accepted":    []string{asset.MediaPNG, asset.MediaJPEG},
					"remedy":      "supply a raster alongside the vector; Vellum will not rasterise, because it never renders",
				})
		}
		drawing.FallbackIndex = fallback
	}

	return []Content{para(Paragraph{
		StyleID:   StyleNormal,
		Alignment: AlignCenter,
		Runs:      []Run{{Drawing: drawing}},
	})}, nil
}

// rasterFallback finds a raster to pair with a vector.
//
// v1 looks for exactly one raster in the manifest, which is the shape a host
// following the layout query produces: one artifact per (role, box), rendered
// as a pair. A document mixing several vectors and several rasters has no
// unambiguous pairing, and guessing one would put the wrong picture in the
// document — so the ambiguous case is reported as no fallback at all.
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
	idx, ok := l.mediaIndex[assetIndex]
	return idx, ok
}

// mediaFrom collects the embeddable assets, ordered by content hash.
//
// Ordered by hash rather than by first use, because media part names are
// derived from position and a part name that depended on mention order would
// make two documents with the same pictures in a different order produce
// different packages. Font programs are excluded: a face is referenced by name
// in OOXML, so a font in the manifest is provenance rather than a part.
func mediaFrom(d *fragment.Doc) []Media {
	out := make([]Media, 0, len(d.Assets))
	for i := range d.Assets {
		a := &d.Assets[i]
		switch a.MediaType {
		case asset.MediaPNG, asset.MediaJPEG, asset.MediaSVG:
			out = append(out, Media{Hash: a.Hash, MediaType: a.MediaType, Bytes: a.Bytes})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hash < out[j].Hash })
	return out
}

// runProperties flattens a resolved text style into direct formatting.
func runProperties(d *fragment.Doc, s fragment.TextStyle) RunProperties {
	family := ""
	if s.FaceIndex >= 0 && s.FaceIndex < len(d.Fonts) {
		family = d.Fonts[s.FaceIndex].Family
	}
	return RunProperties{
		Font:      family,
		SizeEMU:   s.SizeEMU,
		Color:     s.Color,
		Highlight: s.Background,
		Bold:      s.Bold,
		Italic:    s.Italic,
		Underline: s.Underline,
	}
}

// subtract removes from a run's properties everything its style already says.
//
// The result is what the run genuinely overrides, which is usually nothing —
// so most runs emit no rPr at all. Emitting the full set on every run would
// produce a document that looks identical and cannot be restyled, because
// direct formatting wins over a style and a reader changing the style would
// see nothing happen.
func subtract(got, style RunProperties) RunProperties {
	out := got
	if out.Font == style.Font {
		out.Font = ""
	}
	if out.SizeEMU == style.SizeEMU {
		out.SizeEMU = 0
	}
	if out.Color == style.Color {
		out.Color = ""
	}
	if out.Highlight == style.Highlight {
		out.Highlight = ""
	}
	// The booleans are subtracted only when the style already asserts them.
	// A style that is bold and a run that is not cannot be expressed by
	// omission — it needs an explicit off — so that case keeps the flag and the
	// writer emits w:b w:val="0".
	if style.Bold && out.Bold {
		out.Bold = false
	}
	if style.Italic && out.Italic {
		out.Italic = false
	}
	if style.Underline && out.Underline {
		out.Underline = false
	}
	return out
}

// styleRun returns the character formatting a style ID carries, so the
// subtraction has something to subtract.
func (l *lowering) styleRun(styleID string) RunProperties {
	for i := range l.out.Styles.Paragraph {
		if l.out.Styles.Paragraph[i].ID == styleID {
			return l.out.Styles.Paragraph[i].Run
		}
	}
	return RunProperties{}
}

func pageFrom(p fragment.Page) PageSetup {
	return PageSetup{
		Width:        p.Width,
		Height:       p.Height,
		MarginTop:    p.MarginTop,
		MarginRight:  p.MarginRight,
		MarginBottom: p.MarginBottom,
		MarginLeft:   p.MarginLeft,
		// WordprocessingML wants header and footer distances and Word writes
		// half an inch when the author has expressed no view. A zero here makes
		// Word place a running head at the very edge of the sheet.
		MarginHeader: emuPerInch / 2,
		MarginFooter: emuPerInch / 2,
		Landscape:    p.Width > p.Height,
	}
}

// scaleFrom reads the theme measurements the styles need off the fragment.
//
// Read from the resolved palette and scale rather than inferred from the runs
// the document happens to carry. The inference was here first and was wrong in
// a way that only showed on a sparse document: a file of nothing but headings
// has no body paragraph to read a body size from, so the styles part declared a
// zero-sized Normal style and Word rendered every later paragraph invisible.
//
// The one thing still read from the content is how many heading levels to
// declare. That is a property of the document rather than of the theme — a
// styles part carrying levels nothing uses is noise a designer has to read
// past — and the sizes for those levels come from the theme all the same.
func scaleFrom(d *fragment.Doc) typeScale {
	s := typeScale{
		BodySize:        d.Scale.Body,
		CaptionSize:     d.Scale.Caption,
		NotesSize:       d.Scale.Notes,
		TableBodySize:   d.Scale.TableBody,
		ParagraphBefore: d.Scale.ParagraphBefore,
		ParagraphAfter:  d.Scale.ParagraphAfter,
		HeadingBefore:   d.Scale.HeadingBefore,
		HeadingAfter:    d.Scale.HeadingAfter,
		LineHeight:      d.Scale.LineHeight,

		TextColor:       d.Palette.Color(theme.ColorText, "000000"),
		MutedColor:      d.Palette.Color(theme.ColorTextMuted, ""),
		HeadingColor:    d.Palette.Color(theme.ColorHeading, ""),
		RuleColor:       d.Palette.Color(theme.ColorRule, ""),
		TableHeaderFill: d.Palette.Color(theme.ColorTableHeaderBackground, ""),
		TableHeaderText: d.Palette.Color(theme.ColorTableHeaderText, ""),
		TableStripeFill: d.Palette.Color(theme.ColorTableStripe, ""),
	}

	for level := 1; level <= deepestHeading(d); level++ {
		s.HeadingSizes = append(s.HeadingSizes, d.Scale.HeadingSize(level))
	}

	applyScaleDefaults(&s)
	return s
}

// deepestHeading returns the deepest outline level the document uses.
func deepestHeading(d *fragment.Doc) int {
	deepest := 0
	for si := range d.Sections {
		for bi := range d.Sections[si].Blocks {
			b := &d.Sections[si].Blocks[bi]
			if b.Kind != spec.BlockHeading || b.Paragraph == nil {
				continue
			}
			if b.Paragraph.OutlineLevel > deepest {
				deepest = b.Paragraph.OutlineLevel
			}
		}
	}
	return deepest
}

// applyScaleDefaults fills in what a sparse document did not exercise.
//
// A document of nothing but headings has no body size to read, and a styles
// part with a zero-sized Normal style makes Word render every paragraph
// invisible. Deriving the gaps from what is present keeps a partial document
// from producing a broken styles part.
func applyScaleDefaults(s *typeScale) {
	const defaultBody = 11 * emuPerHalfPoint * 2 // 11pt

	if s.BodySize == 0 {
		if len(s.HeadingSizes) > 0 {
			s.BodySize = s.HeadingSizes[len(s.HeadingSizes)-1]
		} else {
			s.BodySize = defaultBody
		}
	}
	if s.TableBodySize == 0 {
		s.TableBodySize = s.BodySize
	}
	if s.CaptionSize == 0 {
		s.CaptionSize = s.BodySize
	}
	if s.NotesSize == 0 {
		s.NotesSize = s.BodySize
	}
	if s.HeadingColor == "" {
		s.HeadingColor = s.TextColor
	}
	if s.MutedColor == "" {
		s.MutedColor = s.TextColor
	}
	if s.RuleColor == "" {
		s.RuleColor = "D0D0D0"
	}
	if s.LineHeight <= 0 {
		s.LineHeight = 1.2
	}
}
