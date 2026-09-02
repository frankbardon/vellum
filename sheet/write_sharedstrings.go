package sheet

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// sharedStringsXML emits xl/sharedStrings.xml.
//
// Entries are in first-seen order over the canonical walk [writer.internStrings]
// performed — sheets in [Workbook.Sheets] order, then rows in ascending index,
// then cells in ascending column — which is deterministic because that order
// is itself a property of the model rather than of a map. `count` is the total
// number of cells that reference the table; `uniqueCount` is the size of the
// table itself.
func (w *writer) sharedStringsXML() []byte {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<sst xmlns="` + nsSpreadsheet +
		`" count="` + strconv.Itoa(w.strings.total) +
		`" uniqueCount="` + strconv.Itoa(len(w.strings.values)) + `">`)
	for _, s := range w.strings.values {
		b.WriteString(`<si>`)
		writeText(&b, s)
		b.WriteString(`</si>`)
	}
	b.WriteString(`</sst>`)
	return []byte(b.String())
}

// writeText emits a shared-string or comment `<t>` element.
//
// `xml:space="preserve"` is added whenever the text's leading or trailing
// whitespace is significant, because a reader is otherwise entitled to trim
// it — a table cell whose typed value is a single leading space is trimmed
// silently in exactly the reader that follows the schema strictly, and no
// error anywhere would say so.
func writeText(b *strings.Builder, s string) {
	if needsPreserve(s) {
		b.WriteString(`<t xml:space="preserve">`)
	} else {
		b.WriteString(`<t>`)
	}
	b.WriteString(escapeText(s))
	b.WriteString(`</t>`)
}

func needsPreserve(s string) bool {
	if s == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(s)
	last, _ := utf8.DecodeLastRuneInString(s)
	return unicode.IsSpace(first) || unicode.IsSpace(last)
}
