package anchor

// discoverPPTX is E11-S2's pptx discoverer: it finds one anchor mechanism —
// a top-level shape's own name, KindShape — resolved through the real
// ppt/presentation.xml -> <p:sldIdLst> -> relationship graph rather than a
// hardcoded "ppt/slides/*.xml" glob, mirroring discoverXLSX's own
// resolve-through-the-relationship-graph discipline for the same reason: a
// template assembled by a tool with looser part-naming habits than Word's or
// Excel's own must still resolve correctly.
//
// # Slide order and duplicate scope
//
// Slides are walked in the order <p:sldIdLst> itself lists them — the
// template's own deck order — rather than a bytewise part-name sort the way
// discoverXLSX sorts its own cross-part anchors: xlsx anchor names (a
// defined name, a table's own displayName) are inherently workbook-scoped,
// so a bytewise part sort is merely a tiebreaker; a slide's own presentation
// order is a fact a human reading an Inspect report actually cares about,
// and it costs nothing extra to preserve since the sldIdLst walk already
// produces it.
//
// A shape name is only ever meaningful within its own slide's shape tree —
// PowerPoint has no notion of a shape name shared or scoped across slides —
// but [bind.Frame.Anchors] resolves a Bind statement's anchor name through
// one flat, template-wide map keyed by Name alone (see template/fill.go),
// so two different slides both carrying a shape named, say, "Title 1" (which
// PowerPoint's own default naming makes common) would silently collide: a
// binding statement naming "Title 1" could only ever reach whichever one
// last claimed the map entry. Duplicate detection is therefore global across
// every slide part in the template, exactly like DOCX's single-mainPart rule
// and xlsx's cross-part rule, not scoped per slide — a real deck reusing the
// same shape name across several slides needs those shapes distinctly named
// before it is usable as a fill template for anything beyond a single-slide
// clone target. A slide-clone repeat (see bind.RepeatTargetSlide) is exactly
// the intended way to reuse the same shape names across N instances of one
// slide without tripping this rule: the anchors live once, in the template's
// one un-repeated slide, and cloning produces the copies structurally rather
// than by discovering several same-named originals.

import (
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/xmlcopy"
)

const (
	// nsPresentation is the PresentationML main namespace, matched by
	// resolved URI rather than a literal "p:" prefix guess, mirroring this
	// package's own nsWordprocessing/nsSpreadsheet constants and the same
	// reasoning.
	nsPresentation = "http://schemas.openxmlformats.org/presentationml/2006/main"

	// relSlide is the relationship type ppt/presentation.xml's own
	// <p:sldId r:id="..."/> resolves through to a slide part.
	relSlide = nsRelationships + "/slide"
)

// slideRef is one <p:sldId> entry read from the presentation part: its own
// relationship id, in <p:sldIdLst> document order.
type slideRef struct {
	rID string
}

// shapeCandidate is a top-level <p:sp> shape found while walking one slide's
// own shape tree, carrying a non-empty <p:cNvPr name="...">.
type shapeCandidate struct {
	name  string
	descr string
	span  xmlcopy.Span
}

// discoverPPTX walks the presentation part once for its own <p:sldIdLst>,
// resolves each entry to a real slide part through the relationship graph,
// then walks every slide part once for its own top-level shapes.
func discoverPPTX(pkg *opc.Package, mainPart string) (*Inventory, error) {
	presSrc, err := partBytes(pkg, mainPart)
	if err != nil {
		return nil, err
	}

	slides, err := scanSlideIDList(presSrc)
	if err != nil {
		return nil, err
	}

	var anchors []Anchor
	seen := make(map[string]xlsxSeenEntry, len(slides)) // lookup only, never ranged; shared shape with xlsx.go's own cross-part duplicate tracking

	for _, sl := range slides {
		slidePart, ok := resolveRelFrom(pkg, mainPart, sl.rID, relSlide)
		if !ok {
			continue // a <p:sldId> whose r:id does not resolve to a real slide
			// relationship is a pptx structural problem outside this story's own
			// scope to reject; nothing here needs it to.
		}

		slideSrc, err := partBytes(pkg, slidePart)
		if err != nil {
			return nil, err
		}

		candidates, err := scanSlideShapes(slideSrc)
		if err != nil {
			return nil, err
		}

		for _, c := range candidates {
			if c.name == "" {
				// No usable identifier: not an error, see KindShape's own doc
				// comment on why an empty name is simply not discovered.
				continue
			}
			a := Anchor{
				Name:  c.name,
				Kind:  KindShape,
				Alias: c.descr,
				Part:  slidePart,
				Span:  c.span,
			}
			if prior, dup := seen[a.Name]; dup {
				return nil, duplicateXLSXAnchorErr(a.Name, prior, xlsxSeenEntry{kind: a.Kind, part: a.Part, span: a.Span})
			}
			seen[a.Name] = xlsxSeenEntry{kind: a.Kind, part: a.Part, span: a.Span}
			anchors = append(anchors, a)
		}
	}

	// Deliberately no final sort: anchors is already in the order that
	// matters — slide-presentation order (from the sldIdLst walk), then
	// shape-document order within each slide (natural Walk visitation order
	// for siblings) — see this file's own doc comment for why that beats a
	// bytewise part-name sort here.
	return &Inventory{Anchors: anchors}, nil
}

