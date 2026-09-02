package sheet_test

import (
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/sheet"
	"github.com/frankbardon/vellum/spec"
)

// crosstab's own shape, established once here rather than assumed at each
// call site: RowHeaders nests a band under "Age", so the stub is two columns
// wide (Age, then the band) with Age itself a vertical merge across the two
// body rows; ColumnHeaders nests North/South under "Region" beside a
// standalone "Total", so the banner is two rows and the body three columns
// wide. One cell (North, 18-34) carries an annotation, which is what triggers
// doubling across the whole table: the doubled grid is
// 2 (stub) + 3*2 (body) = 8 columns wide, 2 banner rows + 2 body rows + 1
// caption row = 5 rows tall.

// TestLower_TableTilesTheGrid checks the shape a crosstab actually produces:
// the corner, the two-level banner, the two-column stub with its vertical
// merge, and every body cell present at the position the banner and stub
// declare — nothing sparse across a merged span.
func TestLower_TableTilesTheGrid(t *testing.T) {
	wb := lower(t, crosstab())
	s := wb.Sheets[0]

	if len(s.Rows) != 5 {
		t.Fatalf("Rows = %d, want 5 (2 banner + 2 body + 1 caption)", len(s.Rows))
	}
	for i, want := range []int{8, 8, 8, 8, 1} {
		if got := len(s.Rows[i].Cells); got != want {
			t.Errorf("row %d has %d cells, want %d", i+1, got, want)
		}
	}

	// Every merge is registered exactly where the banner and stub declare
	// one: the two-column Region banner cell, the two-column Total banner
	// cell doubled to four physical columns, and the two-row Age stub merge.
	var sawRegionMerge, sawStubMerge bool
	for _, m := range s.Merges {
		if m.FromRow == 1 && m.FromCol == 3 && m.ToCol == 6 {
			sawRegionMerge = true
		}
		if m.FromCol == 1 && m.ToCol == 1 && m.ToRow-m.FromRow == 1 {
			sawStubMerge = true
		}
	}
	if !sawRegionMerge {
		t.Errorf("no merge spans the doubled Region banner cell: %+v", s.Merges)
	}
	if !sawStubMerge {
		t.Errorf("no vertical merge spans the Age stub: %+v", s.Merges)
	}
}

// TestLower_AStubContinuationRowIsWrittenOnce is the regression this table
// found while it was being built: a merge's continuation row must be written
// by the ordinary per-row walk and not a second time by the code that starts
// the merge, or the row carries two cells claiming the same column.
func TestLower_AStubContinuationRowIsWrittenOnce(t *testing.T) {
	wb := lower(t, crosstab())
	s := wb.Sheets[0]

	seen := map[[2]int]int{}
	for _, row := range s.Rows {
		for _, c := range row.Cells {
			seen[[2]int{row.Index, c.Column}]++
		}
	}
	for pos, n := range seen {
		if n > 1 {
			t.Errorf("(row %d, col %d) was written %d times", pos[0], pos[1], n)
		}
	}
}

// TestLower_ABannerThatDoesNotTileTheGridIsRefused breaks the resolved
// document in the one way this check exists to catch: a row whose cells do
// not cover the grid is not a document that fails to draw, it draws with a
// hole in it.
func TestLower_ABannerThatDoesNotTileTheGridIsRefused(t *testing.T) {
	doc := resolved(t, crosstab())
	doc.Sections[0].Blocks[0].Table.Width++

	_, err := sheet.Lower(doc)
	if !verr.HasCode(err, verr.VELLUM_TABLE_HEADER_SPAN_MISMATCH) {
		t.Fatalf("error = %v, want VELLUM_TABLE_HEADER_SPAN_MISMATCH", err)
	}
}

// TestLower_ARowThatDoesNotTileTheGridIsRefused is the same failure one row
// lower.
func TestLower_ARowThatDoesNotTileTheGridIsRefused(t *testing.T) {
	doc := resolved(t, crosstab())
	body := doc.Sections[0].Blocks[0].Table.Body
	body[0] = body[0][:len(body[0])-1]

	_, err := sheet.Lower(doc)
	if !verr.HasCode(err, verr.VELLUM_TABLE_ROW_ARITY) {
		t.Fatalf("error = %v, want VELLUM_TABLE_ROW_ARITY", err)
	}
}

// TestLower_ATableWithNoColumnsIsRefused pins the other structural check,
// breaking a resolved table directly the way the arity tests above do — a
// specification this empty never reaches resolution in the first place.
func TestLower_ATableWithNoColumnsIsRefused(t *testing.T) {
	doc := resolved(t, crosstab())
	tbl := doc.Sections[0].Blocks[0].Table
	tbl.RowHeaders, tbl.ColumnHeaders, tbl.Body, tbl.Width = nil, nil, nil, 0

	_, err := sheet.Lower(doc)
	if !verr.HasCode(err, verr.VELLUM_TABLE_INVALID) {
		t.Fatalf("error = %v, want VELLUM_TABLE_INVALID", err)
	}
}

