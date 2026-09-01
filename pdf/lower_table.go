package pdf

import (
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/overflow"
	"github.com/frankbardon/vellum/pdf/color"
	"github.com/frankbardon/vellum/pdf/object"
	"github.com/frankbardon/vellum/pdf/text"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/theme"
)

// The table geometry, in points. Stated here rather than taken from the theme
// because a theme describes type and colour, and these are the measurements of
// a rule and a gap — the same in every theme Vellum has been shown.
const (
	// cellPadX and cellPadY are the insets a cell's text sits inside.
	cellPadX = object.Real(5 * object.RealScale)
	cellPadY = object.Real(3 * object.RealScale)

	// tableRuleWidth is the thickness of a cell's hairline.
	tableRuleWidth = object.Real(object.RealScale / 2)
)

// tableCell is one measured cell, ready to place.
type tableCell struct {
	// Column is the grid column the cell starts at, and Span how many it
	// covers. There are no covered-but-present cells here: PDF has no grid to
	// tile, so a spanning cell is simply drawn over its span. That is the
	// opposite of DrawingML and unlike WordprocessingML, and it is why the
	// arity check happens during measurement, where every column still has a
	// cell of its own.
	Column, Span int

	// Rows is how many body rows the cell covers vertically. One is the
	// ordinary value; only a row-header stub uses more.
	Rows int

	// Lines are the cell's wrapped text and Leading its baseline rhythm.
	Lines   []text.Line
	Leading object.Real

	// Fill is a literal sRGB triplet, or empty for none.
	Fill string
}

// height is the vertical space the cell's own text needs, insets included.
func (c tableCell) height() object.Real {
	return object.Real(len(c.Lines))*c.Leading + 2*cellPadY
}

// tableRow is one row of cells with the height they settled on.
type tableRow struct {
	cells  []tableCell
	height object.Real
}

// table lays a resolved table out across as many pages as it needs.
//
// This is the one format where the overflow policy is honoured exactly rather
// than approximately. The three OOXML writers may not measure — Word and
// PowerPoint lay their own content out, with the fonts installed on the machine
// that opens the file, so a measured capacity would put those fonts into the
// split. Vellum lays PDF out completely, so the height of every row is a number
// it computed and will draw, and the split is the split a reader sees.
//
// The policy itself is shared: [overflow.PlanRows] holds the greedy fill and
// the deck's uniform-height planner delegates to the same loop. One
// implementation, so a boundary cannot fall in two places.
func (l *lowering) table(t *fragment.Table, sectionIndex int, sectionID string, blockIndex int) error {
	where := map[string]any{
		"section_index": sectionIndex, "section_id": sectionID, "block_index": blockIndex,
	}

	stubWidth := t.RowHeaders.Depth()
	total := t.GridWidth()
	if total == 0 {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_INVALID,
			"the table has no columns", where)
	}

	l.ensurePage()
	grid := fragment.EvenGrid(int64(l.measure()), total)

	banner, err := l.bannerRows(t, grid, stubWidth, total, where)
	if err != nil {
		return err
	}
	headerHeight := object.Real(0)
	for _, r := range banner {
		headerHeight += r.height
	}

	stub := t.RowHeaders.StubRows(stubWidth)
	heights, err := l.measureBody(t, stub, grid, stubWidth, total, where)
	if err != nil {
		return err
	}

	l.advance(points(l.doc.Scale.ParagraphBefore))
	l.ensurePage()

	// The first page's share is whatever is left under the content already on
	// it, and every page after it gets the whole text box. A table that will
	// not fit entirely does not therefore start on a page of its own: doing
	// that strands the heading above it alone at the foot of the page before,
	// which is a widow the author did not write and cannot remove.
	//
	// What it may not do is begin on a page with room for the banner and
	// nothing under it. That is not a split, it is a page of headings, so the
	// break happens first and the plan runs against a full box.
	full := l.pageBox()
	first := l.cursor - l.bottom()
	if len(heights) > 0 && first < headerHeight+object.Real(heights[0]) {
		l.flush()
		l.newPage(l.geometry)
		first = 0
	}

	plan, err := overflow.PlanRows(overflow.Rows{
		Heights:      heights,
		HeaderHeight: int64(headerHeight),
		Available:    int64(full),
		First:        int64(first),
	})
	if err != nil {
		return verr.Annotate(err, where)
	}

	for _, split := range plan {
		if split.Index > 0 {
			l.flush()
			l.newPage(l.geometry)
		}

		rows := append([]tableRow(nil), banner...)
		part := fragment.ClipStub(stub, split.From, split.Count)
		for i := split.From; i < split.From+split.Count; i++ {
			cells, err := l.bodyCells(t, part, grid, stubWidth, i-split.From, i)
			if err != nil {
				return err
			}
			rows = append(rows, tableRow{cells: cells, height: object.Real(heights[i])})
		}

		l.drawTable(rows, grid)

		l.splits = append(l.splits, TableSplit{
			SectionID:    sectionID,
			SectionIndex: sectionIndex,
			BlockIndex:   blockIndex,
			Page:         len(l.pages),
			Part:         split.Index,
			Parts:        len(plan),
			FromRow:      split.From,
			Rows:         split.Count,
			TotalRows:    len(t.Body),
			HeaderRows:   len(banner),
		})
	}

	// The caption goes under the last part, as ordinary flow. Not a row of the
	// table: a row is capacity the split would have to reserve on every page
	// for something that belongs once, at the end.
	if t.Caption != nil {
		if err := l.paragraph(t.Caption); err != nil {
			return err
		}
	}
	l.advance(points(l.doc.Scale.ParagraphAfter))
	return nil
}

