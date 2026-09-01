package doc

import (
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/spec"
)

// table lowers a resolved table into WordprocessingML's grid idiom.
//
// The hard part is that the two models disagree about what a table is. The
// resolved table has hierarchical headers on both axes — a forest of spanning
// nodes, which is how an analytical banner is actually shaped. WordprocessingML
// has a flat grid of rows and cells with horizontal and vertical spans. The
// conversion flattens the forest into banner rows, one per depth, and it must
// tile exactly: a row whose cells do not cover the grid opens as a table with a
// hole in it, which looks deliberate.
func (l *lowering) table(t *fragment.Table, sectionIndex int, sectionID string, blockIndex int) ([]Content, error) {
	where := map[string]any{
		"section_index": sectionIndex, "section_id": sectionID, "block_index": blockIndex,
	}

	stubWidth := t.RowHeaders.Depth()
	bodyWidth := t.Width
	if bodyWidth <= 0 {
		bodyWidth = fragment.WidestRow(t.Body)
	}
	total := stubWidth + bodyWidth
	if total == 0 {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_INVALID,
			"the table has no columns", where)
	}

	out := &Table{
		StyleID:    StyleTableGrid,
		Grid:       fragment.EvenGrid(l.contentWidth(), total),
		HeaderRows: t.ColumnHeaders.Depth(),
	}

	// Column banners: one row per level of the header forest, each node
	// spanning the leaves beneath it. The stub corner above the row headers is
	// one merged cell, which is what an authored analytical table looks like.
	for _, level := range t.ColumnHeaders.Levels() {
		row := TableRow{Header: true, CantSplit: true}
		if stubWidth > 0 {
			row.Cells = append(row.Cells, TableCell{
				GridSpan:      stubWidth,
				WidthEMU:      fragment.SpanWidth(out.Grid, 0, stubWidth),
				Fill:          l.scale.TableHeaderFill,
				VerticalAlign: VAlignBottom,
				Content:       []Content{para(Paragraph{StyleID: StyleNormal})},
			})
		}
		col := stubWidth
		for _, node := range level {
			span := node.Span
			if span < 1 {
				span = 1
			}
			row.Cells = append(row.Cells, TableCell{
				GridSpan:      span,
				WidthEMU:      fragment.SpanWidth(out.Grid, col, span),
				Fill:          l.scale.TableHeaderFill,
				VerticalAlign: VAlignBottom,
				Content: []Content{para(Paragraph{
					StyleID: StyleNormal,
					Runs:    []Run{{Text: node.Label, Properties: subtract(headerRun(node.Style, l.doc), l.styleRun(StyleNormal))}},
				})},
			})
			col += span
		}
		if got := spanOf(row.Cells); got != total {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_HEADER_SPAN_MISMATCH,
				"a column banner row does not tile the table's grid",
				withValue(withValue(where, "row_span", got), "grid_width", total))
		}
		out.Rows = append(out.Rows, row)
	}

	// Body rows, each prefixed by its slice of the row-header stub.
	stub := t.RowHeaders.StubRows(stubWidth)
	for i := range t.Body {
		row := TableRow{}
		if stubWidth > 0 {
			if i < len(stub) {
				row.Cells = append(row.Cells, l.stubCells(stub[i], out.Grid)...)
			} else {
				// More body rows than the row headers describe. Filling with a
				// blank stub keeps the grid tiled; the alternative is a table
				// Word repairs.
				row.Cells = append(row.Cells, TableCell{
					GridSpan: stubWidth,
					WidthEMU: fragment.SpanWidth(out.Grid, 0, stubWidth),
					Content:  []Content{para(Paragraph{StyleID: StyleNormal})},
				})
			}
		}

		col := stubWidth
		for j := range t.Body[i] {
			cell := &t.Body[i][j]
			span := cell.ColSpan
			if span < 1 {
				span = 1
			}
			row.Cells = append(row.Cells, TableCell{
				GridSpan:      span,
				WidthEMU:      fragment.SpanWidth(out.Grid, col, span),
				Fill:          l.cellFill(cell, i),
				VerticalAlign: VAlignTop,
				Content:       []Content{para(l.cellParagraph(cell))},
			})
			col += span
		}

		if got := spanOf(row.Cells); got != total {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_ROW_ARITY,
				"a body row does not tile the table's grid",
				withValue(withValue(withValue(where, "row", i), "row_span", got), "grid_width", total))
		}
		out.Rows = append(out.Rows, row)
	}

	content := []Content{{Table: out}}
	if t.Caption != nil {
		// The caption is a paragraph in the Caption style, after the table.
		// The table's own w:tblCaption is an accessibility description Word
		// does not render, so a visible caption has to be real content — and
		// being real content is what makes it restylable.
		out.Caption = t.Caption.Text()
		content = append(content, para(l.paragraph(t.Caption, StyleCaption)))
	}
	return content, nil
}

