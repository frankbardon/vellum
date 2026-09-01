package doc

import (
	"strconv"
	"strings"
)

// documentXML emits word/document.xml.
func (w *writer) documentXML() []byte {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<w:document xmlns:w="` + nsWordprocessing +
		`" xmlns:r="` + nsRelationships +
		`" xmlns:wp="` + nsDrawingWP +
		`" xmlns:a="` + nsDrawingMain +
		`" xmlns:pic="` + nsDrawingPicture +
		`" xmlns:mc="` + nsMarkupCompat +
		`" xmlns:asvg="` + nsSVG +
		`" mc:Ignorable="asvg"><w:body>`)

	for i := range w.doc.Sections {
		s := &w.doc.Sections[i]
		last := i == len(w.doc.Sections)-1

		for j := range s.Content {
			c := &s.Content[j]
			// The section properties of a non-final section live in the last
			// paragraph of that section. The final section's live on the body.
			// Getting this backwards produces a document that opens with every
			// page the same size, silently.
			trailing := !last && j == len(s.Content)-1 && c.Paragraph != nil
			w.writeContent(&b, c, trailing, s)
		}

		if !last && !endsWithParagraph(s.Content) {
			// A section whose last item is a table has nowhere to hang its
			// properties, so an empty paragraph carries them. Word writes the
			// same thing.
			b.WriteString(`<w:p>`)
			w.writeSectPrIn(&b, s)
			b.WriteString(`</w:p>`)
		}
	}

	if n := len(w.doc.Sections); n > 0 {
		w.writeSectPr(&b, &w.doc.Sections[n-1])
	}
	b.WriteString(`</w:body></w:document>`)
	return []byte(b.String())
}

func endsWithParagraph(content []Content) bool {
	if len(content) == 0 {
		return false
	}
	return content[len(content)-1].Paragraph != nil
}

func (w *writer) writeContent(b *strings.Builder, c *Content, trailingSection bool, s *Section) {
	switch {
	case c.Paragraph != nil:
		w.writeParagraph(b, c.Paragraph, trailingSection, s)
	case c.Table != nil:
		w.writeTable(b, c.Table)
		// A table must be followed by a paragraph or Word repairs the file:
		// two adjacent tables merge into one, and a table at the end of a cell
		// or body leaves the document without a final paragraph mark.
		b.WriteString(`<w:p/>`)
	}
}

func (w *writer) writeParagraph(b *strings.Builder, p *Paragraph, trailingSection bool, s *Section) {
	b.WriteString(`<w:p>`)
	w.writeParagraphProperties(b, p, trailingSection, s)
	for i := range p.Runs {
		w.writeRun(b, &p.Runs[i])
	}
	b.WriteString(`</w:p>`)
}

// writeParagraphProperties emits w:pPr.
//
// The element order here is not stylistic. CT_PPr is a sequence, and Word
// rejects a document whose properties are out of order — pStyle, keepNext,
// pageBreakBefore, numPr, spacing, jc, outlineLvl, rPr, sectPr. A file with them
// shuffled opens as "unreadable content", which is a failure mode this project
// has already met once.
func (w *writer) writeParagraphProperties(b *strings.Builder, p *Paragraph, trailingSection bool, s *Section) {
	needs := p.StyleID != "" || p.KeepNext || p.PageBreakBefore ||
		p.NumberingID > 0 || p.SpaceBefore != 0 || p.SpaceAfter != 0 ||
		p.Alignment != AlignInherit || p.OutlineLevel > 0 || trailingSection
	if !needs {
		return
	}

	b.WriteString(`<w:pPr>`)
	if p.StyleID != "" {
		b.WriteString(`<w:pStyle w:val="` + escapeAttr(p.StyleID) + `"/>`)
	}
	if p.KeepNext {
		b.WriteString(`<w:keepNext/>`)
	}
	if p.PageBreakBefore {
		b.WriteString(`<w:pageBreakBefore/>`)
	}
	if p.NumberingID > 0 {
		b.WriteString(`<w:numPr><w:ilvl w:val="` + strconv.Itoa(p.NumberingLevel) +
			`"/><w:numId w:val="` + strconv.Itoa(p.NumberingID) + `"/></w:numPr>`)
	}
	if p.SpaceBefore != 0 || p.SpaceAfter != 0 {
		b.WriteString(`<w:spacing`)
		if p.SpaceBefore != 0 {
			b.WriteString(` w:before="` + strconv.Itoa(twips(p.SpaceBefore)) + `"`)
		}
		if p.SpaceAfter != 0 {
			b.WriteString(` w:after="` + strconv.Itoa(twips(p.SpaceAfter)) + `"`)
		}
		b.WriteString(`/>`)
	}
	if p.Alignment != AlignInherit {
		b.WriteString(`<w:jc w:val="` + escapeAttr(string(p.Alignment)) + `"/>`)
	}
	if p.OutlineLevel > 0 {
		// Zero-based on the wire, one-based in the model, because "level 1" is
		// what an author means by a top-level heading.
		b.WriteString(`<w:outlineLvl w:val="` + strconv.Itoa(p.OutlineLevel-1) + `"/>`)
	}
	if trailingSection {
		w.writeSectPrIn(b, s)
	}
	b.WriteString(`</w:pPr>`)
}