// pageBox is the whole vertical space a page gives the flow.
//
// Read without the footnote reserve, because a table's continuation pages are
// pages this call is about to create and no note has been admitted to them yet.
func (l *lowering) pageBox() object.Real {
	return points(l.geometry.Height - l.geometry.MarginTop - l.geometry.MarginBottom)
}

// bannerRows measures the repeated column header, one row per level.
func (l *lowering) bannerRows(t *fragment.Table, grid []int64, stubWidth, total int,
	where map[string]any) ([]tableRow, error) {

	fill := l.doc.Palette.Color(theme.ColorTableHeaderBackground, "")

	var out []tableRow
	for _, level := range t.ColumnHeaders.Levels() {
		var cells []tableCell
		col := 0

		if stubWidth > 0 {
			// The corner above the row headers, one cell across the stub. That
			// is what an authored analytical table looks like.
			cells = append(cells, tableCell{Column: 0, Span: stubWidth, Rows: 1, Fill: fill})
			col = stubWidth
		}

		for i := range level {
			node := &level[i]
			span := node.Span
			if span < 1 {
				span = 1
			}
			cell, err := l.cell(node.Label, &node.Style, grid, col, span)
			if err != nil {
				return nil, err
			}
			cell.Fill = fill
			cells = append(cells, cell)
			col += span
		}

		if col != total {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_HEADER_SPAN_MISMATCH,
				"a column banner row does not tile the table's grid",
				withValue(withValue(where, "row_span", col), "grid_width", total))
		}
		out = append(out, l.measureRow(cells))
	}
	return out, nil
}

// measureBody measures every body row once, before any split is planned.
//
// The measurement deliberately gives every stub cell its own label, including
// the rows that continue a merge from above and carry none. That is what makes
// a row's height independent of the split: [fragment.ClipStub] restarts a merge
// at the first row of a continuation page, so a row that carried no label on
// one plan carries one on another — and a height that changed with the plan
// would be a capacity computed for a table nobody draws.
//
// The cost is that rows under a long stub label are as tall as that label needs
// even where it is not repeated. That is white space, which is visible and
// harmless; the alternative is a table that overflows the page it was measured
// to fit.
func (l *lowering) measureBody(t *fragment.Table, stub [][]fragment.StubCell, grid []int64,
	stubWidth, total int, where map[string]any) ([]int64, error) {

	fill := l.doc.Palette.Color(theme.ColorTableHeaderBackground, "")

	out := make([]int64, 0, len(t.Body))
	for i := range t.Body {
		var cells []tableCell
		col := 0

		for j := 0; j < stubWidth; j++ {
			label, style := stubOwner(stub, i, j)
			cell, err := l.cell(label, style, grid, j, 1)
			if err != nil {
				return nil, err
			}
			cell.Fill = fill
			cells = append(cells, cell)
			col++
		}

		for j := range t.Body[i] {
			c := &t.Body[i][j]
			span := c.ColSpan
			if span < 1 {
				span = 1
			}
			cell, err := l.bodyCell(c, grid, col, span)
			if err != nil {
				return nil, err
			}
			cells = append(cells, cell)
			col += span
		}

		if col != total {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_ROW_ARITY,
				"a body row does not tile the table's grid",
				withValue(withValue(withValue(where, "row", i), "row_span", col), "grid_width", total))
		}
		out = append(out, int64(l.measureRow(cells).height))
	}
	return out, nil
}

