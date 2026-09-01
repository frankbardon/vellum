package deck

import (
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/overflow"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/theme"
)

// table lowers a resolved table onto as many slides as it needs.
//
// The split is the point of this function. A table longer than a slide is the
// normal case rather than the edge case, and what happens to it is declared
// rather than discovered: the rows fill each slide to a capacity derived from
// the theme, the column banner repeats at the top of every one, and the split
// is recorded on the deck so a consumer can see where the rows went.
//
// Capacity is theme-derived and never measured. See [overflow] for why: Vellum
// does not lay out OOXML, so a measured capacity would depend on the fonts
// installed on the machine that measured it, and the same specification would
// break across slides differently on two machines.
func (l *lowering) table(t *fragment.Table, sectionIndex int, sectionID string, blockIndex int) error {
	l.emitBody()

	where := map[string]any{
		"section_index": sectionIndex, "section_id": sectionID, "block_index": blockIndex,
	}

	stubWidth := t.RowHeaders.Depth()
	total := t.GridWidth()
	if total == 0 {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_INVALID,
			"the table has no columns", where)
	}

	region := l.out.contentRegion()
	grid := fragment.EvenGrid(region.Width, total)

	banner := t.ColumnHeaders.Levels()
	rowHeight := l.rowHeight()
	headerHeight := rowHeight

	plan, err := overflow.PlanTable(overflow.Table{
		Rows:         len(t.Body),
		HeaderRows:   len(banner),
		RowHeight:    rowHeight,
		HeaderHeight: headerHeight,
		Available:    region.Height,
	})
	if err != nil {
		return verr.Annotate(err, where)
	}

	stub := t.RowHeaders.StubRows(stubWidth)

	for _, split := range plan {
		out := &Table{
			Columns:    grid,
			FirstRow:   len(banner) > 0,
			BandedRows: true,
		}

		for _, level := range banner {
			row, err := l.bannerRow(level, grid, stubWidth, total, rowHeight, where)
			if err != nil {
				return err
			}
			out.Rows = append(out.Rows, row)
		}

		// The stub is clipped to this part, so a merge that began on an
		// earlier slide restarts here with its label rather than naming a span
		// longer than the table it landed in.
		part := fragment.ClipStub(stub, split.From, split.Count)

		for i := split.From; i < split.From+split.Count; i++ {
			row, err := l.bodyRow(t, part, grid, stubWidth, total, rowHeight, i-split.From, i, where)
			if err != nil {
				return err
			}
			out.Rows = append(out.Rows, row)
		}

		height := int64(len(out.Rows)) * rowHeight
		shapes := append(l.titleShape(), Shape{
			Name:  "Table",
			Frame: Frame{X: region.X, Y: region.Y, Width: region.Width, Height: height},
			Table: out,
		})

		// The caption goes beneath the table on its last slide, in a text box
		// of its own. Not inside the frame: a caption inside a table is a row,
		// and a row is capacity the split would have to reserve on every slide
		// for something that belongs at the end. Not on a slide of its own
		// either — a slide carrying nothing but "Base: all adults" is not a
		// slide anybody meant to make.
		if t.Caption != nil && split.Last(len(t.Body)) {
			shapes = append(shapes, l.captionShape(t.Caption, region, height))
		}

		l.append(Slide{LayoutID: LayoutIDTitleOnly, Shapes: shapes})
		l.titleUsed = true

		l.out.Overflow = append(l.out.Overflow, TableSplit{
			SectionID:    sectionID,
			SectionIndex: sectionIndex,
			BlockIndex:   blockIndex,
			Slide:        len(l.out.Slides) - 1,
			Part:         split.Index,
			Parts:        len(plan),
			FromRow:      split.From,
			Rows:         split.Count,
			TotalRows:    len(t.Body),
			HeaderRows:   len(banner),
		})
	}

	return nil
}

