package anchor

import (
	"sort"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/xmlcopy"
)

// nsWordprocessing is the WordprocessingML main namespace. Matching on the
// resolved namespace URI, as xmlcopy.Element.Name reports it, rather than on
// a literal "w:" prefix is what makes the match independent of whichever
// prefix a particular authoring tool bound it to.
const nsWordprocessing = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

// nativeCandidate is a w:sdt element that carries a non-empty w:tag, found
// while walking the document once.
type nativeCandidate struct {
	name  string
	alias string
	el    xmlcopy.Element
}

// bucketNode is scratch state kept per element while walking, so that when a
// w:sdt or w:sdtPr element closes it can look back at what its own direct
// children were.
type bucketNode struct {
	el     xmlcopy.Element
	hasTag bool
	tag    string
	alias  string
}

// discoverDOCX walks a DOCX main part once, collecting native content
// controls and marker text.
func discoverDOCX(pkg *opc.Package, mainPart string) (*Inventory, error) {
	part, ok := pkg.Get(mainPart)
	if !ok {
		// Reachable only by a caller invoking anchor.Discover directly with a
		// (pkg, mainPart) pair that did not come from a successful
		// template.Open — that call already proved the part exists. Not
		// something an untrusted template's own bytes can trigger.
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"anchor discovery was given a main part name the package does not contain",
			map[string]any{"part": mainPart})
	}
	src, err := part.Bytes()
	if err != nil {
		return nil, err
	}

	natives, paragraphs, texts, err := scanDocument(src)
	if err != nil {
		return nil, err
	}

	// Sort everything into document order before any duplicate check runs, so
	// that "the first anchor with this name" means the one a person reading
	// the document top to bottom would meet first, not an artefact of Walk's
	// post-order visitation (which, for two nested w:sdt, visits the inner
	// one before the outer one that contains it).
	sort.SliceStable(natives, func(i, j int) bool { return natives[i].el.Span.Start < natives[j].el.Span.Start })
	sort.SliceStable(paragraphs, func(i, j int) bool { return paragraphs[i].Span.Start < paragraphs[j].Span.Start })
	sort.SliceStable(texts, func(i, j int) bool { return texts[i].Content.Start < texts[j].Content.Start })

	seen := make(map[string]seenEntry, len(natives)) // lookup only, never ranged
	var anchors []Anchor

	for _, n := range natives {
		entry := seenEntry{kind: KindNative, span: n.el.Span}
		if prior, dup := seen[n.name]; dup {
			return nil, duplicateAnchorErr(mainPart, n.name, prior, entry)
		}
		seen[n.name] = entry
		anchors = append(anchors, Anchor{
			Name:  n.name,
			Kind:  KindNative,
			Alias: n.alias,
			Part:  mainPart,
			Span:  n.el.Span,
		})
	}

	for _, p := range paragraphs {
		text := concatText(src, p, texts)
		names, mErr := scanMarkers(text, mainPart, p.Span)
		if mErr != nil {
			return nil, mErr
		}
		for _, name := range names {
			entry := seenEntry{kind: KindMarker, span: p.Span}
			if prior, dup := seen[name]; dup {
				return nil, duplicateAnchorErr(mainPart, name, prior, entry)
			}
			seen[name] = entry
			anchors = append(anchors, Anchor{
				Name: name,
				Kind: KindMarker,
				Part: mainPart,
				Span: p.Span,
			})
		}
	}

	// Empty, not an error: a template with zero anchors is a fact this
	// package reports rather than refuses.
	sort.SliceStable(anchors, func(i, j int) bool { return anchors[i].Span.Start < anchors[j].Span.Start })
	return &Inventory{Anchors: anchors}, nil
}

// scanDocument walks src once and returns every native w:sdt candidate
// carrying a non-empty w:tag, every w:p paragraph, and every w:t text node —
// in whatever order xmlcopy.Walk's post-order visitation produced them. The
// caller sorts.
//
// Attributing a w:tag / w:alias value to the specific w:sdt that encloses it
// — and not to some other w:sdt elsewhere in the document at the same
// nesting depth, nor to a nested one it happens to contain — needs knowing
// direct parent/child structure, which Walk's flat, depth-only callback does
// not hand over by itself. buckets reconstructs just enough of it: every
// element, when it closes, claims whatever was accumulated in
// buckets[depth+1] since the last time something at this depth claimed it —
// which, in a single linear pass over a well-formed document, is exactly its
// own direct children, because only one element at a given depth can be
// "open" (start tag consumed, end tag not yet) at any moment.
func scanDocument(src []byte) (natives []nativeCandidate, paragraphs, texts []xmlcopy.Element, err error) {
	buckets := make(map[int][]bucketNode) // scratch only: looked up and cleared by depth, never ranged

	walkErr := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		children := buckets[e.Depth+1]
		delete(buckets, e.Depth+1)

		var n bucketNode
		n.el = e

		switch {
		case isWordEl(e, "p"):
			paragraphs = append(paragraphs, e)

		case isWordEl(e, "t"):
			texts = append(texts, e)

		case isWordEl(e, "tag"):
			if v := attrVal(e, "val"); v != "" {
				n.hasTag = true
				n.tag = v
			}

		case isWordEl(e, "alias"):
			n.alias = attrVal(e, "val")

		case isWordEl(e, "sdtPr"):
			for _, c := range children {
				if c.hasTag {
					n.hasTag = true
					n.tag = c.tag
				}
				if c.alias != "" {
					n.alias = c.alias
				}
			}

		case isWordEl(e, "sdt"):
			for _, c := range children {
				if isWordEl(c.el, "sdtPr") {
					n.hasTag = c.hasTag
					n.tag = c.tag
					n.alias = c.alias
				}
			}
			// A w:sdt with no w:tag (or an empty one) names nothing to bind
			// against and is not a fillable anchor — Word inserts these for
			// built-in fields such as the table of contents, and they are
			// skipped rather than surfaced.
			if n.hasTag {
				natives = append(natives, nativeCandidate{name: n.tag, alias: n.alias, el: e})
			}
		}

		buckets[e.Depth] = append(buckets[e.Depth], n)
		return nil
	})
	if walkErr != nil {
		return nil, nil, nil, walkErr
	}
	return natives, paragraphs, texts, nil
}

