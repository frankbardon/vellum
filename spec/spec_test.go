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

func TestValidate_AllBlockKindsAccepted(t *testing.T) {
	blocks := []spec.Block{
		{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 1, Content: "Title"}},
		{Kind: spec.BlockText, Text: &spec.Text{Content: "Prose"}},
		{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: "chart-1", Role: "asset.full", AltText: "A chart"}},
		{Kind: spec.BlockTable, Table: &spec.Table{
			ColumnHeaders: spec.HeaderTree{{Label: "A"}, {Label: "B"}},
			Body:          [][]spec.Cell{{{Text: "1"}, {Text: "2"}}},
		}},
		{Kind: spec.BlockPageBreak, PageBreak: &spec.PageBreak{}},
		{Kind: spec.BlockNotes, Notes: &spec.Notes{Content: "Speaker note"}},
		{Kind: spec.BlockSpacer, Spacer: &spec.Spacer{Height: spec.Points(12)}},
	}
	if len(blocks) != len(spec.AllBlockKinds()) {
		t.Fatalf("this test covers %d kinds but the vocabulary has %d; add the missing one", len(blocks), len(spec.AllBlockKinds()))
	}

	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Sections:      []spec.Section{{ID: "all", Blocks: blocks}},
	}
	if err := s.Validate(); err != nil {
		t.Errorf("a spec exercising every block kind was rejected: %v", err)
	}
}

func TestValidate_MissingArms(t *testing.T) {
	bare := map[spec.BlockKind]spec.Block{
		spec.BlockHeading:   {Kind: spec.BlockHeading},
		spec.BlockText:      {Kind: spec.BlockText},
		spec.BlockAsset:     {Kind: spec.BlockAsset},
		spec.BlockTable:     {Kind: spec.BlockTable},
		spec.BlockPageBreak: {Kind: spec.BlockPageBreak},
		spec.BlockNotes:     {Kind: spec.BlockNotes},
		spec.BlockSpacer:    {Kind: spec.BlockSpacer},
	}
	for _, kind := range spec.AllBlockKinds() {
		t.Run(string(kind), func(t *testing.T) {
			s := &spec.Spec{Sections: []spec.Section{{Blocks: []spec.Block{bare[kind]}}}}
			err := s.Validate()
			if !verr.HasCode(err, verr.VELLUM_SPEC_INVALID) {
				t.Fatalf("error = %v, want VELLUM_SPEC_INVALID", err)
			}
			ce, _ := err.(*verr.CodedError)
			if ce == nil {
				t.Fatal("error is not a CodedError")
			}
			if got, ok := ce.Detail("missing_arm"); !ok || got != string(kind) {
				t.Errorf("detail missing_arm = %v, want %q", got, kind)
			}
		})
	}
}

// TestValidate_RejectsStrayArms covers a construction mistake that would
// otherwise be invisible: a block carrying content for a kind it is not.
// Honouring whichever arm the discriminator names would hide the mistake and
// silently drop the other content.
func TestValidate_RejectsStrayArms(t *testing.T) {
	s := &spec.Spec{Sections: []spec.Section{{Blocks: []spec.Block{{
		Kind:    spec.BlockText,
		Text:    &spec.Text{Content: "the declared arm"},
		Heading: &spec.Heading{Level: 1, Content: "a stray arm"},
	}}}}}

	err := s.Validate()
	if !verr.HasCode(err, verr.VELLUM_SPEC_INVALID) {
		t.Fatalf("error = %v, want VELLUM_SPEC_INVALID", err)
	}
	ce, _ := err.(*verr.CodedError)
	if ce == nil {
		t.Fatal("error is not a CodedError")
	}
	stray, ok := ce.Detail("stray_arms")
	if !ok {
		t.Fatal("the error does not name the stray arm")
	}
	names, ok := stray.([]string)
	if !ok || len(names) != 1 || names[0] != "heading" {
		t.Errorf("stray_arms = %v, want [heading]", stray)
	}
}

// TestValidate_TableFaultCarriesBlockLocation checks that a nested table fault
// reports where in the document it is, not only what it is.
func TestValidate_TableFaultCarriesBlockLocation(t *testing.T) {
	s := &spec.Spec{Sections: []spec.Section{
		{ID: "first", Blocks: []spec.Block{{Kind: spec.BlockText, Text: &spec.Text{Content: "fine"}}}},
		{ID: "second", Blocks: []spec.Block{
			{Kind: spec.BlockText, Text: &spec.Text{Content: "fine"}},
			{Kind: spec.BlockTable, Table: &spec.Table{
				ColumnHeaders: spec.HeaderTree{{Label: "A"}, {Label: "B"}, {Label: "C"}},
				Body:          [][]spec.Cell{{{Text: "1"}}},
			}},
		}},
	}}

	err := s.Validate()
	if !verr.HasCode(err, verr.VELLUM_TABLE_ROW_ARITY) {
		t.Fatalf("error = %v, want VELLUM_TABLE_ROW_ARITY", err)
	}
	ce, _ := err.(*verr.CodedError)
	if ce == nil {
		t.Fatal("error is not a CodedError")
	}
	for key, want := range map[string]any{"section_index": 1, "block_index": 1, "section_id": "second"} {
		if got, ok := ce.Detail(key); !ok || got != want {
			t.Errorf("detail %q = %v, want %v", key, got, want)
		}
	}
	// The table's own coordinates must survive the re-raise, or the caller
	// learns which block is wrong but not which row.
	if got, ok := ce.Detail("table_width"); !ok || got != 3 {
		t.Errorf("detail table_width = %v, want 3", got)
	}
}

// TestMarksAreOpaque is a structural guard: nothing in this package may branch
// on a mark's value. If it ever does, the seam has leaked and the consumer's
// vocabulary has become Vellum's business.
func TestMarksAreOpaque(t *testing.T) {
	build := func(mark string) *spec.Spec {
		return &spec.Spec{
			FormatVersion: spec.FormatVersion,
			Sections: []spec.Section{{
				Marks: []string{mark},
				Blocks: []spec.Block{{
					Kind:  spec.BlockText,
					Text:  &spec.Text{Content: "x"},
					Marks: []string{mark},
				}},
			}},
		}
	}
	for _, mark := range []string{"stale", "low-base", "", "  ", "significance", "中文", "anything at all"} {
		if err := build(mark).Validate(); err != nil {
			t.Errorf("validation rejected the mark %q; marks are opaque and Vellum must never interpret one: %v", mark, err)
		}
	}
}
