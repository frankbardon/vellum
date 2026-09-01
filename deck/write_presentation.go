package deck

import (
	"strconv"
	"strings"
)

// Identifier bases PresentationML reserves.
//
// These are not arbitrary. A slide identifier must be at least 256 and below
// 2147483648, and a master identifier at or above 2147483648 — the two spaces
// are disjoint on purpose and a deck that mixes them is one PowerPoint repairs.
const (
	firstSlideID  = 256
	firstMasterID = 2147483648
)

// presentationXML emits ppt/presentation.xml.
func (w *writer) presentationXML() []byte {
	d := w.deck

	size := d.SlideSize
	if size.IsZero() {
		size = Widescreen
	}
	notes := d.NotesSize
	if notes.IsZero() {
		notes = NotesPortrait
	}

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<p:presentation xmlns:a="` + nsDrawingMain +
		`" xmlns:r="` + nsRelationships +
		`" xmlns:p="` + nsPresentation + `">`)

	b.WriteString(`<p:sldMasterIdLst>`)
	for i := range d.Masters {
		b.WriteString(`<p:sldMasterId id="` + strconv.FormatInt(firstMasterID+int64(i), 10) +
			`" r:id="` + w.masterRels[i] + `"/>`)
	}
	b.WriteString(`</p:sldMasterIdLst>`)

	if w.notesMasterRel != "" {
		b.WriteString(`<p:notesMasterIdLst><p:notesMasterId r:id="` +
			w.notesMasterRel + `"/></p:notesMasterIdLst>`)
	}

	if len(d.Slides) > 0 {
		b.WriteString(`<p:sldIdLst>`)
		for i := range d.Slides {
			b.WriteString(`<p:sldId id="` + strconv.Itoa(firstSlideID+i) +
				`" r:id="` + w.slideRels[i] + `"/>`)
		}
		b.WriteString(`</p:sldIdLst>`)
	}

	b.WriteString(`<p:sldSz cx="` + strconv.FormatInt(size.Width, 10) +
		`" cy="` + strconv.FormatInt(size.Height, 10) + `"/>`)
	b.WriteString(`<p:notesSz cx="` + strconv.FormatInt(notes.Width, 10) +
		`" cy="` + strconv.FormatInt(notes.Height, 10) + `"/>`)

	b.WriteString(`</p:presentation>`)
	return []byte(b.String())
}

// presPropsXML emits ppt/presProps.xml.
//
// Empty, and it still has to exist: PowerPoint reads the presentation
// properties part when it opens a deck and reports a package without one as
// needing repair. Nothing Vellum sets belongs in it — every property it can
// carry is about how the deck is presented rather than what it says.
func (w *writer) presPropsXML() []byte {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<p:presentationPr xmlns:a="` + nsDrawingMain +
		`" xmlns:r="` + nsRelationships +
		`" xmlns:p="` + nsPresentation + `"/>`)
	return []byte(b.String())
}
