package splice

// spliceShape is E11-S2's pptx splice strategy: a shape anchor's Span is the
// whole <p:sp>...</p:sp> element (mirroring spliceNative's own whole-w:sdt
// convention — see anchor.Anchor.Span's own doc), and splicing locates the
// shape's own <p:txBody> child within it, the DrawingML counterpart of
// locateSdtContent finding a w:sdt's own w:sdtContent child.
//
// A <p:txBody> is not purely a paragraph list the way w:sdtContent is: its
// own content model is bodyPr, lstStyle?, p+ — an optional body-properties
// element (autofit, insets, text anchor) and an optional list-style element
// precede one or more paragraphs, and both carry formatting the shape's own
// author set that a splice must not discard. spliceShape therefore replaces
// only the span from the end of whichever of bodyPr/lstStyle is present
// through the end of txBody's own content, not txBody's whole Content span —
// unlike spliceNative, whose w:sdtContent carries no such leading metadata
// to preserve.
//
// A fragment.Sequence's paragraph(s) render as <a:p>/<a:r> DrawingML text
// runs, reusing the template's own local run formatting — the first <a:r>'s
// own <a:rPr> found anywhere inside the txBody, deep-cloned, mirroring
// firstRunRPrIn's own "first run wins" rule exactly. Table and Asset blocks
// are refused with VELLUM_TEMPLATE_SHAPE_BLOCK_UNSUPPORTED: a <p:txBody> is
// text-only by the schema, a permanent scope boundary matching
// VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED's own reasoning, not a "not
// implemented yet" gap.

import (
	"bytes"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/xmlcopy"
)

const (
	// nsPresentation is the PresentationML main namespace, this package's own
	// copy of the constant template/anchor's pptx.go and template/bind's
	// repeat_slide.go each also carry — "own your own constants" applies
	// across this whole subtree, not only within template/anchor.
	nsPresentation = "http://schemas.openxmlformats.org/presentationml/2006/main"
)

// spliceShape renders seq into a's own shape and returns the single
// [xmlcopy.Replacement] needed to bind it.
func spliceShape(a anchor.Anchor, src []byte, seq fragment.Sequence) (xmlcopy.Replacement, error) {
	sp, found, err := elementAtSpanPPTX(src, nsPresentation, "sp", a.Span)
	if err != nil {
		return xmlcopy.Replacement{}, err
	}
	if !found {
		return xmlcopy.Replacement{}, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"a shape anchor's own span no longer matches a <p:sp> element in its part",
			map[string]any{"anchor": a.Name, "part": a.Part})
	}

	txBody, found, err := locateTxBody(src, sp)
	if err != nil {
		return xmlcopy.Replacement{}, err
	}
	if !found {
		return xmlcopy.Replacement{}, verr.NewCodedErrorWithDetails(verr.VELLUM_TEMPLATE_SHAPE_TXBODY_MISSING,
			"the shape has no <p:txBody> child to splice into",
			map[string]any{"anchor": a.Name, "part": a.Part})
	}

	metaEnd, err := txBodyMetaEnd(src, txBody)
	if err != nil {
		return xmlcopy.Replacement{}, err
	}
	target := xmlcopy.Span{Start: metaEnd, End: txBody.Content.End}

	basis, err := firstShapeRunRPrIn(src, txBody.Content)
	if err != nil {
		return xmlcopy.Replacement{}, err
	}

	if len(seq.Blocks) == 0 {
		// A CT_TextBody needs at least one paragraph — the same minimum
		// content-model fallback spliceNative uses for w:sdtContent.
		return target.Replace([]byte(`<a:p/>`)), nil
	}

	var out []byte
	for i := range seq.Blocks {
		blk := &seq.Blocks[i]
		if blk.Paragraph == nil {
			return xmlcopy.Replacement{}, verr.NewCodedErrorWithDetails(verr.VELLUM_TEMPLATE_SHAPE_BLOCK_UNSUPPORTED,
				"a pptx shape's txBody accepts only paragraph blocks",
				map[string]any{"anchor": a.Name, "part": a.Part, "block_index": i, "block_kind": string(blk.Kind)})
		}
		out = append(out, renderShapeParagraph(blk.Paragraph, basis)...)
	}
	return target.Replace(out), nil
}

// elementAtSpanPPTX walks src once looking for the element named local (in
// namespace) whose own Span exactly equals sp — this package's own copy of
// the technique splice/xlsx.go's elementAtSpanXLSX and template/bind's
// repeat.go's elementAtSpan both already use for the same reason: relocating
// a discovery-time Span back to a live xmlcopy.Element.
func elementAtSpanPPTX(src []byte, namespace, local string, sp xmlcopy.Span) (xmlcopy.Element, bool, error) {
	var result xmlcopy.Element
	found := false
	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if found || e.Name.Space != namespace || e.Name.Local != local {
			return nil
		}
		if e.Span == sp {
			result = e
			found = true
		}
		return nil
	})
	if err != nil {
		return xmlcopy.Element{}, false, err
	}
	return result, found, nil
}

