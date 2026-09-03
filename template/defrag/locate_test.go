package defrag_test

import (
	"bytes"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/template/defrag"
	"github.com/frankbardon/vellum/xmlcopy"
)

// TestLocate_MarkerEntirelyWithinOneRun covers the simplest case: the run's
// own text is exactly the matched marker, with nothing else in it, so
// Affected is exactly that run's own span and neither Prefix nor Suffix has
// anything to preserve.
func TestLocate_MarkerEntirelyWithinOneRun(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>{{customer_name}}</w:t></w:r></w:p>`)
	span := paragraphSpan(t, src, 0)
	f := flatten(t, src, span)
	runs := runSpans(t, src, span)
	if len(runs) != 1 {
		t.Fatalf("want 1 run, got %d", len(runs))
	}

	m := oneMatch(t, f, "{{customer_name}}")
	site, err := f.Locate(m.Start, m.End)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}

	if site.Affected != runs[0] {
		t.Errorf("Affected = %+v, want the run's own span %+v", site.Affected, runs[0])
	}
	if site.Prefix != nil {
		t.Errorf("Prefix = %+v, want nil", site.Prefix)
	}
	if site.Suffix != nil {
		t.Errorf("Suffix = %+v, want nil", site.Suffix)
	}
}

// TestLocate_MarkerSplitMidWordAcrossTwoRuns is the spell-checker case: the
// marker's own braces and name are split across two runs, mid-identifier,
// neither boundary aligned with the match. Both halves need Prefix/Suffix.
func TestLocate_MarkerSplitMidWordAcrossTwoRuns(t *testing.T) {
	src := wordDoc(`<w:p>` +
		`<w:r><w:t>Dear {{cust</w:t></w:r>` +
		`<w:r><w:t>omer_name}}, thanks.</w:t></w:r>` +
		`</w:p>`)
	span := paragraphSpan(t, src, 0)
	f := flatten(t, src, span)
	runs := runSpans(t, src, span)
	if len(runs) != 2 {
		t.Fatalf("want 2 runs, got %d", len(runs))
	}

	m := oneMatch(t, f, "{{customer_name}}")
	site, err := f.Locate(m.Start, m.End)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}

	wantAffected := xmlcopy.Span{Start: runs[0].Start, End: runs[1].End}
	if site.Affected != wantAffected {
		t.Errorf("Affected = %+v, want %+v", site.Affected, wantAffected)
	}
	if site.Prefix == nil {
		t.Fatal("Prefix = nil, want non-nil")
	}
	if want := runeSlice(f.Text, 0, m.Start); site.Prefix.Text != want {
		t.Errorf("Prefix.Text = %q, want %q", site.Prefix.Text, want)
	}
	if site.Suffix == nil {
		t.Fatal("Suffix = nil, want non-nil")
	}
	if want := runeSlice(f.Text, m.End, runeLen(f.Text)); site.Suffix.Text != want {
		t.Errorf("Suffix.Text = %q, want %q", site.Suffix.Text, want)
	}
}

// TestLocate_ThreeRunSplitMiddleRunDiscarded covers a marker split across
// three runs where the middle run is entirely consumed by the match — no
// partial boundary of its own. Its w:rPr must not survive anywhere, while
// the two true boundary runs' non-matched text and formatting must.
func TestLocate_ThreeRunSplitMiddleRunDiscarded(t *testing.T) {
	src := wordDoc(`<w:p>` +
		`<w:r><w:rPr><w:b/></w:rPr><w:t>Dear {{cust</w:t></w:r>` +
		`<w:r><w:rPr><w:i/></w:rPr><w:t>omer_na</w:t></w:r>` +
		`<w:r><w:rPr><w:u w:val="single"/></w:rPr><w:t>me}}, thanks.</w:t></w:r>` +
		`</w:p>`)
	span := paragraphSpan(t, src, 0)
	f := flatten(t, src, span)
	runs := runSpans(t, src, span)
	if len(runs) != 3 {
		t.Fatalf("want 3 runs, got %d", len(runs))
	}

	m := oneMatch(t, f, "{{customer_name}}")
	site, err := f.Locate(m.Start, m.End)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}

	wantAffected := xmlcopy.Span{Start: runs[0].Start, End: runs[2].End}
	if site.Affected != wantAffected {
		t.Errorf("Affected = %+v, want %+v (spanning all three runs, including the discarded middle one)", site.Affected, wantAffected)
	}

	if site.Prefix == nil {
		t.Fatal("Prefix = nil, want non-nil")
	}
	if want := "Dear "; site.Prefix.Text != want {
		t.Errorf("Prefix.Text = %q, want %q", site.Prefix.Text, want)
	}
	if want := []byte(`<w:rPr><w:b/></w:rPr>`); !bytes.Equal(site.Prefix.RPr, want) {
		t.Errorf("Prefix.RPr = %q, want %q", site.Prefix.RPr, want)
	}

	if site.Suffix == nil {
		t.Fatal("Suffix = nil, want non-nil")
	}
	if want := ", thanks."; site.Suffix.Text != want {
		t.Errorf("Suffix.Text = %q, want %q", site.Suffix.Text, want)
	}
	if want := []byte(`<w:rPr><w:u w:val="single"/></w:rPr>`); !bytes.Equal(site.Suffix.RPr, want) {
		t.Errorf("Suffix.RPr = %q, want %q", site.Suffix.RPr, want)
	}

	// The middle run's own formatting must not leak into either surviving
	// Piece: it is not the same bytes as either boundary run's RPr.
	middleRPr := []byte(`<w:rPr><w:i/></w:rPr>`)
	if bytes.Equal(site.Prefix.RPr, middleRPr) {
		t.Error("Prefix carries the discarded middle run's formatting")
	}
	if bytes.Equal(site.Suffix.RPr, middleRPr) {
		t.Error("Suffix carries the discarded middle run's formatting")
	}
}

// TestLocate_UnknownRunPropertyPreservedVerbatim proves an rPr child Vellum's
// own style model has no concept of — here, a language mark and a
// synthetic revision-save-style attribute placed directly on rPr — survives
// byte for byte because it is cloned rather than reconstructed.
func TestLocate_UnknownRunPropertyPreservedVerbatim(t *testing.T) {
	rpr := `<w:rPr w:rsidRPr="00AA1234"><w:lang w:val="en-US"/></w:rPr>`
	src := wordDoc(`<w:p><w:r>` + rpr + `<w:t>Dear {{customer_name}}, thanks.</w:t></w:r></w:p>`)
	span := paragraphSpan(t, src, 0)
	f := flatten(t, src, span)

	m := oneMatch(t, f, "{{customer_name}}")
	site, err := f.Locate(m.Start, m.End)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if site.Prefix == nil {
		t.Fatal("Prefix = nil, want non-nil")
	}
	if !bytes.Equal(site.Prefix.RPr, []byte(rpr)) {
		t.Errorf("Prefix.RPr = %q, want %q (verbatim, including the unmodelled w:lang and rsid attribute)", site.Prefix.RPr, rpr)
	}
}

// TestLocate_SplitOnRunBoundaryStartOnly covers a match that begins exactly
// where a run begins (Prefix nil) but ends mid-run (Suffix non-nil).
func TestLocate_SplitOnRunBoundaryStartOnly(t *testing.T) {
	src := wordDoc(`<w:p>` +
		`<w:r><w:t>Dear </w:t></w:r>` +
		`<w:r><w:t>{{customer_name}}, thanks.</w:t></w:r>` +
		`</w:p>`)
	span := paragraphSpan(t, src, 0)
	f := flatten(t, src, span)

	m := oneMatch(t, f, "{{customer_name}}")
	site, err := f.Locate(m.Start, m.End)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if site.Prefix != nil {
		t.Errorf("Prefix = %+v, want nil (match starts exactly on the run boundary)", site.Prefix)
	}
	if site.Suffix == nil {
		t.Fatal("Suffix = nil, want non-nil")
	}
	if want := ", thanks."; site.Suffix.Text != want {
		t.Errorf("Suffix.Text = %q, want %q", site.Suffix.Text, want)
	}
}

// TestLocate_SplitOnRunBoundaryEndOnly mirrors the previous test: the match
// ends exactly where a run ends (Suffix nil) but starts mid-run (Prefix
// non-nil).
func TestLocate_SplitOnRunBoundaryEndOnly(t *testing.T) {
	src := wordDoc(`<w:p>` +
		`<w:r><w:t>Dear {{customer_name}}</w:t></w:r>` +
		`<w:r><w:t>, thanks.</w:t></w:r>` +
		`</w:p>`)
	span := paragraphSpan(t, src, 0)
	f := flatten(t, src, span)

	m := oneMatch(t, f, "{{customer_name}}")
	site, err := f.Locate(m.Start, m.End)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if site.Prefix == nil {
		t.Fatal("Prefix = nil, want non-nil")
	}
	if want := "Dear "; site.Prefix.Text != want {
		t.Errorf("Prefix.Text = %q, want %q", site.Prefix.Text, want)
	}
	if site.Suffix != nil {
		t.Errorf("Suffix = %+v, want nil (match ends exactly on the run boundary)", site.Suffix)
	}
}

// TestLocate_RunWithNoRPrProducesNilPieceRPr covers a boundary run carrying
// no w:rPr at all: Piece.RPr must be nil, not an empty non-nil slice, so
// RenderRun knows to omit the element entirely rather than emit an empty one.
func TestLocate_RunWithNoRPrProducesNilPieceRPr(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>Dear {{customer_name}}, thanks.</w:t></w:r></w:p>`)
	span := paragraphSpan(t, src, 0)
	f := flatten(t, src, span)

	m := oneMatch(t, f, "{{customer_name}}")
	site, err := f.Locate(m.Start, m.End)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if site.Prefix == nil || site.Suffix == nil {
		t.Fatalf("expected both Prefix and Suffix, got %+v", site)
	}
	if site.Prefix.RPr != nil {
		t.Errorf("Prefix.RPr = %q, want nil", site.Prefix.RPr)
	}
	if site.Suffix.RPr != nil {
		t.Errorf("Suffix.RPr = %q, want nil", site.Suffix.RPr)
	}

	rendered := defrag.RenderRun(site.Prefix)
	if bytes.Contains(rendered, []byte("<w:rPr")) {
		t.Errorf("RenderRun emitted a w:rPr element for a Piece with nil RPr: %s", rendered)
	}
	if !bytes.Contains(rendered, []byte("<w:r><w:t")) {
		t.Errorf("RenderRun did not open <w:r><w:t as expected: %s", rendered)
	}
}

