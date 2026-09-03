package bind_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/frankbardon/vellum/template/bind"
)

func hashFixture() *bind.Binding {
	return &bind.Binding{
		FormatVersion: bind.FormatVersion,
		Statements: []bind.Statement{
			{Kind: bind.StatementBind, Bind: &bind.Bind{
				Anchor: "customer_name", Expr: "data.customer.name",
			}},
			{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
				Over: "data.line_items", As: "item", Target: bind.RepeatTargetRow,
				Body: []bind.Statement{
					{Kind: bind.StatementIf, If: &bind.If{
						When: "item.discounted",
						Then: []bind.Statement{
							{Kind: bind.StatementWith, With: &bind.With{
								As: "price", Value: "item.discounted_price",
								Body: []bind.Statement{
									{Kind: bind.StatementBind, Bind: &bind.Bind{
										Anchor: "line_total", Expr: "price", Format: "#,##0.00",
									}},
								},
							}},
						},
						Else: []bind.Statement{
							{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "line_total", Expr: "item.price"}},
						},
					}},
				},
			}},
			{Kind: bind.StatementBind, Skip: "not data.show_notes", Bind: &bind.Bind{
				Anchor: "notes", Expr: "data.notes", Optional: true,
			}},
		},
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
// about the binding rather than about how it was typed.
func TestHash_DefaultsNormalise(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*bind.Binding)
	}{
		{
			name:   "omitted format version equals the current one",
			mutate: func(b *bind.Binding) { b.FormatVersion = "" },
		},
	}

	base := hashFixture()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutated := hashFixture()
			tt.mutate(mutated)
			if mutated.Hash() != base.Hash() {
				t.Errorf("normalisation failed: the spelling changed the hash")
			}
		})
	}
}

// TestHash_NilElseEqualsEmptyElse checks the nil/empty collapse directly,
// the same way spec's own empty-value-arm test does: change only the
// spelling, holding the shared baseline separately since both fixtures must
// carry the *same* else content to isolate the representation from the
// content.
func TestHash_NilElseEqualsEmptyElse(t *testing.T) {
	a := hashFixture()
	a.Statements[1].Repeat.Body[0].If.Else = nil
	b := hashFixture()
	b.Statements[1].Repeat.Body[0].If.Else = []bind.Statement{}
	if a.Hash() != b.Hash() {
		t.Error("a nil else hashed differently from an empty else")
	}
}

// TestHash_OptionalAnchorsIsASetNotASequence proves OptionalAnchors' own doc
// comment: order and duplicate entries do not participate in the hash,
// because two bindings differing only in how they spelled the same set of
// deliberately-unbound anchor names mean the same thing.
func TestHash_OptionalAnchorsIsASetNotASequence(t *testing.T) {
	a := hashFixture()
	a.OptionalAnchors = []string{"footer_note", "legal_disclaimer"}
	b := hashFixture()
	b.OptionalAnchors = []string{"legal_disclaimer", "footer_note"}
	if a.Hash() != b.Hash() {
		t.Error("OptionalAnchors order moved the hash")
	}

	c := hashFixture()
	c.OptionalAnchors = []string{"footer_note", "footer_note", "legal_disclaimer"}
	if a.Hash() != c.Hash() {
		t.Error("a duplicate OptionalAnchors entry moved the hash")
	}

	d := hashFixture()
	d.OptionalAnchors = nil
	e := hashFixture()
	e.OptionalAnchors = []string{}
	if d.Hash() != e.Hash() {
		t.Error("a nil OptionalAnchors hashed differently from an empty one")
	}
}

func TestHash_ContentChangesMoveIt(t *testing.T) {
	base := hashFixture().Hash()

	tests := []struct {
		name   string
		mutate func(*bind.Binding)
	}{
		{"anchor", func(b *bind.Binding) { b.Statements[0].Bind.Anchor = "other" }},
		{"expr", func(b *bind.Binding) { b.Statements[0].Bind.Expr = "data.other" }},
		{"format", func(b *bind.Binding) { b.Statements[0].Bind.Format = "0.0%" }},
		{"optional", func(b *bind.Binding) { b.Statements[0].Bind.Optional = true }},
		{"skip", func(b *bind.Binding) { b.Statements[0].Skip = "data.hide" }},
		{"repeat over", func(b *bind.Binding) { b.Statements[1].Repeat.Over = "data.other_items" }},
		{"repeat as", func(b *bind.Binding) { b.Statements[1].Repeat.As = "row" }},
		{"repeat target", func(b *bind.Binding) { b.Statements[1].Repeat.Target = bind.RepeatTargetBlock }},
		{"if when", func(b *bind.Binding) { b.Statements[1].Repeat.Body[0].If.When = "item.other" }},
		{"with as", func(b *bind.Binding) {
			b.Statements[1].Repeat.Body[0].If.Then[0].With.As = "p"
		}},
		{"with value", func(b *bind.Binding) {
			b.Statements[1].Repeat.Body[0].If.Then[0].With.Value = "item.other_price"
		}},
		{"nested bind anchor", func(b *bind.Binding) {
			b.Statements[1].Repeat.Body[0].If.Then[0].With.Body[0].Bind.Anchor = "other"
		}},
		{"statement order", func(b *bind.Binding) {
			s := b.Statements
			s[0], s[2] = s[2], s[0]
		}},
		{"optional anchors content", func(b *bind.Binding) {
			b.OptionalAnchors = []string{"footer_note"}
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

// TestHash_ForwardCompatible mirrors spec's own guarantee: a future
// omitempty field must not move the hash of a binding that omits it.
func TestHash_ForwardCompatible(t *testing.T) {
	raw, err := json.Marshal(hashFixture())
	if err != nil {
		t.Fatal(err)
	}
	var round bind.Binding
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatal(err)
	}
	if round.Hash() != hashFixture().Hash() {
		t.Error("a JSON round trip moved the hash; a field is not surviving encode-decode")
	}
}

// TestHash_StableAcrossProcesses spawns a real subprocess, so it catches a
// hash depending on something fixed for a process's lifetime rather than on
// content.
func TestHash_StableAcrossProcesses(t *testing.T) {
	if os.Getenv("VELLUM_BIND_HASH_CHILD") != "" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locating the test binary: %v", err)
	}

	want := hashFixture().Hash()
	for range 5 {
		cmd := exec.Command(exe, "-test.run=TestHashChildPrints", "-test.v=false")
		cmd.Env = append(os.Environ(), "VELLUM_BIND_HASH_CHILD=1")
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
	if os.Getenv("VELLUM_BIND_HASH_CHILD") == "" {
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
	var b *bind.Binding
	got := b.Hash()
	if got == "" {
		t.Error("hashing a nil binding produced the empty string")
	}
	if got == hashFixture().Hash() {
		t.Error("a nil binding hashed the same as a real one")
	}
}

// TestHash_DoesNotMutate proves that asking for a hash does not quietly
// rewrite the caller's binding.
func TestHash_DoesNotMutate(t *testing.T) {
	b := hashFixture()
	b.FormatVersion = ""
	before, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}

	_ = b.Hash()

	after, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("Hash mutated the binding:\nbefore %s\nafter  %s", before, after)
	}
}
