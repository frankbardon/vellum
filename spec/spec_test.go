package spec_test

import (
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/spec"
)

func TestAllBlockKinds_RegistryIsComplete(t *testing.T) {
	kinds := spec.AllBlockKinds()
	want := []spec.BlockKind{
		spec.BlockHeading, spec.BlockText, spec.BlockAsset, spec.BlockTable,
		spec.BlockPageBreak, spec.BlockNotes, spec.BlockSpacer,
	}
	if len(kinds) != len(want) {
		t.Fatalf("AllBlockKinds returned %d kinds, want %d: %v", len(kinds), len(want), kinds)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("kind %d = %q, want %q", i, kinds[i], want[i])
		}
	}
}

func TestAllBlockKinds_ReturnsCopy(t *testing.T) {
	first := spec.AllBlockKinds()
	original := first[0]
	first[0] = "mutated"

	if spec.AllBlockKinds()[0] != original {
		t.Error("AllBlockKinds returned the backing slice; the capability matrix and the published schema both read this registry")
	}
}

func TestValidBlockKind(t *testing.T) {
	for _, k := range spec.AllBlockKinds() {
		if !spec.ValidBlockKind(k) {
			t.Errorf("registered kind %q reported invalid", k)
		}
	}
	for _, k := range []spec.BlockKind{"", "sidebar", "Heading", "cover", "executive_summary"} {
		if spec.ValidBlockKind(k) {
			t.Errorf("kind %q reported valid; the vocabulary is closed and semantic section types belong to the consumer", k)
		}
	}
}

func TestValidate(t *testing.T) {
	ok := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Sections: []spec.Section{{
			ID: "s1",
			Blocks: []spec.Block{
				{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 1, Content: "Title"}},
				{Kind: spec.BlockText, Text: &spec.Text{Content: "Body"}},
			},
		}},
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("a well-formed spec failed validation: %v", err)
	}

	tests := []struct {
		name string
		spec *spec.Spec
		code verr.Code
	}{
		{"nil spec", nil, verr.VELLUM_SPEC_INVALID},
		{"no sections", &spec.Spec{}, verr.VELLUM_SPEC_INVALID},
		{"section with no blocks", &spec.Spec{Sections: []spec.Section{{ID: "empty"}}}, verr.VELLUM_SPEC_INVALID},
		{
			name: "unknown kind",
			spec: &spec.Spec{Sections: []spec.Section{{Blocks: []spec.Block{{Kind: "sidebar"}}}}},
			code: verr.VELLUM_SPEC_BLOCK_KIND_UNKNOWN,
		},
		{
			name: "heading arm missing",
			spec: &spec.Spec{Sections: []spec.Section{{Blocks: []spec.Block{{Kind: spec.BlockHeading}}}}},
			code: verr.VELLUM_SPEC_INVALID,
		},
		{
			name: "text arm missing",
			spec: &spec.Spec{Sections: []spec.Section{{Blocks: []spec.Block{{Kind: spec.BlockText}}}}},
			code: verr.VELLUM_SPEC_INVALID,
		},
		{
			name: "heading level below one",
			spec: &spec.Spec{Sections: []spec.Section{{Blocks: []spec.Block{
				{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 0, Content: "x"}},
			}}}},
			code: verr.VELLUM_SPEC_INVALID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if !verr.HasCode(err, tt.code) {
				t.Errorf("error = %v, want %s", err, tt.code)
			}
		})
	}
}

// TestValidate_NamesTheLocation checks that a failure is actionable: a
// consumer should not have to search their document to find the offending
// block.
func TestValidate_NamesTheLocation(t *testing.T) {
	s := &spec.Spec{
		Sections: []spec.Section{
			{ID: "first", Blocks: []spec.Block{{Kind: spec.BlockText, Text: &spec.Text{Content: "fine"}}}},
			{ID: "second", Blocks: []spec.Block{
				{Kind: spec.BlockText, Text: &spec.Text{Content: "fine"}},
				{Kind: "sidebar"},
			}},
		},
	}

	err := s.Validate()
	code, ok := verr.CodeOf(err)
	if !ok || code != verr.VELLUM_SPEC_BLOCK_KIND_UNKNOWN {
		t.Fatalf("error = %v, want VELLUM_SPEC_BLOCK_KIND_UNKNOWN", err)
	}

	ce, _ := err.(*verr.CodedError)
	if ce == nil {
		t.Fatal("error is not a CodedError")
	}
	for key, want := range map[string]any{"section_index": 1, "block_index": 1, "section_id": "second", "kind": "sidebar"} {
		got, present := ce.Detail(key)
		if !present {
			t.Errorf("detail %q is missing", key)
			continue
		}
		if got != want {
			t.Errorf("detail %q = %v, want %v", key, got, want)
		}
	}
}

// TestNoSemanticSectionVocabulary is a guard against the single most likely
// wrong turn in this model: adding "cover", "executive summary" or similar as
// block kinds. That vocabulary belongs to whoever is composing the document.
func TestNoSemanticSectionVocabulary(t *testing.T) {
	forbidden := []string{"cover", "summary", "recommendation", "appendix", "methodology", "conclusion", "abstract"}
	for _, k := range spec.AllBlockKinds() {
		for _, f := range forbidden {
			if string(k) == f {
				t.Errorf("block kind %q is a consumer's semantic vocabulary, not a generic block", k)
			}
		}
	}
}