// TestLower_ValuesAreLiveNotText is the whole reason xlsx is in scope at all:
// a workbook's numbers are typed and computable, not text a reader can only
// look at.
func TestLower_ValuesAreLiveNotText(t *testing.T) {
	wb := lower(t, crosstab())
	s := wb.Sheets[0]

	var found bool
	for _, row := range s.Rows {
		for _, c := range row.Cells {
			if c.Value.Kind == sheet.CellNumber && c.Value.Number == 0.41 {
				found = true
			}
			if c.Value.Kind == sheet.CellText && c.Value.Text == "0.41" {
				t.Fatalf("a numeric value was written as text: %+v", c)
			}
		}
	}
	if !found {
		t.Fatal("the 0.41 value cell was not found as a live number anywhere in the sheet")
	}
}

// TestLower_AFormatCodeBecomesTheCellsNumFmt checks that a table's declared
// format code reaches the styles part rather than being discarded once the
// value is typed — and that it applies to an ordinary percent, not only to a
// date.
func TestLower_AFormatCodeBecomesTheCellsNumFmt(t *testing.T) {
	wb := lower(t, crosstab())
	var sawPercent bool
	for _, f := range wb.Styles.NumFmts {
		if f.Code == "0.0%" {
			sawPercent = true
		}
	}
	if !sawPercent {
		t.Fatalf("no numFmt entry carries the table's own format code; NumFmts = %+v", wb.Styles.NumFmts)
	}
}

// TestLower_ATotalRowGetsTheStripeFill checks that a class-marked row is
// distinguishable by its class rather than by its numbers, which is the
// entire reason [spec.CellClass] exists.
func TestLower_ATotalRowGetsTheStripeFill(t *testing.T) {
	wb := lower(t, crosstab())
	s := wb.Sheets[0]

	// The total row is the second (and last) body row: row index 3 (1-based
	// row 4), one before the caption.
	total := s.Rows[3]
	var sawFill bool
	for _, c := range total.Cells {
		if wb.Styles.Formats[c.StyleID].FillIndex != 0 {
			sawFill = true
		}
	}
	if !sawFill {
		t.Errorf("no cell in the total row carries a fill: %+v", total)
	}
}

// TestLower_AnAnnotationAppendsAColumnAndPreservesTheValue pins the literal
// shape [capability.FeatureTableCellAnnotation]'s degradation describes:
// "text appended to the cell, with the typed value preserved in a
// neighbouring column".
func TestLower_AnAnnotationAppendsAColumnAndPreservesTheValue(t *testing.T) {
	wb := lower(t, crosstab())
	s := wb.Sheets[0]

	// Doubling applies to the whole table: 2 stub columns (undoubled) + 3
	// body columns doubled to 6 = 8 physical columns, and no cell should
	// land past it.
	for _, row := range s.Rows {
		for _, c := range row.Cells {
			if c.Column > 8 {
				t.Fatalf("a cell landed in column %d, past the doubled grid width of 8: %+v", c.Column, row)
			}
		}
	}

	value := findCell(s, sheet.CellNumber, 0.41)
	if value == nil {
		t.Fatal("the annotated value cell was not found")
	}
	annotation := findCellAt(s, value.row, value.cell.Column+1)
	if annotation == nil || annotation.cell.Value.Kind != sheet.CellText || annotation.cell.Value.Text != "a" {
		t.Fatalf("the cell after the annotated value is %+v, want the text \"a\"", annotation)
	}
}

// TestLower_NoAnnotationsMeansNoDoubling is the other half: a table with no
// cell-level annotation anywhere gets no extra columns, because the doubling
// is applied uniformly across the whole table or not at all.
func TestLower_NoAnnotationsMeansNoDoubling(t *testing.T) {
	plain := spec.Block{Kind: spec.BlockTable, Table: &spec.Table{
		ColumnHeaders: spec.HeaderTree{{Label: "North"}, {Label: "South"}},
		Body: [][]spec.Cell{{
			{Value: &spec.Value{Kind: spec.ValueNumber, Number: 1}},
			{Value: &spec.Value{Kind: spec.ValueNumber, Number: 2}},
		}},
	}}
	wb := lower(t, plain)
	s := wb.Sheets[0]

	for _, row := range s.Rows {
		for _, c := range row.Cells {
			if c.Column > 2 {
				t.Fatalf("a cell landed in column %d; an unannotated table should not double", c.Column)
			}
		}
	}
}

