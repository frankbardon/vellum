package sheet

import (
	"strconv"
	"strings"
)

// worksheetXML emits one worksheet part.
//
// Element order follows CT_Worksheet: sheetViews, cols, sheetData, autoFilter,
// mergeCells, legacyDrawing. Every one of these is optional in the schema and
// every one here is written only when the sheet actually carries it, which is
// what keeps a sheet with no frozen pane and no comments from acquiring bytes
// that state "nothing is frozen" and "there is no legacy drawing" — the
// absence of the element already says that.
func (w *writer) worksheetXML(index int, s *Sheet) []byte {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<worksheet xmlns="` + nsSpreadsheet + `" xmlns:r="` + nsRelationships + `">`)

	writeSheetViews(&b, s.Freeze)
	writeCols(&b, s.Columns)
	writeSheetData(&b, s, w.strings)

	if s.AutoFilter != nil {
		b.WriteString(`<autoFilter ref="` + rangeRef(s.AutoFilter.FromRow, s.AutoFilter.FromCol,
			s.AutoFilter.ToRow, s.AutoFilter.ToCol) + `"/>`)
	}
	if len(s.Merges) > 0 {
		b.WriteString(`<mergeCells count="` + strconv.Itoa(len(s.Merges)) + `">`)
		for _, m := range s.Merges {
			b.WriteString(`<mergeCell ref="` + rangeRef(m.FromRow, m.FromCol, m.ToRow, m.ToCol) + `"/>`)
		}
		b.WriteString(`</mergeCells>`)
	}
	if len(s.Comments) > 0 {
		b.WriteString(`<legacyDrawing r:id="` + w.vmlRels[index] + `"/>`)
	}

	b.WriteString(`</worksheet>`)
	return []byte(b.String())
}

// writeSheetViews emits the frozen-pane declaration, or nothing.
//
// The split point is one past the frozen extent — a freeze of two rows and one
// column parks the reader's first scrollable cell at B3 — and `topLeftCell`
// states that cell explicitly rather than leaving a reader to compute it,
// because a reader that gets this wrong scrolls the frozen band itself.
func writeSheetViews(b *strings.Builder, f *FreezePane) {
	if f == nil || (f.Rows <= 0 && f.Cols <= 0) {
		return
	}
	topLeft := cellRef(f.Rows+1, f.Cols+1)
	b.WriteString(`<sheetViews><sheetView workbookViewId="0">`)
	b.WriteString(`<pane xSplit="` + strconv.Itoa(f.Cols) + `" ySplit="` + strconv.Itoa(f.Rows) +
		`" topLeftCell="` + topLeft + `" activePane="bottomRight" state="frozen"/>`)
	b.WriteString(`</sheetView></sheetViews>`)
}

// writeCols emits column-width overrides, or nothing when the sheet declares
// none.
func writeCols(b *strings.Builder, cols []ColumnWidth) {
	if len(cols) == 0 {
		return
	}
	b.WriteString(`<cols>`)
	for _, c := range cols {
		b.WriteString(`<col min="` + strconv.Itoa(c.Column) + `" max="` + strconv.Itoa(c.Column) +
			`" width="` + strconv.FormatFloat(c.Width, 'f', -1, 64) + `" customWidth="1"/>`)
	}
	b.WriteString(`</cols>`)
}

// writeSheetData emits the populated rows and cells.
//
// A sheet is sparse and this preserves that: a row absent from [Sheet.Rows] is
// absent from the XML, and within a row a position [Row.Cells] does not name
// is simply not written. Excel treats an unwritten position as an empty cell,
// which is what it is.
func writeSheetData(b *strings.Builder, s *Sheet, strs *stringTable) {
	b.WriteString(`<sheetData>`)
	for _, row := range s.Rows {
		b.WriteString(`<row r="` + strconv.Itoa(row.Index) + `">`)
		for _, cell := range row.Cells {
			writeCell(b, row.Index, &cell, strs)
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData>`)
}

func writeCell(b *strings.Builder, row int, c *Cell, strs *stringTable) {
	ref := cellRef(row, c.Column)
	b.WriteString(`<c r="` + ref + `"`)
	if c.StyleID != 0 {
		b.WriteString(` s="` + strconv.Itoa(c.StyleID) + `"`)
	}

	switch c.Value.Kind {
	case CellEmpty:
		b.WriteString(`/>`)
		return
	case CellText:
		b.WriteString(` t="s"><v>` + strconv.Itoa(strs.lookup(c.Value.Text)) + `</v></c>`)
		return
	case CellBool:
		v := "0"
		if c.Value.Bool {
			v = "1"
		}
		b.WriteString(` t="b"><v>` + v + `</v></c>`)
		return
	default:
		// CellNumber and CellDate are both live numbers; only the style
		// applied to the cell's own numFmtId tells a reader which one it is
		// looking at, which is exactly how SpreadsheetML itself draws that
		// distinction — a date has never been a distinct storage type.
		b.WriteString(`><v>` + formatNumber(c.Value.Number) + `</v></c>`)
	}
}

// formatNumber renders a live numeric value.
//
// strconv's shortest round-tripping form, which is deterministic for a given
// float64 input — Go's algorithm does not vary between runs or platforms — and
// is what keeps a value written here reading back as the exact bits it started
// as rather than as a truncated approximation.
func formatNumber(n float64) string {
	return strconv.FormatFloat(n, 'g', -1, 64)
}
