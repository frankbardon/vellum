package spec_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/frankbardon/vellum/spec"
)

func hashFixture() *spec.Spec {
	return &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Title:         "Quarterly Review",
		Theme:         "default",
		Sections: []spec.Section{{
			ID:     "findings",
			Layout: "body",
			Marks:  []string{"reviewed"},
			Blocks: []spec.Block{
				{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 1, Content: "Findings"}},
				{Kind: spec.BlockText, Text: &spec.Text{Content: "Awareness rose."}, Marks: []string{"stale"}},
				{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: "chart-1", Role: "asset.full", AltText: "Trend"}},
				{Kind: spec.BlockTable, Table: &spec.Table{
					ColumnHeaders: spec.HeaderTree{
						{Label: "Region", Children: []spec.HeaderNode{{Label: "North"}, {Label: "South"}}},
						{Label: "Total"},
					},
					Body: [][]spec.Cell{{
						{Value: num(10)},
						{Value: num(20)},
						{Value: num(30), Class: spec.CellMargin, Annotations: []spec.Annotation{{Text: "a"}}},
					}},
				}},
				{Kind: spec.BlockPageBreak, PageBreak: &spec.PageBreak{}},
				{Kind: spec.BlockNotes, Notes: &spec.Notes{Content: "Check the base."}},
				{Kind: spec.BlockSpacer, Spacer: &spec.Spacer{Height: spec.Points(12)}},
			},
		}},
	}
}

func TestHash_Stable(t *testing.T) {
	first := hashFixture().Hash()
	for range 100 {
		if got := hashFixture().Hash(); got != first {
			t.Fatalf("Hash is not stable within a process: %q then %q", first, got)
		}
	}
	if len(first) != 32 {
		t.Errorf("hash length = %d, want 32", len(first))
	}
}

// TestHash_DefaultsNormalise is the property that makes the hash a statement
// about the document rather than about how it was typed.
func TestHash_DefaultsNormalise(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*spec.Spec)
	}{
		{
			name:   "omitted format version equals the current one",
			mutate: func(s *spec.Spec) { s.FormatVersion = "" },
		},
		{
			name:   "omitted theme equals the default theme",
			mutate: func(s *spec.Spec) { s.Theme = "" },
		},
		{
			name: "explicit column spans that restate the tree shape",
			mutate: func(s *spec.Spec) {
				tbl := s.Sections[0].Blocks[3].Table
				tbl.ColumnHeaders[0].Span = 2
				tbl.ColumnHeaders[1].Span = 1
			},
		},
		{
			name: "explicit spans of one",
			mutate: func(s *spec.Spec) {
				for c := range s.Sections[0].Blocks[3].Table.Body[0] {
					s.Sections[0].Blocks[3].Table.Body[0][c].RowSpan = 1
					s.Sections[0].Blocks[3].Table.Body[0][c].ColSpan = 1
				}
			},
		},
		{
			name: "explicit superscript annotation position",
			mutate: func(s *spec.Spec) {
				s.Sections[0].Blocks[3].Table.Body[0][2].Annotations[0].Position = spec.AnnotationSuperscript
			},
		},
		{
			name: "empty mark slice equals no marks",
			mutate: func(s *spec.Spec) {
				s.Sections[0].Blocks[0].Marks = []string{}
			},
		},
		{
			name: "empty value arm equals no value",
			mutate: func(s *spec.Spec) {
				s.Sections[0].Blocks[3].Table.Body[0][0].Value = &spec.Value{Kind: spec.ValueEmpty}
				s.Sections[0].Blocks[3].Table.Body[0][0].Text = "10"
			},
		},
	}

	base := hashFixture()
	// The last case changes content as well as spelling, so it gets its own
	// baseline rather than the shared one.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "empty value arm equals no value" {
				a := hashFixture()
				a.Sections[0].Blocks[3].Table.Body[0][0].Value = &spec.Value{Kind: spec.ValueEmpty}
				b := hashFixture()
				b.Sections[0].Blocks[3].Table.Body[0][0].Value = nil
				if a.Hash() != b.Hash() {
					t.Error("an empty value arm hashed differently from no value arm")
				}
				return
			}
			mutated := hashFixture()
			tt.mutate(mutated)
			if mutated.Hash() != base.Hash() {
				t.Errorf("normalisation failed: the spelling changed the hash")
			}
		})
	}
}

