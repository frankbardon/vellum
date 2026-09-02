package sheet

import (
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/numfmt"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/theme"
)

// table lowers a resolved table onto consecutive rows of the current sheet.
//
// Nothing here paginates. [capability.FeatureOverflowContinue] degrades to
// "one continuous sheet" rather than to a split: a sheet has no page, so a
// table of any length is simply more rows in the sheet it started on. The
// overflow policy [overflow.PlanTable] gives the deck and PDF writers has
// nothing to plan here, because there is no capacity to plan against.
func (l *lowering) table(t *fragment.Table, sectionIndex int, sectionID string, blockIndex int) error {
	l.ensureSheet()

	where := map[string]any{
		"section_index": sectionIndex, "section_id": sectionID, "block_index": blockIndex,
	}

	stubWidth := t.RowHeaders.Depth()
	total := t.GridWidth()
	if total == 0 {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_INVALID,
			"the table has no columns", where)
	}
	bodyWidth := total - stubWidth

	banner := t.ColumnHeaders.Levels()
	stub := t.RowHeaders.StubRows(stubWidth)
	annotated := tableHasAnnotations(t)

	startRow := l.row
	const startCol = 1

	for lvl, level := range banner {
		row := startRow + lvl
		col := startCol

		if stubWidth > 0 {
			l.mergeBlock(row, col, 1, stubWidth, CellEmpty, l.headerFormat(fragment.TextStyle{}), "")
			col += stubWidth
		}

		covered := 0
		for i := range level {
			node := &level[i]
			span := node.Span
			if span < 1 {
				span = 1
			}
			physical := span
			if annotated {
				physical = span * 2
			}
			l.mergeBlock(row, col, 1, physical, CellText, l.headerFormat(node.Style), node.Label)
			col += physical
			covered += span
		}

		if covered != bodyWidth {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_HEADER_SPAN_MISMATCH,
				"a column banner row does not tile the table's grid",
				withValue(withValue(where, "row_span", covered), "grid_width", bodyWidth))
		}
	}
	bannerRows := len(banner)

	for i := range t.Body {
		row := startRow + bannerRows + i
		col := startCol

		if stubWidth > 0 {
			if i < len(stub) {
				l.stubRow(row, col, stub[i])
			} else {
				l.mergeBlock(row, col, 1, stubWidth, CellEmpty, l.styles.format(CellFormat{}), "")
			}
			col += stubWidth
		}

		covered := 0
		for j := range t.Body[i] {
			cell := &t.Body[i][j]
			span := cell.ColSpan
			if span < 1 {
				span = 1
			}
			l.bodyCell(row, col, span, cell, annotated)
			physical := span
			if annotated {
				physical = span * 2
			}
			col += physical
			covered += span
		}

		if covered != bodyWidth {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_ROW_ARITY,
				"a body row does not tile the table's grid",
				withValue(withValue(withValue(where, "row", i), "row_span", covered), "grid_width", bodyWidth))
		}
	}

	totalPhysical := stubWidth + bodyWidth
	if annotated {
		totalPhysical = stubWidth + bodyWidth*2
	}
	lastBodyRow := startRow + bannerRows + len(t.Body) - 1

	l.current.Freeze = &FreezePane{
		Rows: startRow + bannerRows - 1,
		Cols: startCol + stubWidth - 1,
	}
	if len(t.Body) > 0 && bodyWidth > 0 {
		l.current.AutoFilter = &AutoFilter{
			FromRow: startRow + bannerRows - 1,
			FromCol: startCol + stubWidth,
			ToRow:   lastBodyRow,
			ToCol:   startCol + totalPhysical - 1,
		}
	}

	l.row = lastBodyRow + 1

	if t.Caption != nil {
		l.caption(t.Caption)
	}
	return nil
}

