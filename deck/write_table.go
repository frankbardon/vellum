package deck

import (
	"strconv"
	"strings"
)

// tableStylesXML emits ppt/tableStyles.xml.
//
// The part has to exist and the table has to name it. A table referencing a
// style the package does not carry draws with no fills and no borders at all,
// which reads as a table nobody styled rather than as an error — so a deck with
// tables and no style part is worse than one with no tables.
//
// The child order below is CT_TableStyle's sequence, and it is not the order it
// reads in: firstRow comes after lastRow, near the end. That is the schema's
// and it is the sort of thing that produces a slide a reader reports as
// unreadable, with every element present and nothing malformed.
func (w *writer) tableStylesXML() []byte {
	s := w.deck.TableStyle

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<a:tblStyleLst xmlns:a="` + nsDrawingMain +
		`" def="` + escapeAttr(TableStyleID) + `">`)
	b.WriteString(`<a:tblStyle styleId="` + escapeAttr(TableStyleID) +
		`" styleName="` + escapeAttr(styleNameOr(s.Name)) + `">`)

	// The whole table: body text colour, and a hairline on every edge and
	// between every cell.
	b.WriteString(`<a:wholeTbl>`)
	writeTableTextStyle(&b, s.BodyText, false)
	b.WriteString(`<a:tcStyle>`)
	writeTableBorders(&b, s.RuleColor, s.RuleWidth)
	b.WriteString(`<a:fill><a:noFill/></a:fill>`)
	b.WriteString(`</a:tcStyle>`)
	b.WriteString(`</a:wholeTbl>`)

	// The banding. Only the odd band is filled; the even one takes the whole
	// table's fill, which is what makes banding read as a tint rather than as
	// two alternating colours.
	if s.BandFill != "" {
		b.WriteString(`<a:band1H><a:tcStyle><a:fill>`)
		writeSolidFill(&b, s.BandFill)
		b.WriteString(`</a:fill></a:tcStyle></a:band1H>`)
	}

	// firstRow last, because that is where CT_TableStyle's sequence puts it.
	if s.HeaderFill != "" || s.HeaderText != "" {
		b.WriteString(`<a:firstRow>`)
		writeTableTextStyle(&b, s.HeaderText, true)
		b.WriteString(`<a:tcStyle>`)
		writeTableBorders(&b, s.RuleColor, s.RuleWidth)
		b.WriteString(`<a:fill>`)
		writeSolidFill(&b, s.HeaderFill)
		b.WriteString(`</a:fill>`)
		b.WriteString(`</a:tcStyle>`)
		b.WriteString(`</a:firstRow>`)
	}

	b.WriteString(`</a:tblStyle>`)
	b.WriteString(`</a:tblStyleLst>`)
	return []byte(b.String())
}

// writeTableTextStyle emits a:tcTxStyle.
//
// The font is a reference to the theme's minor face rather than a family name,
// so a table follows the theme like everything else. fontRef carries a colour
// of its own which nothing reads here and which the schema requires present.
func writeTableTextStyle(b *strings.Builder, color string, bold bool) {
	b.WriteString(`<a:tcTxStyle`)
	if bold {
		b.WriteString(` b="on"`)
	}
	b.WriteString(`>`)
	b.WriteString(`<a:fontRef idx="minor"><a:scrgbClr r="0" g="0" b="0"/></a:fontRef>`)
	writeSchemeOrLiteral(b, color)
	b.WriteString(`</a:tcTxStyle>`)
}

// writeTableBorders emits a:tcBdr with the same hairline on every edge.
//
// Every edge, in the schema's order. A table with borders on some edges and not
// others is a design decision, and this is not the layer that makes it: the
// theme declares one rule colour, so one rule is what it gets.
func writeTableBorders(b *strings.Builder, color string, width int64) {
	if color == "" || width <= 0 {
		return
	}
	b.WriteString(`<a:tcBdr>`)
	for _, edge := range []string{"left", "right", "top", "bottom", "insideH", "insideV"} {
		b.WriteString(`<a:` + edge + `><a:ln w="` + strconv.FormatInt(width, 10) + `" cmpd="sng">`)
		writeSolidFill(b, color)
		b.WriteString(`</a:ln></a:` + edge + `>`)
	}
	b.WriteString(`</a:tcBdr>`)
}

// writeSchemeOrLiteral emits a bare colour element, without a fill wrapper.
func writeSchemeOrLiteral(b *strings.Builder, value string) {
	if value == "" {
		return
	}
	if slot, ok := strings.CutPrefix(value, schemePrefix); ok {
		b.WriteString(`<a:schemeClr val="` + escapeAttr(slot) + `"/>`)
		return
	}
	b.WriteString(`<a:srgbClr val="` + escapeAttr(value) + `"/>`)
}

func styleNameOr(name string) string {
	if name == "" {
		return "Vellum Table"
	}
	return name
}