// emptyCell is the text body an unoccupied cell carries.
//
// A paragraph, not nothing — a cell with no paragraph is one a reader repairs —
// and the paragraph mark carries the table's own type size.
//
// That last part is the whole reason this exists. An empty paragraph takes its
// height from the formatting of its mark, and a mark that states nothing is
// sized at DrawingML's default of eighteen points. A crosstab is mostly merged
// corners and continued stub cells, so one unstated empty paragraph per row
// sets every row's height to twice what the split computed — and the table
// silently overflows the slide it was measured to fit.
func (l *lowering) emptyCell() *TextBody {
	return &TextBody{Paragraphs: []Paragraph{{
		Bullet:        Bullet{Kind: BulletNone},
		LineHeightEMU: l.lineBox(),
		EndStyle:      RunStyle{SizeEMU: l.doc.Scale.TableBody},
	}}}
}

// captionShape places a table's caption below it.
//
// Whatever room is left under the table, or one row's worth when the table
// filled the region. A caption drawn off the foot of a slide is one nobody
// reads, and a table that leaves it no room is a table the theme sized wrong.
func (l *lowering) captionShape(caption *fragment.Paragraph, region Frame, used int64) Shape {
	remaining := region.Height - used
	if remaining < captionMinHeight {
		remaining = captionMinHeight
	}
	return Shape{
		Name:  "Caption",
		Frame: Frame{X: region.X, Y: region.Y + used, Width: region.Width, Height: remaining},
		Text: &TextBody{Paragraphs: []Paragraph{{
			Bullet: Bullet{Kind: BulletNone},
			Runs:   l.captionRuns(caption),
		}}},
	}
}

// captionRuns renders a caption at the theme's caption size and muted colour.
//
// Stated rather than inherited: a free-standing text box takes the master's
// other style, which is body size in the body colour, and a caption is neither.
func (l *lowering) captionRuns(p *fragment.Paragraph) []Run {
	in := inherited{
		size:  l.out.Masters[0].TextStyles.Other.Levels[0].SizeEMU,
		color: l.doc.Palette.Color(theme.ColorText, ""),
		font:  l.out.Theme.Minor.Latin,
	}

	out := make([]Run, 0, len(p.Runs))
	for i := range p.Runs {
		out = append(out, Run{Text: p.Runs[i].Text, Style: l.runStyle(&p.Runs[i].Style, in)})
	}
	return out
}

// captionMinHeight is the room a caption gets when the table filled the region.
const captionMinHeight = 2 * emuPerPoint * 12

// bannerRow builds one level of the repeated column header.
func (l *lowering) bannerRow(level fragment.HeaderTree, grid []int64,
	stubWidth, total int, height int64, where map[string]any) (Row, error) {

	row := Row{Height: height}
	if stubWidth > 0 {
		// The corner above the row headers, one merged cell. That is what an
		// authored analytical table looks like, and it is what keeps the
		// banner tiling the grid.
		row.Cells = append(row.Cells, Cell{
			GridSpan: stubWidth,
			Anchor:   AnchorBottom,
			Text:     l.emptyCell(),
		})
		for i := 1; i < stubWidth; i++ {
			row.Cells = append(row.Cells, Cell{HorizontalMerge: true,
				Text: l.emptyCell()})
		}
	}

	for _, node := range level {
		span := node.Span
		if span < 1 {
			span = 1
		}
		row.Cells = append(row.Cells, Cell{
			GridSpan: span,
			Anchor:   AnchorBottom,
			Text: &TextBody{Paragraphs: []Paragraph{{
				Bullet:        Bullet{Kind: BulletNone},
				LineHeightEMU: l.lineBox(),
				Runs:          []Run{{Text: node.Label, Style: l.bannerStyle(&node.Style)}},
			}}},
		})
		for i := 1; i < span; i++ {
			row.Cells = append(row.Cells, Cell{HorizontalMerge: true,
				Text: l.emptyCell()})
		}
	}

	if got := len(row.Cells); got != total {
		return Row{}, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_HEADER_SPAN_MISMATCH,
			"a column banner row does not tile the table's grid",
			withValue(withValue(where, "row_span", got), "grid_width", total))
	}
	return row, nil
}