// TestLower_ANoteAnnotationBecomesACommentOnTheCell pins the one place a
// note-positioned annotation is not dropped: it is the richest channel any
// writer in this library gives it, because xlsx is the one format with a real
// comment mechanism.
func TestLower_ANoteAnnotationBecomesACommentOnTheCell(t *testing.T) {
	withNote := spec.Block{Kind: spec.BlockTable, Table: &spec.Table{
		ColumnHeaders: spec.HeaderTree{{Label: "Share"}},
		Body: [][]spec.Cell{{
			{Value: &spec.Value{Kind: spec.ValueNumber, Number: 0.5}, Annotations: []spec.Annotation{
				{Text: "small base", Position: spec.AnnotationNote},
			}},
		}},
	}}
	wb := lower(t, withNote)
	s := wb.Sheets[0]
	if len(s.Comments) != 1 {
		t.Fatalf("Comments = %+v, want one", s.Comments)
	}
	if s.Comments[0].Text != "small base" {
		t.Errorf("comment text = %q", s.Comments[0].Text)
	}

	// A note-position annotation is not also appended as a neighbouring
	// column — a table whose only annotation is note-positioned should not
	// double at all.
	for _, row := range s.Rows {
		for _, c := range row.Cells {
			if c.Column > 1 {
				t.Fatalf("a note-only annotation still doubled the grid: %+v", row)
			}
		}
	}
}

// TestLower_FreezePanesTheHeaderAndStub checks the freeze is computed
// relative to where the table actually starts, not to row and column 1 — a
// heading precedes the table in this fixture, so the freeze must account for
// it.
func TestLower_FreezePanesTheHeaderAndStub(t *testing.T) {
	wb := lower(t, heading(1, "Awareness"), crosstab())
	s := wb.Sheets[0]
	if s.Freeze == nil {
		t.Fatal("no freeze pane was set")
	}
	// Heading occupies row 1; the table starts at row 2 and carries a
	// two-level banner (rows 2-3), so rows 1-3 should be frozen. The stub is
	// two columns wide, so columns 1-2 should be frozen.
	if s.Freeze.Rows != 3 {
		t.Errorf("Freeze.Rows = %d, want 3", s.Freeze.Rows)
	}
	if s.Freeze.Cols != 2 {
		t.Errorf("Freeze.Cols = %d, want 2", s.Freeze.Cols)
	}
}

// TestLower_AutoFilterCoversTheLeafHeaderThroughTheLastBodyRow pins the
// ordinary spreadsheet convention: the filter dropdowns sit on the row
// immediately above the data, spanning the data columns and not the
// row-header stub.
func TestLower_AutoFilterCoversTheLeafHeaderThroughTheLastBodyRow(t *testing.T) {
	wb := lower(t, crosstab())
	s := wb.Sheets[0]
	if s.AutoFilter == nil {
		t.Fatal("no autofilter was set")
	}
	// Leaf banner row is row 2. Data starts at column 3, past the two-column
	// stub. Last body row is row 4. The grid is doubled to 8 columns.
	want := sheet.AutoFilter{FromRow: 2, FromCol: 3, ToRow: 4, ToCol: 8}
	if *s.AutoFilter != want {
		t.Errorf("AutoFilter = %+v, want %+v", *s.AutoFilter, want)
	}
}

// TestLower_CaptionFollowsTheTable checks the caption lands as a wrapped cell
// on the row immediately after the table's last body row, not on a sheet of
// its own and not inside the grid.
func TestLower_CaptionFollowsTheTable(t *testing.T) {
	wb := lower(t, crosstab())
	s := wb.Sheets[0]
	last := s.Rows[len(s.Rows)-1]
	if last.Cells[0].Value.Text != "Percentages. Base: all respondents." {
		t.Fatalf("last row = %+v, want the caption", last)
	}
	if last.Index != 5 {
		t.Errorf("caption is on row %d, want 5 (after 2 banner + 2 body rows)", last.Index)
	}
	format := wb.Styles.Formats[last.Cells[0].StyleID]
	if !format.WrapText {
		t.Error("the caption cell does not wrap")
	}
}

type cellAt struct {
	row  int
	cell sheet.Cell
}

// findCell locates the first cell of the given kind and number anywhere in
// the sheet, returning its row index alongside it.
func findCell(s sheet.Sheet, kind sheet.CellKind, number float64) *cellAt {
	for _, row := range s.Rows {
		for _, c := range row.Cells {
			if c.Value.Kind == kind && c.Value.Number == number {
				return &cellAt{row: row.Index, cell: c}
			}
		}
	}
	return nil
}

// findCellAt locates the cell at an exact (row, column), if any.
func findCellAt(s sheet.Sheet, row, col int) *cellAt {
	for _, r := range s.Rows {
		if r.Index != row {
			continue
		}
		for _, c := range r.Cells {
			if c.Column == col {
				return &cellAt{row: row, cell: c}
			}
		}
	}
	return nil
}