// writeTableFrame emits a p:graphicFrame holding a DrawingML table.
//
// PresentationML has no table shape. A table is a graphic frame whose graphic
// data carries a a:tbl, addressed by a URI that a reader matches exactly — a
// frame with the wrong URI is one PowerPoint draws as an empty box.
func (w *writer) writeTableFrame(b *strings.Builder, s *Shape, id int) {
	t := s.Table

	b.WriteString(`<p:graphicFrame>`)
	b.WriteString(`<p:nvGraphicFramePr>`)
	b.WriteString(`<p:cNvPr id="` + strconv.Itoa(id) + `" name="` +
		escapeAttr(shapeName(s, id)) + `"/>`)
	b.WriteString(`<p:cNvGraphicFramePr><a:graphicFrameLocks noGrp="1"/></p:cNvGraphicFramePr>`)
	b.WriteString(`<p:nvPr>`)
	writePlaceholder(b, s.Placeholder)
	b.WriteString(`</p:nvPr>`)
	b.WriteString(`</p:nvGraphicFramePr>`)

	// p:xfrm, not a:xfrm. A graphic frame carries its transform in the
	// presentation namespace, and the drawing-namespace element a shape uses is
	// one a reader ignores here — the table draws at the origin with no size.
	b.WriteString(`<p:xfrm>`)
	b.WriteString(`<a:off x="` + strconv.FormatInt(s.Frame.X, 10) +
		`" y="` + strconv.FormatInt(s.Frame.Y, 10) + `"/>`)
	b.WriteString(`<a:ext cx="` + strconv.FormatInt(s.Frame.Width, 10) +
		`" cy="` + strconv.FormatInt(s.Frame.Height, 10) + `"/>`)
	b.WriteString(`</p:xfrm>`)

	b.WriteString(`<a:graphic><a:graphicData uri="` + nsDrawingTable + `">`)
	b.WriteString(`<a:tbl>`)

	b.WriteString(`<a:tblPr`)
	if t.FirstRow {
		b.WriteString(` firstRow="1"`)
	}
	if t.BandedRows {
		b.WriteString(` bandRow="1"`)
	}
	b.WriteString(`>`)
	b.WriteString(`<a:tableStyleId>` + escapeText(tableStyleOr(t.StyleID)) + `</a:tableStyleId>`)
	b.WriteString(`</a:tblPr>`)

	b.WriteString(`<a:tblGrid>`)
	for _, width := range t.Columns {
		b.WriteString(`<a:gridCol w="` + strconv.FormatInt(width, 10) + `"/>`)
	}
	b.WriteString(`</a:tblGrid>`)

	for i := range t.Rows {
		w.writeTableRow(b, &t.Rows[i])
	}

	b.WriteString(`</a:tbl>`)
	b.WriteString(`</a:graphicData></a:graphic>`)
	b.WriteString(`</p:graphicFrame>`)
}

func tableStyleOr(id string) string {
	if id == "" {
		return TableStyleID
	}
	return id
}

// writeTableRow emits a:tr.
func (w *writer) writeTableRow(b *strings.Builder, r *Row) {
	b.WriteString(`<a:tr h="` + strconv.FormatInt(r.Height, 10) + `">`)
	for i := range r.Cells {
		w.writeTableCell(b, &r.Cells[i])
	}
	b.WriteString(`</a:tr>`)
}

// writeTableCell emits a:tc.
//
// The cell properties come after the text body, which is CT_TableCell's
// sequence and the reverse of how every other shape in this format is ordered.
func (w *writer) writeTableCell(b *strings.Builder, c *Cell) {
	b.WriteString(`<a:tc`)
	if n := c.span(); n > 1 {
		b.WriteString(` gridSpan="` + strconv.Itoa(n) + `"`)
	}
	if c.RowSpan > 1 {
		b.WriteString(` rowSpan="` + strconv.Itoa(c.RowSpan) + `"`)
	}
	if c.HorizontalMerge {
		b.WriteString(` hMerge="1"`)
	}
	if c.VerticalMerge {
		b.WriteString(` vMerge="1"`)
	}
	b.WriteString(`>`)

	w.writeCellTextBody(b, c.Text)

	// The insets are written on every cell rather than left to the reader,
	// because the overflow split's row height was computed from them. Letting
	// the reader supply its own would mean the capacity was computed for a
	// table Vellum did not write.
	b.WriteString(`<a:tcPr marL="` + strconv.FormatInt(cellMarginH, 10) +
		`" marR="` + strconv.FormatInt(cellMarginH, 10) +
		`" marT="` + strconv.FormatInt(cellMarginV, 10) +
		`" marB="` + strconv.FormatInt(cellMarginV, 10) + `"`)
	if c.Anchor != AnchorTop {
		b.WriteString(` anchor="` + escapeAttr(string(c.Anchor)) + `"`)
	}
	if c.Fill == "" {
		b.WriteString(`/>`)
	} else {
		b.WriteString(`>`)
		writeSolidFill(b, c.Fill)
		b.WriteString(`</a:tcPr>`)
	}

	b.WriteString(`</a:tc>`)
}

// writeCellTextBody emits a cell's a:txBody.
//
// Distinct from the shape's, because a cell's is in the drawing namespace and a
// shape's is in the presentation one. The elements inside are identical, which
// is exactly what makes the mistake easy to write and hard to see.
func (w *writer) writeCellTextBody(b *strings.Builder, t *TextBody) {
	b.WriteString(`<a:txBody><a:bodyPr/><a:lstStyle/>`)
	if t == nil || len(t.Paragraphs) == 0 {
		b.WriteString(`<a:p/>`)
	} else {
		for i := range t.Paragraphs {
			writeParagraph(b, &t.Paragraphs[i])
		}
	}
	b.WriteString(`</a:txBody>`)
}