// bodyCells builds one body row's cells for drawing, merges included.
//
// Unlike the measurement pass this omits the cells a merge above covers, which
// is what PDF wants: there is no grid to tile, so a covered position is drawn
// by the cell that covers it and by nothing else.
func (l *lowering) bodyCells(t *fragment.Table, stub [][]fragment.StubCell, grid []int64,
	stubWidth, stubIndex, index int) ([]tableCell, error) {

	fill := l.doc.Palette.Color(theme.ColorTableHeaderBackground, "")

	var cells []tableCell
	col := 0

	if stubWidth > 0 {
		if stubIndex < len(stub) {
			for _, sc := range stub[stubIndex] {
				if !sc.First {
					// A merge from above already covers this position.
					continue
				}
				rows := sc.Rows
				if rows < 1 {
					rows = 1
				}
				cell, err := l.cell(sc.Label, &sc.Style, grid, sc.Column, 1)
				if err != nil {
					return nil, err
				}
				cell.Rows, cell.Fill = rows, fill
				cells = append(cells, cell)
			}
		} else {
			// More body rows than the row headers describe. A blank stub is
			// what keeps the left edge of the table continuous.
			cells = append(cells, tableCell{Column: 0, Span: stubWidth, Rows: 1, Fill: fill})
		}
		col = stubWidth
	}

	for j := range t.Body[index] {
		c := &t.Body[index][j]
		span := c.ColSpan
		if span < 1 {
			span = 1
		}
		cell, err := l.bodyCell(c, grid, col, span)
		if err != nil {
			return nil, err
		}
		cells = append(cells, cell)
		col += span
	}
	return cells, nil
}

// stubOwner returns the label and style a stub position shows, following a
// merge back to the row that begins it.
func stubOwner(stub [][]fragment.StubCell, row, col int) (string, *fragment.TextStyle) {
	if row >= len(stub) || col >= len(stub[row]) {
		return "", nil
	}
	owner := row
	for owner > 0 && !stub[owner][col].First {
		owner--
	}
	c := &stub[owner][col]
	return c.Label, &c.Style
}

// cell wraps one label into a cell of a given span.
func (l *lowering) cell(label string, style *fragment.TextStyle, grid []int64,
	column, span int) (tableCell, error) {

	out := tableCell{Column: column, Span: span, Rows: 1, Leading: l.tableLeading()}
	if label == "" || style == nil {
		return out, nil
	}

	sp, err := l.span(label, style)
	if err != nil {
		return tableCell{}, err
	}
	lines, err := text.Wrap([]text.Span{sp}, text.WrapOptions{Width: cellMeasure(grid, column, span)})
	if err != nil {
		return tableCell{}, err
	}
	out.Lines = lines
	out.Leading = leadingFor(lines, l.doc.Scale.LineHeight)
	return out, nil
}

