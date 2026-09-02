package xmlcopy

import "strings"

// EscapeText escapes s for use as XML character data.
//
// Hand-rolled rather than via encoding/xml.EscapeText for the same reason
// [doc], [sheet] and [deck]'s own escapers are: that function also escapes
// tabs, newlines and carriage returns as numeric character references, which
// is correct but noisier than what Word, Excel and PowerPoint themselves
// write, and a replacement fragment spliced into an authored part is worth
// keeping close to what that part's own author would have written. This
// mirrors sheet's escapeText rather than inventing a new convention; every
// OOXML-writing package in this tree carries its own copy of this shape
// because none of them may import another.
func EscapeText(s string) string {
	if !strings.ContainsAny(s, "&<>") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// EscapeAttr escapes s for use as an XML attribute value, additionally
// escaping the quote characters EscapeText leaves alone.
func EscapeAttr(s string) string {
	if !strings.ContainsAny(s, `&<>"'`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