// isWordEl reports whether e is the named element in the WordprocessingML
// namespace.
func isWordEl(e xmlcopy.Element, local string) bool {
	return e.Name.Space == nsWordprocessing && e.Name.Local == local
}

// attrVal returns the value of the named attribute in the WordprocessingML
// namespace (w:val, in every case this package reads), or the empty string.
func attrVal(e xmlcopy.Element, local string) string {
	for _, a := range e.Attr {
		if a.Name.Space == nsWordprocessing && a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// concatText joins the text of every w:t element contained in paragraph p,
// in document order. texts must already be sorted by Content.Start; the
// filter preserves that order, so this concatenation is immune to run
// fragmentation for detection purposes even without defrag's run-level
// position map: a {{marker}} Word split mid-word across several w:r runs
// still reads back as one contiguous string here, because the runs
// themselves are contiguous in the source even when their boundaries are
// not where the marker's braces happen to fall.
//
// This intentionally does not decode XML entities. The two characters this
// package searches for, '{' and '}', are never represented as entities, so a
// raw byte slice of the content span is sufficient and does not risk
// mis-locating a marker inside a run that happens to carry an "&amp;" or
// similar.
func concatText(src []byte, p xmlcopy.Element, texts []xmlcopy.Element) string {
	var b strings.Builder
	for _, t := range texts {
		if t.Content.Start >= p.Content.Start && t.Content.End <= p.Content.End {
			b.Write(src[t.Content.Start:t.Content.End])
		}
	}
	return b.String()
}

// scanMarkers finds every {{name}} occurrence in text and returns the names,
// in the order they appear.
//
// A marker name is the text strictly between "{{" and the next "}}", with
// leading and trailing whitespace trimmed — so "{{ name }}" and "{{name}}"
// bind against the same key, which is the friendlier reading of what a
// template author who added a space meant. It must be non-empty after
// trimming, and it may not itself contain "{{" or "}}": Vellum does not
// attempt a best-effort interpretation of malformed marker syntax (CLAUDE.md:
// no lenient mode), so a "{{" with no closing "}}" before the paragraph ends,
// and a "{{}}" with nothing (or only whitespace) inside it, are both refused
// rather than silently skipped or silently accepted as a marker with a
// strange name.
//
// Deeper validation of what a name may contain — restricting it to a legal
// identifier, or to a FEEL expression — belongs to E10's binding story, which
// knows the expression grammar. This is detection only.
func scanMarkers(text, part string, paragraph xmlcopy.Span) ([]string, error) {
	var names []string
	i := 0
	for {
		start := strings.Index(text[i:], "{{")
		if start < 0 {
			break
		}
		start += i

		rel := strings.Index(text[start+2:], "}}")
		if rel < 0 {
			return nil, markerMalformedErr(part, paragraph, start,
				"a {{ marker has no closing }} before the paragraph ends")
		}
		end := start + 2 + rel // index of the closing "}}"

		inner := text[start+2 : end]
		if strings.Contains(inner, "{{") {
			// A second "{{" opened before the first one closed: the first
			// marker never got a legitimate close.
			return nil, markerMalformedErr(part, paragraph, start,
				"a {{ marker has no closing }} before another {{ begins")
		}

		name := strings.TrimSpace(inner)
		if name == "" {
			return nil, markerMalformedErr(part, paragraph, start,
				"a {{}} marker's name is empty")
		}

		names = append(names, name)
		i = end + 2
	}
	return names, nil
}

func markerMalformedErr(part string, paragraph xmlcopy.Span, offset int, message string) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_ANCHOR_MARKER_MALFORMED, message,
		map[string]any{
			"part":                     part,
			"paragraph_span_start":     paragraph.Start,
			"paragraph_span_end":       paragraph.End,
			"offset_in_paragraph_text": offset,
		})
}

// seenEntry records where an anchor name was first claimed, so a later
// collision — native-native, native-marker, or marker-marker — can name both
// locations.
type seenEntry struct {
	kind Kind
	span xmlcopy.Span
}

func duplicateAnchorErr(part, name string, first, second seenEntry) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_ANCHOR_DUPLICATE,
		"two anchors in the same part share one name",
		map[string]any{
			"part":              part,
			"name":              name,
			"first_kind":        string(first.kind),
			"first_span_start":  first.span.Start,
			"first_span_end":    first.span.End,
			"second_kind":       string(second.kind),
			"second_span_start": second.span.Start,
			"second_span_end":   second.span.End,
		})
}
