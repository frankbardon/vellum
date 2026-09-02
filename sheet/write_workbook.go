package sheet

import (
	"strconv"
	"strings"
)

// workbookXML emits xl/workbook.xml.
//
// Sheets are listed in [Workbook.Sheets] order, and each `sheetId` is that
// position, one-based — a plain counter over an order that is itself a field
// on the model rather than something this writer decides, so it carries no
// determinism risk of its own.
func (w *writer) workbookXML() []byte {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<workbook xmlns="` + nsSpreadsheet + `" xmlns:r="` + nsRelationships + `">`)
	b.WriteString(`<sheets>`)
	for i := range w.wb.Sheets {
		b.WriteString(`<sheet name="` + escapeAttr(w.wb.Sheets[i].Name) +
			`" sheetId="` + strconv.Itoa(i+1) +
			`" r:id="` + w.sheetRels[i] + `"/>`)
	}
	b.WriteString(`</sheets>`)
	b.WriteString(`</workbook>`)
	return []byte(b.String())
}
