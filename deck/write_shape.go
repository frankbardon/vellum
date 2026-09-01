package deck

import (
	"strconv"
	"strings"
)

// shapeIDs start at 2. Identifier 1 belongs to the shape tree's own group
// shape, which every spTree carries, so a shape numbered 1 collides with it and
// PowerPoint reports the slide as needing repair.
const firstShapeID = 2

// writeShapeTree emits a p:spTree and the shapes inside it.
//
// Shape identifiers are assigned by position, so they are a function of the
// slide rather than of a counter shared across the deck. Two slides carrying
// the same shapes therefore produce the same identifiers, which is what makes
// two parts comparable.
func (w *writer) writeShapeTree(b *strings.Builder, shapes []Shape) {
	b.WriteString(`<p:spTree>`)
	b.WriteString(`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>`)
	b.WriteString(`<p:grpSpPr><a:xfrm>` +
		`<a:off x="0" y="0"/><a:ext cx="0" cy="0"/>` +
		`<a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/>` +
		`</a:xfrm></p:grpSpPr>`)

	for i := range shapes {
		w.writeShape(b, &shapes[i], firstShapeID+i, -1)
	}
	b.WriteString(`</p:spTree>`)
}

// writeSlideShapeTree emits a slide's shape tree, which is the only one whose
// pictures can name a relationship.
func (w *writer) writeSlideShapeTree(b *strings.Builder, slide int) {
	b.WriteString(`<p:spTree>`)
	b.WriteString(`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>`)
	b.WriteString(`<p:grpSpPr><a:xfrm>` +
		`<a:off x="0" y="0"/><a:ext cx="0" cy="0"/>` +
		`<a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/>` +
		`</a:xfrm></p:grpSpPr>`)

	shapes := w.deck.Slides[slide].Shapes
	for i := range shapes {
		w.writeShape(b, &shapes[i], firstShapeID+i, slide)
	}
	b.WriteString(`</p:spTree>`)
}

// writeShape emits one shape. slide is the slide index for a picture's
// relationship lookup, or negative when the shape is on a master or layout.
func (w *writer) writeShape(b *strings.Builder, s *Shape, id, slide int) {
	switch {
	case s.Picture != nil:
		w.writePicture(b, s, id, slide)
	case s.Table != nil:
		w.writeTableFrame(b, s, id)
	default:
		w.writeTextShape(b, s, id)
	}
}

// writeTextShape emits a p:sp.
func (w *writer) writeTextShape(b *strings.Builder, s *Shape, id int) {
	b.WriteString(`<p:sp>`)
	b.WriteString(`<p:nvSpPr>`)
	b.WriteString(`<p:cNvPr id="` + strconv.Itoa(id) + `" name="` +
		escapeAttr(shapeName(s, id)) + `"/>`)
	// noGrp locks the shape out of being grouped, which is what a placeholder
	// is: a slot in the layout rather than a shape a user drags around.
	if s.Placeholder != nil {
		b.WriteString(`<p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>`)
	} else {
		b.WriteString(`<p:cNvSpPr txBox="1"/>`)
	}
	b.WriteString(`<p:nvPr>`)
	writePlaceholder(b, s.Placeholder)
	b.WriteString(`</p:nvPr>`)
	b.WriteString(`</p:nvSpPr>`)

	b.WriteString(`<p:spPr>`)
	writeFrame(b, s.Frame)
	b.WriteString(`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom>`)
	b.WriteString(`</p:spPr>`)

	w.writeTextBody(b, s.Text, s.Placeholder == nil)
	b.WriteString(`</p:sp>`)
}

// writePicture emits a p:pic.
func (w *writer) writePicture(b *strings.Builder, s *Shape, id, slide int) {
	p := s.Picture

	rel := ""
	svgRel := ""
	if slide >= 0 {
		rel = w.slideMediaRels[slide][p.MediaIndex]
		if p.SVGMediaIndex > 0 {
			svgRel = w.slideMediaRels[slide][p.SVGMediaIndex-1]
		}
	}

	b.WriteString(`<p:pic>`)
	b.WriteString(`<p:nvPicPr>`)
	b.WriteString(`<p:cNvPr id="` + strconv.Itoa(id) + `" name="` +
		escapeAttr(shapeName(s, id)) + `"`)
	if p.AltText != "" {
		// descr is where a screen reader looks. It is the only place the
		// accessible description has to go in this format, which is why an
		// asset carrying one renders it here rather than degrading.
		b.WriteString(` descr="` + escapeAttr(p.AltText) + `"`)
	}
	b.WriteString(`/>`)
	b.WriteString(`<p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr>`)
	b.WriteString(`<p:nvPr>`)
	writePlaceholder(b, s.Placeholder)
	b.WriteString(`</p:nvPr>`)
	b.WriteString(`</p:nvPicPr>`)

	b.WriteString(`<p:blipFill>`)
	b.WriteString(`<a:blip r:embed="` + escapeAttr(rel) + `"`)
	if svgRel == "" {
		b.WriteString(`/>`)
	} else {
		// The vector rendition rides in an extension beside the raster rather
		// than replacing it. A reader that knows the extension draws the SVG;
		// every other reader draws the raster and is not told anything went
		// wrong. That is the only way to carry a vector asset into OOXML
		// without making the file unreadable to the readers that predate it.
		b.WriteString(`>`)
		b.WriteString(`<a:extLst><a:ext uri="{96DAC541-7B7A-43D3-8B79-37D633B846F1}">`)
		b.WriteString(`<asvg:svgBlip xmlns:asvg="` + nsSVG + `" r:embed="` + escapeAttr(svgRel) + `"/>`)
		b.WriteString(`</a:ext></a:extLst>`)
		b.WriteString(`</a:blip>`)
	}
	b.WriteString(`<a:stretch><a:fillRect/></a:stretch>`)
	b.WriteString(`</p:blipFill>`)

	b.WriteString(`<p:spPr>`)
	writeFrame(b, s.Frame)
	b.WriteString(`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom>`)
	b.WriteString(`</p:spPr>`)
	b.WriteString(`</p:pic>`)
}