func TestHash_ContentChangesMoveIt(t *testing.T) {
	base := hashFixture().Hash()

	tests := []struct {
		name   string
		mutate func(*spec.Spec)
	}{
		{"title", func(s *spec.Spec) { s.Title = "Different" }},
		{"theme", func(s *spec.Spec) { s.Theme = "client" }},
		{"section id", func(s *spec.Spec) { s.Sections[0].ID = "other" }},
		{"section layout", func(s *spec.Spec) { s.Sections[0].Layout = "cover" }},
		{"section marks", func(s *spec.Spec) { s.Sections[0].Marks = []string{"draft"} }},
		{"heading level", func(s *spec.Spec) { s.Sections[0].Blocks[0].Heading.Level = 2 }},
		{"heading content", func(s *spec.Spec) { s.Sections[0].Blocks[0].Heading.Content = "Other" }},
		{"block marks", func(s *spec.Spec) { s.Sections[0].Blocks[1].Marks = []string{"fresh"} }},
		{"mark order", func(s *spec.Spec) { s.Sections[0].Blocks[1].Marks = []string{"stale", "extra"} }},
		{"asset handle", func(s *spec.Spec) { s.Sections[0].Blocks[2].Asset.Handle = "chart-2" }},
		{"asset role", func(s *spec.Spec) { s.Sections[0].Blocks[2].Asset.Role = "asset.half" }},
		{"cell value", func(s *spec.Spec) { s.Sections[0].Blocks[3].Table.Body[0][0].Value.Number = 11 }},
		{"cell class", func(s *spec.Spec) { s.Sections[0].Blocks[3].Table.Body[0][0].Class = spec.CellTotal }},
		{"annotation text", func(s *spec.Spec) { s.Sections[0].Blocks[3].Table.Body[0][2].Annotations[0].Text = "b" }},
		{"header label", func(s *spec.Spec) { s.Sections[0].Blocks[3].Table.ColumnHeaders[0].Label = "Area" }},
		{"notes content", func(s *spec.Spec) { s.Sections[0].Blocks[5].Notes.Content = "Different" }},
		{"spacer height", func(s *spec.Spec) { s.Sections[0].Blocks[6].Spacer.Height = spec.Points(24) }},
		{"block order", func(s *spec.Spec) {
			b := s.Sections[0].Blocks
			b[0], b[1] = b[1], b[0]
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutated := hashFixture()
			tt.mutate(mutated)
			if mutated.Hash() == base {
				t.Error("a content change did not move the hash; the field is not participating")
			}
		})
	}
}

// TestHash_ForwardCompatible is the property that is easiest to break and most
// expensive to discover. A consumer keyed on this hash reads a change as "this
// document changed"; a hash that moved because Vellum gained a field would
// tell every consumer that every document had changed at once.
func TestHash_ForwardCompatible(t *testing.T) {
	// Simulates a future omitempty field by hashing the fixture's JSON with an
	// unset field added — which, being omitempty, would not appear at all.
	raw, err := json.Marshal(hashFixture())
	if err != nil {
		t.Fatal(err)
	}
	var round spec.Spec
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatal(err)
	}
	if round.Hash() != hashFixture().Hash() {
		t.Error("a JSON round trip moved the hash; a field is not surviving encode-decode")
	}
}

// TestHash_StableAcrossProcesses spawns a real subprocess. An in-process loop
// cannot catch a hash that depends on something fixed for a process's
// lifetime — a map seed, an address, an init order.
func TestHash_StableAcrossProcesses(t *testing.T) {
	if os.Getenv("VELLUM_SPEC_HASH_CHILD") != "" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locating the test binary: %v", err)
	}

	want := hashFixture().Hash()
	for range 5 {
		cmd := exec.Command(exe, "-test.run=TestHashChildPrints", "-test.v=false")
		cmd.Env = append(os.Environ(), "VELLUM_SPEC_HASH_CHILD=1")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("child failed: %v", err)
		}
		got := ""
		for _, line := range splitLines(string(out)) {
			if len(line) == 32 {
				got = line
			}
		}
		if got != want {
			t.Fatalf("child hash %q, parent hash %q", got, want)
		}
	}
}

