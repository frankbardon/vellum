package fragment

// The arithmetic that turns a resolved table into a grid.
//
// It lives here rather than in a writer because every format needs the same
// answers and none of the answers is format-specific. A resolved table has
// hierarchical headers on both axes — a forest of spanning nodes, which is how
// an analytical banner is actually shaped — and every target has a flat grid of
// rows and cells with spans. The flattening is the same each time, and two
// implementations of it are two chances to produce a banner that does not tile
// its grid, which every reader renders as a table with a hole in it.

// Depth returns how many levels a header forest has.
func (t HeaderTree) Depth() int {
	best := 0
	for i := range t {
		d := 1
		if sub := t[i].Children.Depth(); sub > 0 {
			d += sub
		}
		if d > best {
			best = d
		}
	}
	return best
}

// Levels flattens a header forest into one slice of nodes per depth.
//
// A leaf shallower than the forest is deep is repeated into the deeper levels,
// because the banner has to tile: a level with a gap in it leaves a hole where
// the shallow branch should have continued.
func (t HeaderTree) Levels() []HeaderTree {
	depth := t.Depth()
	if depth == 0 {
		return nil
	}
	out := make([]HeaderTree, depth)

	var walk func(nodes HeaderTree, level int)
	walk = func(nodes HeaderTree, level int) {
		for i := range nodes {
			n := &nodes[i]
			out[level] = append(out[level], *n)
			if len(n.Children) > 0 {
				walk(n.Children, level+1)
				continue
			}
			for deeper := level + 1; deeper < depth; deeper++ {
				out[deeper] = append(out[deeper], HeaderNode{Span: n.Span, Style: n.Style})
			}
		}
	}
	walk(t, 0)
	return out
}

// Leaves returns how many leaves a node covers, which is how many body rows a
// row-header node spans.
func (n HeaderNode) Leaves() int {
	if len(n.Children) == 0 {
		return 1
	}
	total := 0
	for i := range n.Children {
		total += n.Children[i].Leaves()
	}
	return total
}

// StubCell is one cell of a flattened row-header stub.
//
// A stub is what an analytical table has down its left edge: a grouping
// variable whose label appears once against the block of rows it covers.
type StubCell struct {
	// Label is the node's text, carried only on the first row of a merge.
	Label string

	// Style is the node's resolved character style.
	Style TextStyle

	// Column is the stub column this cell occupies, from zero.
	Column int

	// Rows is how many body rows the merge covers.
	Rows int

	// First reports whether this is the row the merge begins on. A cell that is
	// not first continues a merge from above and carries no label.
	First bool
}

// StubRows flattens a row-header forest into one stub per body row.
//
// depth is the stub's column count, which is the forest's own depth. A branch
// shallower than that is padded, so every body row's stub covers the same
// number of columns and the grid stays tiled.
func (t HeaderTree) StubRows(depth int) [][]StubCell {
	if depth == 0 {
		return nil
	}

	var out [][]StubCell
	var walk func(nodes HeaderTree, level int, prefix []StubCell)

	walk = func(nodes HeaderTree, level int, prefix []StubCell) {
		for i := range nodes {
			n := &nodes[i]
			cell := StubCell{Label: n.Label, Style: n.Style, Column: level,
				Rows: n.Leaves(), First: true}
			row := append(append([]StubCell(nil), prefix...), cell)

			if len(n.Children) == 0 {
				for c := level + 1; c < depth; c++ {
					row = append(row, StubCell{Column: c, Rows: 1, First: true})
				}
				out = append(out, row)
				continue
			}
			walk(n.Children, level+1, row)
		}
	}
	walk(t, 0, nil)

	// After the walk, only the first row of a merged span carries the label;
	// the rest continue it.
	seen := make(map[int]int)
	for i := range out {
		for j := range out[i] {
			if remaining, ok := seen[j]; ok && remaining > 0 {
				out[i][j] = StubCell{Column: j, Rows: 1, First: false}
				seen[j] = remaining - 1
				continue
			}
			seen[j] = out[i][j].Rows - 1
		}
	}
	return out
}

// GridWidth returns the table's total column count: the row-header stub plus
// the body.
func (t *Table) GridWidth() int {
	body := t.Width
	if body <= 0 {
		body = WidestRow(t.Body)
	}
	return t.RowHeaders.Depth() + body
}

// WidestRow returns the widest body row's span total.
//
// Used when a table does not declare its own width. The widest row rather than
// the first, because a table whose first row happens to carry a span would
// otherwise declare a grid narrower than its own content.
func WidestRow(body [][]Cell) int {
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

// EvenGrid divides a width into equal columns, distributing the remainder so
// the columns sum exactly to the width.
//
// Exactly, not approximately. A grid whose columns do not sum to the declared
// table width makes a reader silently re-measure, and a table that re-measures
// on open is a table whose appearance depends on the reader.
func EvenGrid(width int64, columns int) []int64 {
	if columns <= 0 {
		return nil
	}
	out := make([]int64, columns)
	base := width / int64(columns)
	remainder := width - base*int64(columns)
	for i := range out {
		out[i] = base
		if int64(i) < remainder {
			out[i]++
		}
	}
	return out
}

// SpanWidth sums the grid columns a span covers.
func SpanWidth(grid []int64, start, span int) int64 {
	total := int64(0)
	for i := start; i < start+span && i < len(grid); i++ {
		total += grid[i]
	}
	return total
}

// ClipStub restricts a flattened stub to a window of body rows.
//
// Needed wherever a table is split across containers. A merge computed over the
// whole table names a span that reaches past the container it lands in — a stub
// cell claiming twenty-six rows inside a table that carries eighteen — and a
// reader handed that either refuses the table or grows every row trying to
// honour it. Both look like something else went wrong.
//
// A merge the window begins in the middle of is restarted at the window's first
// row, carrying the label again, which is the same rule the column banner
// follows: a continuation container repeats the heading rather than leaving the
// rows under it unlabelled.
func ClipStub(stub [][]StubCell, from, count int) [][]StubCell {
	if from == 0 && count >= len(stub) {
		return stub
	}
	to := from + count
	if to > len(stub) {
		to = len(stub)
	}
	if from >= to {
		return nil
	}

	out := make([][]StubCell, 0, to-from)
	for row := from; row < to; row++ {
		cells := make([]StubCell, len(stub[row]))
		copy(cells, stub[row])

		for col := range cells {
			c := &cells[col]

			if row == from && !c.First {
				// Restart the merge here. The owner is the nearest row above
				// that begins one in this column.
				owner := row
				for owner > 0 && !stub[owner][col].First {
					owner--
				}
				c.Label = stub[owner][col].Label
				c.Style = stub[owner][col].Style
				c.First = true
				c.Rows = stub[owner][col].Rows - (row - owner)
			}

			if c.First && row+c.Rows > to {
				c.Rows = to - row
			}
		}
		out = append(out, cells)
	}
	return out
}
