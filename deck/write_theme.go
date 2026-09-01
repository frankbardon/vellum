package deck

import (
	"strconv"
	"strings"
)

// themeXML emits a theme part.
//
// The theme is what makes a deck restylable. Every colour a master, layout or
// slide states is a reference into the scheme declared here rather than a
// literal, so replacing this part restyles the whole deck — which is the
// property a literal colour on a slide destroys silently.
func (w *writer) themeXML(t Theme) []byte {
	name := t.Name
	if name == "" {
		name = defaultProducer
	}

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<a:theme xmlns:a="` + nsDrawingMain + `" name="` + escapeAttr(name) + `">`)
	b.WriteString(`<a:themeElements>`)

	b.WriteString(`<a:clrScheme name="` + escapeAttr(name) + `">`)
	// The order is the schema's sequence, and it is also the order the slots
	// are paired in: the two dark/light pairs, then the accents, then the two
	// link colours.
	for _, slot := range []struct {
		name  string
		value string
	}{
		{"dk1", t.Colors.Dark1},
		{"lt1", t.Colors.Light1},
		{"dk2", t.Colors.Dark2},
		{"lt2", t.Colors.Light2},
		{"accent1", t.Colors.Accent1},
		{"accent2", t.Colors.Accent2},
		{"accent3", t.Colors.Accent3},
		{"accent4", t.Colors.Accent4},
		{"accent5", t.Colors.Accent5},
		{"accent6", t.Colors.Accent6},
		{"hlink", t.Colors.Hyperlink},
		{"folHlink", t.Colors.FollowedHyperlink},
	} {
		b.WriteString(`<a:` + slot.name + `><a:srgbClr val="` +
			escapeAttr(hexOr(slot.value, "000000")) + `"/></a:` + slot.name + `>`)
	}
	b.WriteString(`</a:clrScheme>`)

	b.WriteString(`<a:fontScheme name="` + escapeAttr(name) + `">`)
	writeSchemeFont(&b, "major", t.Major)
	writeSchemeFont(&b, "minor", t.Minor)
	b.WriteString(`</a:fontScheme>`)

	writeFormatScheme(&b, name)

	b.WriteString(`</a:themeElements>`)
	// Both are required by the schema and both are empty here. A theme that
	// declared object defaults would be stating an opinion about every shape
	// drawn on top of it, which is the theme reaching past colour and type
	// into the layout's business.
	b.WriteString(`<a:objectDefaults/><a:extraClrSchemeLst/>`)
	b.WriteString(`</a:theme>`)
	return []byte(b.String())
}

// writeSchemeFont emits one of the two scheme font slots.
//
// The east-Asian and complex-script faces are declared empty rather than
// omitted, which is what the schema requires and what tells a reader to fall
// back to its own defaults for a script this theme says nothing about. Naming
// the Latin face for them would be claiming it covers scripts it does not.
func writeSchemeFont(b *strings.Builder, kind string, f Typeface) {
	b.WriteString(`<a:` + kind + `Font>`)
	b.WriteString(`<a:latin typeface="` + escapeAttr(f.Latin) + `"/>`)
	b.WriteString(`<a:ea typeface=""/><a:cs typeface=""/>`)
	b.WriteString(`</a:` + kind + `Font>`)
}

// writeFormatScheme emits the fill, line and effect styles.
//
// Three of each, because the schema requires exactly three and PowerPoint's
// shape gallery indexes into them. Every one is the plainest form of its kind:
// a solid fill in the placeholder colour, a hairline in the placeholder colour,
// and no effect at all.
//
// The alternative is the gradient-and-shadow set PowerPoint's own themes carry,
// which is a great deal of markup describing an appearance Vellum never asks
// for. Vellum draws no shape that reaches for style index two or three, so the
// entries exist to satisfy the schema and say the least they can.
func writeFormatScheme(b *strings.Builder, name string) {
	b.WriteString(`<a:fmtScheme name="` + escapeAttr(name) + `">`)

	b.WriteString(`<a:fillStyleLst>`)
	for i := 0; i < 3; i++ {
		b.WriteString(`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>`)
	}
	b.WriteString(`</a:fillStyleLst>`)

	b.WriteString(`<a:lnStyleLst>`)
	// Half a point, one point, one and a half. The widths a reader offers as
	// thin, medium and thick.
	for _, width := range []int64{emuPerPoint / 2, emuPerPoint, emuPerPoint * 3 / 2} {
		b.WriteString(`<a:ln w="` + strconv.FormatInt(width, 10) +
			`" cap="flat" cmpd="sng" algn="ctr">`)
		b.WriteString(`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>`)
		b.WriteString(`<a:prstDash val="solid"/>`)
		b.WriteString(`</a:ln>`)
	}
	b.WriteString(`</a:lnStyleLst>`)

	b.WriteString(`<a:effectStyleLst>`)
	for i := 0; i < 3; i++ {
		b.WriteString(`<a:effectStyle><a:effectLst/></a:effectStyle>`)
	}
	b.WriteString(`</a:effectStyleLst>`)

	b.WriteString(`<a:bgFillStyleLst>`)
	for i := 0; i < 3; i++ {
		b.WriteString(`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>`)
	}
	b.WriteString(`</a:bgFillStyleLst>`)

	b.WriteString(`</a:fmtScheme>`)
}

// hexOr returns a colour value or a fallback when it is unset.
func hexOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