// bodyRow builds one body row, prefixed by its slice of the row-header stub.
func (l *lowering) bodyRow(t *fragment.Table, stub [][]fragment.StubCell, grid []int64,
	stubWidth, total int, height int64, stubIndex, index int, where map[string]any) (Row, error) {

	row := Row{Height: height}

	if stubWidth > 0 {
		if stubIndex < len(stub) {
			row.Cells = append(row.Cells, l.stubCells(stub[stubIndex])...)
		} else {
			// More body rows than the row headers describe. A blank stub keeps
			// the grid tiled; the alternative is a table a reader repairs.
			row.Cells = append(row.Cells, Cell{GridSpan: stubWidth,
				Text: l.emptyCell()})
			for i := 1; i < stubWidth; i++ {
				row.Cells = append(row.Cells, Cell{HorizontalMerge: true,
					Text: l.emptyCell()})
			}
		}
	}

	for j := range t.Body[index] {
		cell := &t.Body[index][j]
		span := cell.ColSpan
		if span < 1 {
			span = 1
		}
		row.Cells = append(row.Cells, Cell{
			GridSpan: span,
			Fill:     l.cellFill(cell),
			Text:     &TextBody{Paragraphs: []Paragraph{l.cellParagraph(cell)}},
		})
		for i := 1; i < span; i++ {
			row.Cells = append(row.Cells, Cell{HorizontalMerge: true,
				Text: l.emptyCell()})
		}
	}

	if got := len(row.Cells); got != total {
		return Row{}, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_ROW_ARITY,
			"a body row does not tile the table's grid",
			withValue(withValue(withValue(where, "row", index), "row_span", got), "grid_width", total))
	}
	return row, nil
}

// stubCells renders one body row's slice of the row-header stub.
//
// A merge that began above becomes a vMerge cell rather than an absent one. The
// grid is rectangular and every position has to be occupied, which is a rule
// DrawingML shares with WordprocessingML and states differently.
func (l *lowering) stubCells(cells []fragment.StubCell) []Cell {
	out := make([]Cell, 0, len(cells))
	for _, c := range cells {
		cell := Cell{Text: l.emptyCell()}
		switch {
		case !c.First:
			cell.VerticalMerge = true
		case c.Rows > 1:
			cell.RowSpan = c.Rows
		}
		if c.First && c.Label != "" {
			cell.Text = &TextBody{Paragraphs: []Paragraph{{
				Bullet:        Bullet{Kind: BulletNone},
				LineHeightEMU: l.lineBox(),
				Runs:          []Run{{Text: c.Label, Style: l.stubStyle(&c.Style)}},
			}}}
		}
		// The stub carries the header band's fill. Resolution gave these cells
		// the header text colour, which is chosen to sit on that fill — drop
		// the fill and the label is the header's colour on the body's
		// background, which for a light theme is white on white.
		cell.Fill = l.schemeRef(c.Style.Background)
		out = append(out, cell)
	}
	return out
}

// cellParagraph renders a cell's text and its annotations.
//
// An annotation attaches to the value rather than replacing it, so it becomes
// an extra run beside the value's. A note-positioned annotation is not rendered
// inline at all; a deck has a speaker-note channel and putting a footnote's
// text in the middle of a table is not what the author asked for.
func (l *lowering) cellParagraph(c *fragment.Cell) Paragraph {
	in := l.tableInherit()
	p := Paragraph{Bullet: Bullet{Kind: BulletNone}, LineHeightEMU: l.lineBox()}

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
		p.Runs = append(p.Runs, Run{Text: a.Text, Style: l.runStyle(&a.Style, in)})
	}
	if c.Text != "" {
		p.Runs = append(p.Runs, Run{Text: c.Text, Style: l.runStyle(&c.Style, in)})
	}
	for _, a := range suffix {
		p.Runs = append(p.Runs, Run{Text: a.Text, Style: l.runStyle(&a.Style, in)})
	}
	for _, a := range superscript {
		// DrawingML has no superscript switch this model carries, so the
		// annotation is set beside the value at its own resolved style. It is a
		// significance letter either way; raising it is a refinement, and one
		// that would need a baseline field nothing else uses.
		p.Runs = append(p.Runs, Run{Text: a.Text, Style: l.runStyle(&a.Style, in)})
	}
	if len(p.Runs) == 0 {
		p.EndStyle = RunStyle{SizeEMU: l.doc.Scale.TableBody}
	}
	return p
}

