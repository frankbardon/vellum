package deck

import (
	"strings"
)

// slideXML emits one slide.
func (w *writer) slideXML(index int) []byte {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<p:sld xmlns:a="` + nsDrawingMain +
		`" xmlns:r="` + nsRelationships +
		`" xmlns:p="` + nsPresentation + `">`)

	b.WriteString(`<p:cSld>`)
	w.writeSlideShapeTree(&b, index)
	b.WriteString(`</p:cSld>`)

	b.WriteString(`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>`)
	b.WriteString(`</p:sld>`)
	return []byte(b.String())
}

// notesMasterXML emits the notes master.
//
// A notes slide that references no master is legal by the schema and is
// something PowerPoint offers to repair, so a deck with notes carries one. It
// is the plainest master the format allows: the body placeholder that holds the
// note, and nothing else. There is no notes-page design to make here — the
// notes page is printed matter for a presenter, not part of the deck.
func (w *writer) notesMasterXML() []byte {
	size := w.deck.NotesSize
	if size.IsZero() {
		size = NotesPortrait
	}

	// The note body fills the lower half of the page. The upper half is where
	// PowerPoint draws the slide image, which it places itself.
	body := Frame{
		X:      size.Width / 12,
		Y:      size.Height / 2,
		Width:  size.Width * 5 / 6,
		Height: size.Height*5/12 - size.Height/12,
	}

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<p:notesMaster xmlns:a="` + nsDrawingMain +
		`" xmlns:r="` + nsRelationships +
		`" xmlns:p="` + nsPresentation + `">`)

	b.WriteString(`<p:cSld>`)
	w.writeShapeTree(&b, []Shape{{
		Name:        "Notes Placeholder",
		Frame:       body,
		Placeholder: &Placeholder{Type: PlaceholderBody, Index: 1},
		Text:        &TextBody{Paragraphs: []Paragraph{{}}},
	}})
	b.WriteString(`</p:cSld>`)

	b.WriteString(`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2"` +
		` accent1="accent1" accent2="accent2" accent3="accent3"` +
		` accent4="accent4" accent5="accent5" accent6="accent6"` +
		` hlink="hlink" folHlink="folHlink"/>`)

	b.WriteString(`<p:notesStyle>`)
	writeLevelStyle(&b, "a:lvl1pPr", LevelStyle{
		Align:   AlignLeft,
		SizeEMU: notesSize,
		Font:    FontMinor,
		Color:   SchemeColor(SchemeText1),
		Bullet:  Bullet{Kind: BulletNone},
	})
	b.WriteString(`</p:notesStyle>`)

	b.WriteString(`</p:notesMaster>`)
	return []byte(b.String())
}

// notesSize is the type size a speaker note is printed at: twelve points.
//
// A constant rather than a design field, because the notes page is not part of
// the deck's appearance — it is printed matter for one reader standing at a
// lectern, and a theme that restyled it would be restyling something nobody
// in the audience sees.
const notesSize = 12 * emuPerPoint

// notesSlideXML emits the notes slide for a slide that carries notes.
func (w *writer) notesSlideXML(slide int) []byte {
	note := w.deck.Slides[slide].Notes

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<p:notes xmlns:a="` + nsDrawingMain +
		`" xmlns:r="` + nsRelationships +
		`" xmlns:p="` + nsPresentation + `">`)

	b.WriteString(`<p:cSld>`)
	// The frame is inherited from the notes master's body placeholder, which is
	// why the shape carries none: a notes slide restating its master's geometry
	// is a notes page that stops following the master.
	w.writeShapeTree(&b, []Shape{{
		Name:        "Notes Placeholder",
		Placeholder: &Placeholder{Type: PlaceholderBody, Index: 1},
		Text:        &TextBody{Paragraphs: notesParagraphs(note)},
	}})
	b.WriteString(`</p:cSld>`)

	b.WriteString(`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>`)
	b.WriteString(`</p:notes>`)
	return []byte(b.String())
}

// notesParagraphs splits a note on its line breaks.
//
// A note is a string rather than a paragraph list because that is what a
// speaker note is — prose typed into a box. Splitting on newlines is what makes
// a two-paragraph note arrive as two paragraphs rather than as one paragraph
// containing a character no reader renders.
func notesParagraphs(note string) []Paragraph {
	lines := strings.Split(strings.ReplaceAll(note, "\r\n", "\n"), "\n")
	out := make([]Paragraph, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			out = append(out, Paragraph{})
			continue
		}
		out = append(out, Paragraph{Runs: []Run{{Text: line}}})
	}
	return out
}