// mergeBlock writes one styled block spanning (rowSpan, colSpan) grid
// positions, top-left carrying the value and every covered position written
// individually with the same style.
//
// Every covered position, not just the corners. A merged region left sparse
// is a fill or a hairline that stops partway across the range in the readers
// that draw what is actually there rather than what the merge implies — the
// same lesson pptx table banners and stubs already pinned, here for the same
// reason.
func (l *lowering) mergeBlock(row, col, rowSpan, colSpan int, kind CellKind, styleID int, text string) {
	for r := 0; r < rowSpan; r++ {
		for c := 0; c < colSpan; c++ {
			v := CellValue{}
			if r == 0 && c == 0 {
				if kind == CellText {
					v = Text(text)
				}
			}
			l.setCell(row+r, col+c, v, styleID)
		}
	}
	if rowSpan > 1 || colSpan > 1 {
		l.current.Merges = append(l.current.Merges, Merge{
			FromRow: row, FromCol: col, ToRow: row + rowSpan - 1, ToCol: col + colSpan - 1,
		})
	}
	if kind == CellText {
		l.measure(col, text, false)
	}
}

// setCell writes one cell into the sheet being built, appending to the row it
// belongs to (creating the row if this is its first cell).
//
// Table rows are dense across the columns a table's own grid covers, so this
// always finds or creates the target row rather than assuming append-only
// order — a merge's continuation cells and a body row's cells interleave as
// this function is called left to right, but a caption or a later table's
// blank-stub fallback can still call it out of the strict left-to-right order
// [lowering.place] gets to assume for flow content.
func (l *lowering) setCell(row, col int, v CellValue, styleID int) {
	for i := range l.current.Rows {
		if l.current.Rows[i].Index == row {
			l.current.Rows[i].Cells = append(l.current.Rows[i].Cells, Cell{Column: col, Value: v, StyleID: styleID})
			return
		}
	}
	l.current.Rows = append(l.current.Rows, Row{Index: row, Cells: []Cell{{Column: col, Value: v, StyleID: styleID}}})
}

// stubRow writes one body row's slice of the row-header stub.
//
// A merge that began above becomes a written-but-blank continuation cell
// rather than an absent one — see [lowering.mergeBlock]'s doc comment — and
// only the row a merge begins on registers the [Merge] itself.
func (l *lowering) stubRow(row, col int, cells []fragment.StubCell) {
	for _, c := range cells {
		format := l.styles.format(CellFormat{
			FontIndex:   l.styles.font(l.fontFrom(c.Style, true)),
			FillIndex:   l.styles.fill(Fill{Color: c.Style.Background}),
			VerticalTop: c.Rows > 1,
		})
		switch {
		case c.First && c.Rows > 1:
			// [lowering.stubRow] is called once per body row, in order, so a
			// merge's own continuation rows write themselves on the later
			// calls that reach the `default` arm below — this call writes
			// only the label and registers the range. Writing the whole span
			// here, the way [lowering.mergeBlock] does for a banner cell,
			// would double-write every continuation row when this function
			// is called again for it.
			l.setCell(row, col+c.Column, Text(c.Label), format)
			l.measure(col+c.Column, c.Label, false)
			l.current.Merges = append(l.current.Merges, Merge{
				FromRow: row, FromCol: col + c.Column, ToRow: row + c.Rows - 1, ToCol: col + c.Column,
			})
		case c.First:
			l.setCell(row, col+c.Column, Text(c.Label), format)
			l.measure(col+c.Column, c.Label, false)
		default:
			// A continuation row of a merge that began above. The merge
			// itself was registered when the owning row was written; this
			// position still needs a cell of its own, styled to match, so the
			// fill and the border draw across the whole span rather than
			// stopping partway through it.
			l.setCell(row, col+c.Column, CellValue{}, format)
		}
	}
}

