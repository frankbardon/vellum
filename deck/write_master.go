package deck

import (
	"strconv"
	"strings"
)

// firstLayoutID is the base of the layout identifier space, which shares its
// range with the masters'.
const firstLayoutID = 2147483649

// masterXML emits one slide master.
func (w *writer) masterXML(index int) []byte {
	d := w.deck
	m := &d.Masters[index]

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<p:sldMaster xmlns:a="` + nsDrawingMain +
		`" xmlns:r="` + nsRelationships +
		`" xmlns:p="` + nsPresentation + `">`)

	b.WriteString(`<p:cSld>`)
	writeBackground(&b, m.Background)
	w.writeShapeTree(&b, m.Shapes)
	b.WriteString(`</p:cSld>`)

	// The colour map is what turns the theme's dark/light pairs into the
	// background/text pairs a shape refers to. It is the one place a deck
	// states whether it is a light deck or a dark one, and inverting it is how
	// a dark theme is built without touching a single slide.
	b.WriteString(`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2"` +
		` accent1="accent1" accent2="accent2" accent3="accent3"` +
		` accent4="accent4" accent5="accent5" accent6="accent6"` +
		` hlink="hlink" folHlink="folHlink"/>`)

	b.WriteString(`<p:sldLayoutIdLst>`)
	next := int64(firstLayoutID)
	for li := range d.Layouts {
		if d.Layouts[li].MasterID != m.ID {
			continue
		}
		b.WriteString(`<p:sldLayoutId id="` + strconv.FormatInt(next, 10) +
			`" r:id="` + w.layoutRels[li] + `"/>`)
		next++
	}
	b.WriteString(`</p:sldLayoutIdLst>`)

	b.WriteString(`<p:txStyles>`)
	writeListStyle(&b, "p:titleStyle", m.TextStyles.Title)
	writeListStyle(&b, "p:bodyStyle", m.TextStyles.Body)
	writeListStyle(&b, "p:otherStyle", m.TextStyles.Other)
	b.WriteString(`</p:txStyles>`)

	b.WriteString(`</p:sldMaster>`)
	return []byte(b.String())
}

// layoutXML emits one slide layout.
func (w *writer) layoutXML(index int) []byte {
	l := &w.deck.Layouts[index]

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<p:sldLayout xmlns:a="` + nsDrawingMain +
		`" xmlns:r="` + nsRelationships +
		`" xmlns:p="` + nsPresentation + `"`)
	if l.Type != "" {
		b.WriteString(` type="` + escapeAttr(string(l.Type)) + `"`)
	}
	// preserve keeps the layout in the deck even when no slide uses it, which
	// is what lets a consumer add a slide on a layout the generated deck did
	// not happen to need. Without it PowerPoint may drop the layout on save.
	b.WriteString(` preserve="1">`)

	b.WriteString(`<p:cSld`)
	if l.Name != "" {
		b.WriteString(` name="` + escapeAttr(l.Name) + `"`)
	}
	b.WriteString(`>`)
	w.writeShapeTree(&b, l.Shapes)
	b.WriteString(`</p:cSld>`)

	// The layout takes the master's colour map unchanged. An override here
	// would be a layout that reads its theme differently from every other
	// layout in the same deck.
	b.WriteString(`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>`)
	b.WriteString(`</p:sldLayout>`)
	return []byte(b.String())
}

// writeBackground emits p:bg, or nothing for a master taking the scheme's own
// background.
func writeBackground(b *strings.Builder, value string) {
	if value == "" {
		return
	}
	b.WriteString(`<p:bg><p:bgPr>`)
	writeSolidFill(b, value)
	b.WriteString(`<a:effectLst/>`)
	b.WriteString(`</p:bgPr></p:bg>`)
}

// writeListStyle emits one of the master's three list styles.
//
// Levels are written as lvl1pPr through lvl9pPr, and only for the levels the
// style declares. A style with three levels leaves the deeper six to the
// reader's defaults, which is the right answer: writing six more levels of
// invented sizes would be Vellum deciding what a fourth-level bullet looks
// like in a theme that never mentioned one.
func writeListStyle(b *strings.Builder, element string, s ListStyle) {
	if len(s.Levels) == 0 {
		b.WriteString(`<` + element + `/>`)
		return
	}

	b.WriteString(`<` + element + `>`)
	for i, lvl := range s.Levels {
		if i >= 9 {
			break
		}
		writeLevelStyle(b, "a:lvl"+strconv.Itoa(i+1)+"pPr", lvl)
	}
	b.WriteString(`</` + element + `>`)
}

// writeLevelStyle emits one level's paragraph properties.
func writeLevelStyle(b *strings.Builder, element string, l LevelStyle) {
	b.WriteString(`<` + element)
	if l.MarginLeft != 0 {
		b.WriteString(` marL="` + strconv.FormatInt(l.MarginLeft, 10) + `"`)
	}
	if l.Indent != 0 {
		b.WriteString(` indent="` + strconv.FormatInt(l.Indent, 10) + `"`)
	}
	if l.Align != AlignInherit {
		b.WriteString(` algn="` + escapeAttr(string(l.Align)) + `"`)
	}
	b.WriteString(`>`)

	if l.LineHeight > 0 {
		// Thousandths of a percent, which is DrawingML's unit for a spacing
		// multiple: 1.2 is 120000. A value in points here is legal and means
		// something entirely different.
		b.WriteString(`<a:lnSpc><a:spcPct val="` +
			strconv.FormatInt(percent(l.LineHeight), 10) + `"/></a:lnSpc>`)
	}
	if l.SpaceBefore != 0 {
		b.WriteString(`<a:spcBef><a:spcPts val="` +
			strconv.FormatInt(hundredthPoints(l.SpaceBefore), 10) + `"/></a:spcBef>`)
	}
	writeBullet(b, l.Bullet)

	writeRunProperties(b, "a:defRPr", RunStyle{
		Font:    l.Font,
		SizeEMU: l.SizeEMU,
		Color:   l.Color,
		Bold:    Set(l.Bold, false),
		Italic:  Set(l.Italic, false),
	})

	b.WriteString(`</` + element + `>`)
}
