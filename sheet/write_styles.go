package sheet

import "strconv"
import "strings"

// stylesXML emits xl/styles.xml.
//
// The element order is fixed by the schema and is not negotiable:
// numFmts, fonts, fills, borders, cellStyleXfs, cellXfs, cellStyles — in that
// order and no other. Getting the order right is necessary and not
// sufficient; two of these collections carry entries this function must place
// exactly where ECMA-376 says regardless of what the model carries, which is
// what the rest of this file's comments are about.
func (w *writer) stylesXML() []byte {
	sheet := w.wb.Styles
	if len(sheet.Formats) == 0 {
		// A styles part with no index 0 is one Excel refuses, and every cell
		// this writer emits carries a StyleID that defaults to zero. A
		// hand-built Workbook that left Formats empty gets the same default
		// format [newStyleBuilder] seeds for a lowered one.
		sheet.Formats = []CellFormat{{}}
	}

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<styleSheet xmlns="` + nsSpreadsheet + `">`)

	writeNumFmts(&b, sheet.NumFmts)
	writeFonts(&b, sheet.DefaultFont, sheet.Fonts)
	writeFills(&b, sheet.Fills)
	writeBorders(&b, sheet.Borders)

	// cellStyleXfs and cellStyles are not a modelling decision: every
	// SpreadsheetML workbook needs exactly one of each, referencing the
	// built-in "Normal" style, and every cellXfs entry's xfId points at it.
	b.WriteString(`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>`)

	writeCellXfs(&b, sheet.Formats)

	b.WriteString(`<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>`)
	b.WriteString(`</styleSheet>`)
	return []byte(b.String())
}

// writeNumFmts emits the custom number-format codes, or nothing when there are
// none — an empty `numFmts` element is legal and pointless, and every cell
// with no custom code already resolves to General without one.
func writeNumFmts(b *strings.Builder, codes []NumFmt) {
	if len(codes) == 0 {
		return
	}
	b.WriteString(`<numFmts count="` + strconv.Itoa(len(codes)) + `">`)
	for i, f := range codes {
		b.WriteString(`<numFmt numFmtId="` + strconv.Itoa(xmlNumFmtID(i+1)) +
			`" formatCode="` + escapeAttr(f.Code) + `"/>`)
	}
	b.WriteString(`</numFmts>`)
}

// writeFonts emits the font collection: the workbook default at index 0,
// always, then the custom fonts a [CellFormat.FontIndex] of one or more
// selects.
func writeFonts(b *strings.Builder, def Font, fonts []Font) {
	b.WriteString(`<fonts count="` + strconv.Itoa(len(fonts)+1) + `">`)
	writeFont(b, def)
	for _, f := range fonts {
		writeFont(b, f)
	}
	b.WriteString(`</fonts>`)
}

func writeFont(b *strings.Builder, f Font) {
	b.WriteString(`<font>`)
	if f.Bold {
		b.WriteString(`<b/>`)
	}
	if f.Italic {
		b.WriteString(`<i/>`)
	}
	size := formatPoints(pointTenths(f.SizeEMU))
	if f.SizeEMU == 0 {
		size = "11"
	}
	b.WriteString(`<sz val="` + size + `"/>`)
	if f.Color != "" {
		b.WriteString(`<color rgb="FF` + escapeAttr(f.Color) + `"/>`)
	}
	name := f.Name
	if name == "" {
		name = "Calibri"
	}
	b.WriteString(`<name val="` + escapeAttr(name) + `"/>`)
	b.WriteString(`</font>`)
}

// writeFills emits the fill collection: the two entries ECMA-376 reserves,
// always and first, then the workbook's custom fills.
//
// Reserved rather than negotiable: index 0 must be the "none" pattern and
// index 1 must be "gray125", present whether or not any cell in this
// workbook ever selects them, because a reader that does not find them there
// is a reader that refuses the file — this is the one collection where
// getting an index wrong does not degrade the workbook, it breaks it.
func writeFills(b *strings.Builder, fills []Fill) {
	b.WriteString(`<fills count="` + strconv.Itoa(len(fills)+2) + `">`)
	b.WriteString(`<fill><patternFill patternType="none"/></fill>`)
	b.WriteString(`<fill><patternFill patternType="gray125"/></fill>`)
	for _, f := range fills {
		b.WriteString(`<fill><patternFill patternType="solid">` +
			`<fgColor rgb="FF` + escapeAttr(f.Color) + `"/>` +
			`<bgColor indexed="64"/>` +
			`</patternFill></fill>`)
	}
	b.WriteString(`</fills>`)
}

// writeBorders emits the border collection: no border at index 0, always,
// then the workbook's custom borders, each a single hairline weight and
// colour repeated on all four edges.
func writeBorders(b *strings.Builder, borders []Border) {
	b.WriteString(`<borders count="` + strconv.Itoa(len(borders)+1) + `">`)
	b.WriteString(`<border><left/><right/><top/><bottom/><diagonal/></border>`)
	for _, br := range borders {
		edge := `<color rgb="FF` + escapeAttr(br.Color) + `"/></`
		b.WriteString(`<border>` +
			`<left style="thin">` + edge + `left>` +
			`<right style="thin">` + edge + `right>` +
			`<top style="thin">` + edge + `top>` +
			`<bottom style="thin">` + edge + `bottom>` +
			`<diagonal/>` +
			`</border>`)
	}
	b.WriteString(`</borders>`)
}

// writeCellXfs emits `cellXfs`: the collection [Cell.StyleID] indexes.
//
// Every entry beyond index 0 carries the `apply*` attributes, because a
// reader is not required to honour a referenced font, fill, border, number
// format or alignment unless told to — the base cell style an xf's `xfId`
// points at otherwise wins, which is how a workbook's own custom formatting
// goes quietly unapplied in exactly the readers that follow the schema
// strictly.
func writeCellXfs(b *strings.Builder, formats []CellFormat) {
	b.WriteString(`<cellXfs count="` + strconv.Itoa(len(formats)) + `">`)
	for i, f := range formats {
		if i == 0 && f == (CellFormat{}) {
			b.WriteString(`<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>`)
			continue
		}

		b.WriteString(`<xf numFmtId="` + strconv.Itoa(xmlNumFmtID(f.NumFmtIndex)) +
			`" fontId="` + strconv.Itoa(xmlFontID(f.FontIndex)) +
			`" fillId="` + strconv.Itoa(xmlFillID(f.FillIndex)) +
			`" borderId="` + strconv.Itoa(xmlBorderID(f.BorderIndex)) +
			`" xfId="0"`)
		if f.NumFmtIndex != 0 {
			b.WriteString(` applyNumberFormat="1"`)
		}
		if f.FontIndex != 0 {
			b.WriteString(` applyFont="1"`)
		}
		if f.FillIndex != 0 {
			b.WriteString(` applyFill="1"`)
		}
		if f.BorderIndex != 0 {
			b.WriteString(` applyBorder="1"`)
		}
		if f.WrapText || f.VerticalTop {
			b.WriteString(` applyAlignment="1">`)
			b.WriteString(`<alignment`)
			if f.WrapText {
				b.WriteString(` wrapText="1"`)
			}
			if f.VerticalTop {
				b.WriteString(` vertical="top"`)
			}
			b.WriteString(`/></xf>`)
			continue
		}
		b.WriteString(`/>`)
	}
	b.WriteString(`</cellXfs>`)
}