// bodyCell writes one resolved grid cell, its annotations and its comment.
//
// An annotation attaches to the value rather than replacing it. Per
// [FeatureTableCellAnnotation]'s declared degradation, it becomes "text
// appended to the cell, with the typed value preserved in a neighbouring
// column" — the only writer in this library that can keep both, because it is
// the only one whose cell is a typed, sortable value rather than text a
// reader can only look at. A note-positioned annotation is not appended at
// all: it becomes a [Comment] on the value cell itself, which is a richer
// channel than any other writer's inline run and the one place in this
// library where the degradation for a note is not "dropped".
func (l *lowering) bodyCell(row, col, span int, c *fragment.Cell, annotated bool) {
	numFmt := 0
	if c.FormatCode != "" {
		// Applied whenever the resolved cell carries a code, not only for a
		// date. A percent or a currency is exactly as much a live, formatted
		// number as a date is — the code is xlsx's own vocabulary regardless
		// of what kind of value it is dressing.
		numFmt = l.styles.numFmt(c.FormatCode)
	}
	format := l.styles.format(CellFormat{
		FontIndex:   l.styles.font(l.fontFrom(c.Style, false)),
		FillIndex:   l.styles.fill(Fill{Color: l.cellFill(c)}),
		NumFmtIndex: numFmt,
	})

	value := cellValueFrom(c.Value, c.Text)
	physical := span
	if annotated {
		physical = span * 2
	}

	if span > 1 || (annotated && physical > 1) {
		valueSpan := physical
		if annotated {
			valueSpan--
		}
		l.mergeValueBlock(row, col, valueSpan, value, format)
	} else {
		l.setCell(row, col, value, format)
	}
	if text := valueText(value); text != "" {
		l.measure(col, text, false)
	}

	var prefix, suffix, superscript []fragment.Annotation
	var note *fragment.Annotation
	for i := range c.Annotations {
		a := &c.Annotations[i]
		switch a.Position {
		case spec.AnnotationPrefix:
			prefix = append(prefix, *a)
		case spec.AnnotationSuffix:
			suffix = append(suffix, *a)
		case spec.AnnotationNote:
			note = a
		default:
			superscript = append(superscript, *a)
		}
	}

	if note != nil {
		l.current.Comments = append(l.current.Comments, Comment{Row: row, Col: col, Text: note.Text})
	}

	if annotated {
		text := joinAnnotations(prefix, suffix, superscript)
		annotationCol := col + physical - 1
		muted := l.styles.format(CellFormat{FontIndex: l.styles.font(Font{
			Name: l.theme.BodyFont, SizeEMU: l.theme.BodySize, Color: l.theme.MutedColor,
		})})
		l.setCell(row, annotationCol, Text(text), muted)
		l.measure(annotationCol, text, false)
	}
}

// mergeValueBlock writes a spanning value cell: the value at the top-left and
// a blank, matching-style cell at every other covered position.
func (l *lowering) mergeValueBlock(row, col, span int, v CellValue, styleID int) {
	l.setCell(row, col, v, styleID)
	for c := 1; c < span; c++ {
		l.setCell(row, col+c, CellValue{}, styleID)
	}
	if span > 1 {
		l.current.Merges = append(l.current.Merges, Merge{FromRow: row, FromCol: col, ToRow: row, ToCol: col + span - 1})
	}
}

// caption writes a table's caption below it, as one wrapped, muted cell —
// following the same shape [lowering.paragraph] gives an ordinary text block,
// styled with the theme's muted colour so it reads as an aside rather than as
// another row of data.
func (l *lowering) caption(p *fragment.Paragraph) {
	text := p.Text()
	format := l.styles.format(CellFormat{
		WrapText:  true,
		FontIndex: l.styles.font(Font{Name: l.theme.BodyFont, SizeEMU: l.theme.BodySize, Color: l.theme.MutedColor, Italic: true}),
	})
	l.setCell(l.row, 1, Text(text), format)
	l.row++
}

// cellFill decides a body cell's background.
//
// Only where the cell's own resolved style cannot: a margin or total row is
// distinguishable from data by its class rather than by its numbers, which is
// why the class exists — a consumer marks the row and Vellum renders the
// distinction without learning what a margin is.
func (l *lowering) cellFill(c *fragment.Cell) string {
	if c.Style.Background != "" {
		return c.Style.Background
	}
	switch c.Class {
	case spec.CellMargin, spec.CellTotal:
		return l.doc.Palette.Color(theme.ColorTableStripe, "")
	}
	return ""
}

