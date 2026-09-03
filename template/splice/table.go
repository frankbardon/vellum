package splice

import (
	"strconv"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
)

// defaultTableWidthEMU is the width a spliced table is fitted to.
//
// Fill mode has no page geometry to measure against — no theme, no section,
// no doc.Document — so there is no honest "content width" this package could
// derive the way doc/lower_table.go derives one from the current section.
// Six inches is a reasonable content width for a letter or A4 page's usable
// area after ordinary margins, and it is only ever a starting point: the
// columns still sum to it exactly (fragment.EvenGrid guarantees that), so the
// table Word draws is internally consistent even though its absolute width is
// a documented default rather than a measurement.
const defaultTableWidthEMU int64 = 6 * emuPerInch

// renderTable renders a resolved analytical table as a plain bordered
// WordprocessingML grid: no theme-driven banding or shading (fill mode has
// none to draw from), borders on every side and every internal line, and
// header-row and stub-label text bold. This is the "reasonable, honest
// minimum" the story sets, not a rebuild of doc/lower_table.go's fuller
// styling — mirrored for shape (iterating ColumnHeaders.Levels() and
// RowHeaders.StubRows(), spanning with w:gridSpan and w:vMerge exactly as
// WordprocessingML's own merge model requires), not for behaviour.
func renderTable(t *fragment.Table) ([]byte, error) {
	stubWidth := t.RowHeaders.Depth()
	bodyWidth := t.Width
	if bodyWidth <= 0 {
		bodyWidth = fragment.WidestRow(t.Body)
	}
	total := stubWidth + bodyWidth
	if total <= 0 {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_INVALID,
			"the table has no columns", nil)
	}
	grid := fragment.EvenGrid(defaultTableWidthEMU, total)

	var b strings.Builder
	b.WriteString(`<w:tbl><w:tblPr>`)
	b.WriteString(`<w:tblW w:w="` + strconv.Itoa(twips(sum(grid))) + `" w:type="dxa"/>`)
	b.WriteString(`<w:tblBorders>`)
	for _, side := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		b.WriteString(`<w:` + side + ` w:val="single" w:sz="4" w:space="0" w:color="auto"/>`)
	}
	b.WriteString(`</w:tblBorders></w:tblPr><w:tblGrid>`)
	for _, col := range grid {
		b.WriteString(`<w:gridCol w:w="` + strconv.Itoa(twips(col)) + `"/>`)
	}
	b.WriteString(`</w:tblGrid>`)

	for _, level := range t.ColumnHeaders.Levels() {
		b.WriteString(`<w:tr>`)
		if stubWidth > 0 {
			b.WriteString(cornerCellXML(fragment.SpanWidth(grid, 0, stubWidth), stubWidth))
		}
		col := stubWidth
		for _, node := range level {
			span := node.Span
			if span < 1 {
				span = 1
			}
			b.WriteString(headerCellXML(node.Label, fragment.SpanWidth(grid, col, span), span))
			col += span
		}
		b.WriteString(`</w:tr>`)
	}

	stub := t.RowHeaders.StubRows(stubWidth)
	for i := range t.Body {
		b.WriteString(`<w:tr>`)
		if stubWidth > 0 {
			if i < len(stub) {
				for _, c := range stub[i] {
					b.WriteString(stubCellXML(c, grid))
				}
			} else {
				// More body rows than the row headers describe: a blank
				// stub keeps the grid tiled rather than leaving the row
				// short, which is the shape Word repairs on open.
				b.WriteString(cornerCellXML(fragment.SpanWidth(grid, 0, stubWidth), stubWidth))
			}
		}
		col := stubWidth
		for j := range t.Body[i] {
			cell := &t.Body[i][j]
			span := cell.ColSpan
			if span < 1 {
				span = 1
			}
			b.WriteString(bodyCellXML(cell, fragment.SpanWidth(grid, col, span), span))
			col += span
		}
		b.WriteString(`</w:tr>`)
	}

	b.WriteString(`</w:tbl>`)
	return []byte(b.String()), nil
}

func tcOpen(widthEMU int64, span int) string {
	var b strings.Builder
	b.WriteString(`<w:tc><w:tcPr><w:tcW w:w="` + strconv.Itoa(twips(widthEMU)) + `" w:type="dxa"/>`)
	if span > 1 {
		b.WriteString(`<w:gridSpan w:val="` + strconv.Itoa(span) + `"/>`)
	}
	return b.String()
}

func cornerCellXML(widthEMU int64, span int) string {
	return tcOpen(widthEMU, span) + `</w:tcPr><w:p/></w:tc>`
}

func headerCellXML(label string, widthEMU int64, span int) string {
	var b strings.Builder
	b.WriteString(tcOpen(widthEMU, span))
	b.WriteString(`</w:tcPr>`)
	b.WriteString(boldParagraph(label))
	b.WriteString(`</w:tc>`)
	return b.String()
}

func stubCellXML(c fragment.StubCell, grid []int64) string {
	width := fragment.SpanWidth(grid, c.Column, 1)
	var b strings.Builder
	b.WriteString(tcOpen(width, 1))
	switch {
	case !c.First:
		b.WriteString(`<w:vMerge/>`)
	case c.Rows > 1:
		b.WriteString(`<w:vMerge w:val="restart"/>`)
	}
	b.WriteString(`</w:tcPr>`)
	if c.First {
		b.WriteString(boldParagraph(c.Label))
	} else {
		b.WriteString(`<w:p/>`)
	}
	b.WriteString(`</w:tc>`)
	return b.String()
}

func bodyCellXML(cell *fragment.Cell, widthEMU int64, span int) string {
	var b strings.Builder
	b.WriteString(tcOpen(widthEMU, span))
	b.WriteString(`</w:tcPr>`)
	if cell.Text == "" {
		b.WriteString(`<w:p/>`)
	} else {
		b.WriteString(`<w:p><w:r>` + wt(cell.Text) + `</w:r></w:p>`)
	}
	b.WriteString(`</w:tc>`)
	return b.String()
}

// boldParagraph renders a single-run bold paragraph, or an empty paragraph
// for an empty label — a cell must contain at least one block-level element,
// and a cell with none makes Word repair the file.
func boldParagraph(text string) string {
	if text == "" {
		return `<w:p/>`
	}
	var b strings.Builder
	b.WriteString(`<w:p><w:r><w:rPr><w:b/><w:bCs/></w:rPr>` + wt(text) + `</w:r></w:p>`)
	return b.String()
}
