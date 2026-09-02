package xmlcopy_test

import (
	stderrors "errors"
	"slices"
	"strings"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/xmlcopy"
)

// collect runs Walk and returns every element it visited, in the order Walk
// called visit — which is document order over closing tags, so the deepest
// element of a nested pair is collected before its parent.
func collect(t *testing.T, src []byte) []xmlcopy.Element {
	t.Helper()
	var out []xmlcopy.Element
	if err := xmlcopy.Walk(src, func(el xmlcopy.Element) error {
		out = append(out, el)
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	return out
}

func byLocalName(els []xmlcopy.Element, local string) []xmlcopy.Element {
	var out []xmlcopy.Element
	for _, el := range els {
		if el.Name.Local == local {
			out = append(out, el)
		}
	}
	return out
}

// TestWalk_SpanReproducesExactSourceBytes asserts the fundamental contract of
// every Element: Span always begins with '<' and ends with '>', and Content
// always falls within Span — for every element Walk visits over a realistic
// document, not just the ones a hand-picked assertion happens to check.
func TestWalk_SpanReproducesExactSourceBytes(t *testing.T) {
	src := []byte(wordSnippet)
	for _, el := range collect(t, src) {
		span := src[el.Span.Start:el.Span.End]
		if len(span) == 0 || span[0] != '<' {
			t.Errorf("element %s span does not start with '<': %q", el.Name.Local, span)
		}
		if span[len(span)-1] != '>' {
			t.Errorf("element %s span does not end with '>': %q", el.Name.Local, span)
		}
		if el.Content.Start < el.Span.Start || el.Content.End > el.Span.End {
			t.Errorf("element %s content span %v falls outside its own span %v", el.Name.Local, el.Content, el.Span)
		}
	}
}

// TestWalk_NamespacePrefixResolvedToURI confirms elements are matched by
// namespace URI rather than by the prefix a particular authoring tool chose,
// which is the whole reason Name is an expanded xml.Name.
func TestWalk_NamespacePrefixResolvedToURI(t *testing.T) {
	els := collect(t, []byte(wordSnippet))
	sdts := byLocalName(els, "sdt")
	if len(sdts) == 0 {
		t.Fatal("no <w:sdt> elements found")
	}
	for _, el := range sdts {
		if el.Name.Space != nsWordprocessing {
			t.Errorf("w:sdt Name.Space = %q, want %q", el.Name.Space, nsWordprocessing)
		}
	}
}

// TestWalk_NestedSameNameElements is the depth-counting case the anchor
// discovery story depends on: a <w:sdt> nested inside another <w:sdt> must be
// told apart from its sibling by depth, not merely by "the next close tag".
func TestWalk_NestedSameNameElements(t *testing.T) {
	src := []byte(wordSnippet)
	els := collect(t, src)
	sdts := byLocalName(els, "sdt")
	if len(sdts) != 3 {
		t.Fatalf("got %d <w:sdt> elements, want 3 (client-name, outer wrapper, inner nested)", len(sdts))
	}

	// Visit order is post-order over closing tags: the client-name control
	// closes (and is visited) entirely before the outer/inner pair begins: it
	// is element 0. Within the outer/inner pair the inner one closes first,
	// so it precedes its own parent: inner is element 1, outer is element 2.
	clientCtrl, innerCtrl, outerCtrl := sdts[0], sdts[1], sdts[2]

	if clientCtrl.Depth != outerCtrl.Depth {
		t.Errorf("client control depth %d != outer control depth %d; both are direct children of a <w:p>",
			clientCtrl.Depth, outerCtrl.Depth)
	}
	if innerCtrl.Depth != outerCtrl.Depth+2 {
		t.Errorf("inner control depth %d, want outer depth (%d) + 2 for the intervening <w:sdtContent>",
			innerCtrl.Depth, outerCtrl.Depth)
	}

	// The inner control's whole span must fall strictly inside the outer
	// control's span (nested), and the client control's span must not
	// overlap the outer control's span at all (sibling subtree).
	if innerCtrl.Span.Start < outerCtrl.Span.Start || innerCtrl.Span.End > outerCtrl.Span.End {
		t.Errorf("inner control span %v is not contained in outer control span %v", innerCtrl.Span, outerCtrl.Span)
	}
	if clientCtrl.Span.End > outerCtrl.Span.Start {
		t.Errorf("client control span %v overlaps outer control span %v; they are siblings under different <w:p> elements",
			clientCtrl.Span, outerCtrl.Span)
	}

	// The content each control's span covers is exactly what a fill-mode
	// splice would need: the client control's raw content still carries the
	// unresolved entity, proving Walk never decodes what it is only meant to
	// locate.
	clientContent := string(src[clientCtrl.Content.Start:clientCtrl.Content.End])
	if !strings.Contains(clientContent, "Acme &amp; Co.") {
		t.Errorf("client control content = %q, want it to contain the raw (undecoded) run text", clientContent)
	}
	innerContent := string(src[innerCtrl.Content.Start:innerCtrl.Content.End])
	if !strings.Contains(innerContent, "nested") {
		t.Errorf("inner control content = %q, want it to contain %q", innerContent, "nested")
	}
	outerContent := string(src[outerCtrl.Content.Start:outerCtrl.Content.End])
	if !strings.Contains(outerContent, "<w:sdt>") {
		t.Errorf("outer control content = %q, want it to contain the whole nested <w:sdt>", outerContent)
	}
}

// TestWalk_SelfClosingElement covers a self-closing tag with no children:
// content is empty and SelfClosing reports true, distinguishing it from an
// explicit empty pair written as two tags.
func TestWalk_SelfClosingElement(t *testing.T) {
	src := []byte(wordSnippet)
	els := collect(t, src)

	paras := byLocalName(els, "p")
	if len(paras) != 3 {
		t.Fatalf("got %d <w:p> elements, want 3", len(paras))
	}
	// The third <w:p> in the fixture is written as <w:p/>.
	selfClosing := paras[2]
	if !selfClosing.SelfClosing() {
		t.Errorf("<w:p/> not reported as self-closing")
	}
	if !selfClosing.Content.Empty() {
		t.Errorf("self-closing element content span is not empty: %v", selfClosing.Content)
	}
	if string(src[selfClosing.Span.Start:selfClosing.Span.End]) != "<w:p/>" {
		t.Errorf("self-closing span = %q, want %q", src[selfClosing.Span.Start:selfClosing.Span.End], "<w:p/>")
	}

	// The first two <w:p> elements are written with explicit content and are
	// not self-closing, even the ones whose *content* happens to be empty
	// were there to be one.
	if paras[0].SelfClosing() {
		t.Errorf("first <w:p> incorrectly reported self-closing")
	}
}

// TestWalk_ExplicitEmptyElementIsNotSelfClosing distinguishes <a></a> from
// <a/>: both parse to an element with empty Content, but only the former
// consumed bytes for a real closing tag.
func TestWalk_ExplicitEmptyElementIsNotSelfClosing(t *testing.T) {
	src := []byte(`<root><a></a><b/></root>`)
	els := collect(t, src)

	a := byLocalName(els, "a")[0]
	b := byLocalName(els, "b")[0]

	if a.SelfClosing() {
		t.Error("<a></a> reported as self-closing")
	}
	if !a.Content.Empty() {
		t.Errorf("<a></a> content span is not empty: %v", a.Content)
	}
	if string(src[a.Span.Start:a.Span.End]) != "<a></a>" {
		t.Errorf("<a></a> span = %q", src[a.Span.Start:a.Span.End])
	}

	if !b.SelfClosing() {
		t.Error("<b/> not reported as self-closing")
	}
	if string(src[b.Span.Start:b.Span.End]) != "<b/>" {
		t.Errorf("<b/> span = %q", src[b.Span.Start:b.Span.End])
	}
}

// TestWalk_AttributesInDocumentOrder confirms Attr preserves source order and
// resolves prefixed attribute names to their namespace URI, including
// xmlns declarations themselves, which the decoder reports rather than
// consuming silently.
func TestWalk_AttributesInDocumentOrder(t *testing.T) {
	src := []byte(wordSnippet)
	els := collect(t, src)

	paras := byLocalName(els, "p")
	first := paras[0]
	if len(first.Attr) != 2 {
		t.Fatalf("first <w:p> has %d attributes, want 2: %+v", len(first.Attr), first.Attr)
	}
	if first.Attr[0].Name.Local != "paraId" || first.Attr[0].Name.Space != nsWord14 {
		t.Errorf("attr 0 = %+v, want local paraId in %s", first.Attr[0], nsWord14)
	}
	if first.Attr[0].Value != "1A2B3C4D" {
		t.Errorf("attr 0 value = %q, want %q", first.Attr[0].Value, "1A2B3C4D")
	}
	if first.Attr[1].Name.Local != "rsidR" || first.Attr[1].Name.Space != nsWordprocessing {
		t.Errorf("attr 1 = %+v, want local rsidR in %s", first.Attr[1], nsWordprocessing)
	}

	// Three content controls in the fixture each carry a <w:tag>: the
	// client-name control (first <w:p>) and the outer/inner pair (second
	// <w:p>), in that document order.
	tags := byLocalName(els, "tag")
	if len(tags) != 3 {
		t.Fatalf("got %d <w:tag> elements, want 3", len(tags))
	}
	var vals []string
	for _, tagEl := range tags {
		for _, a := range tagEl.Attr {
			if a.Name.Local == "val" {
				vals = append(vals, a.Value)
			}
		}
	}
	if want := []string{"client_name", "outer", "inner"}; !slices.Equal(vals, want) {
		t.Errorf("tag values = %v, want %v", vals, want)
	}
}

// TestWalk_XMLSpacePreserveAttributeSurvives confirms an attribute with no
// namespace prefix — xml:space is special-cased by the XML spec itself as
// bound to a fixed namespace regardless of a document's own declarations —
// is captured intact rather than dropped.
func TestWalk_XMLSpacePreserveAttributeSurvives(t *testing.T) {
	src := []byte(wordSnippet)
	els := collect(t, src)
	ts := byLocalName(els, "t")
	if len(ts) == 0 {
		t.Fatal("no <w:t> elements found")
	}
	first := ts[0]
	var found bool
	for _, a := range first.Attr {
		if a.Name.Local == "space" && a.Value == "preserve" {
			found = true
		}
	}
	if !found {
		t.Errorf("first <w:t> attributes = %+v, want an xml:space=\"preserve\" attribute", first.Attr)
	}
}

// TestWalk_CDATASurvivesUninterpreted proves Walk locates spans without
// decoding what is inside them: the CDATA markers and the raw '<', '&'
// characters they protect stay exactly as written in Content, because Walk
// never reconstructs content — it only records where it starts and ends.
func TestWalk_CDATASurvivesUninterpreted(t *testing.T) {
	src := []byte(cdataSnippet)
	els := collect(t, src)
	refs := byLocalName(els, "schemaRefs")
	if len(refs) != 1 {
		t.Fatalf("got %d <ds:schemaRefs> elements, want 1", len(refs))
	}
	content := string(src[refs[0].Content.Start:refs[0].Content.End])
	want := "<![CDATA[raw <data> & stuff]]>"
	if content != want {
		t.Errorf("content = %q, want %q", content, want)
	}
}

// TestWalk_VisitErrorStopsEarly confirms a visit error aborts the walk
// immediately, and is returned to the caller unwrapped, so a caller can use a
// sentinel to mean "found it, stop" and distinguish that from a genuine
// failure with errors.Is.
func TestWalk_VisitErrorStopsEarly(t *testing.T) {
	sentinel := stderrors.New("found it")
	src := []byte(wordSnippet)

	var visited int
	err := xmlcopy.Walk(src, func(el xmlcopy.Element) error {
		visited++
		if el.Name.Local == "tag" {
			return sentinel
		}
		return nil
	})
	if !stderrors.Is(err, sentinel) {
		t.Fatalf("Walk error = %v, want it to wrap %v", err, sentinel)
	}

	total := len(collect(t, src))
	if visited == 0 || visited >= total {
		t.Errorf("visited %d elements before stopping, want somewhere strictly between 0 and the full %d", visited, total)
	}
}

// TestWalk_MalformedSourceIsCoded covers the family of documents that do not
// parse: a mismatched close tag, an unterminated attribute, and a document
// truncated mid-element. Each must fail with
// VELLUM_TEMPLATE_XML_MALFORMED, and never panic.
func TestWalk_MalformedSourceIsCoded(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"mismatched close tag", `<a><b></c></a>`},
		{"unterminated attribute", `<a b="1></a>`},
		{"truncated mid-element", `<a><b>`},
		{"close tag with no open", `</a>`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := xmlcopy.Walk([]byte(tt.src), func(xmlcopy.Element) error { return nil })
			if err == nil {
				t.Fatal("Walk succeeded on malformed input")
			}
			if !verr.HasCode(err, verr.VELLUM_TEMPLATE_XML_MALFORMED) {
				t.Errorf("Walk error = %v, want VELLUM_TEMPLATE_XML_MALFORMED", err)
			}
		})
	}
}

// TestWalk_WellFormedFragmentWithNoElementsIsFine confirms an XML document
// consisting only of a single empty element does not error, and that Walk on
// a truly empty byte slice (io.EOF on the very first Token call) returns no
// error and visits nothing.
func TestWalk_WellFormedFragmentWithNoElementsIsFine(t *testing.T) {
	if err := xmlcopy.Walk(nil, func(xmlcopy.Element) error { return nil }); err != nil {
		t.Errorf("Walk(nil, ...) = %v, want nil", err)
	}
	var visited int
	if err := xmlcopy.Walk([]byte(`<a/>`), func(xmlcopy.Element) error { visited++; return nil }); err != nil {
		t.Errorf("Walk = %v, want nil", err)
	}
	if visited != 1 {
		t.Errorf("visited = %d, want 1", visited)
	}
}
