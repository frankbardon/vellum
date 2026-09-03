package splice

import (
	"testing"

	"github.com/frankbardon/vellum/fragment"
)

// White-box tests for layerRPr/insertRPrChild, in package splice (not
// splice_test) since the schema-ordered insertion point is exactly the
// judgement call this story documents and is worth pinning directly, not
// just observing through a full Splice round trip.

func TestLayerRPr_NilBasisWithNoOverridesStaysNil(t *testing.T) {
	if got := layerRPr(nil, fragment.TextStyle{}); got != nil {
		t.Errorf("got %q, want nil", got)
	}
}

func TestLayerRPr_NilBasisBuildsFreshRPr(t *testing.T) {
	got := layerRPr(nil, fragment.TextStyle{Bold: true})
	want := "<w:rPr><w:b/><w:bCs/></w:rPr>"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLayerRPr_AllThreeInSchemaOrder(t *testing.T) {
	got := layerRPr(nil, fragment.TextStyle{Bold: true, Italic: true, Underline: true})
	want := `<w:rPr><w:b/><w:bCs/><w:i/><w:iCs/><w:u w:val="single"/></w:rPr>`
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLayerRPr_DoesNotDuplicateAnExistingToggle(t *testing.T) {
	basis := []byte("<w:rPr><w:b/></w:rPr>")
	got := layerRPr(basis, fragment.TextStyle{Bold: true})
	want := "<w:rPr><w:b/></w:rPr>"
	if string(got) != want {
		t.Errorf("got %q, want %q (unchanged)", got, want)
	}
}

func TestLayerRPr_InsertsBoldBeforeALaterSchemaElement(t *testing.T) {
	// w:sz sits well after b/bCs in CT_RPr's own sequence; bold must land
	// before it, not simply appended at the end of the element.
	basis := []byte(`<w:rPr><w:sz w:val="24"/></w:rPr>`)
	got := layerRPr(basis, fragment.TextStyle{Bold: true})
	want := `<w:rPr><w:b/><w:bCs/><w:sz w:val="24"/></w:rPr>`
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLayerRPr_InsertsUnderlineAfterHighlightBeforeLang(t *testing.T) {
	// w:u sits between w:highlight and w:lang in CT_RPr's sequence: adding
	// underline to an rPr that already carries both must land in between,
	// not before highlight and not after lang.
	basis := []byte(`<w:rPr><w:highlight w:val="yellow"/><w:lang w:val="en-US"/></w:rPr>`)
	got := layerRPr(basis, fragment.TextStyle{Underline: true})
	want := `<w:rPr><w:highlight w:val="yellow"/><w:u w:val="single"/><w:lang w:val="en-US"/></w:rPr>`
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLayerRPr_BoldAndUnderlineLandOnOppositeSidesOfAnInterveningElement(t *testing.T) {
	// Adding both bold and underline to an rPr that already carries w:sz
	// (between b/i and u in the schema) must place bold before sz and
	// underline after it — two different insertion points for two
	// different additions in the same call.
	basis := []byte(`<w:rPr><w:sz w:val="24"/></w:rPr>`)
	got := layerRPr(basis, fragment.TextStyle{Bold: true, Underline: true})
	want := `<w:rPr><w:b/><w:bCs/><w:sz w:val="24"/><w:u w:val="single"/></w:rPr>`
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLayerRPr_SelfClosingRPrIsExpanded(t *testing.T) {
	basis := []byte("<w:rPr/>")
	got := layerRPr(basis, fragment.TextStyle{Italic: true})
	want := "<w:rPr><w:i/><w:iCs/></w:rPr>"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLayerRPr_BasisIsNeverMutated(t *testing.T) {
	basis := []byte(`<w:rPr><w:sz w:val="24"/></w:rPr>`)
	original := string(basis)
	_ = layerRPr(basis, fragment.TextStyle{Bold: true, Italic: true, Underline: true})
	if string(basis) != original {
		t.Errorf("basis was mutated: got %q, want %q", basis, original)
	}
}

func TestHasChild_DoesNotFalsePositiveOnPrefixedSibling(t *testing.T) {
	// "b" must not match inside "bCs" or "bdo".
	if hasChild([]byte("<w:rPr><w:bCs/></w:rPr>"), "b") {
		t.Error("hasChild incorrectly matched <w:bCs/> as <w:b>")
	}
	if !hasChild([]byte("<w:rPr><w:b/><w:bCs/></w:rPr>"), "b") {
		t.Error("hasChild failed to find the real <w:b/>")
	}
}
