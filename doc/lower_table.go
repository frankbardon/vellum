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

	stubWidth := depthOf(t.RowHeaders)
	bodyWidth := t.Width
	if bodyWidth <= 0 {
		bodyWidth = widestRow(t.Body)
	}
	total := stubWidth + bodyWidth
	if total == 0 {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_INVALID,
			"the table has no columns", where)
	}

	out := &Table{
		StyleID:    StyleTableGrid,
		Grid:       evenGrid(l.contentWidth(), total),
		HeaderRows: depthOf(t.ColumnHeaders),
	}

	// Column banners: one row per level of the header forest, each node
	// spanning the leaves beneath it. The stub corner above the row headers is
	// one merged cell, which is what an authored analytical table looks like.
	for _, level := range levelsOf(t.ColumnHeaders) {
		row := TableRow{Header: true, CantSplit: true}
		if stubWidth > 0 {
			row.Cells = append(row.Cells, TableCell{
				GridSpan:      stubWidth,
				WidthEMU:      spanWidth(out.Grid, 0, stubWidth),
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
				WidthEMU:      spanWidth(out.Grid, col, span),
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
	stub := stubRows(t.RowHeaders, stubWidth)
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
					WidthEMU: spanWidth(out.Grid, 0, stubWidth),
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
				WidthEMU:      spanWidth(out.Grid, col, span),
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
func stubRows(tree fragment.HeaderTree, depth int) [][]stubCell {
	if depth == 0 {
		return nil
	}
	var out [][]stubCell
	var walk func(nodes fragment.HeaderTree, level int, prefix []stubCell)

	walk = func(nodes fragment.HeaderTree, level int, prefix []stubCell) {
		for i := range nodes {
			n := &nodes[i]
			cell := stubCell{label: n.Label, style: n.Style, column: level,
				rows: leafCount(n), first: true}
			row := append(append([]stubCell(nil), prefix...), cell)

			if len(n.Children) == 0 {
				// Pad a shallow branch out to the full stub depth, so every
				// body row's stub covers the same number of columns.
				for c := level + 1; c < depth; c++ {
					row = append(row, stubCell{column: c, rows: 1, first: true})
				}
				out = append(out, row)
				continue
			}
			walk(n.Children, level+1, row)
		}
	}
	walk(tree, 0, nil)

	// After the walk, only the first row of a merged span carries the label;
	// the rest continue it.
	seen := make(map[int]int)
	for i := range out {
		for j := range out[i] {
			if remaining, ok := seen[j]; ok && remaining > 0 {
				out[i][j] = stubCell{column: j, rows: 1, first: false}
				seen[j] = remaining - 1
				continue
			}
			seen[j] = out[i][j].rows - 1
		}
	}
	return out
}

// stubCell is one cell of a flattened row-header stub.
type stubCell struct {
	label  string
	style  fragment.TextStyle
	column int
	rows   int
	first  bool
}

func (l *lowering) stubCells(cells []stubCell, grid []int64) []TableCell {
	out := make([]TableCell, 0, len(cells))
	for _, c := range cells {
		cell := TableCell{
			GridSpan:      1,
			WidthEMU:      spanWidth(grid, c.column, 1),
			VerticalAlign: VAlignTop,
			Content:       []Content{para(Paragraph{StyleID: StyleNormal})},
		}
		switch {
		case !c.first:
			cell.VerticalMerge = MergeContinue
		case c.rows > 1:
			cell.VerticalMerge = MergeRestart
		}
		if c.first && c.label != "" {
			props := subtract(runProperties(l.doc, c.style), l.styleRun(StyleNormal))
			props.Bold = true
			cell.Content = []Content{para(Paragraph{StyleID: StyleNormal,
				Runs: []Run{{Text: c.label, Properties: props}}})}
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

// depthOf returns how many levels a header forest has.
func depthOf(tree fragment.HeaderTree) int {
	best := 0
	for i := range tree {
		d := 1
		if sub := depthOf(tree[i].Children); sub > 0 {
			d += sub
		}
		if d > best {
			best = d
		}
	}
	return best
}

// levelsOf flattens a header forest into one slice of nodes per depth.
func levelsOf(tree fragment.HeaderTree) []fragment.HeaderTree {
	depth := depthOf(tree)
	if depth == 0 {
		return nil
	}
	out := make([]fragment.HeaderTree, depth)

	var walk func(nodes fragment.HeaderTree, level int)
	walk = func(nodes fragment.HeaderTree, level int) {
		for i := range nodes {
			n := &nodes[i]
			out[level] = append(out[level], *n)
			if len(n.Children) > 0 {
				walk(n.Children, level+1)
			} else {
				// A leaf shallower than the forest is deep must still tile the
				// rows beneath it, or the banner leaves a hole.
				for deeper := level + 1; deeper < depth; deeper++ {
					out[deeper] = append(out[deeper], fragment.HeaderNode{Span: n.Span, Style: n.Style})
				}
			}
		}
	}
	walk(tree, 0)
	return out
}

// leafCount returns how many leaves a node covers, which is how many body rows
// a row-header node spans.
func leafCount(n *fragment.HeaderNode) int {
	if len(n.Children) == 0 {
		return 1
	}
	total := 0
	for i := range n.Children {
		total += leafCount(&n.Children[i])
	}
	return total
}

func widestRow(body [][]fragment.Cell) int {
	best := 0
	for _, row := range body {
		width := 0
		for i := range row {
			span := row[i].ColSpan
			if span < 1 {
				span = 1
			}
			width += span
		}
		if width > best {
			best = width
		}
	}
	return best
}

func spanOf(cells []TableCell) int {
	total := 0
	for _, c := range cells {
		total += c.span()
	}
	return total
}

// evenGrid divides a width into equal columns, distributing the remainder so
// the columns sum exactly to the width.
//
// Exactly, not approximately: a grid whose columns do not sum to the declared
// table width makes Word silently re-measure, and a table that re-measures on
// open is a table whose appearance depends on the reader.
func evenGrid(width int64, columns int) []int64 {
	if columns <= 0 {
		return nil
	}
	out := make([]int64, columns)
	base := width / int64(columns)
	rem := width - base*int64(columns)
	for i := range out {
		out[i] = base
		if int64(i) < rem {
			out[i]++
		}
	}
	return out
}

func spanWidth(grid []int64, start, span int) int64 {
	total := int64(0)
	for i := start; i < start+span && i < len(grid); i++ {
		total += grid[i]
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
