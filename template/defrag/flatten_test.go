package defrag_test

import (
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/template/defrag"
	"github.com/frankbardon/vellum/xmlcopy"
)

// TestFlatten_ConcatenatesTextAcrossRuns is the base case a marker split by
// Word's own editing depends on: three separate w:r runs read back as one
// contiguous string.
func TestFlatten_ConcatenatesTextAcrossRuns(t *testing.T) {
	src := wordDoc(`<w:p>` +
		`<w:r><w:t>Dear {{cust</w:t></w:r>` +
		`<w:r><w:t>omer_na</w:t></w:r>` +
		`<w:r><w:t>me}}, thanks.</w:t></w:r>` +
		`</w:p>`)
	f := flatten(t, src, paragraphSpan(t, src, 0))
	want := "Dear {{customer_name}}, thanks."
	if f.Text != want {
		t.Errorf("Text = %q, want %q", f.Text, want)
	}
}

// TestFlatten_DecodesEntities confirms the flattened text is the decoded
// reader-visible text, not the still-escaped source bytes — required for
// matching to work against text a marker actually named, and for a real
// ampersand elsewhere in the paragraph not to be mistaken for markup.
func TestFlatten_DecodesEntities(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>Widgets &amp; &lt;Gadgets&gt; Ltd. &#38; Co.</w:t></w:r></w:p>`)
	f := flatten(t, src, paragraphSpan(t, src, 0))
	want := "Widgets & <Gadgets> Ltd. & Co."
	if f.Text != want {
		t.Errorf("Text = %q, want %q", f.Text, want)
	}
}

// TestFlatten_NeverTrimsOrNormalisesWhitespace is the load-bearing test the
// story explicitly calls for: it fails if anyone "helpfully" adds a
// strings.TrimSpace, or any other whitespace normalisation, on this path.
// xml:space="preserve" means exactly what it says, and even without it
// Word's own whitespace inside a w:t is meaningful.
func TestFlatten_NeverTrimsOrNormalisesWhitespace(t *testing.T) {
	src := wordDoc(`<w:p>` +
		`<w:r><w:t xml:space="preserve">  leading, trailing and   internal   spaces  </w:t></w:r>` +
		`<w:r><w:t xml:space="preserve">	a tab and
a newline</w:t></w:r>` +
		`</w:p>`)
	f := flatten(t, src, paragraphSpan(t, src, 0))
	want := "  leading, trailing and   internal   spaces  " + "\ta tab and\na newline"
	if f.Text != want {
		t.Errorf("Text = %q, want %q — whitespace was altered", f.Text, want)
	}
}

// TestFlatten_RunWithNoTextContributesNothing covers the documented scope
// boundary: a run holding only a w:tab or a bookmark contributes zero runes,
// and the surrounding runs' text still reads back contiguous around it.
func TestFlatten_RunWithNoTextContributesNothing(t *testing.T) {
	src := wordDoc(`<w:p>` +
		`<w:r><w:t>before</w:t></w:r>` +
		`<w:r><w:tab/></w:r>` +
		`<w:bookmarkStart w:id="0" w:name="mark"/>` +
		`<w:bookmarkEnd w:id="0"/>` +
		`<w:r><w:t>after</w:t></w:r>` +
		`</w:p>`)
	f := flatten(t, src, paragraphSpan(t, src, 0))
	want := "beforeafter"
	if f.Text != want {
		t.Errorf("Text = %q, want %q", f.Text, want)
	}
}

// TestFlatten_DescendantRunInsideNestedContentControlCounted confirms a run
// nested inside a content control within the paragraph still contributes —
// the same "everything inside this span" reading template/anchor's own
// marker detection already uses over w:t elements.
func TestFlatten_DescendantRunInsideNestedContentControlCounted(t *testing.T) {
	src := wordDoc(`<w:p>` +
		`<w:r><w:t>Dear </w:t></w:r>` +
		`<w:sdt><w:sdtPr><w:tag w:val="x"/></w:sdtPr>` +
		`<w:sdtContent><w:r><w:t>{{name}}</w:t></w:r></w:sdtContent></w:sdt>` +
		`<w:r><w:t>, thanks.</w:t></w:r>` +
		`</w:p>`)
	f := flatten(t, src, paragraphSpan(t, src, 0))
	want := "Dear {{name}}, thanks."
	if f.Text != want {
		t.Errorf("Text = %q, want %q", f.Text, want)
	}
}

// TestFlatten_ContainerNotFoundIsCodedError confirms a span that does not
// match any element boundary in src is a coded, internal-invariant class
// failure rather than a panic or a silently empty Flat.
func TestFlatten_ContainerNotFoundIsCodedError(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>hello</w:t></w:r></w:p>`)
	bogus := xmlcopy.Span{Start: 0, End: 1}
	_, err := defrag.Flatten(src, bogus)
	if !verr.HasCode(err, verr.VELLUM_DEFRAG_CONTAINER_NOT_FOUND) {
		t.Fatalf("err = %v, want VELLUM_DEFRAG_CONTAINER_NOT_FOUND", err)
	}
}

// TestFlatten_EmptyContainerProducesEmptyText covers a paragraph with no
// runs at all.
func TestFlatten_EmptyContainerProducesEmptyText(t *testing.T) {
	src := wordDoc(`<w:p/>`)
	f := flatten(t, src, paragraphSpan(t, src, 0))
	if f.Text != "" {
		t.Errorf("Text = %q, want empty", f.Text)
	}
}

// TestFlatten_MalformedSourcePropagatesXMLMalformed confirms a source that
// does not parse as well-formed XML surfaces Walk's own coded error rather
// than a bare decode failure.
func TestFlatten_MalformedSourcePropagatesXMLMalformed(t *testing.T) {
	src := []byte(`<w:document><w:body><w:p><w:r><w:t>oops</w:r></w:p></w:body></w:document>`)
	_, err := defrag.Flatten(src, xmlcopy.Span{Start: 0, End: int64(len(src))})
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_XML_MALFORMED) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_XML_MALFORMED", err)
	}
}