// TestLocate_ExplicitPreserveSurvivesWithoutWhitespaceNeed covers the source
// w:t carrying xml:space="preserve" explicitly, on a Piece whose own sliced
// text has no leading or trailing whitespace to trigger the heuristic on its
// own — Preserve must still be true, because the source attribute is
// honoured on its own terms.
func TestLocate_ExplicitPreserveSurvivesWithoutWhitespaceNeed(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t xml:space="preserve">plain{{marker}}</w:t></w:r></w:p>`)
	span := paragraphSpan(t, src, 0)
	f := flatten(t, src, span)

	m := oneMatch(t, f, "{{marker}}")
	site, err := f.Locate(m.Start, m.End)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if site.Prefix == nil {
		t.Fatal("Prefix = nil, want non-nil")
	}
	if site.Prefix.Text != "plain" {
		t.Fatalf("Prefix.Text = %q, want %q", site.Prefix.Text, "plain")
	}
	if !site.Prefix.Preserve {
		t.Error(`Preserve = false, want true: the source w:t declared xml:space="preserve" explicitly`)
	}
	if site.Suffix != nil {
		t.Errorf("Suffix = %+v, want nil", site.Suffix)
	}
}

// TestLocate_WhitespaceHeuristicWithoutExplicitPreserve is the reverse: no
// xml:space attribute in the source, but the surviving Piece text has
// significant leading whitespace of its own — Preserve must be true from the
// heuristic alone.
func TestLocate_WhitespaceHeuristicWithoutExplicitPreserve(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>{{marker}} tail</w:t></w:r></w:p>`)
	span := paragraphSpan(t, src, 0)
	f := flatten(t, src, span)

	m := oneMatch(t, f, "{{marker}}")
	site, err := f.Locate(m.Start, m.End)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if site.Prefix != nil {
		t.Errorf("Prefix = %+v, want nil", site.Prefix)
	}
	if site.Suffix == nil {
		t.Fatal("Suffix = nil, want non-nil")
	}
	if want := " tail"; site.Suffix.Text != want {
		t.Fatalf("Suffix.Text = %q, want %q", site.Suffix.Text, want)
	}
	if !site.Suffix.Preserve {
		t.Error("Preserve = false, want true: the surviving text has significant leading whitespace")
	}
}