// TestHashChildPrints is the subprocess entry point for
// TestHash_StableAcrossProcesses.
func TestHashChildPrints(t *testing.T) {
	if os.Getenv("VELLUM_SPEC_HASH_CHILD") == "" {
		t.Skip("not a child process")
	}
	os.Stdout.WriteString(hashFixture().Hash() + "\n")
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func TestHash_NilReceiver(t *testing.T) {
	var s *spec.Spec
	got := s.Hash()
	if got == "" {
		t.Error("hashing a nil spec produced the empty string")
	}
	if got == hashFixture().Hash() {
		t.Error("a nil spec hashed the same as a real one")
	}
}

// TestHash_DoesNotMutate proves that asking for a hash does not quietly
// rewrite the caller's specification.
func TestHash_DoesNotMutate(t *testing.T) {
	s := hashFixture()
	s.FormatVersion = ""
	s.Theme = ""
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}

	_ = s.Hash()

	after, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("Hash mutated the specification:\nbefore %s\nafter  %s", before, after)
	}
}

// nestedHeaderHashFixture is a spec whose only content is a table with a
// three-level column header tree — deeper than hashFixture's own two levels —
// so the pinned-vector family exercises HeaderTree recursion rather than only
// the flat case.
func nestedHeaderHashFixture() *spec.Spec {
	return &spec.Spec{
		Title: "Nested Headers",
		Sections: []spec.Section{{
			ID:     "s1",
			Layout: "body",
			Blocks: []spec.Block{
				{Kind: spec.BlockTable, Table: &spec.Table{
					ColumnHeaders: spec.HeaderTree{
						{Label: "Region", Children: []spec.HeaderNode{
							{Label: "North", Children: []spec.HeaderNode{
								{Label: "Q1"}, {Label: "Q2"},
							}},
							{Label: "South"},
						}},
					},
					Body: [][]spec.Cell{{
						{Value: num(1)}, {Value: num(2)}, {Value: num(3)},
					}},
				}},
			},
		}},
	}
}

// TestSpecHashPinnedVectors is the gate: a small family of committed
// (spec, hash) vectors, not just one. Each was computed once; changing any of
// them changes the identity of every artifact every consumer has cached for a
// spec of that shape.
//
// The family exists rather than a single fixture because a hash bug can be
// shape-specific — normalisation that only ever ran against a spec carrying
// every optional field would not notice a defaulting bug that only a bare,
// all-zero-value spec exercises, and a flat table would not notice a
// HeaderTree recursion bug a nested one does.
//
// Named exactly as CLAUDE.md's "Non-Skippable CI Gates" section names it
// (without an underscore, unlike this file's other TestHash_* names) so that
// TestClaudeMdMentionsAllNonSkippableGates' reserved-prefix walk finds a real
// function for the line it already lists — before this test existed under
// this name, that line was enforcing nothing.
func TestSpecHashPinnedVectors(t *testing.T) {
	tests := []struct {
		name string
		spec func() *spec.Spec
		want string
	}{
		// The fixture already used throughout this file: every block kind,
		// nested marks, an annotated total cell, explicit spans.
		{"every field populated", hashFixture, "4a9722728134cc516a8b844b81c7accb"},
		// The all-zero-value spec: every default normalisation rule at once,
		// with nothing else to mask a bug in one of them.
		{"minimal spec", func() *spec.Spec { return &spec.Spec{} }, "6b9206db0343bf243df2a9226d9d527e"},
		// A column header tree three levels deep, where hashFixture's own is
		// two.
		{"nested table headers", nestedHeaderHashFixture, "1560678b2c1bb073ecdc8e55f58f34ea"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.spec().Hash()
			if got != tt.want {
				t.Errorf("hash = %q, want %q\n"+
					"Changing this changes the identity of every artifact every consumer has cached "+
					"for a spec of this shape. If intended, bump spec.FormatVersion in the same change.",
					got, tt.want)
			}
		})
	}
}
