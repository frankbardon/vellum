package defrag

import (
	"sort"
	"strings"
	"unicode/utf8"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/xmlcopy"
)

// nsWordprocessing is the WordprocessingML main namespace, matched the same
// way template/anchor matches it: on the resolved namespace URI Walk reports,
// independent of whatever prefix the authoring tool bound it to.
const nsWordprocessing = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

// runInfo is one w:r run contributing to a Flat's flattened text.
type runInfo struct {
	// span is the whole <w:r>...</w:r> element.
	span xmlcopy.Span

	// rpr is the verbatim source bytes of the run's own <w:rPr>...</w:rPr>
	// child, already cloned out of the source so it outlives it independent
	// of what else touches the backing array. Nil when the run has no w:rPr.
	rpr []byte

	// text is this run's own decoded text: every w:t child's content,
	// concatenated in document order. Empty for a run holding only a w:tab,
	// w:br, a bookmark, or nothing at all — see the package doc's "scope
	// boundaries" section.
	text string

	// textLen is the rune count of text, cached so Locate does not
	// recompute it while resolving every match.
	textLen int

	// textStart is the rune offset into the owning Flat.Text where this
	// run's own text begins.
	textStart int

	// hadPreserve is true when at least one of the run's own w:t children
	// carried an explicit xml:space="preserve" attribute. It participates in
	// Piece.Preserve alongside the whitespace heuristic, per the package's
	// documented rule: an explicit source declaration is honoured even when
	// the surviving substring's own edges do not look like they need it.
	hadPreserve bool
}

// Flat is a container element's text and runs, flattened for matching and
// for resplicing. Built by [Flatten].
type Flat struct {
	// Text is the flattened, decoded text: every contributing run's own text,
	// concatenated in document order. Never normalised or trimmed —
	// xml:space="preserve" and Word's own meaningful whitespace inside a w:t
	// are exactly what a reader would see, and this string is what a caller
	// matches marker text against.
	Text string

	runs    []runInfo
	runeLen int
	content xmlcopy.Span // the container's own Content span
}

// Flatten walks src once and builds a [Flat] over every w:r run nested
// anywhere inside container — direct child or descendant, so a run sitting
// inside a nested content control within the same paragraph still
// contributes, matching the same "everything inside this span" reading
// template/anchor's own marker detection already uses.
//
// container is the whole element's span — open tag through close tag, the
// same shape [anchor.Anchor.Span] carries for a marker anchor — not its
// Content span; Flatten locates the element's own Content span itself while
// walking, so a zero-width [Flat.Locate] at the very start or end of an
// otherwise run-less container still has somewhere correct to anchor to.
//
// src must be the full source bytes container's span was computed against:
// Flatten walks all of src, not a slice of it, because a sub-slice starting
// partway through the document would lose whatever ancestor xmlns
// declaration resolves the very namespace prefixes container's own runs use.
func Flatten(src []byte, container xmlcopy.Span) (*Flat, error) {
	type wtChild struct {
		text     string
		preserve bool
	}
	type bucketNode struct {
		isWT    bool
		wt      wtChild
		isRPr   bool
		rprSpan xmlcopy.Span
	}

	buckets := make(map[int][]bucketNode) // scratch only: looked up and cleared by depth, never ranged
	var collected []runInfo
	var containerContent xmlcopy.Span
	var containerFound bool

	walkErr := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		children := buckets[e.Depth+1]
		delete(buckets, e.Depth+1)

		if e.Span.Start == container.Start && e.Span.End == container.End {
			containerContent = e.Content
			containerFound = true
		}

		var n bucketNode
		switch {
		case isWordEl(e, "t"):
			text, err := xmlcopy.DecodeText(src[e.Content.Start:e.Content.End])
			if err != nil {
				return err
			}
			n.isWT = true
			n.wt = wtChild{text: text, preserve: xmlSpacePreserve(e)}

		case isWordEl(e, "rPr"):
			n.isRPr = true
			n.rprSpan = e.Span

		case isWordEl(e, "r"):
			if withinSpan(e.Span, container) {
				var b strings.Builder
				var preserve bool
				var rprSpan xmlcopy.Span
				var hasRPr bool
				for _, c := range children {
					if c.isWT {
						b.WriteString(c.wt.text)
						if c.wt.preserve {
							preserve = true
						}
					}
					if c.isRPr {
						rprSpan = c.rprSpan
						hasRPr = true
					}
				}
				var rprBytes []byte
				if hasRPr {
					rprBytes = append([]byte(nil), src[rprSpan.Start:rprSpan.End]...)
				}
				text := b.String()
				collected = append(collected, runInfo{
					span:        e.Span,
					rpr:         rprBytes,
					text:        text,
					textLen:     utf8.RuneCountInString(text),
					hadPreserve: preserve,
				})
			}
		}

		buckets[e.Depth] = append(buckets[e.Depth], n)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	if !containerFound {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_DEFRAG_CONTAINER_NOT_FOUND,
			"the container span does not match any element found while walking the source",
			map[string]any{
				"container_span_start": container.Start,
				"container_span_end":   container.End,
			})
	}

	// Post-order visitation of a well-formed document already yields runs in
	// ascending document order — two elements at any depth close in the same
	// left-to-right order their start tags appeared, since XML nesting never
	// overlaps — but the sort is kept anyway, stable, for the same reason
	// template/anchor's own discovery sorts what Walk hands it: correctness
	// here should not rest on an unstated property of a traversal order this
	// package does not own.
	sort.SliceStable(collected, func(i, j int) bool { return collected[i].span.Start < collected[j].span.Start })

	var b strings.Builder
	pos := 0
	for i := range collected {
		collected[i].textStart = pos
		b.WriteString(collected[i].text)
		pos += collected[i].textLen
	}

	return &Flat{
		Text:    b.String(),
		runs:    collected,
		runeLen: pos,
		content: containerContent,
	}, nil
}

// isWordEl reports whether e is the named element in the WordprocessingML
// namespace, mirroring template/anchor's own helper of the same shape.
func isWordEl(e xmlcopy.Element, local string) bool {
	return e.Name.Space == nsWordprocessing && e.Name.Local == local
}

// xmlSpacePreserve reports whether e carries an explicit xml:space="preserve"
// attribute. Matched on the local name alone, the same as
// [xmlcopy]'s own test suite does, because xml: is a namespace the XML
// specification itself binds regardless of a document's own declarations —
// there is no prefix ambiguity to resolve by checking the namespace URI too.
func xmlSpacePreserve(e xmlcopy.Element) bool {
	for _, a := range e.Attr {
		if a.Name.Local == "space" {
			return a.Value == "preserve"
		}
	}
	return false
}

// withinSpan reports whether inner falls entirely inside outer, inclusive of
// touching boundaries — the containment test that decides whether a run
// belongs to the container Flatten was asked to flatten.
func withinSpan(inner, outer xmlcopy.Span) bool {
	return inner.Start >= outer.Start && inner.End <= outer.End
}