// cellFill decides a body cell's background.
//
// Only where the table style cannot: a margin or total row is distinguishable
// from data by its class rather than by its position, and no band expresses
// that. Ordinary banding is left to the style, which is what keeps a table
// restylable.
func (l *lowering) cellFill(c *fragment.Cell) string {
	if c.Style.Background != "" {
		return l.schemeRef(c.Style.Background)
	}
	switch c.Class {
	case spec.CellMargin, spec.CellTotal:
		return l.out.TableStyle.BandFill
	}
	return ""
}

// bannerStyle is a column-header label's character style, against the first-row
// band it sits in.
func (l *lowering) bannerStyle(s *fragment.TextStyle) RunStyle {
	return l.runStyle(s, l.headerInherit())
}

// stubStyle is a row-header label's character style.
//
// A stub label sits in a body row rather than in the header band, so the weight
// it shares with the banner has to be stated here.
func (l *lowering) stubStyle(s *fragment.TextStyle) RunStyle {
	out := l.runStyle(s, l.tableInherit())
	out.Bold = ToggleOn
	return out
}

// tableInherit is the formatting a table cell's run takes from the table style.
//
// The size is deliberately zero, which means every cell run states its own. A
// table style can state a cell's colour and its font — tcTxStyle carries both —
// and there is no slot anywhere in it for a type size. A cell that says nothing
// renders at DrawingML's own default of eighteen points, which is twice the
// theme's table size and produces a table that overflows the slide the split
// said it would fit.
//
// So this is not an override being emitted needlessly. It is the only statement
// of the size anywhere in the file.
func (l *lowering) tableInherit() inherited {
	return inherited{
		color: l.doc.Palette.Color(theme.ColorText, ""),
		font:  l.out.Theme.Minor.Latin,
	}
}

// headerInherit is what a banner cell's run takes from the style's first-row
// band, which states a colour of its own and sets the weight.
func (l *lowering) headerInherit() inherited {
	return inherited{
		color: l.doc.Palette.Color(theme.ColorTableHeaderText, ""),
		font:  l.out.Theme.Minor.Latin,
		bold:  true,
	}
}

// lineBox is the exact height one line of table text occupies.
//
// Written onto every table cell's paragraph as a fixed line spacing, and used
// to compute the split. The two must be the same number: a capacity computed
// from a line box the file does not state is a capacity for a document the
// reader will not produce.
//
// Fixed rather than proportional for the same reason the insets are written
// out. A proportional line box is a proportion of the font's own line height,
// every face declares a different one, and a reader substituting a face changes
// the row height — so a table measured to fit a slide would overflow it on a
// machine missing the theme's font.
func (l *lowering) lineBox() int64 {
	line := l.doc.Scale.LineHeight
	if line <= 0 {
		line = 1.2
	}
	size := l.doc.Scale.TableBody
	if size <= 0 {
		size = l.doc.Scale.Body
	}
	return roundDiv(int64(line*100000)*size, 100000)
}

// rowHeight is what one table row occupies: the stated line box plus the
// stated vertical insets.
//
// Both halves are values this writer puts in the file, so the arithmetic
// describes the table that was written rather than one a reader might produce.
// What it cannot predict is a cell whose text wraps to a second line, which
// makes the row taller than this and is a theme whose columns are too narrow
// for its type — an error visible in the same place on every machine, which is
// the property that matters.
func (l *lowering) rowHeight() int64 {
	return l.lineBox() + 2*cellMarginV
}

func withValue(where map[string]any, key string, value any) map[string]any {
	out := make(map[string]any, len(where)+1)
	for k, v := range where {
		out[k] = v
	}
	out[key] = value
	return out
}