// headerFormat interns a header band cell's style: the resolved style a
// [fragment.HeaderNode] carries, or the plain header fill for a corner cell
// that has no node of its own.
func (l *lowering) headerFormat(ts fragment.TextStyle) int {
	fill := ts.Background
	if fill == "" {
		fill = l.doc.Palette.Color(theme.ColorTableHeaderBackground, "")
	}
	font := l.fontFrom(ts, true)
	if ts.Color == "" {
		font.Color = l.doc.Palette.Color(theme.ColorTableHeaderText, "")
	}
	font.Bold = true
	return l.styles.format(CellFormat{
		FontIndex: l.styles.font(font),
		FillIndex: l.styles.fill(Fill{Color: fill}),
	})
}

// fontFrom resolves a font from a resolved character style, falling back to
// the table's own body font and size where the style carries none — which a
// corner or a continuation cell's borrowed style sometimes does not.
func (l *lowering) fontFrom(ts fragment.TextStyle, header bool) Font {
	name := l.theme.BodyFont
	if ts.FaceIndex >= 0 && ts.FaceIndex < len(l.doc.Fonts) {
		name = l.doc.Fonts[ts.FaceIndex].Family
	}
	size := ts.SizeEMU
	if size == 0 {
		size = l.doc.Scale.TableBody
	}
	if size == 0 {
		size = l.theme.BodySize
	}
	color := ts.Color
	if color == "" {
		color = l.theme.TextColor
	}
	return Font{Name: name, SizeEMU: size, Color: color, Bold: header || ts.Bold, Italic: ts.Italic}
}

// tableHasAnnotations reports whether any body cell carries a value-adjacent
// annotation: prefix, suffix or superscript.
//
// A note-positioned annotation does not count. It never occupies a
// neighbouring column at all — see [lowering.bodyCell] — it becomes a
// [Comment] on the value cell itself, which is a complete channel of its own.
// A table using only note annotations that still doubled every column would
// be paying for a neighbouring column nothing ever writes into.
//
// The doubling this decides is applied uniformly across the whole table or
// not at all — never per column depending on which columns happen to have
// one — which is what keeps the doubled structure predictable rather than a
// shape that changes with the data.
func tableHasAnnotations(t *fragment.Table) bool {
	for i := range t.Body {
		for _, a := range t.Body[i] {
			for _, an := range a.Annotations {
				if an.Position != spec.AnnotationNote {
					return true
				}
			}
		}
	}
	return false
}

// joinAnnotations concatenates the value-adjacent annotation positions —
// prefix, then suffix, then superscript — with a space. A note-positioned
// annotation never reaches here; see [lowering.bodyCell].
func joinAnnotations(prefix, suffix, superscript []fragment.Annotation) string {
	var out string
	add := func(list []fragment.Annotation) {
		for _, a := range list {
			if out != "" && a.Text != "" {
				out += " "
			}
			out += a.Text
		}
	}
	add(prefix)
	add(suffix)
	add(superscript)
	return out
}

// cellValueFrom converts a resolved typed value into a live cell value,
// falling back to the cell's rendered text when resolution left no typed
// value at all — an annotation's own text, or a cell the specification set
// with a bare string and no format code.
func cellValueFrom(v *numfmt.Value, text string) CellValue {
	if v == nil {
		return Text(text)
	}
	switch v.Kind {
	case numfmt.KindNumber:
		return Number(v.Number)
	case numfmt.KindDate:
		return Date(numfmt.Serial(v.Time))
	case numfmt.KindBool:
		return Bool(v.Bool)
	case numfmt.KindText:
		return Text(v.Text)
	default:
		return CellValue{}
	}
}

// valueText returns what a cell value reads as, for the column-width
// heuristic. A live number is measured by its rendered text length even
// though the cell does not store that text, because the heuristic is about
// what a reader sees in the column, not about what is in the shared string
// table.
func valueText(v CellValue) string {
	switch v.Kind {
	case CellText:
		return v.Text
	case CellNumber, CellDate:
		return formatNumber(v.Number)
	case CellBool:
		if v.Bool {
			return "TRUE"
		}
		return "FALSE"
	}
	return ""
}

// withValue returns a copy of a detail map with one more key.
func withValue(where map[string]any, key string, value any) map[string]any {
	out := make(map[string]any, len(where)+1)
	for k, v := range where {
		out[k] = v
	}
	out[key] = value
	return out
}
