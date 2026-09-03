package defrag_test

import (
	"reflect"
	"testing"

	"github.com/frankbardon/vellum/template/defrag"
)

func TestFindAll_SingleOccurrence(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>Dear {{customer_name}}, thanks.</w:t></w:r></w:p>`)
	f := flatten(t, src, paragraphSpan(t, src, 0))

	got := f.FindAll("{{customer_name}}")
	want := []defrag.Match{{Start: 5, End: 5 + runeLen("{{customer_name}}")}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FindAll = %+v, want %+v", got, want)
	}
	if got := runeSlice(f.Text, want[0].Start, want[0].End); got != "{{customer_name}}" {
		t.Errorf("matched slice = %q, want %q", got, "{{customer_name}}")
	}
}

func TestFindAll_MultipleNonOverlappingOccurrences(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>{{a}} and {{a}} and {{a}}</w:t></w:r></w:p>`)
	f := flatten(t, src, paragraphSpan(t, src, 0))

	got := f.FindAll("{{a}}")
	if len(got) != 3 {
		t.Fatalf("got %d matches, want 3: %+v", len(got), got)
	}
	for i, m := range got {
		if runeSlice(f.Text, m.Start, m.End) != "{{a}}" {
			t.Errorf("match %d slice = %q, want %q", i, runeSlice(f.Text, m.Start, m.End), "{{a}}")
		}
	}
	// Left to right, in ascending order, with no overlap.
	for i := 1; i < len(got); i++ {
		if got[i].Start < got[i-1].End {
			t.Errorf("match %d starts before match %d ends: %+v", i, i-1, got)
		}
	}
}

func TestFindAll_NoOccurrenceReturnsNil(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>nothing to find here</w:t></w:r></w:p>`)
	f := flatten(t, src, paragraphSpan(t, src, 0))

	if got := f.FindAll("{{missing}}"); got != nil {
		t.Errorf("FindAll = %+v, want nil", got)
	}
}

func TestFindAll_EmptyLiteralReturnsNil(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>anything</w:t></w:r></w:p>`)
	f := flatten(t, src, paragraphSpan(t, src, 0))

	if got := f.FindAll(""); got != nil {
		t.Errorf("FindAll(\"\") = %+v, want nil", got)
	}
}

// TestFindAll_MarkerFragmentedAcrossRunsStillMatches proves FindAll operates
// on the flattened text, so a marker Word split mid-word across several runs
// is found exactly like an unfragmented one.
func TestFindAll_MarkerFragmentedAcrossRunsStillMatches(t *testing.T) {
	src := wordDoc(`<w:p>` +
		`<w:r><w:t>Dear {{cust</w:t></w:r>` +
		`<w:r><w:t>omer_na</w:t></w:r>` +
		`<w:r><w:t>me}}, thanks.</w:t></w:r>` +
		`</w:p>`)
	f := flatten(t, src, paragraphSpan(t, src, 0))

	got := f.FindAll("{{customer_name}}")
	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1: %+v", len(got), got)
	}
}

// TestFindAll_UnicodeOffsetsAreRuneIndicesNotByteIndices exercises a
// multi-byte character before the match, proving Start/End are rune indices
// rather than byte offsets into f.Text.
func TestFindAll_UnicodeOffsetsAreRuneIndicesNotByteIndices(t *testing.T) {
	src := wordDoc(`<w:p><w:r><w:t>café {{name}}</w:t></w:r></w:p>`)
	f := flatten(t, src, paragraphSpan(t, src, 0))

	got := f.FindAll("{{name}}")
	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1: %+v", len(got), got)
	}
	wantStart := runeLen("café ") // "é" is one rune but two UTF-8 bytes
	if got[0].Start != wantStart {
		t.Errorf("Start = %d, want %d (rune index, not byte index)", got[0].Start, wantStart)
	}
	if got := runeSlice(f.Text, got[0].Start, got[0].End); got != "{{name}}" {
		t.Errorf("matched slice = %q, want %q", got, "{{name}}")
	}
}
