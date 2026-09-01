package doc

import (
	"strconv"
	"strings"
)

// writeSectPr emits a body-level w:sectPr.
func (w *writer) writeSectPr(b *strings.Builder, s *Section) {
	b.WriteString(`<w:sectPr>`)
	w.writeSectPrBody(b, s)
	b.WriteString(`</w:sectPr>`)
}

// writeSectPrIn emits a paragraph-level w:sectPr.
func (w *writer) writeSectPrIn(b *strings.Builder, s *Section) {
	b.WriteString(`<w:sectPr>`)
	w.writeSectPrBody(b, s)
	b.WriteString(`</w:sectPr>`)
}

// writeSectPrBody emits the section properties, in CT_SectPr's required order:
// headerReference, footerReference, type, pgSz, pgMar, cols, titlePg.
func (w *writer) writeSectPrBody(b *strings.Builder, s *Section) {
	if id, ok := w.headerRel(s.HeaderID); ok {
		b.WriteString(`<w:headerReference w:type="default" r:id="` + id + `"/>`)
	}
	if id, ok := w.footerRel(s.FooterID); ok {
		b.WriteString(`<w:footerReference w:type="default" r:id="` + id + `"/>`)
	}
	if s.Type != SectionNextPage {
		b.WriteString(`<w:type w:val="` + escapeAttr(string(s.Type)) + `"/>`)
	}

	b.WriteString(`<w:pgSz w:w="` + strconv.Itoa(twips(s.Page.Width)) +
		`" w:h="` + strconv.Itoa(twips(s.Page.Height)) + `"`)
	if s.Page.Landscape {
		b.WriteString(` w:orient="landscape"`)
	}
	b.WriteString(`/>`)

	b.WriteString(`<w:pgMar w:top="` + strconv.Itoa(twips(s.Page.MarginTop)) +
		`" w:right="` + strconv.Itoa(twips(s.Page.MarginRight)) +
		`" w:bottom="` + strconv.Itoa(twips(s.Page.MarginBottom)) +
		`" w:left="` + strconv.Itoa(twips(s.Page.MarginLeft)) +
		`" w:header="` + strconv.Itoa(twips(s.Page.MarginHeader)) +
		`" w:footer="` + strconv.Itoa(twips(s.Page.MarginFooter)) +
		`" w:gutter="0"/>`)

	b.WriteString(`<w:cols w:space="708"/>`)
	if s.TitlePage {
		b.WriteString(`<w:titlePg/>`)
	}
}

// writeTable emits a w:tbl.
func (w *writer) writeTable(b *strings.Builder, t *Table) {
	b.WriteString(`<w:tbl><w:tblPr>`)
	if t.StyleID != "" {
		b.WriteString(`<w:tblStyle w:val="` + escapeAttr(t.StyleID) + `"/>`)
	}
	b.WriteString(`<w:tblW w:w="` + strconv.Itoa(twips(sum(t.Grid))) + `" w:type="dxa"/>`)
	if !t.AutoFit {
		// Fixed layout, deliberately. Autofit makes Word re-measure the table on
		// open against whatever fonts the machine has, so the same document
		// renders differently for different readers.
		b.WriteString(`<w:tblLayout w:type="fixed"/>`)
	}
	b.WriteString(`<w:tblLook w:val="04A0" w:firstRow="1" w:lastRow="0" w:firstColumn="1" w:lastColumn="0" w:noHBand="0" w:noVBand="1"/>`)
	// tblCaption follows tblLook. CT_TblPrBase is a sequence, and an element
	// out of order is the defect class that reads to a user as "Word found
	// unreadable content" — everything looks present and the reader refuses it.
	if t.Caption != "" {
		b.WriteString(`<w:tblCaption w:val="` + escapeAttr(t.Caption) + `"/>`)
	}
	b.WriteString(`</w:tblPr><w:tblGrid>`)
	for _, col := range t.Grid {
		b.WriteString(`<w:gridCol w:w="` + strconv.Itoa(twips(col)) + `"/>`)
	}
	b.WriteString(`</w:tblGrid>`)

	for i := range t.Rows {
		w.writeTableRow(b, &t.Rows[i])
	}
	b.WriteString(`</w:tbl>`)
}