// cellParagraph renders a cell's text and its annotations.
//
// An annotation attaches to the value rather than replacing it, so it becomes
// an extra run beside the value's — superscript by default, which is where a
// significance letter belongs. A note-positioned annotation is not rendered
// inline at all; it is left for the footnote channel, and rendering it beside
// the value would put a footnote's text in the middle of a table.
func (l *lowering) cellParagraph(c *fragment.Cell) Paragraph {
	base := l.styleRun(StyleNormal)
	p := Paragraph{StyleID: StyleNormal}

	var prefix, suffix, superscript []fragment.Annotation
	for _, a := range c.Annotations {
		switch a.Position {
		case spec.AnnotationPrefix:
			prefix = append(prefix, a)
		case spec.AnnotationSuffix:
			suffix = append(suffix, a)
		case spec.AnnotationNote:
			// Deliberately dropped from the inline run. See above.
		default:
			superscript = append(superscript, a)
		}
	}

	for _, a := range prefix {
		p.Runs = append(p.Runs, Run{Text: a.Text,
			Properties: subtract(runProperties(l.doc, a.Style), base)})
	}
	if c.Text != "" {
		p.Runs = append(p.Runs, Run{Text: c.Text,
			Properties: subtract(runProperties(l.doc, c.Style), base)})
	}
	for _, a := range suffix {
		p.Runs = append(p.Runs, Run{Text: a.Text,
			Properties: subtract(runProperties(l.doc, a.Style), base)})
	}
	for _, a := range superscript {
		props := subtract(runProperties(l.doc, a.Style), base)
		props.Superscript = true
		p.Runs = append(p.Runs, Run{Text: a.Text, Properties: props})
	}

	if len(p.Runs) == 0 {
		// A cell must contain a paragraph, and a paragraph with no runs is the
		// shape an empty cell has. A cell with no paragraph at all makes Word
		// repair the file.
		p.Runs = nil
	}
	return p
}

// cellFill decides a body cell's background.
//
// A margin or total row is distinguishable from data by its class rather than
// by its numbers, which is why the class exists — a consumer marks the row and
// Vellum renders the distinction without learning what a margin is.
func (l *lowering) cellFill(c *fragment.Cell, row int) string {
	if c.Style.Background != "" {
		return c.Style.Background
	}
	switch c.Class {
	case spec.CellMargin, spec.CellTotal:
		return l.scale.TableStripeFill
	}
	if l.scale.TableStripeFill != "" && row%2 == 1 {
		return l.scale.TableStripeFill
	}
	return ""
}

// stubRows flattens a row-header forest into one cell list per body row.
//
// Each leaf of the forest is a body row, and each ancestor becomes a vertically
// merged cell spanning its descendants. The result is the stub an analytical
// table has: a grouping variable on the left whose label appears once against
// the block of rows it covers.
func (l *lowering) stubCells(cells []fragment.StubCell, grid []int64) []TableCell {
	out := make([]TableCell, 0, len(cells))
	for _, c := range cells {
		cell := TableCell{
			GridSpan:      1,
			WidthEMU:      fragment.SpanWidth(grid, c.Column, 1),
			VerticalAlign: VAlignTop,
			Content:       []Content{para(Paragraph{StyleID: StyleNormal})},
		}
		switch {
		case !c.First:
			cell.VerticalMerge = MergeContinue
		case c.Rows > 1:
			cell.VerticalMerge = MergeRestart
		}
		// The stub carries the header band's fill. Resolution gave these cells
		// the header text colour, which is chosen to sit on that fill — drop
		// the fill and the label is the header's colour on the body's
		// background, which for a light theme is white on white.
		cell.Fill = c.Style.Background
		if c.First && c.Label != "" {
			props := subtract(runProperties(l.doc, c.Style), l.styleRun(StyleNormal))
			props.Bold = true
			cell.Content = []Content{para(Paragraph{StyleID: StyleNormal,
				Runs: []Run{{Text: c.Label, Properties: props}}})}
		}
		out = append(out, cell)
	}
	return out
}

// headerRun keeps a banner's own colour, which sits on the header fill and is
// therefore not the body text colour.
func headerRun(s fragment.TextStyle, d *fragment.Doc) RunProperties {
	props := runProperties(d, s)
	props.Bold = true
	// The fill is on the cell, not the run: a highlight behind the text would
	// paint a box the width of the words rather than the width of the cell.
	props.Highlight = ""
	return props
}

func spanOf(cells []TableCell) int {
	total := 0
	for _, c := range cells {
		total += c.span()
	}
	return total
}

// contentWidth is the column a table is fitted to: the current section's, or
// the first section's when none has been opened yet.
func (l *lowering) contentWidth() int64 {
	if n := len(l.out.Sections); n > 0 {
		return l.out.Sections[n-1].Page.ContentWidth()
	}
	return A4Portrait().ContentWidth()
}

func withValue(where map[string]any, key string, value any) map[string]any {
	out := make(map[string]any, len(where)+1)
	for k, v := range where {
		out[k] = v
	}
	out[key] = value
	return out
}