// locateTxBody finds sp's own direct <p:txBody> child, using the same
// per-depth direct-child rule locateSdtContent and template/anchor's own
// scanDocument/scanSlideShapes already establish: a direct child is
// identified by Depth equalling the parent's own Depth+1 together with
// falling entirely inside the parent's own Content span.
func locateTxBody(src []byte, sp xmlcopy.Element) (xmlcopy.Element, bool, error) {
	var result xmlcopy.Element
	found := false
	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if found {
			return nil
		}
		if e.Depth != sp.Depth+1 {
			return nil
		}
		if e.Span.Start < sp.Content.Start || e.Span.End > sp.Content.End {
			return nil
		}
		if e.Name.Space == nsPresentation && e.Name.Local == "txBody" {
			result = e
			found = true
		}
		return nil
	})
	if err != nil {
		return xmlcopy.Element{}, false, err
	}
	return result, found, nil
}

// txBodyMetaEnd returns the byte offset immediately after whichever of
// txBody's own direct <a:bodyPr> or <a:lstStyle> children ends last, or
// txBody.Content.Start when neither is present. CT_TextBody's own content
// model (bodyPr, lstStyle?, p+) places both before every paragraph, so the
// later of the two (when both are present) is always lstStyle — this walks
// generically rather than assuming that ordering, so a template's own bytes
// are trusted over the schema's stated order.
func txBodyMetaEnd(src []byte, txBody xmlcopy.Element) (int64, error) {
	metaEnd := txBody.Content.Start
	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if e.Depth != txBody.Depth+1 {
			return nil
		}
		if e.Span.Start < txBody.Content.Start || e.Span.End > txBody.Content.End {
			return nil
		}
		if e.Name.Space != nsDrawingMain {
			return nil
		}
		if e.Name.Local != "bodyPr" && e.Name.Local != "lstStyle" {
			return nil
		}
		if e.Span.End > metaEnd {
			metaEnd = e.Span.End
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return metaEnd, nil
}

// firstShapeRunRPrIn is firstRunRPrIn's DrawingML counterpart: it returns
// the verbatim <a:rPr .../> or <a:rPr>...</a:rPr> bytes of the first <a:r>
// run found anywhere inside container, deep-cloned so it outlives src, or
// nil when container holds no run at all or that first run carries no rPr.
func firstShapeRunRPrIn(src []byte, container xmlcopy.Span) ([]byte, error) {
	type node struct {
		el      xmlcopy.Element
		hasRPr  bool
		rprSpan xmlcopy.Span
	}
	buckets := make(map[int][]node) // scratch only: looked up and cleared by depth, never ranged
	var runs []node

	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		children := buckets[e.Depth+1]
		delete(buckets, e.Depth+1)

		var n node
		n.el = e
		switch {
		case e.Name.Space == nsDrawingMain && e.Name.Local == "rPr":
			n.hasRPr = true
			n.rprSpan = e.Span

		case e.Name.Space == nsDrawingMain && e.Name.Local == "r":
			if withinSpan(e.Span, container) {
				for _, c := range children {
					if c.hasRPr {
						n.hasRPr = true
						n.rprSpan = c.rprSpan
					}
				}
				runs = append(runs, n)
			}
		}

		buckets[e.Depth] = append(buckets[e.Depth], n)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, nil
	}

	// runs is already in document order: xmlcopy.Walk visits post-order, and
	// for sibling <a:r> elements (which never nest one inside another) that
	// coincides with left-to-right document order — the same reasoning
	// docx.go's own scanDocument doc comment gives for its own natives slice,
	// so no extra sort is needed here either.
	first := runs[0]
	if !first.hasRPr {
		return nil, nil
	}
	return append([]byte(nil), src[first.rprSpan.Start:first.rprSpan.End]...), nil
}

// renderShapeParagraph renders a whole <a:p>...</a:p> element: every run,
// in order. Unlike renderParagraph's own DOCX counterpart, this emits no
// w:pPr-equivalent spacing element — fragment.Paragraph's SpaceBefore,
// SpaceAfter and LineHeight are left unrendered for a shape splice, a
// deliberate v1 scope decision (not an oversight): DrawingML's own
// paragraph-spacing element (a:spcBef/a:spcAft as a:spcPts or a:spcPct) uses
// a different unit and percentage-of-font-height convention than
// WordprocessingML's twips, and getting that translation right belongs to a
// future story that actually needs it rather than being guessed at here.
func renderShapeParagraph(p *fragment.Paragraph, basis []byte) []byte {
	var b strings.Builder
	b.WriteString("<a:p>")
	for i := range p.Runs {
		b.Write(renderShapeRun(p.Runs[i].Text, basis, p.Runs[i].Style))
	}
	b.WriteString("</a:p>")
	return []byte(b.String())
}