func (w *writer) writeTableRow(b *strings.Builder, r *TableRow) {
	b.WriteString(`<w:tr>`)
	if r.Header || r.CantSplit || r.HeightEMU > 0 {
		b.WriteString(`<w:trPr>`)
		if r.CantSplit {
			b.WriteString(`<w:cantSplit/>`)
		}
		if r.HeightEMU > 0 {
			b.WriteString(`<w:trHeight w:val="` + strconv.Itoa(twips(r.HeightEMU)) + `" w:hRule="atLeast"/>`)
		}
		if r.Header {
			// tblHeader is what makes a banner repeat when the table breaks
			// across pages. Without it a multi-page table shows its headings
			// once, on the first page.
			b.WriteString(`<w:tblHeader/>`)
		}
		b.WriteString(`</w:trPr>`)
	}
	for i := range r.Cells {
		w.writeTableCell(b, &r.Cells[i])
	}
	b.WriteString(`</w:tr>`)
}

// writeTableCell emits a w:tc, whose properties follow CT_TcPr's order:
// tcW, gridSpan, vMerge, shd, vAlign.
func (w *writer) writeTableCell(b *strings.Builder, c *TableCell) {
	b.WriteString(`<w:tc><w:tcPr>`)
	b.WriteString(`<w:tcW w:w="` + strconv.Itoa(twips(c.WidthEMU)) + `" w:type="dxa"/>`)
	if c.span() > 1 {
		b.WriteString(`<w:gridSpan w:val="` + strconv.Itoa(c.span()) + `"/>`)
	}
	switch c.VerticalMerge {
	case MergeRestart:
		b.WriteString(`<w:vMerge w:val="restart"/>`)
	case MergeContinue:
		b.WriteString(`<w:vMerge/>`)
	}
	if c.Fill != "" {
		b.WriteString(`<w:shd w:val="clear" w:color="auto" w:fill="` + escapeAttr(c.Fill) + `"/>`)
	}
	if c.VerticalAlign != VAlignTop {
		b.WriteString(`<w:vAlign w:val="` + escapeAttr(string(c.VerticalAlign)) + `"/>`)
	}
	b.WriteString(`</w:tcPr>`)

	if len(c.Content) == 0 {
		// A cell must contain at least one block-level element. An empty one
		// makes Word declare the document unreadable.
		b.WriteString(`<w:p/>`)
	}
	for i := range c.Content {
		w.writeContent(b, &c.Content[i], false, nil)
	}
	b.WriteString(`</w:tc>`)
}