// scanSlideIDList walks the presentation part once for its own
// <p:sldIdLst><p:sldId r:id="..."/></p:sldIdLst> entries, in document order.
func scanSlideIDList(src []byte) ([]slideRef, error) {
	var refs []slideRef
	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if e.Name.Space == nsPresentation && e.Name.Local == "sldId" {
			if rid, ok := attrVal2(e, nsRelationships, "id"); ok {
				refs = append(refs, slideRef{rID: rid})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return refs, nil
}

// scanSlideShapes walks one slide part once, collecting every top-level
// <p:sp> shape — a direct child of the slide's own <p:spTree>, not one
// nested inside a <p:grpSp> group or inside a <p:graphicFrame> table cell —
// together with its own <p:cNvPr name="..."> and descr="..." attributes.
//
// This uses the same per-depth bucket technique docx.go's own scanDocument
// and template/defrag's flatten.go use to attribute a direct child
// correctly: every element, when it closes, claims whatever accumulated in
// buckets[depth+1] since the last claim, which in a single linear pass over
// well-formed XML is exactly its own direct children. A <p:cNvPr> nested
// inside a <p:pic>, a <p:graphicFrame> or a <p:grpSp> is never propagated up
// to a matched "sp" or "spTree" case here, which is what keeps a picture's,
// a table's or a grouped shape's own name out of this story's v1 scope
// without needing a separate exclusion rule: the switch below simply never
// looks at those element kinds.
func scanSlideShapes(src []byte) ([]shapeCandidate, error) {
	type bucketNode struct {
		el       xmlcopy.Element
		isSp     bool
		hasCNvPr bool
		name     string
		descr    string
	}
	buckets := make(map[int][]bucketNode) // scratch only: looked up and cleared by depth, never ranged

	var candidates []shapeCandidate

	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		children := buckets[e.Depth+1]
		delete(buckets, e.Depth+1)

		var n bucketNode
		n.el = e

		switch {
		case e.Name.Space == nsPresentation && e.Name.Local == "cNvPr":
			n.hasCNvPr = true
			n.name, _ = attrVal2(e, "", "name")
			n.descr, _ = attrVal2(e, "", "descr")

		case e.Name.Space == nsPresentation && e.Name.Local == "nvSpPr":
			for _, c := range children {
				if c.hasCNvPr {
					n.hasCNvPr = true
					n.name = c.name
					n.descr = c.descr
				}
			}

		case e.Name.Space == nsPresentation && e.Name.Local == "sp":
			n.isSp = true
			for _, c := range children {
				if c.el.Name.Space == nsPresentation && c.el.Name.Local == "nvSpPr" {
					n.hasCNvPr = c.hasCNvPr
					n.name = c.name
					n.descr = c.descr
				}
			}

		case e.Name.Space == nsPresentation && e.Name.Local == "spTree":
			for _, c := range children {
				if c.isSp {
					candidates = append(candidates, shapeCandidate{
						name:  c.name,
						descr: c.descr,
						span:  c.el.Span,
					})
				}
			}
		}

		buckets[e.Depth] = append(buckets[e.Depth], n)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return candidates, nil
}
