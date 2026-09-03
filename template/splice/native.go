package splice

import (
	"sort"
	"strconv"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/xmlcopy"
)

// spliceNative renders seq's blocks into WordprocessingML block content and
// replaces the whole of the anchor's w:sdtContent Content span with it.
//
// A native anchor's content is block-level: any mix of paragraphs, a table
// and an image, in order, is legal. The formatting basis for every new run in
// every rendered paragraph is the same rPr, taken once from the *first* run
// found anywhere inside the existing sdtContent — a template author's own
// placeholder text is the signal for how the real content should look — or no
// w:rPr at all when the placeholder carries no run (an empty control, or one
// whose only placeholder is a table or a picture).
func spliceNative(pkg *opc.Package, a anchor.Anchor, src []byte, seq fragment.Sequence) (xmlcopy.Replacement, error) {
	content, found, err := locateSdtContent(src, a.Span)
	if err != nil {
		return xmlcopy.Replacement{}, err
	}
	if !found {
		return xmlcopy.Replacement{}, verr.NewCodedErrorWithDetails(verr.VELLUM_TEMPLATE_SDT_CONTENT_MISSING,
			"the content control has no w:sdtContent child to splice into",
			map[string]any{"anchor": a.Name, "part": a.Part})
	}

	basis, err := firstRunRPrIn(src, content.Span)
	if err != nil {
		return xmlcopy.Replacement{}, err
	}

	if len(seq.Blocks) == 0 {
		// A block container needs at least one block-level child; an empty
		// paragraph is the same fallback writeTableCell and headerFooterXML
		// use for an empty container elsewhere in this codebase.
		return content.Content.Replace([]byte(`<w:p/>`)), nil
	}

	var out []byte
	nextDrawingID := 1
	for i := range seq.Blocks {
		blk := &seq.Blocks[i]
		switch {
		case blk.Paragraph != nil:
			out = append(out, renderParagraph(blk.Paragraph, basis)...)

		case blk.Table != nil:
			tbl, err := renderTable(blk.Table)
			if err != nil {
				return xmlcopy.Replacement{}, err
			}
			out = append(out, tbl...)

		case blk.Asset != nil:
			p, err := renderAssetParagraph(pkg, a.Part, seq, blk.Asset, nextDrawingID)
			if err != nil {
				return xmlcopy.Replacement{}, err
			}
			nextDrawingID++
			out = append(out, p...)

		default:
			return xmlcopy.Replacement{}, verr.NewCodedErrorWithDetails(verr.VELLUM_TEMPLATE_BLOCK_UNSUPPORTED,
				"a block kind template/splice does not render into a content control",
				map[string]any{"anchor": a.Name, "part": a.Part, "block_index": i, "block_kind": string(blk.Kind)})
		}
	}
	return content.Content.Replace(out), nil
}

// locateSdtContent finds a.Span's own direct w:sdtContent child.
//
// A content control can nest another content control inside its own
// sdtContent, so "the sdtContent somewhere inside this w:sdt's span" is not
// enough — that could resolve to a nested control's own content instead of
// the anchor's. This uses the same per-depth bucket technique
// template/anchor's own docx.go and template/defrag's own flatten.go use to
// attribute a direct child correctly: every element, when it closes, claims
// whatever accumulated in buckets[depth+1] since the last claim, which in a
// single linear pass over well-formed XML is exactly its own direct children.
func locateSdtContent(src []byte, sdt xmlcopy.Span) (xmlcopy.Element, bool, error) {
	buckets := make(map[int][]xmlcopy.Element) // scratch only: looked up and cleared by depth, never ranged
	var result xmlcopy.Element
	var found bool

	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		children := buckets[e.Depth+1]
		delete(buckets, e.Depth+1)

		if e.Span == sdt {
			for _, c := range children {
				if isWordEl(c, "sdtContent") {
					result = c
					found = true
				}
			}
		}

		buckets[e.Depth] = append(buckets[e.Depth], e)
		return nil
	})
	if err != nil {
		return xmlcopy.Element{}, false, err
	}
	return result, found, nil
}

// firstRunRPrIn returns the verbatim <w:rPr>...</w:rPr> bytes of the first
// w:r run found anywhere inside container (direct child or descendant), in
// document order, deep-cloned so it outlives src — or nil when container
// holds no run at all, or when that first run carries no rPr of its own.
//
// This deliberately does not hunt further for a *later* run's rPr when the
// first one has none: the rule this package documents is "the rPr of the
// first run found ... if the placeholder content has one", and a mix of
// "first run's own formatting" and "some other run's formatting" would be a
// choice this package has no principled way to make.
func firstRunRPrIn(src []byte, container xmlcopy.Span) ([]byte, error) {
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
		case isWordEl(e, "rPr"):
			n.hasRPr = true
			n.rprSpan = e.Span

		case isWordEl(e, "r"):
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
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].el.Span.Start < runs[j].el.Span.Start })

	first := runs[0]
	if !first.hasRPr {
		return nil, nil
	}
	return append([]byte(nil), src[first.rprSpan.Start:first.rprSpan.End]...), nil
}

// isWordEl reports whether e is the named element in the WordprocessingML
// namespace, mirroring template/anchor's and template/defrag's own helper of
// the same shape.
func isWordEl(e xmlcopy.Element, local string) bool {
	return e.Name.Space == nsWordprocessing && e.Name.Local == local
}

// withinSpan reports whether inner falls entirely inside outer, inclusive of
// touching boundaries.
func withinSpan(inner, outer xmlcopy.Span) bool {
	return inner.Start >= outer.Start && inner.End <= outer.End
}

// renderParagraph renders a whole <w:p>...</w:p> element: an optional
// minimal w:pPr carrying only spacing (fill mode has no theme to derive
// fuller paragraph styling from, and a specification's own alignment is not
// carried on [fragment.Paragraph] at all), followed by every run.
func renderParagraph(p *fragment.Paragraph, basis []byte) []byte {
	var b strings.Builder
	b.WriteString("<w:p>")
	if ppr := renderParagraphSpacing(p); ppr != "" {
		b.WriteString(ppr)
	}
	for i := range p.Runs {
		b.Write(renderRun(p.Runs[i].Text, basis, p.Runs[i].Style))
	}
	b.WriteString("</w:p>")
	return []byte(b.String())
}

// renderParagraphSpacing emits a <w:pPr><w:spacing .../></w:pPr> carrying
// SpaceBefore, SpaceAfter and LineHeight when any is set, or "" when none is:
// a paragraph whose fragment carries no rhythm gets Word's own default rather
// than an empty, pointless w:pPr.
func renderParagraphSpacing(p *fragment.Paragraph) string {
	if p.SpaceBefore == 0 && p.SpaceAfter == 0 && p.LineHeight == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<w:pPr><w:spacing`)
	if p.SpaceBefore != 0 {
		b.WriteString(` w:before="` + strconv.Itoa(twips(p.SpaceBefore)) + `"`)
	}
	if p.SpaceAfter != 0 {
		b.WriteString(` w:after="` + strconv.Itoa(twips(p.SpaceAfter)) + `"`)
	}
	if p.LineHeight != 0 {
		b.WriteString(` w:line="` + strconv.Itoa(lineRule(p.LineHeight)) + `" w:lineRule="auto"`)
	}
	b.WriteString(`/></w:pPr>`)
	return b.String()
}