// bodyCell wraps a resolved grid cell, its annotations included.
//
// An annotation attaches to the value rather than replacing it, so it becomes a
// run beside the value's. A note-positioned annotation is not rendered inline
// at all — that is the footnote channel's, and putting a footnote's text in the
// middle of a table is not what the author asked for.
func (l *lowering) bodyCell(c *fragment.Cell, grid []int64, column, span int) (tableCell, error) {
	out := tableCell{Column: column, Span: span, Rows: 1,
		Leading: l.tableLeading(), Fill: l.cellFill(c)}

	var prefix, suffix, raised []fragment.Annotation
	for _, a := range c.Annotations {
		switch a.Position {
		case spec.AnnotationPrefix:
			prefix = append(prefix, a)
		case spec.AnnotationSuffix:
			suffix = append(suffix, a)
		case spec.AnnotationNote:
			// Deliberately dropped from the inline run. See above.
		default:
			raised = append(raised, a)
		}
	}

	var spans []text.Span
	add := func(txt string, style *fragment.TextStyle) error {
		if txt == "" {
			return nil
		}
		sp, err := l.span(txt, style)
		if err != nil {
			return err
		}
		spans = append(spans, sp)
		return nil
	}

	for i := range prefix {
		if err := add(prefix[i].Text, &prefix[i].Style); err != nil {
			return tableCell{}, err
		}
	}
	if err := add(c.Text, &c.Style); err != nil {
		return tableCell{}, err
	}
	for i := range suffix {
		if err := add(suffix[i].Text, &suffix[i].Style); err != nil {
			return tableCell{}, err
		}
	}
	for i := range raised {
		// Set beside the value at its own resolved style rather than raised.
		// This model places lines, not glyphs within a line, so a raised run
		// would need a baseline offset nothing else uses — and a significance
		// letter is legible either way.
		if err := add(raised[i].Text, &raised[i].Style); err != nil {
			return tableCell{}, err
		}
	}

	if len(spans) == 0 {
		return out, nil
	}
	lines, err := text.Wrap(spans, text.WrapOptions{Width: cellMeasure(grid, column, span)})
	if err != nil {
		return tableCell{}, err
	}
	out.Lines = lines
	out.Leading = leadingFor(lines, l.doc.Scale.LineHeight)
	return out, nil
}

// cellFill decides a body cell's background.
//
// Only where nothing else can: a margin or total row is distinguishable from
// data by its class rather than by its numbers, which is why the class exists.
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

// span builds one styled span from resolved text.
func (l *lowering) span(txt string, style *fragment.TextStyle) (text.Span, error) {
	if style == nil || style.FaceIndex < 0 || style.FaceIndex >= len(l.faces) {
		index := -1
		if style != nil {
			index = style.FaceIndex
		}
		return text.Span{}, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"a table cell names a face outside the document's font manifest",
			map[string]any{"face_index": index, "faces": len(l.faces)})
	}
	rgb, err := color.ParseHex(style.Color)
	if err != nil {
		return text.Span{}, err
	}
	return text.Span{
		Text: txt,
		Style: text.Style{
			Face:   l.faces[style.FaceIndex],
			Shaper: l.shapers[style.FaceIndex],
			Size:   points(style.SizeEMU),
			Color:  rgb,
		},
	}, nil
}

// measureRow settles a row's height.
//
// A cell spanning several rows is skipped: its text is shared across them and
// cannot decide the height of any one. Nothing has to be checked afterwards,
// because the measurement pass gives every stub position a cell of its own —
// see [lowering.measureBody] — so a merge is never taller than the rows it was
// measured one at a time across.
func (l *lowering) measureRow(cells []tableCell) tableRow {
	row := tableRow{cells: cells, height: l.tableLeading() + 2*cellPadY}
	for _, c := range cells {
		if c.Rows > 1 {
			continue
		}
		row.height = max(row.height, c.height())
	}
	return row
}

// tableLeading is the baseline rhythm of a cell set at the theme's table size.
//
// The floor under every row, so a row of empty cells is the same height as a
// row of full ones. A table whose blank rows collapse is one a reader reads as
// two tables.
func (l *lowering) tableLeading() object.Real {
	size := l.doc.Scale.TableBody
	if size <= 0 {
		size = l.doc.Scale.Body
	}
	multiple := l.doc.Scale.LineHeight
	if multiple <= 0 {
		multiple = 1.2
	}
	return object.Real(int64(float64(points(size))*multiple + 0.5))
}

// cellMeasure is the width a cell's text is broken to: its columns less the
// insets it is drawn with.
func cellMeasure(grid []int64, column, span int) object.Real {
	w := object.Real(fragment.SpanWidth(grid, column, span)) - 2*cellPadX
	if w < 0 {
		return 0
	}
	return w
}

// columnLeft is the x offset of a grid column from the table's left edge.
func columnLeft(grid []int64, column int) object.Real {
	return object.Real(fragment.SpanWidth(grid, 0, column))
}

