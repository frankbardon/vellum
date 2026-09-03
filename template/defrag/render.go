package defrag

import (
	"strings"

	"github.com/frankbardon/vellum/xmlcopy"
)

// RenderRun turns p back into a well-formed <w:r>...</w:r> element's raw
// bytes: the run's own cloned rPr (if any), a rebuilt w:t carrying
// xml:space="preserve" when p.Preserve says so, and p.Text re-escaped for
// XML character data.
//
// This is what a caller (template/splice) turns a Site's Prefix or Suffix
// into when assembling the final [xmlcopy.Replacement.Data]: Prefix's
// rendering, then the new content, then Suffix's rendering, spliced into
// exactly [Site.Affected] in one [xmlcopy.Apply] pass.
//
// p must not be nil; a nil Prefix or Suffix means there is nothing to
// render, and a caller checks for that before calling RenderRun rather than
// calling it with nil.
func RenderRun(p *Piece) []byte {
	var b strings.Builder
	b.WriteString("<w:r>")
	if p.RPr != nil {
		b.Write(p.RPr)
	}
	b.WriteString("<w:t")
	if p.Preserve {
		b.WriteString(` xml:space="preserve"`)
	}
	b.WriteByte('>')
	b.WriteString(xmlcopy.EscapeText(p.Text))
	b.WriteString("</w:t></w:r>")
	return []byte(b.String())
}