// writeDrawing emits an inline image.
//
// The docPr id is derived from the media index rather than from a counter, so
// two documents with the same pictures produce the same ids however the writer
// walked them.
func (w *writer) writeDrawing(b *strings.Builder, d *Drawing) {
	id := strconv.Itoa(d.MediaIndex + 1)
	primaryRel, ok := w.imageRel(d.MediaIndex)
	if !ok {
		return
	}

	// When a vector has a raster fallback, the raster goes in the blip and the
	// vector goes in an extension beside it. That ordering is the whole trick:
	// a reader that does not understand the extension shows the raster, and one
	// that does prefers the vector.
	blipRel := primaryRel
	var svgRel string
	if d.FallbackIndex >= 0 {
		if rasterRel, ok := w.imageRel(d.FallbackIndex); ok {
			blipRel, svgRel = rasterRel, primaryRel
		}
	}

	width := strconv.FormatInt(d.WidthEMU, 10)
	height := strconv.FormatInt(d.HeightEMU, 10)

	b.WriteString(`<w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0">`)
	b.WriteString(`<wp:extent cx="` + width + `" cy="` + height + `"/>`)
	b.WriteString(`<wp:effectExtent l="0" t="0" r="0" b="0"/>`)
	b.WriteString(`<wp:docPr id="` + id + `" name="` + escapeAttr(d.Name) + `"`)
	if d.AltText != "" {
		b.WriteString(` descr="` + escapeAttr(d.AltText) + `"`)
	}
	b.WriteString(`/>`)
	b.WriteString(`<wp:cNvGraphicFramePr><a:graphicFrameLocks xmlns:a="` + nsDrawingMain + `" noChangeAspect="1"/></wp:cNvGraphicFramePr>`)
	b.WriteString(`<a:graphic xmlns:a="` + nsDrawingMain + `"><a:graphicData uri="` + nsDrawingPicture + `">`)
	b.WriteString(`<pic:pic xmlns:pic="` + nsDrawingPicture + `">`)
	b.WriteString(`<pic:nvPicPr><pic:cNvPr id="` + id + `" name="` + escapeAttr(d.Name) + `"`)
	if d.AltText != "" {
		b.WriteString(` descr="` + escapeAttr(d.AltText) + `"`)
	}
	b.WriteString(`/><pic:cNvPicPr/></pic:nvPicPr>`)

	b.WriteString(`<pic:blipFill><a:blip r:embed="` + blipRel + `">`)
	if svgRel != "" {
		b.WriteString(`<a:extLst><a:ext uri="{96DAC541-7B7A-43D3-8B79-37D633B846F1}">`)
		b.WriteString(`<asvg:svgBlip xmlns:asvg="` + nsSVG + `" r:embed="` + svgRel + `"/>`)
		b.WriteString(`</a:ext></a:extLst>`)
	}
	b.WriteString(`</a:blip><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`)

	b.WriteString(`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="` + width + `" cy="` + height + `"/></a:xfrm>`)
	b.WriteString(`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>`)
	b.WriteString(`</pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing>`)
}