// drawTable places one part's rows, starting at the cursor, and leaves the
// cursor under it.
//
// Three passes rather than one, and the order is the point. PDF paints in the
// order the operators appear, so a cell's fill drawn after its neighbour's
// hairline covers the hairline, and text drawn before a fill disappears under
// it. Fills, then rules, then text is the only order in which every cell can be
// emitted independently and still land right.
func (l *lowering) drawTable(rows []tableRow, grid []int64) {
	l.ensurePage()

	left := points(l.geometry.MarginLeft)
	top := l.cursor
	width := object.Real(fragment.SpanWidth(grid, 0, len(grid)))

	// The top of each row, and the whole part's height.
	tops := make([]object.Real, len(rows))
	y := top
	for i, r := range rows {
		tops[i] = y
		y -= r.height
	}
	height := top - y

	box := func(i int, c tableCell) (x, cellTop, w, h object.Real) {
		x = left + columnLeft(grid, c.Column)
		w = object.Real(fragment.SpanWidth(grid, c.Column, c.Span))
		cellTop = tops[i]
		covered := c.Rows
		if covered < 1 {
			covered = 1
		}
		for r := i; r < i+covered && r < len(tops); r++ {
			h += rowHeightAt(tops, r, y)
		}
		return x, cellTop, w, h
	}

	for i, r := range rows {
		for _, c := range r.cells {
			if c.Fill == "" {
				continue
			}
			rgb, err := color.ParseHex(c.Fill)
			if err != nil {
				// A resolved fill that does not parse is a resolution defect,
				// not a drawing one. Drawing nothing here loses a band; the
				// text still lands, and the resolver's own error path is where
				// a bad colour is reported.
				continue
			}
			x, cellTop, w, h := box(i, c)
			l.current.Items = append(l.current.Items, Rule(RuleItem{
				X: x, Y: cellTop - h, Width: w, Height: h, Color: rgb,
			}))
		}
	}

	rule := color.RGB{R: 0x80, G: 0x80, B: 0x80}
	if v, ok := l.doc.Palette.Lookup(theme.ColorRule); ok {
		if rgb, err := color.ParseHex(v); err == nil {
			rule = rgb
		}
	}

	// Every cell draws its own top and left edge; the part draws its own right
	// and bottom once. That is a complete grid with no line crossing a merged
	// cell — a merge simply has no cell of its own at the covered positions, so
	// nothing draws there.
	for i, r := range rows {
		for _, c := range r.cells {
			x, cellTop, w, h := box(i, c)
			l.current.Items = append(l.current.Items,
				Rule(RuleItem{X: x, Y: cellTop - tableRuleWidth, Width: w,
					Height: tableRuleWidth, Color: rule}),
				Rule(RuleItem{X: x, Y: cellTop - h, Width: tableRuleWidth,
					Height: h, Color: rule}))
		}
	}
	l.current.Items = append(l.current.Items,
		Rule(RuleItem{X: left + width - tableRuleWidth, Y: y, Width: tableRuleWidth,
			Height: height, Color: rule}),
		Rule(RuleItem{X: left, Y: y, Width: width, Height: tableRuleWidth, Color: rule}))

	for i, r := range rows {
		for _, c := range r.cells {
			if len(c.Lines) == 0 {
				continue
			}
			x, cellTop, w, _ := box(i, c)
			l.current.Items = append(l.current.Items, Text(TextItem{
				X:       x + cellPadX,
				Y:       cellTop - cellPadY - c.Leading,
				Width:   w - 2*cellPadX,
				Align:   text.AlignLeft,
				Leading: c.Leading,
				Lines:   c.Lines,
			}))
		}
	}

	l.cursor = y
}

// rowHeightAt is the height of one row, read back from the tops.
//
// Read back rather than carried, so a merged cell's height is exactly the sum
// of the rows it covers — the same numbers the split was planned from, rather
// than a second addition of the same quantities that could round differently.
func rowHeightAt(tops []object.Real, row int, bottom object.Real) object.Real {
	if row+1 < len(tops) {
		return tops[row] - tops[row+1]
	}
	return tops[row] - bottom
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
