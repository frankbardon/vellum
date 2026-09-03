package splice

import (
	"bytes"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/xmlcopy"
)

// renderRun turns text and style into a well-formed <w:r>...</w:r> element's
// raw bytes, using basis as the formatting to start from — the template's own
// rPr at the splice site, verbatim, never reconstructed — with style.Bold,
// style.Italic and style.Underline layered on top as additions. See
// [layerRPr] and the package doc's "Fill has no theme" section for why only
// those three fields of TextStyle are honoured at all.
func renderRun(text string, basis []byte, style fragment.TextStyle) []byte {
	rpr := layerRPr(basis, style)

	var b strings.Builder
	b.WriteString("<w:r>")
	if rpr != nil {
		b.Write(rpr)
	}
	b.WriteString(wt(text))
	b.WriteString("</w:r>")
	return []byte(b.String())
}

// wt renders a <w:t>...</w:t> element, adding xml:space="preserve" when text
// has leading or trailing whitespace a reader would otherwise be entitled to
// collapse — the same rule defrag's own needsPreserve applies to a preserved
// boundary Piece.
func wt(text string) string {
	var b strings.Builder
	b.WriteString("<w:t")
	if needsPreserveText(text) {
		b.WriteString(` xml:space="preserve"`)
	}
	b.WriteByte('>')
	b.WriteString(xmlcopy.EscapeText(text))
	b.WriteString("</w:t>")
	return b.String()
}

func needsPreserveText(s string) bool {
	if s == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(s)
	last, _ := utf8.DecodeLastRuneInString(s)
	return unicode.IsSpace(first) || unicode.IsSpace(last)
}

// layerRPr returns basis with the character switches style.Bold,
// style.Italic and style.Underline added, in that order, whenever the
// corresponding source flag is true and basis does not already carry the
// element.
//
// This never turns a switch *off*: [fragment.TextStyle] has no way to
// represent "explicitly not bold" as distinct from "the binding did not set
// this", and CLAUDE.md's splicing rule is that a splice reuses the template's
// own run properties — stripping formatting the template's author put there,
// because a binding's style happened not to ask for it, would be reconstructing
// rather than reusing. So an override is additive only, and only when it is
// not already present, which is the one case where adding it changes nothing
// a reader would see (a font that is already bold rendered "bold" again is
// invisible either way) but *does* matter for a reader that is not already
// bold and needs to become so.
//
// basis is never mutated; every returned slice is either basis unchanged or a
// freshly allocated copy with content inserted, so a caller may safely use
// the same basis bytes for more than one run — the native-splice caller does
// exactly that, applying the same placeholder-derived basis to every run of
// every paragraph it renders.
func layerRPr(basis []byte, style fragment.TextStyle) []byte {
	rpr := basis
	if style.Bold && !hasChild(rpr, "b") {
		rpr = insertRPrChild(rpr, "<w:b/><w:bCs/>", afterBold)
	}
	if style.Italic && !hasChild(rpr, "i") {
		rpr = insertRPrChild(rpr, "<w:i/><w:iCs/>", afterItalic)
	}
	if style.Underline && !hasChild(rpr, "u") {
		rpr = insertRPrChild(rpr, `<w:u w:val="single"/>`, afterUnderline)
	}
	return rpr
}