// writePlaceholder emits p:ph, or nothing for a free-standing shape.
//
// A title carries no index and everything else does. That asymmetry is the
// schema's and it is worth stating here rather than at the call site, because
// getting it wrong produces a slide whose placeholders inherit from the wrong
// layout shapes and land on top of one another — a deck that opens and is
// visibly wrong, with nothing in it that a reader would call an error.
func writePlaceholder(b *strings.Builder, ph *Placeholder) {
	if ph == nil {
		return
	}
	b.WriteString(`<p:ph`)
	if ph.Type != PlaceholderContent {
		b.WriteString(` type="` + escapeAttr(string(ph.Type)) + `"`)
	}
	if ph.Type != PlaceholderTitle && ph.Type != PlaceholderCenterTitle {
		b.WriteString(` idx="` + strconv.Itoa(ph.Index) + `"`)
	}
	b.WriteString(`/>`)
}

// writeFrame emits a:xfrm, or nothing for a frame that inherits.
func writeFrame(b *strings.Builder, f Frame) {
	if f.IsZero() {
		return
	}
	b.WriteString(`<a:xfrm>`)
	b.WriteString(`<a:off x="` + strconv.FormatInt(f.X, 10) +
		`" y="` + strconv.FormatInt(f.Y, 10) + `"/>`)
	b.WriteString(`<a:ext cx="` + strconv.FormatInt(f.Width, 10) +
		`" cy="` + strconv.FormatInt(f.Height, 10) + `"/>`)
	b.WriteString(`</a:xfrm>`)
}

// shapeName returns the shape's name, or one derived from its kind.
func shapeName(s *Shape, id int) string {
	if s.Name != "" {
		return s.Name
	}
	kind := "TextBox"
	switch {
	case s.Picture != nil:
		kind = "Picture"
	case s.Table != nil:
		kind = "Table"
	}
	return kind + " " + strconv.Itoa(id)
}

// writeTextBody emits p:txBody.
//
// autofit is set only on a free-standing text box. A placeholder takes its
// autofit from the layout, and restating it here is exactly the kind of
// override that makes a deck stop following its master.
func (w *writer) writeTextBody(b *strings.Builder, t *TextBody, freeStanding bool) {
	b.WriteString(`<p:txBody>`)

	b.WriteString(`<a:bodyPr`)
	if t != nil && t.Anchor != AnchorTop {
		b.WriteString(` anchor="` + escapeAttr(string(t.Anchor)) + `"`)
	}
	if t != nil && t.NoWrap {
		b.WriteString(` wrap="none"`)
	}
	if freeStanding {
		b.WriteString(`><a:spAutoFit/></a:bodyPr>`)
	} else {
		b.WriteString(`/>`)
	}

	// Empty, and it has to be present. The list style on a shape is the third
	// rung of the inheritance chain — master, layout, shape — and a reader
	// that finds no element here uses the layout's, which is what a shape
	// stating no overrides wants.
	b.WriteString(`<a:lstStyle/>`)

	if t == nil || len(t.Paragraphs) == 0 {
		// A text body must carry at least one paragraph. A shape with none is
		// one PowerPoint reports as needing repair.
		b.WriteString(`<a:p/>`)
	} else {
		for i := range t.Paragraphs {
			writeParagraph(b, &t.Paragraphs[i])
		}
	}
	b.WriteString(`</p:txBody>`)
}

// writeParagraph emits a:p.
func writeParagraph(b *strings.Builder, p *Paragraph) {
	b.WriteString(`<a:p>`)
	writeParagraphProperties(b, p)
	for i := range p.Runs {
		writeRun(b, &p.Runs[i])
	}
	if !p.EndStyle.IsZero() {
		// The paragraph mark's own formatting, which is what sizes an empty
		// paragraph. A blank line between two paragraphs takes its height from
		// here and from nowhere else.
		writeRunProperties(b, "a:endParaRPr", p.EndStyle)
	}
	b.WriteString(`</a:p>`)
}