func (w *writer) writeRun(b *strings.Builder, r *Run) {
	if r.Field != nil {
		w.writeField(b, r)
		return
	}

	b.WriteString(`<w:r>`)
	w.writeRunProperties(b, r)

	switch {
	case r.Break != BreakNone:
		if r.Break == BreakLine {
			b.WriteString(`<w:br/>`)
		} else {
			b.WriteString(`<w:br w:type="` + escapeAttr(string(r.Break)) + `"/>`)
		}
	case r.Drawing != nil:
		w.writeDrawing(b, r.Drawing)
	case r.FootnoteRef > 0:
		b.WriteString(`<w:footnoteReference w:id="` + strconv.Itoa(footnoteWireID(r.FootnoteRef)) + `"/>`)
	case r.Tab:
		b.WriteString(`<w:tab/>`)
	case r.Text != "":
		// xml:space="preserve" on every text node, unconditionally. Leading and
		// trailing whitespace is content, and deciding case by case whether to
		// preserve it is how a writer comes to eat a space it was given.
		b.WriteString(`<w:t xml:space="preserve">` + escapeText(r.Text) + `</w:t>`)
	}
	b.WriteString(`</w:r>`)
}

// writeRunProperties emits w:rPr, in CT_RPr's required order: rStyle, rFonts,
// b, i, u, vertAlign, color, sz, szCs, highlight.
func (w *writer) writeRunProperties(b *strings.Builder, r *Run) {
	p := r.Properties
	if r.StyleID == "" && p.IsZero() {
		return
	}

	b.WriteString(`<w:rPr>`)
	if r.StyleID != "" {
		b.WriteString(`<w:rStyle w:val="` + escapeAttr(r.StyleID) + `"/>`)
	}
	if p.Font != "" {
		f := escapeAttr(p.Font)
		// All four script slots, because a run whose complex-script or
		// east-Asian family is unset falls back to a different face than its
		// Latin neighbour — a subtle defect that only shows on some machines.
		b.WriteString(`<w:rFonts w:ascii="` + f + `" w:hAnsi="` + f +
			`" w:cs="` + f + `" w:eastAsia="` + f + `"/>`)
	}
	if p.Bold {
		b.WriteString(`<w:b/><w:bCs/>`)
	}
	if p.Italic {
		b.WriteString(`<w:i/><w:iCs/>`)
	}
	if p.Color != "" {
		b.WriteString(`<w:color w:val="` + escapeAttr(p.Color) + `"/>`)
	}
	if p.SizeEMU > 0 {
		size := strconv.Itoa(halfPoints(p.SizeEMU))
		// szCs sets the complex-script size. Omitting it lets a complex-script
		// run fall back to a different size than its Latin neighbour.
		b.WriteString(`<w:sz w:val="` + size + `"/><w:szCs w:val="` + size + `"/>`)
	}
	if p.Underline {
		b.WriteString(`<w:u w:val="single"/>`)
	}
	if p.Highlight != "" {
		b.WriteString(`<w:shd w:val="clear" w:color="auto" w:fill="` + escapeAttr(p.Highlight) + `"/>`)
	}
	switch {
	case p.Superscript:
		b.WriteString(`<w:vertAlign w:val="superscript"/>`)
	case p.Subscript:
		b.WriteString(`<w:vertAlign w:val="subscript"/>`)
	}
	b.WriteString(`</w:rPr>`)
}

// writeField emits a Word field as its three-run begin/instruction/end form.
//
// A field is not one element. Word models it as a run holding a begin
// character, a run holding the instruction text, an optional cached result, and
// a run holding the end character. Emitting a single element produces a
// document that opens with the field text visible as literal prose.
func (w *writer) writeField(b *strings.Builder, r *Run) {
	f := r.Field

	b.WriteString(`<w:r>`)
	w.writeRunProperties(b, r)
	b.WriteString(`<w:fldChar w:fldCharType="begin"`)
	if f.Dirty {
		// The TOC's whole design: Vellum does not paginate WordprocessingML,
		// Word does, so a page-number table Vellum computed would disagree with
		// the document it indexes. Marking the field dirty asks Word to build
		// it on open, which is the only answer that is right.
		b.WriteString(` w:dirty="true"`)
	}
	b.WriteString(`/></w:r>`)

	b.WriteString(`<w:r>`)
	w.writeRunProperties(b, r)
	b.WriteString(`<w:instrText xml:space="preserve">` + escapeText(f.Instruction) + `</w:instrText></w:r>`)

	b.WriteString(`<w:r>`)
	w.writeRunProperties(b, r)
	b.WriteString(`<w:fldChar w:fldCharType="separate"/></w:r>`)

	b.WriteString(`<w:r>`)
	w.writeRunProperties(b, r)
	b.WriteString(`<w:t xml:space="preserve">` + escapeText(f.Result) + `</w:t></w:r>`)

	b.WriteString(`<w:r>`)
	w.writeRunProperties(b, r)
	b.WriteString(`<w:fldChar w:fldCharType="end"/></w:r>`)
}

// footnoteWireID converts a one-based model index to Word's footnote id.
//
// Word reserves ids -1 and 0 for the separator and continuation-separator
// footnotes, which every document carries whether or not it has footnotes of
// its own. Real footnotes therefore start at 1, which the model's one-based
// index already matches — this function exists to make that relationship
// explicit rather than incidental.
func footnoteWireID(index int) int { return index }
