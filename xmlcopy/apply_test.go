package xmlcopy_test

import (
	"bytes"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/xmlcopy"
)

// TestApply_NoReplacementsIsByteIdentical is the round-trip identity every
// other guarantee in this package rests on: an unmutated part passed through
// Apply with nothing to replace comes back exactly as it went in, including
// the self-closing tags, mixed namespace prefixes, out-of-alphabetical-order
// attributes, entities and xml:space="preserve" the fixture carries.
func TestApply_NoReplacementsIsByteIdentical(t *testing.T) {
	for name, src := range map[string]string{
		"wordSnippet":  wordSnippet,
		"cdataSnippet": cdataSnippet,
	} {
		t.Run(name, func(t *testing.T) {
			out, err := xmlcopy.Apply([]byte(src), nil)
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if !bytes.Equal(out, []byte(src)) {
				t.Errorf("Apply with no replacements changed the bytes")
			}
		})
	}
}

// TestApply_WalkThenApplyRoundTrips is the end-to-end shape fill mode
// actually uses: locate a content control's Content span with Walk, replace
// it, and confirm the byte outside that span is untouched while the span
// itself carries the new content — escaped, because the caller (here, the
// test standing in for template/splice) is the one responsible for that.
func TestApply_WalkThenApplyRoundTrips(t *testing.T) {
	src := []byte(wordSnippet)

	// A real splice replaces the content control's sdtContent, not the whole
	// control: sdtPr carries the alias, tag and id fill mode identified the
	// control by, and those must survive the edit untouched. Find the
	// client-name control, then find its own direct sdtContent child (not
	// the unrelated one belonging to the other, nested pair of controls).
	var target xmlcopy.Element
	var targetFound bool
	var sdtContents []xmlcopy.Element
	if err := xmlcopy.Walk(src, func(el xmlcopy.Element) error {
		switch el.Name.Local {
		case "sdtContent":
			sdtContents = append(sdtContents, el)
		case "sdt":
			if bytes.Contains(src[el.Content.Start:el.Content.End], []byte("client_name")) {
				target = el
				targetFound = true
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if !targetFound {
		t.Fatal("did not find the client-name content control")
	}

	var content xmlcopy.Element
	var contentFound bool
	for _, sc := range sdtContents {
		if sc.Depth == target.Depth+1 && sc.Span.Start >= target.Span.Start && sc.Span.End <= target.Span.End {
			content, contentFound = sc, true
			break
		}
	}
	if !contentFound {
		t.Fatal("did not find the control's own sdtContent")
	}

	newText := xmlcopy.EscapeText(`Widgets & <Gadgets> Ltd.`)
	replacement := []byte(`<w:r><w:t>` + newText + `</w:t></w:r>`)

	out, err := xmlcopy.Apply(src, []xmlcopy.Replacement{
		content.Content.Replace(replacement),
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Everything before the replaced span is untouched byte for byte.
	if !bytes.Equal(out[:content.Content.Start], src[:content.Content.Start]) {
		t.Errorf("bytes before the replaced span changed")
	}
	// Everything after the replaced span is untouched too: the tail of the
	// output past the new content must equal the tail of the source past the
	// old content, since only the one span in between changed length.
	if !bytes.Equal(out[len(out)-(len(src)-int(content.Content.End)):], src[content.Content.End:]) {
		t.Errorf("bytes after the replaced span changed")
	}

	if bytes.Contains(out, []byte("Acme &amp; Co.")) {
		t.Errorf("old run text is still present after replacement")
	}
	if !bytes.Contains(out, []byte("Widgets &amp; &lt;Gadgets&gt; Ltd.")) {
		t.Errorf("new escaped run text is not present: %s", out)
	}
	if !bytes.Contains(out, []byte(`<w:tag w:val="client_name"/>`)) {
		t.Errorf("sdtPr metadata, which was not touched, is missing from the output")
	}
	if !bytes.Contains(out, []byte(`<w:tag w:val="inner"/>`)) {
		t.Errorf("the unrelated nested content control was disturbed by the splice")
	}

	// The result must itself be well-formed XML.
	if err := xmlcopy.Walk(out, func(xmlcopy.Element) error { return nil }); err != nil {
		t.Errorf("spliced output does not parse: %v", err)
	}
}

// TestApply_SingleReplacement covers the simplest case in isolation.
func TestApply_SingleReplacement(t *testing.T) {
	src := []byte("0123456789")
	out, err := xmlcopy.Apply(src, []xmlcopy.Replacement{
		{Start: 3, End: 6, Data: []byte("XYZ")},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, want := string(out), "012XYZ6789"; got != want {
		t.Errorf("Apply = %q, want %q", got, want)
	}
}

// TestApply_MultipleNonAdjacentReplacements covers several replacements
// spread across the source with untouched bytes between them.
func TestApply_MultipleNonAdjacentReplacements(t *testing.T) {
	src := []byte("0123456789")
	out, err := xmlcopy.Apply(src, []xmlcopy.Replacement{
		{Start: 1, End: 2, Data: []byte("A")},
		{Start: 4, End: 5, Data: []byte("BB")},
		{Start: 8, End: 9, Data: []byte("C")},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, want := string(out), "0A23BB567C9"; got != want {
		t.Errorf("Apply = %q, want %q", got, want)
	}
}

// TestApply_ReplacementAtStart covers a replacement beginning at offset 0.
func TestApply_ReplacementAtStart(t *testing.T) {
	src := []byte("0123456789")
	out, err := xmlcopy.Apply(src, []xmlcopy.Replacement{
		{Start: 0, End: 3, Data: []byte("X")},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, want := string(out), "X3456789"; got != want {
		t.Errorf("Apply = %q, want %q", got, want)
	}
}

// TestApply_ReplacementAtEnd covers a replacement ending exactly at len(src).
func TestApply_ReplacementAtEnd(t *testing.T) {
	src := []byte("0123456789")
	out, err := xmlcopy.Apply(src, []xmlcopy.Replacement{
		{Start: 7, End: 10, Data: []byte("Y")},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, want := string(out), "0123456Y"; got != want {
		t.Errorf("Apply = %q, want %q", got, want)
	}
}

// TestApply_ReplacementCoveringWholeSource covers replacing everything.
func TestApply_ReplacementCoveringWholeSource(t *testing.T) {
	src := []byte("0123456789")
	out, err := xmlcopy.Apply(src, []xmlcopy.Replacement{
		{Start: 0, End: 10, Data: []byte("<empty/>")},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, want := string(out), "<empty/>"; got != want {
		t.Errorf("Apply = %q, want %q", got, want)
	}
}

// TestApply_AdjacentReplacementsAreNotOverlapping confirms two replacements
// that touch at a boundary (one's End equals the next one's Start) are
// accepted: they do not share a byte, so they are not overlapping.
func TestApply_AdjacentReplacementsAreNotOverlapping(t *testing.T) {
	src := []byte("0123456789")
	out, err := xmlcopy.Apply(src, []xmlcopy.Replacement{
		{Start: 0, End: 5, Data: []byte("A")},
		{Start: 5, End: 10, Data: []byte("B")},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, want := string(out), "AB"; got != want {
		t.Errorf("Apply = %q, want %q", got, want)
	}
}

// TestApply_ZeroWidthInsertion covers a pure insertion at a point, and two
// insertions at the very same point in the order supplied.
func TestApply_ZeroWidthInsertion(t *testing.T) {
	src := []byte("0123456789")
	out, err := xmlcopy.Apply(src, []xmlcopy.Replacement{
		{Start: 5, End: 5, Data: []byte("[first]")},
		{Start: 5, End: 5, Data: []byte("[second]")},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, want := string(out), "01234[first][second]56789"; got != want {
		t.Errorf("Apply = %q, want %q", got, want)
	}
}

// TestApply_RejectsOverlap covers two replacements whose spans share bytes.
func TestApply_RejectsOverlap(t *testing.T) {
	src := []byte("0123456789")
	_, err := xmlcopy.Apply(src, []xmlcopy.Replacement{
		{Start: 0, End: 5, Data: []byte("A")},
		{Start: 4, End: 8, Data: []byte("B")},
	})
	if err == nil {
		t.Fatal("Apply succeeded on overlapping spans")
	}
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_XML_SPAN_INVALID) {
		t.Errorf("error = %v, want VELLUM_TEMPLATE_XML_SPAN_INVALID", err)
	}
}

// TestApply_RejectsOutOfOrder covers two non-overlapping spans supplied in
// descending order: Apply does not sort them, it rejects the input.
func TestApply_RejectsOutOfOrder(t *testing.T) {
	src := []byte("0123456789")
	_, err := xmlcopy.Apply(src, []xmlcopy.Replacement{
		{Start: 6, End: 8, Data: []byte("A")},
		{Start: 1, End: 2, Data: []byte("B")},
	})
	if err == nil {
		t.Fatal("Apply succeeded on out-of-order spans")
	}
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_XML_SPAN_INVALID) {
		t.Errorf("error = %v, want VELLUM_TEMPLATE_XML_SPAN_INVALID", err)
	}
}

// TestApply_RejectsOutOfBounds covers every shape of an invalid span:
// negative start, end past the source, and end before start.
func TestApply_RejectsOutOfBounds(t *testing.T) {
	src := []byte("0123456789")
	cases := []struct {
		name string
		r    xmlcopy.Replacement
	}{
		{"negative start", xmlcopy.Replacement{Start: -1, End: 2, Data: []byte("A")}},
		{"end past source", xmlcopy.Replacement{Start: 5, End: 11, Data: []byte("A")}},
		{"end before start", xmlcopy.Replacement{Start: 5, End: 3, Data: []byte("A")}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := xmlcopy.Apply(src, []xmlcopy.Replacement{tt.r})
			if err == nil {
				t.Fatal("Apply succeeded on an invalid span")
			}
			if !verr.HasCode(err, verr.VELLUM_TEMPLATE_XML_SPAN_INVALID) {
				t.Errorf("error = %v, want VELLUM_TEMPLATE_XML_SPAN_INVALID", err)
			}
		})
	}
}

// TestApply_EmptySourceWithNoReplacements is the degenerate case: an empty
// part with nothing to do produces empty output, not nil-vs-empty surprises
// or a panic.
func TestApply_EmptySourceWithNoReplacements(t *testing.T) {
	out, err := xmlcopy.Apply(nil, nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("Apply(nil, nil) = %q, want empty", out)
	}
}

// TestSpan_ReplaceHelper covers the small convenience constructor.
func TestSpan_ReplaceHelper(t *testing.T) {
	s := xmlcopy.Span{Start: 2, End: 5}
	r := s.Replace([]byte("x"))
	if r.Start != 2 || r.End != 5 || string(r.Data) != "x" {
		t.Errorf("Replace = %+v", r)
	}
	if got := r.Span(); got != s {
		t.Errorf("Replacement.Span() = %+v, want %+v", got, s)
	}
}

// TestSpan_Empty covers the zero-width predicate directly.
func TestSpan_Empty(t *testing.T) {
	if !(xmlcopy.Span{Start: 5, End: 5}).Empty() {
		t.Error("Span{5,5}.Empty() = false, want true")
	}
	if (xmlcopy.Span{Start: 5, End: 6}).Empty() {
		t.Error("Span{5,6}.Empty() = true, want false")
	}
}
