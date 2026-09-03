package defrag_test

import (
	"bytes"
	"testing"

	"github.com/frankbardon/vellum/template/defrag"
	"github.com/frankbardon/vellum/xmlcopy"
)

// TestApply_EndToEndComposesOneReplacement is the test that actually proves
// this package's design composes with xmlcopy.Apply the way the package doc
// claims: Flatten, FindAll and Locate on a real multi-run paragraph, a
// Replacement assembled from Site/Piece/RenderRun exactly the way
// template/splice (E9-S4) is expected to, one xmlcopy.Apply pass, and then a
// full re-walk of the result to confirm every guarantee held at once —
// preserved formatting on both boundary runs, the discarded middle run gone,
// and nothing outside Affected disturbed by even one byte.
func TestApply_EndToEndComposesOneReplacement(t *testing.T) {
	const before = "Before "
	const after = " after."

	src := wordDoc(`<w:p>` +
		`<w:r><w:t>` + before + `</w:t></w:r>` +
		`<w:r><w:rPr><w:b/></w:rPr><w:t>Dear {{cust</w:t></w:r>` +
		`<w:r><w:rPr><w:i/></w:rPr><w:t>omer_na</w:t></w:r>` +
		`<w:r><w:rPr><w:u w:val="single"/></w:rPr><w:t>me}}, thanks</w:t></w:r>` +
		`<w:r><w:t>` + after + `</w:t></w:r>` +
		`</w:p>`)
	span := paragraphSpan(t, src, 0)
	f := flatten(t, src, span)
	runs := runSpans(t, src, span)
	if len(runs) != 5 {
		t.Fatalf("want 5 runs, got %d", len(runs))
	}

	m := oneMatch(t, f, "{{customer_name}}")
	site, err := f.Locate(m.Start, m.End)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}

	// The affected span covers runs 1..3 (bold, italic, underline), leaving
	// run 0 ("Before ") and run 4 (" after.") untouched.
	wantAffected := xmlcopy.Span{Start: runs[1].Start, End: runs[3].End}
	if site.Affected != wantAffected {
		t.Fatalf("Affected = %+v, want %+v", site.Affected, wantAffected)
	}

	newValue := `Acme & <Co>` // deliberately needs XML escaping
	var data []byte
	if site.Prefix != nil {
		data = append(data, defrag.RenderRun(site.Prefix)...)
	}
	data = append(data, []byte(`<w:r><w:t>`+xmlcopy.EscapeText(newValue)+`</w:t></w:r>`)...)
	if site.Suffix != nil {
		data = append(data, defrag.RenderRun(site.Suffix)...)
	}

	out, err := xmlcopy.Apply(src, []xmlcopy.Replacement{
		site.Affected.Replace(data),
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// The result must still parse.
	if err := xmlcopy.Walk(out, func(xmlcopy.Element) error { return nil }); err != nil {
		t.Fatalf("output does not parse: %v", err)
	}

	// Nothing before Affected moved by a single byte.
	if !bytes.Equal(out[:site.Affected.Start], src[:site.Affected.Start]) {
		t.Error("bytes before the affected span changed")
	}
	// Nothing after Affected moved by a single byte either: the tail of the
	// output past the new content equals the tail of the source past the old
	// content, since only the one span in between changed length.
	tailLen := len(src) - int(site.Affected.End)
	if !bytes.Equal(out[len(out)-tailLen:], src[site.Affected.End:]) {
		t.Error("bytes after the affected span changed")
	}

	// The discarded middle run's <w:i/> formatting is gone entirely.
	if bytes.Contains(out, []byte("<w:i/>")) {
		t.Error("the discarded middle run's <w:i/> formatting survived the splice")
	}

	// The two boundary runs' own formatting survived, attached to the
	// rebuilt Prefix/Suffix runs.
	if !bytes.Contains(out, []byte("<w:b/>")) {
		t.Error("the bold prefix run's own formatting did not survive")
	}
	if !bytes.Contains(out, []byte(`<w:u w:val="single"/>`)) {
		t.Error("the underlined suffix run's own formatting did not survive")
	}

	// The new value is present, correctly escaped in the raw bytes...
	if !bytes.Contains(out, []byte(`Acme &amp; &lt;Co&gt;`)) {
		t.Errorf("escaped new value not found in output: %s", out)
	}
	// ...and the paragraph reads back correctly once re-flattened, proving
	// the whole splice — trimmed boundary text, discarded middle run, and
	// substituted content — composed into one coherent, well-formed result.
	newSpan := paragraphSpan(t, out, 0)
	nf := flatten(t, out, newSpan)
	wantText := before + "Dear " + newValue + ", thanks" + after
	if nf.Text != wantText {
		t.Errorf("re-flattened text = %q, want %q", nf.Text, wantText)
	}
}