// TestLocate_EntitiesInMatchedAndPreservedTextRoundTrip covers an ampersand
// and angle brackets both inside the matched text's surroundings — proving
// the flattened text is correctly decoded for matching, and that RenderRun's
// re-escaping of the preserved edges round-trips through a real parse.
func TestLocate_EntitiesInMatchedAndPreservedTextRoundTrip(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>Acme &amp; Co. {{marker}} &lt;tag&gt; end</w:t></w:r></w:p>`)
	span := paragraphSpan(t, src, 0)
	f := flatten(t, src, span)

	m := oneMatch(t, f, "{{marker}}")
	site, err := f.Locate(m.Start, m.End)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if site.Prefix == nil || site.Suffix == nil {
		t.Fatalf("expected both Prefix and Suffix, got %+v", site)
	}
	if want := "Acme & Co. "; site.Prefix.Text != want {
		t.Errorf("Prefix.Text = %q, want %q", site.Prefix.Text, want)
	}
	if want := " <tag> end"; site.Suffix.Text != want {
		t.Errorf("Suffix.Text = %q, want %q", site.Suffix.Text, want)
	}

	for _, tt := range []struct {
		name string
		raw  []byte
		want string
	}{
		{"prefix", defrag.RenderRun(site.Prefix), "Acme & Co. "},
		{"suffix", defrag.RenderRun(site.Suffix), " <tag> end"},
	} {
		// Wrap the rendered run in a minimal paragraph and re-walk it: the
		// re-escaped bytes must parse, and decoding the w:t content back
		// must reproduce exactly the original decoded text.
		wrapped := append(append([]byte(`<w:p>`), tt.raw...), []byte(`</w:p>`)...)
		var gotText string
		var found bool
		if err := xmlcopy.Walk(wrapped, func(e xmlcopy.Element) error {
			if e.Name.Local == "t" {
				decoded, derr := xmlcopy.DecodeText(wrapped[e.Content.Start:e.Content.End])
				if derr != nil {
					return derr
				}
				gotText = decoded
				found = true
			}
			return nil
		}); err != nil {
			t.Fatalf("%s: Walk on rendered run: %v (%s)", tt.name, err, wrapped)
		}
		if !found {
			t.Fatalf("%s: rendered run carries no w:t: %s", tt.name, tt.raw)
		}
		if gotText != tt.want {
			t.Errorf("%s: round-tripped text = %q, want %q", tt.name, gotText, tt.want)
		}
	}
}

// TestLocate_ZeroWidthInsertionMidRunSplits covers the legitimate zero-width
// match: an insertion point strictly inside a run's own text splits that run
// into a Prefix and a Suffix around the point, with nothing removed.
func TestLocate_ZeroWidthInsertionMidRunSplits(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>Hello world</w:t></w:r></w:p>`)
	span := paragraphSpan(t, src, 0)
	f := flatten(t, src, span)
	runs := runSpans(t, src, span)

	pos := runeLen("Hello") // insert right after "Hello", before the space
	site, err := f.Locate(pos, pos)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if site.Affected != runs[0] {
		t.Errorf("Affected = %+v, want the whole run %+v", site.Affected, runs[0])
	}
	if site.Prefix == nil || site.Suffix == nil {
		t.Fatalf("expected both Prefix and Suffix for a mid-run insertion, got %+v", site)
	}
	if site.Prefix.Text != "Hello" {
		t.Errorf("Prefix.Text = %q, want %q", site.Prefix.Text, "Hello")
	}
	if site.Suffix.Text != " world" {
		t.Errorf("Suffix.Text = %q, want %q", site.Suffix.Text, " world")
	}
}

