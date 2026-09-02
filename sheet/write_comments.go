package sheet

import (
	"strconv"
	"strings"
)

// commentsXML emits one sheet's comments part.
//
// Authors are interned in first-seen order over [Sheet.Comments], the same
// discipline the shared string table follows and for the same reason: the
// emitted order is part of the bytes.
func (w *writer) commentsXML(s *Sheet) []byte {
	authors := newStringTable()
	for _, c := range s.Comments {
		authors.intern(c.Author)
	}

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<comments xmlns="` + nsSpreadsheet + `">`)
	b.WriteString(`<authors>`)
	for _, a := range authors.values {
		b.WriteString(`<author>` + escapeText(a) + `</author>`)
	}
	b.WriteString(`</authors>`)
	b.WriteString(`<commentList>`)
	for _, c := range s.Comments {
		b.WriteString(`<comment ref="` + cellRef(c.Row, c.Col) +
			`" authorId="` + strconv.Itoa(authors.lookup(c.Author)) + `">`)
		b.WriteString(`<text><r><t>` + escapeText(c.Text) + `</t></r></text>`)
		b.WriteString(`</comment>`)
	}
	b.WriteString(`</commentList>`)
	b.WriteString(`</comments>`)
	return []byte(b.String())
}

// vmlDrawingXML emits the legacy VML drawing that carries a sheet's comment
// shapes.
//
// A comment part alone is not enough: Excel and every reader that follows it
// draw a note's little red triangle and its flyout box from a shape defined
// here, not from the comments part, which holds only the text. This is the
// "legacy" drawing mechanism — the one Excel has supported without
// interruption since notes existed — and it is deliberately what this writer
// uses rather than the newer threaded-comment schema introduced alongside
// Office 365, which needs an author-identity part this library has no source
// for. A threaded comment a reader half the installed base cannot see is not
// a degradation worth making; a shape every reader back to Excel 2007 draws
// is.
//
// Unlike every other part this writer emits, a VML drawing carries no XML
// declaration and is not itself namespaced XML in the strict sense — it is
// the format Office used before OOXML existed, wrapped unchanged into an OPC
// part, and every reader's VML parser expects it exactly this way.
func (w *writer) vmlDrawingXML(index int, s *Sheet) []byte {
	var b strings.Builder
	b.WriteString(`<xml xmlns:v="` + nsVML + `" xmlns:o="urn:schemas-microsoft-com:office:office"` +
		` xmlns:x="` + nsExcelVML + `">`)
	b.WriteString(`<o:shapelayout v:ext="edit"><o:idmap v:ext="edit" data="` + strconv.Itoa(index+1) + `"/></o:shapelayout>`)
	b.WriteString(`<v:shapetype id="_x0000_t202" coordsize="21600,21600" o:spt="202" ` +
		`path="m,l,21600r21600,l21600,xe"><v:stroke joinstyle="miter"/>` +
		`<v:path gradientshapeok="t" o:connecttype="rect"/></v:shapetype>`)

	for i, c := range s.Comments {
		id := vmlShapeID(index, i)
		b.WriteString(`<v:shape id="` + id + `" type="#_x0000_t202" ` +
			`style='position:absolute;margin-left:59.25pt;margin-top:1.5pt;width:108pt;height:59.25pt;` +
			`z-index:` + strconv.Itoa(i+1) + `;visibility:hidden' fillcolor="#ffffe1" o:insetmode="auto">`)
		b.WriteString(`<v:fill color2="#ffffe1"/><v:shadow on="t" color="black" obscured="t"/>`)
		b.WriteString(`<v:path o:connecttype="none"/>`)
		b.WriteString(`<v:textbox style='mso-direction-alt:auto'><div style='text-align:left'/></v:textbox>`)
		b.WriteString(`<x:ClientData ObjectType="Note"><x:MoveWithCells/><x:SizeWithCells/>`)
		b.WriteString(`<x:Anchor>1, 15, ` + strconv.Itoa(c.Row-1) + `, 2, 3, 15, ` + strconv.Itoa(c.Row+4) + `, 4</x:Anchor>`)
		b.WriteString(`<x:AutoFill>False</x:AutoFill>`)
		b.WriteString(`<x:Row>` + strconv.Itoa(c.Row-1) + `</x:Row>`)
		b.WriteString(`<x:Column>` + strconv.Itoa(c.Col-1) + `</x:Column>`)
		b.WriteString(`</x:ClientData></v:shape>`)
	}

	b.WriteString(`</xml>`)
	return []byte(b.String())
}

// vmlShapeID names one comment's shape.
//
// A pure function of the sheet index and the comment's position in
// [Sheet.Comments] — both properties of the model rather than of a counter
// this writer keeps across calls — so the same workbook produces the same id
// however many times it is written. 1025 is where Excel's own IDs for this
// shape type conventionally start; nothing requires it, but a reader that has
// only ever seen files Excel wrote is one more reader a familiar-looking id
// costs nothing to keep working smoothly with.
func vmlShapeID(sheetIndex, commentIndex int) string {
	return "_x0000_s" + strconv.Itoa(1025+sheetIndex*1000+commentIndex)
}
