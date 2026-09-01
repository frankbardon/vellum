package doc

// Table is a WordprocessingML table.
//
// The grid is explicit: WordprocessingML has no automatic column sizing that a
// writer can rely on, so every table declares its column widths and every row
// declares cells that tile them. A table whose cells do not tile its grid opens
// with a hole in it, which looks deliberate and is therefore worse than a
// refusal.
type Table struct {
	// StyleID names the table style. Empty means no style.
	StyleID string

	// Grid is the column widths in EMU, left to right. Their sum is the table's
	// width.
	Grid []int64

	// Rows are the table's rows, in order.
	Rows []TableRow

	// Caption is the accessible description Word exposes, and is not rendered.
	// A visible caption is a separate paragraph in the Caption style, because
	// that is what a reader can restyle.
	Caption string

	// Layout fixes the column widths rather than letting Word autofit them.
	// Fixed is the default here: autofit makes Word re-measure on open, so the
	// same document renders differently depending on the fonts installed.
	AutoFit bool

	// HeaderRows is how many leading rows repeat when the table breaks across
	// pages. Word needs this marked per row; the writer expands it.
	HeaderRows int
}

// TableRow is one row.
type TableRow struct {
	// Cells are the row's cells, left to right.
	Cells []TableCell

	// HeightEMU is a minimum row height, or zero for automatic.
	HeightEMU int64

	// CantSplit keeps the row from breaking across a page.
	CantSplit bool

	// Header marks the row as one that repeats at the top of each page.
	Header bool
}

// TableCell is one cell.
type TableCell struct {
	// WidthEMU is the cell's width. For a merged cell it is the sum of the
	// columns it spans.
	WidthEMU int64

	// GridSpan is how many grid columns this cell occupies. Zero means one.
	GridSpan int

	// VerticalMerge continues a merge begun above. See [VerticalMerge].
	VerticalMerge VerticalMerge

	// Fill is a background colour, an uppercase hex triplet, or empty.
	Fill string

	// VerticalAlign is the cell's vertical alignment.
	VerticalAlign VerticalAlign

	// Content is the cell's block content. A cell must contain at least one
	// paragraph — an empty cell in WordprocessingML is a paragraph with no
	// runs, not an absent one, and a cell with neither makes Word repair the
	// file.
	Content []Content
}

// VerticalMerge is a cell's part in a vertical merge.
type VerticalMerge string

const (
	// MergeNone is not merged. The zero value.
	MergeNone VerticalMerge = ""

	// MergeRestart begins a vertical merge.
	MergeRestart VerticalMerge = "restart"

	// MergeContinue continues one begun above.
	MergeContinue VerticalMerge = "continue"
)

// VerticalAlign is a cell's vertical alignment.
type VerticalAlign string

const (
	// VAlignTop is the zero value and Word's own default.
	VAlignTop    VerticalAlign = ""
	VAlignCenter VerticalAlign = "center"
	VAlignBottom VerticalAlign = "bottom"
)

// span returns a cell's effective grid span, treating zero as one.
func (c TableCell) span() int {
	if c.GridSpan < 1 {
		return 1
	}
	return c.GridSpan
}