// TestLocate_ZeroWidthInsertionAtRunBoundaryDoesNotSplit covers a zero-width
// match aligned exactly with a run boundary: neither run is touched, and
// Affected is a zero-width span sitting exactly at the boundary.
func TestLocate_ZeroWidthInsertionAtRunBoundaryDoesNotSplit(t *testing.T) {
	src := wordDoc(`<w:p>` +
		`<w:r><w:t>Hello </w:t></w:r>` +
		`<w:r><w:t>world</w:t></w:r>` +
		`</w:p>`)
	span := paragraphSpan(t, src, 0)
	f := flatten(t, src, span)
	runs := runSpans(t, src, span)

	pos := runeLen("Hello ")
	site, err := f.Locate(pos, pos)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if !site.Affected.Empty() {
		t.Errorf("Affected = %+v, want a zero-width span", site.Affected)
	}
	if site.Affected.Start != runs[1].Start {
		t.Errorf("Affected.Start = %d, want the second run's own start %d", site.Affected.Start, runs[1].Start)
	}
	if site.Prefix != nil || site.Suffix != nil {
		t.Errorf("expected neither Prefix nor Suffix, got %+v", site)
	}
}

// TestLocate_ZeroWidthInsertionAtVeryEnd covers an insertion point past
// every run's own text — appending at the very end of the container's
// content — which anchors at the end of the last run rather than at any run
// boundary in the middle.
func TestLocate_ZeroWidthInsertionAtVeryEnd(t *testing.T) {
	src := wordDoc(`<w:p>` +
		`<w:r><w:t>Hello </w:t></w:r>` +
		`<w:r><w:t>world</w:t></w:r>` +
		`</w:p>`)
	span := paragraphSpan(t, src, 0)
	f := flatten(t, src, span)
	runs := runSpans(t, src, span)

	end := runeLen("Hello world")
	site, err := f.Locate(end, end)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if !site.Affected.Empty() {
		t.Errorf("Affected = %+v, want a zero-width span", site.Affected)
	}
	if site.Affected.Start != runs[len(runs)-1].End {
		t.Errorf("Affected.Start = %d, want the last run's own end %d", site.Affected.Start, runs[len(runs)-1].End)
	}
	if site.Prefix != nil || site.Suffix != nil {
		t.Errorf("expected neither Prefix nor Suffix, got %+v", site)
	}
}