// stylesXML emits word/styles.xml.
func (w *writer) stylesXML() []byte {
	s := &w.doc.Styles

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<w:styles xmlns:w="` + nsWordprocessing + `">`)

	b.WriteString(`<w:docDefaults><w:rPrDefault><w:rPr>`)
	if s.DefaultFont != "" {
		f := escapeAttr(s.DefaultFont)
		b.WriteString(`<w:rFonts w:ascii="` + f + `" w:hAnsi="` + f + `" w:cs="` + f + `" w:eastAsia="` + f + `"/>`)
	}
	if s.DefaultSizeEMU > 0 {
		size := strconv.Itoa(halfPoints(s.DefaultSizeEMU))
		b.WriteString(`<w:sz w:val="` + size + `"/><w:szCs w:val="` + size + `"/>`)
	}
	b.WriteString(`</w:rPr></w:rPrDefault><w:pPrDefault><w:pPr>`)
	b.WriteString(`<w:spacing w:line="` + strconv.Itoa(lineRule(s.DefaultLineHeight)) + `" w:lineRule="auto"/>`)
	b.WriteString(`</w:pPr></w:pPrDefault></w:docDefaults>`)

	for i := range s.Paragraph {
		w.writeParagraphStyle(&b, &s.Paragraph[i])
	}
	for i := range s.Character {
		c := &s.Character[i]
		b.WriteString(`<w:style w:type="character" w:styleId="` + escapeAttr(c.ID) + `">`)
		b.WriteString(`<w:name w:val="` + escapeAttr(c.Name) + `"/>`)
		w.writeStyleRunProperties(&b, c.Run)
		b.WriteString(`</w:style>`)
	}
	w.writeTableStyle(&b, &s.Table)

	b.WriteString(`</w:styles>`)
	return []byte(b.String())
}

// writeParagraphStyle emits one w:style, in CT_Style's required order: name,
// basedOn, next, qFormat, pPr, rPr.
func (w *writer) writeParagraphStyle(b *strings.Builder, s *ParagraphStyle) {
	b.WriteString(`<w:style w:type="paragraph" w:styleId="` + escapeAttr(s.ID) + `"`)
	if s.ID == StyleNormal {
		b.WriteString(` w:default="1"`)
	}
	b.WriteString(`><w:name w:val="` + escapeAttr(s.Name) + `"/>`)
	if s.BasedOn != "" {
		b.WriteString(`<w:basedOn w:val="` + escapeAttr(s.BasedOn) + `"/>`)
	}
	if s.NextStyleID != "" {
		b.WriteString(`<w:next w:val="` + escapeAttr(s.NextStyleID) + `"/>`)
	}
	if s.Primary {
		// qFormat is what puts a style in Word's gallery. Without it a
		// generated document's styles exist but are invisible to the person
		// trying to restyle it, which defeats the reason for emitting styles.
		b.WriteString(`<w:qFormat/>`)
	}

	b.WriteString(`<w:pPr>`)
	if s.KeepNext {
		b.WriteString(`<w:keepNext/>`)
	}
	b.WriteString(`<w:spacing`)
	if s.SpaceBefore != 0 {
		b.WriteString(` w:before="` + strconv.Itoa(twips(s.SpaceBefore)) + `"`)
	}
	if s.SpaceAfter != 0 {
		b.WriteString(` w:after="` + strconv.Itoa(twips(s.SpaceAfter)) + `"`)
	}
	if s.LineHeight > 0 {
		b.WriteString(` w:line="` + strconv.Itoa(lineRule(s.LineHeight)) + `" w:lineRule="auto"`)
	}
	b.WriteString(`/>`)
	if s.OutlineLevel > 0 {
		b.WriteString(`<w:outlineLvl w:val="` + strconv.Itoa(s.OutlineLevel-1) + `"/>`)
	}
	b.WriteString(`</w:pPr>`)

	w.writeStyleRunProperties(b, s.Run)
	b.WriteString(`</w:style>`)
}

func (w *writer) writeStyleRunProperties(b *strings.Builder, p RunProperties) {
	if p.IsZero() {
		return
	}
	b.WriteString(`<w:rPr>`)
	if p.Font != "" {
		f := escapeAttr(p.Font)
		b.WriteString(`<w:rFonts w:ascii="` + f + `" w:hAnsi="` + f + `" w:cs="` + f + `" w:eastAsia="` + f + `"/>`)
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

func (w *writer) writeTableStyle(b *strings.Builder, t *TableStyle) {
	if t.ID == "" {
		return
	}
	b.WriteString(`<w:style w:type="table" w:styleId="` + escapeAttr(t.ID) + `">`)
	b.WriteString(`<w:name w:val="` + escapeAttr(t.Name) + `"/><w:qFormat/>`)
	// rPr before tblPr: CT_Style is a sequence, not a bag.
	w.writeStyleRunProperties(b, t.Run)
	b.WriteString(`<w:tblPr><w:tblBorders>`)
	border := `<w:%s w:val="single" w:sz="4" w:space="0" w:color="` + escapeAttr(nonEmpty(t.BorderColor, "auto")) + `"/>`
	for _, side := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		b.WriteString(strings.Replace(border, "%s", side, 1))
	}
	b.WriteString(`</w:tblBorders>`)
	b.WriteString(`<w:tblCellMar>` +
		`<w:top w:w="60" w:type="dxa"/><w:left w:w="80" w:type="dxa"/>` +
		`<w:bottom w:w="60" w:type="dxa"/><w:right w:w="80" w:type="dxa"/>` +
		`</w:tblCellMar></w:tblPr>`)
	b.WriteString(`</w:style>`)
}

// numberingXML emits word/numbering.xml.
func (w *writer) numberingXML() []byte {
	n := &w.doc.Numbering

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<w:numbering xmlns:w="` + nsWordprocessing + `">`)

	for i := range n.Abstract {
		// Abstract IDs are the slice index, so they are derived from definition
		// order rather than from a counter whose value could depend on which
		// code path built the document.
		b.WriteString(`<w:abstractNum w:abstractNumId="` + strconv.Itoa(i) + `">`)
		for level, l := range n.Abstract[i].Levels {
			b.WriteString(`<w:lvl w:ilvl="` + strconv.Itoa(level) + `">`)
			b.WriteString(`<w:start w:val="1"/>`)
			b.WriteString(`<w:numFmt w:val="` + escapeAttr(l.Format) + `"/>`)
			b.WriteString(`<w:lvlText w:val="` + escapeAttr(l.Text) + `"/>`)
			b.WriteString(`<w:lvlJc w:val="left"/>`)
			b.WriteString(`<w:pPr><w:ind w:left="` + strconv.Itoa(twips(l.IndentEMU)) +
				`" w:hanging="` + strconv.Itoa(twips(l.HangingEMU)) + `"/></w:pPr>`)
			if l.Font != "" {
				f := escapeAttr(l.Font)
				b.WriteString(`<w:rPr><w:rFonts w:ascii="` + f + `" w:hAnsi="` + f + `" w:hint="default"/></w:rPr>`)
			}
			b.WriteString(`</w:lvl>`)
		}
		b.WriteString(`</w:abstractNum>`)
	}
	for _, inst := range n.Instances {
		b.WriteString(`<w:num w:numId="` + strconv.Itoa(inst.ID) + `">`)
		b.WriteString(`<w:abstractNumId w:val="` + strconv.Itoa(inst.AbstractIndex) + `"/></w:num>`)
	}

	b.WriteString(`</w:numbering>`)
	return []byte(b.String())
}