// writeParagraphProperties emits a:pPr.
//
// The child order is CT_TextParagraphProperties' sequence: spacing, then bullet
// colour and size, then the bullet itself. An element out of order is a slide a
// reader reports as unreadable — the same failure this project has already met
// in WordprocessingML, in a format that gives no better diagnostic.
func writeParagraphProperties(b *strings.Builder, p *Paragraph) {
	needs := p.Level > 0 || p.Align != AlignInherit || p.LineHeightEMU != 0 ||
		p.SpaceBefore != 0 || p.Bullet.Kind != BulletInherit
	if !needs {
		return
	}

	b.WriteString(`<a:pPr`)
	if p.Level > 0 {
		b.WriteString(` lvl="` + strconv.Itoa(p.Level) + `"`)
	}
	if p.Align != AlignInherit {
		b.WriteString(` algn="` + escapeAttr(string(p.Align)) + `"`)
	}

	if p.SpaceBefore == 0 && p.LineHeightEMU == 0 && p.Bullet.Kind == BulletInherit {
		b.WriteString(`/>`)
		return
	}
	b.WriteString(`>`)

	if p.LineHeightEMU != 0 {
		b.WriteString(`<a:lnSpc><a:spcPts val="` +
			strconv.FormatInt(hundredthPoints(p.LineHeightEMU), 10) + `"/></a:lnSpc>`)
	}
	if p.SpaceBefore != 0 {
		b.WriteString(`<a:spcBef><a:spcPts val="` +
			strconv.FormatInt(hundredthPoints(p.SpaceBefore), 10) + `"/></a:spcBef>`)
	}
	writeBullet(b, p.Bullet)
	b.WriteString(`</a:pPr>`)
}

// writeBullet emits the bullet elements, in schema order.
func writeBullet(b *strings.Builder, bu Bullet) {
	switch bu.Kind {
	case BulletInherit:
	case BulletNone:
		b.WriteString(`<a:buNone/>`)
	case BulletChar:
		if bu.Font != "" {
			// The glyph and the family it is drawn from travel together. A
			// bullet character from a family that does not carry it renders as
			// a missing-glyph box, which looks like a font problem and is a
			// markup one.
			b.WriteString(`<a:buFont typeface="` + escapeAttr(bu.Font) + `"/>`)
		}
		b.WriteString(`<a:buChar char="` + escapeAttr(bu.Char) + `"/>`)
	case BulletNumber:
		format := bu.Format
		if format == "" {
			format = "arabicPeriod"
		}
		if bu.Font != "" {
			b.WriteString(`<a:buFont typeface="` + escapeAttr(bu.Font) + `"/>`)
		}
		b.WriteString(`<a:buAutoNum type="` + escapeAttr(format) + `"/>`)
	}
}

// writeRun emits a:r.
func writeRun(b *strings.Builder, r *Run) {
	b.WriteString(`<a:r>`)
	if !r.Style.IsZero() {
		writeRunProperties(b, "a:rPr", r.Style)
	}
	// xml:space is not optional here. DrawingML collapses leading and trailing
	// whitespace in a text node otherwise, and a run that is a single space
	// between two styled words disappears.
	b.WriteString(`<a:t xml:space="preserve">` + escapeText(r.Text) + `</a:t>`)
	b.WriteString(`</a:r>`)
}

// writeRunProperties emits a:rPr, a:defRPr or a:endParaRPr.
//
// One function for the three because they are the same complex type, and the
// child order below is that type's sequence: the fill, then the three script
// faces. Attributes carry the size and the weight.
func writeRunProperties(b *strings.Builder, element string, s RunStyle) {
	b.WriteString(`<` + element)
	if s.SizeEMU != 0 {
		b.WriteString(` sz="` + strconv.FormatInt(hundredthPoints(s.SizeEMU), 10) + `"`)
	}
	if v, ok := s.Bold.attr(); ok {
		b.WriteString(` b="` + v + `"`)
	}
	if v, ok := s.Italic.attr(); ok {
		b.WriteString(` i="` + v + `"`)
	}

	if s.Color == "" && s.Font == "" {
		b.WriteString(`/>`)
		return
	}
	b.WriteString(`>`)
	writeSolidFill(b, s.Color)
	if s.Font != "" {
		b.WriteString(`<a:latin typeface="` + escapeAttr(s.Font) + `"/>`)
	}
	b.WriteString(`</` + element + `>`)
}

// writeSolidFill emits a solid fill from a colour value, which is either a
// scheme reference or a literal hex triplet.
//
// The distinction is the whole reason [SchemeColor] exists: a scheme reference
// follows the theme and a literal does not, and the difference is invisible
// until somebody changes the theme.
func writeSolidFill(b *strings.Builder, value string) {
	if value == "" {
		return
	}
	b.WriteString(`<a:solidFill>`)
	if slot, ok := strings.CutPrefix(value, schemePrefix); ok {
		b.WriteString(`<a:schemeClr val="` + escapeAttr(slot) + `"/>`)
	} else {
		b.WriteString(`<a:srgbClr val="` + escapeAttr(value) + `"/>`)
	}
	b.WriteString(`</a:solidFill>`)
}