// afterBold, afterItalic and afterUnderline are the CT_RPr child elements
// that ECMA-376 §17.3.2.1 places *after* the element being inserted — the
// suffix of the schema's own sequence starting just past b/bCs, i/iCs and u
// respectively. A schema-ordered insertion point is found by searching an
// existing rPr's bytes for the *first* occurrence, by byte position, of any
// tag in this list: whatever comes before that position is guaranteed (for
// an rPr Word itself wrote, which every rPr reaching this package is) to
// already be schema-ordered, so inserting immediately before it is correct
// without parsing the rPr at all.
//
// This is not an exhaustive CT_RPr sequence — rStyle, rFonts and the small
// set of on/off switches between b/i and the rest are omitted because
// nothing here ever needs to search *for* them, only skip *past* them, which
// happens for free since they sort earlier in the byte stream than anything
// in these lists. It is deliberately not a full order-aware rPr rewriter:
// fill mode's run-property layering is additive and small (three booleans),
// and this is scoped to exactly what that needs, documented rather than
// silently assumed complete.
var (
	afterBold = []string{
		"i", "iCs", "caps", "smallCaps", "strike", "dstrike", "outline",
		"shadow", "emboss", "imprint", "noProof", "snapToGrid", "vanish",
		"webHidden", "color", "spacing", "w", "kern", "position", "sz",
		"szCs", "highlight", "u", "effect", "bdr", "shd", "fitText",
		"vertAlign", "rtl", "cs", "em", "lang", "eastAsianLayout",
		"specVanish", "oMath", "rPrChange",
	}
	afterItalic    = afterBold[2:] // starting at "caps", past b/bCs and i/iCs
	afterUnderline = []string{
		"effect", "bdr", "shd", "fitText", "vertAlign", "rtl", "cs", "em",
		"lang", "eastAsianLayout", "specVanish", "oMath", "rPrChange",
	}
)

// insertRPrChild returns rpr with xmlToInsert added as a new child, at the
// position the first tag in afterTags occupies (schema-ordered), or at the
// very end of rpr's own children when none of afterTags is present.
//
// rpr is nil, "<w:rPr/>" (a self-closing empty element), or a well-formed
// "<w:rPr>...</w:rPr>" pair — the only three shapes [defrag.Piece.RPr],
// [defrag.Site.RunRPr] and this package's own firstRunRPrIn ever produce,
// since all three are cloned directly from an xmlcopy.Element's own Span.
func insertRPrChild(rpr []byte, xmlToInsert string, afterTags []string) []byte {
	if rpr == nil {
		return []byte("<w:rPr>" + xmlToInsert + "</w:rPr>")
	}
	if bytes.Equal(rpr, []byte("<w:rPr/>")) {
		return []byte("<w:rPr>" + xmlToInsert + "</w:rPr>")
	}

	const closeTag = "</w:rPr>"
	if !bytes.HasSuffix(rpr, []byte(closeTag)) {
		// Defensive: every rPr this package is ever handed is one of the
		// three shapes documented above. Leaving the override off rather
		// than risking a malformed splice is the safer failure here — a run
		// that stays exactly as formatted as its basis, rather than one
		// spliced with broken XML.
		return rpr
	}

	if pos := earliestChildPos(rpr, afterTags); pos >= 0 {
		out := make([]byte, 0, len(rpr)+len(xmlToInsert))
		out = append(out, rpr[:pos]...)
		out = append(out, xmlToInsert...)
		out = append(out, rpr[pos:]...)
		return out
	}

	insertAt := len(rpr) - len(closeTag)
	out := make([]byte, 0, len(rpr)+len(xmlToInsert))
	out = append(out, rpr[:insertAt]...)
	out = append(out, xmlToInsert...)
	out = append(out, rpr[insertAt:]...)
	return out
}

// hasChild reports whether rpr already carries a direct <w:LOCAL> child,
// self-closing or not, distinguishing "<w:b" from a same-prefixed sibling
// like "<w:bCs" or "<w:bdo" by checking the byte immediately following the
// candidate match is one of '/', ' ' or '>'.
func hasChild(rpr []byte, local string) bool {
	return findChild(rpr, local) >= 0
}

// findChild returns the byte offset of rpr's first "<w:LOCAL" occurrence
// whose boundary check passes, or -1.
func findChild(rpr []byte, local string) int {
	needle := []byte("<w:" + local)
	searched := 0
	for {
		idx := bytes.Index(rpr[searched:], needle)
		if idx < 0 {
			return -1
		}
		pos := searched + idx
		after := pos + len(needle)
		if after < len(rpr) {
			switch rpr[after] {
			case '/', ' ', '>':
				return pos
			}
		}
		searched = pos + 1
	}
}

// earliestChildPos returns the smallest findChild result among tags, or -1
// if none of them is present.
func earliestChildPos(rpr []byte, tags []string) int {
	best := -1
	for _, t := range tags {
		if p := findChild(rpr, t); p >= 0 && (best < 0 || p < best) {
			best = p
		}
	}
	return best
}