// renderShapeRun turns text and style into a well-formed <a:r>...</a:r>
// element's raw bytes, using basis as the formatting to start from — the
// template's own rPr at the splice site, verbatim — with style.Bold,
// style.Italic and style.Underline layered on top as additions, mirroring
// renderRun's own "Fill has no theme" discipline exactly.
func renderShapeRun(text string, basis []byte, style fragment.TextStyle) []byte {
	rpr := layerRPrShape(basis, style)

	var b strings.Builder
	b.WriteString("<a:r>")
	if rpr != nil {
		b.Write(rpr)
	}
	b.WriteString(shapeT(text))
	b.WriteString("</a:r>")
	return []byte(b.String())
}

// shapeT renders an <a:t>...</a:t> element, adding xml:space="preserve" when
// text has leading or trailing whitespace a reader would otherwise be
// entitled to collapse, reusing render.go's own needsPreserveText — the same
// rule applies to DrawingML text as to WordprocessingML text, since
// xml:space is a generic XML mechanism, not a WordprocessingML-specific one.
func shapeT(text string) string {
	var b strings.Builder
	b.WriteString("<a:t")
	if needsPreserveText(text) {
		b.WriteString(` xml:space="preserve"`)
	}
	b.WriteByte('>')
	b.WriteString(xmlcopy.EscapeText(text))
	b.WriteString("</a:t>")
	return b.String()
}

// layerRPrShape returns basis with the character switches style.Bold,
// style.Italic and style.Underline added, additively and only when not
// already present — mirroring layerRPr's own rule exactly, but at the
// attribute level rather than the child-element level, because
// CT_TextCharacterProperties (a:rPr) states bold/italic/underline as its own
// attributes (b, i, u) rather than as child elements the way CT_RPr (w:rPr)
// does. XML attribute order carries no schema meaning (unlike CT_RPr's own
// child-element sequence, which is exactly why layerRPr needs a
// schema-ordered insertion point and this function does not), so a new
// attribute is simply inserted right after the element's own tag name.
func layerRPrShape(basis []byte, style fragment.TextStyle) []byte {
	rpr := basis
	if style.Bold {
		rpr = insertRPrAttrShape(rpr, "b", "1")
	}
	if style.Italic {
		rpr = insertRPrAttrShape(rpr, "i", "1")
	}
	if style.Underline {
		rpr = insertRPrAttrShape(rpr, "u", "sng")
	}
	return rpr
}

// insertRPrAttrShape returns rpr (an <a:rPr .../>, an <a:rPr>...</a:rPr>, or
// nil) with attrName="attrValue" added to its own opening tag, only when rpr
// does not already carry that attribute at all — additive only, the
// attribute-level counterpart of insertRPrChild's own "never turns a switch
// off" rule: an attribute already present (whichever value it carries) means
// the template's own author made an explicit statement about it, and this
// does not touch it either way.
func insertRPrAttrShape(rpr []byte, attrName, attrValue string) []byte {
	if rpr == nil {
		return []byte(`<a:rPr ` + attrName + `="` + attrValue + `"/>`)
	}
	if hasRPrAttrShape(rpr, attrName) {
		return rpr
	}

	const tagName = "<a:rPr"
	if !bytes.HasPrefix(rpr, []byte(tagName)) {
		// Defensive: every rPr this function is ever handed comes from
		// firstShapeRunRPrIn, which only ever clones a real <a:rPr ...>
		// element's own Span. Leaving the override off rather than risking
		// a malformed splice is the same safe failure insertRPrChild's own
		// defensive branch chooses.
		return rpr
	}

	insertAt := len(tagName)
	out := make([]byte, 0, len(rpr)+len(attrName)+len(attrValue)+4)
	out = append(out, rpr[:insertAt]...)
	out = append(out, ' ')
	out = append(out, attrName...)
	out = append(out, '=', '"')
	out = append(out, attrValue...)
	out = append(out, '"')
	out = append(out, rpr[insertAt:]...)
	return out
}

// hasRPrAttrShape reports whether rpr's own opening tag already carries an
// attribute named name, searching only the opening tag (up to its first
// unquoted '>') so a coincidental match inside an attribute's own value —
// unlikely for b/i/u's own values, but checked properly rather than assumed
// — can never produce a false positive.
func hasRPrAttrShape(rpr []byte, name string) bool {
	tag := rpr[:openTagEndShape(rpr)]
	return bytes.Contains(tag, []byte(" "+name+`="`))
}

// openTagEndShape returns the byte offset of rpr's own opening tag's first
// unquoted '>', respecting quoted attribute values so a '>' character
// legitimately appearing inside one (not a realistic case for b/i/u/lang/sz,
// but a defensive general reader rather than one that only handles the
// shapes this package happens to produce) is not mistaken for the tag's own
// end.
func openTagEndShape(rpr []byte) int {
	var inQuote byte
	for i, c := range rpr {
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inQuote = c
		case '>':
			return i
		}
	}
	return len(rpr)
}