// TestLocate_ZeroWidthInsertionInEmptyContainer covers a container with no
// runs at all: the only possible insertion point is the container's own
// Content span, which is what a caller anchors a first run to.
func TestLocate_ZeroWidthInsertionInEmptyContainer(t *testing.T) {
	src := wordDoc(`<w:p/>`)
	span := paragraphSpan(t, src, 0)
	f := flatten(t, src, span)

	site, err := f.Locate(0, 0)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if !site.Affected.Empty() {
		t.Errorf("Affected = %+v, want a zero-width span", site.Affected)
	}
	if site.Prefix != nil || site.Suffix != nil {
		t.Errorf("expected neither Prefix nor Suffix, got %+v", site)
	}
	// The anchor point sits within the paragraph's own span — not before its
	// opening tag — which for a self-closing <w:p/> means exactly at its
	// Content span, positioned where content would begin per
	// xmlcopy.Element.SelfClosing's own doc comment.
	if site.Affected.Start < span.Start || site.Affected.Start > span.End {
		t.Errorf("Affected.Start = %d, want it within the paragraph span %+v", site.Affected.Start, span)
	}
}

// TestLocate_ErrorsOnInvalidRanges covers every shape of an invalid Locate
// call: negative start, end past the flattened text's bounds, and an
// inverted range.
func TestLocate_ErrorsOnInvalidRanges(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>hello</w:t></w:r></w:p>`)
	span := paragraphSpan(t, src, 0)
	f := flatten(t, src, span)

	cases := []struct {
		name       string
		start, end int
	}{
		{"negative start", -1, 2},
		{"end past bounds", 0, 999},
		{"inverted range", 3, 1},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.Locate(tt.start, tt.end)
			if !verr.HasCode(err, verr.VELLUM_DEFRAG_RANGE_INVALID) {
				t.Fatalf("err = %v, want VELLUM_DEFRAG_RANGE_INVALID", err)
			}
		})
	}
}