// footnotesXML emits word/footnotes.xml.
//
// Ids -1 and 0 are reserved for the separator and continuation separator, which
// every document carries whether or not it has footnotes of its own. Omitting
// them makes Word draw a footnote with no rule above it, or repair the file.
func (w *writer) footnotesXML() []byte {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<w:footnotes xmlns:w="` + nsWordprocessing + `">`)

	b.WriteString(`<w:footnote w:type="separator" w:id="-1"><w:p><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr><w:r><w:separator/></w:r></w:p></w:footnote>`)
	b.WriteString(`<w:footnote w:type="continuationSeparator" w:id="0"><w:p><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr><w:r><w:continuationSeparator/></w:r></w:p></w:footnote>`)

	for i := range w.doc.Footnotes {
		b.WriteString(`<w:footnote w:id="` + strconv.Itoa(footnoteWireID(i+1)) + `">`)
		if len(w.doc.Footnotes[i].Content) == 0 {
			b.WriteString(`<w:p/>`)
		}
		for j := range w.doc.Footnotes[i].Content {
			w.writeContent(&b, &w.doc.Footnotes[i].Content[j], false, nil)
		}
		b.WriteString(`</w:footnote>`)
	}

	b.WriteString(`</w:footnotes>`)
	return []byte(b.String())
}

// headerFooterXML emits a header or footer part.
func (w *writer) headerFooterXML(element string, hf *HeaderFooter) []byte {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<w:` + element + ` xmlns:w="` + nsWordprocessing +
		`" xmlns:r="` + nsRelationships + `">`)
	if len(hf.Content) == 0 {
		b.WriteString(`<w:p/>`)
	}
	for i := range hf.Content {
		w.writeContent(&b, &hf.Content[i], false, nil)
	}
	b.WriteString(`</w:` + element + `>`)
	return []byte(b.String())
}

// settingsXML emits word/settings.xml.
//
// It exists for one attribute: updateFields. A TOC field marked dirty is only
// recalculated if the document also asks Word to update fields on open, and a
// document with a dirty TOC and no such request opens showing the field's
// cached prompt instead of a table of contents.
func (w *writer) settingsXML() []byte {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<w:settings xmlns:w="` + nsWordprocessing + `">`)
	if w.hasDirtyField() {
		b.WriteString(`<w:updateFields w:val="true"/>`)
	}
	b.WriteString(`<w:footnotePr><w:footnote w:id="-1"/><w:footnote w:id="0"/></w:footnotePr>`)
	b.WriteString(`</w:settings>`)
	return []byte(b.String())
}

func sum(v []int64) int64 {
	total := int64(0)
	for _, n := range v {
		total += n
	}
	return total
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
