package bind_test

import (
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/template/bind"
)

func TestAllStatementKinds_RegistryIsComplete(t *testing.T) {
	kinds := bind.AllStatementKinds()
	want := []bind.StatementKind{bind.StatementBind, bind.StatementRepeat, bind.StatementIf, bind.StatementWith}
	if len(kinds) != len(want) {
		t.Fatalf("AllStatementKinds returned %d kinds, want %d: %v", len(kinds), len(want), kinds)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("kind %d = %q, want %q", i, kinds[i], want[i])
		}
	}
}

func TestAllStatementKinds_ReturnsCopy(t *testing.T) {
	first := bind.AllStatementKinds()
	original := first[0]
	first[0] = "mutated"

	if bind.AllStatementKinds()[0] != original {
		t.Error("AllStatementKinds returned the backing slice")
	}
}

func TestValidStatementKind(t *testing.T) {
	for _, k := range bind.AllStatementKinds() {
		if !bind.ValidStatementKind(k) {
			t.Errorf("registered kind %q reported invalid", k)
		}
	}
	for _, k := range []bind.StatementKind{"", "skip", "Bind", "loop"} {
		if bind.ValidStatementKind(k) {
			t.Errorf("kind %q reported valid", k)
		}
	}
}

func TestAllRepeatTargets_RegistryIsComplete(t *testing.T) {
	targets := bind.AllRepeatTargets()
	want := []bind.RepeatTarget{bind.RepeatTargetRow, bind.RepeatTargetBlock, bind.RepeatTargetTableRow}
	if len(targets) != len(want) {
		t.Fatalf("AllRepeatTargets returned %d, want %d: %v", len(targets), len(want), targets)
	}
	for i := range want {
		if targets[i] != want[i] {
			t.Errorf("target %d = %q, want %q", i, targets[i], want[i])
		}
	}
}

func TestValidRepeatTarget(t *testing.T) {
	for _, tgt := range bind.AllRepeatTargets() {
		if !bind.ValidRepeatTarget(tgt) {
			t.Errorf("registered target %q reported invalid", tgt)
		}
	}
	// The zero value is deliberately not valid: a repeat target is never
	// defaulted, see RepeatTarget's doc comment.
	for _, tgt := range []bind.RepeatTarget{"", "cell", "Row"} {
		if bind.ValidRepeatTarget(tgt) {
			t.Errorf("target %q reported valid", tgt)
		}
	}
}

func okBind(anchor string) bind.Statement {
	return bind.Statement{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: anchor, Expr: "data." + anchor}}
}

func TestValidate(t *testing.T) {
	ok := &bind.Binding{
		FormatVersion: bind.FormatVersion,
		Statements:    []bind.Statement{okBind("name")},
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("a well-formed binding failed validation: %v", err)
	}

	tests := []struct {
		name string
		b    *bind.Binding
		code verr.Code
	}{
		{"nil binding", nil, verr.VELLUM_BIND_INVALID},
		{"no statements", &bind.Binding{}, verr.VELLUM_BIND_INVALID},
		{
			name: "unknown statement kind",
			b:    &bind.Binding{Statements: []bind.Statement{{Kind: "loop"}}},
			code: verr.VELLUM_BIND_STATEMENT_KIND_UNKNOWN,
		},
		{
			name: "bind with no anchor",
			b:    &bind.Binding{Statements: []bind.Statement{{Kind: bind.StatementBind, Bind: &bind.Bind{Expr: "x"}}}},
			code: verr.VELLUM_BIND_INVALID,
		},
		{
			name: "bind with no expr",
			b:    &bind.Binding{Statements: []bind.Statement{{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "a"}}}},
			code: verr.VELLUM_BIND_INVALID,
		},
		{
			name: "repeat with no over",
			b: &bind.Binding{Statements: []bind.Statement{{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
				As: "item", Target: bind.RepeatTargetRow, Body: []bind.Statement{okBind("x")},
			}}}},
			code: verr.VELLUM_BIND_INVALID,
		},
		{
			name: "repeat with no as",
			b: &bind.Binding{Statements: []bind.Statement{{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
				Over: "data.rows", Target: bind.RepeatTargetRow, Body: []bind.Statement{okBind("x")},
			}}}},
			code: verr.VELLUM_BIND_INVALID,
		},
		{
			name: "repeat with no target",
			b: &bind.Binding{Statements: []bind.Statement{{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
				Over: "data.rows", As: "item", Body: []bind.Statement{okBind("x")},
			}}}},
			code: verr.VELLUM_BIND_REPEAT_TARGET_UNKNOWN,
		},
		{
			name: "repeat with unknown target",
			b: &bind.Binding{Statements: []bind.Statement{{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
				Over: "data.rows", As: "item", Target: "cell", Body: []bind.Statement{okBind("x")},
			}}}},
			code: verr.VELLUM_BIND_REPEAT_TARGET_UNKNOWN,
		},
		{
			name: "if with no when",
			b: &bind.Binding{Statements: []bind.Statement{{Kind: bind.StatementIf, If: &bind.If{
				Then: []bind.Statement{okBind("x")},
			}}}},
			code: verr.VELLUM_BIND_INVALID,
		},
		{
			name: "with, no as",
			b: &bind.Binding{Statements: []bind.Statement{{Kind: bind.StatementWith, With: &bind.With{
				Value: "data.customer", Body: []bind.Statement{okBind("x")},
			}}}},
			code: verr.VELLUM_BIND_INVALID,
		},
		{
			name: "with, no value",
			b: &bind.Binding{Statements: []bind.Statement{{Kind: bind.StatementWith, With: &bind.With{
				As: "c", Body: []bind.Statement{okBind("x")},
			}}}},
			code: verr.VELLUM_BIND_INVALID,
		},
		{
			name: "nested statement invalid inside repeat body",
			b: &bind.Binding{Statements: []bind.Statement{{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
				Over: "data.rows", As: "row", Target: bind.RepeatTargetRow,
				Body: []bind.Statement{{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "cell"}}},
			}}}},
			code: verr.VELLUM_BIND_INVALID,
		},
		{
			name: "nested statement invalid inside if.else",
			b: &bind.Binding{Statements: []bind.Statement{{Kind: bind.StatementIf, If: &bind.If{
				When: "data.flag",
				Then: []bind.Statement{okBind("x")},
				Else: []bind.Statement{{Kind: "loop"}},
			}}}},
			code: verr.VELLUM_BIND_STATEMENT_KIND_UNKNOWN,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.b.Validate()
			if !verr.HasCode(err, tt.code) {
				t.Errorf("error = %v, want %s", err, tt.code)
			}
		})
	}
}

// TestValidate_MissingArms mirrors spec's own test of the same name: a
// statement declaring a kind but carrying no content for it must be
// rejected, and the error must name which arm is missing.
func TestValidate_MissingArms(t *testing.T) {
	bare := map[bind.StatementKind]bind.Statement{
		bind.StatementBind:   {Kind: bind.StatementBind},
		bind.StatementRepeat: {Kind: bind.StatementRepeat},
		bind.StatementIf:     {Kind: bind.StatementIf},
		bind.StatementWith:   {Kind: bind.StatementWith},
	}
	for _, kind := range bind.AllStatementKinds() {
		t.Run(string(kind), func(t *testing.T) {
			b := &bind.Binding{Statements: []bind.Statement{bare[kind]}}
			err := b.Validate()
			if !verr.HasCode(err, verr.VELLUM_BIND_INVALID) {
				t.Fatalf("error = %v, want VELLUM_BIND_INVALID", err)
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
// otherwise be invisible: a statement carrying content for a kind it is not.
func TestValidate_RejectsStrayArms(t *testing.T) {
	b := &bind.Binding{Statements: []bind.Statement{{
		Kind: bind.StatementBind,
		Bind: &bind.Bind{Anchor: "a", Expr: "the declared arm"},
		With: &bind.With{As: "x", Value: "a stray arm", Body: []bind.Statement{okBind("y")}},
	}}}

	err := b.Validate()
	if !verr.HasCode(err, verr.VELLUM_BIND_INVALID) {
		t.Fatalf("error = %v, want VELLUM_BIND_INVALID", err)
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
	if !ok || len(names) != 1 || names[0] != "with" {
		t.Errorf("stray_arms = %v, want [with]", stray)
	}
}

// TestValidate_NamesTheLocation checks that a nested failure reports where
// in the tree it is, not only what it is.
func TestValidate_NamesTheLocation(t *testing.T) {
	b := &bind.Binding{Statements: []bind.Statement{
		okBind("fine"),
		{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
			Over: "data.rows", As: "row", Target: bind.RepeatTargetRow,
			Body: []bind.Statement{
				okBind("fine-too"),
				{Kind: "loop"},
			},
		}},
	}}

	err := b.Validate()
	code, ok := verr.CodeOf(err)
	if !ok || code != verr.VELLUM_BIND_STATEMENT_KIND_UNKNOWN {
		t.Fatalf("error = %v, want VELLUM_BIND_STATEMENT_KIND_UNKNOWN", err)
	}
	ce, _ := err.(*verr.CodedError)
	if ce == nil {
		t.Fatal("error is not a CodedError")
	}
	got, present := ce.Detail("path")
	if !present {
		t.Fatal("detail \"path\" is missing")
	}
	if got != "1/repeat/body/1" {
		t.Errorf("path = %v, want %q", got, "1/repeat/body/1")
	}
}

// TestValidate_DeeplyNestedControlFlow exercises a repeat containing an if
// containing a with containing a bind, which is the shape E10-S3's own
// brief calls out explicitly.
func TestValidate_DeeplyNestedControlFlow(t *testing.T) {
	b := &bind.Binding{Statements: []bind.Statement{
		{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
			Over: "data.line_items", As: "item", Target: bind.RepeatTargetRow,
			Body: []bind.Statement{
				{Kind: bind.StatementIf, If: &bind.If{
					When: "item.discounted",
					Then: []bind.Statement{
						{Kind: bind.StatementWith, With: &bind.With{
							As: "price", Value: "item.discounted_price",
							Body: []bind.Statement{okBind("line_total")},
						}},
					},
					Else: []bind.Statement{okBind("line_total")},
				}},
			},
		}},
	}}
	if err := b.Validate(); err != nil {
		t.Fatalf("deeply nested control flow rejected: %v", err)
	}
}

// TestValidate_OptionalAnchorsRejectsEmptyEntry checks that an empty entry
// in OptionalAnchors is caught the same way an empty Bind.Anchor is, rather
// than silently doing nothing.
func TestValidate_OptionalAnchorsRejectsEmptyEntry(t *testing.T) {
	b := &bind.Binding{
		Statements:      []bind.Statement{okBind("name")},
		OptionalAnchors: []string{"unbound_note", ""},
	}
	err := b.Validate()
	if !verr.HasCode(err, verr.VELLUM_BIND_INVALID) {
		t.Fatalf("error = %v, want VELLUM_BIND_INVALID", err)
	}
}

// TestValidate_OptionalAnchorsAcceptsAWellFormedList checks that a
// well-formed OptionalAnchors list does not itself cause a validation
// failure, independent of whatever [Reconcile] later does with it.
func TestValidate_OptionalAnchorsAcceptsAWellFormedList(t *testing.T) {
	b := &bind.Binding{
		Statements:      []bind.Statement{okBind("name")},
		OptionalAnchors: []string{"unbound_note", "unbound_footer"},
	}
	if err := b.Validate(); err != nil {
		t.Fatalf("a well-formed OptionalAnchors list failed validation: %v", err)
	}
}

// TestSkip_IsAnAnyStatementModifier checks that Skip is decodable and
// carried on every statement kind, not only bind.
func TestSkip_IsAnAnyStatementModifier(t *testing.T) {
	stmts := []bind.Statement{
		{Kind: bind.StatementBind, Skip: "not data.include", Bind: &bind.Bind{Anchor: "a", Expr: "data.a"}},
		{Kind: bind.StatementRepeat, Skip: "count(data.rows) = 0", Repeat: &bind.Repeat{
			Over: "data.rows", As: "row", Target: bind.RepeatTargetRow, Body: []bind.Statement{okBind("x")},
		}},
		{Kind: bind.StatementIf, Skip: "data.hidden", If: &bind.If{When: "true", Then: []bind.Statement{okBind("x")}}},
		{Kind: bind.StatementWith, Skip: "data.hidden", With: &bind.With{As: "c", Value: "data.c", Body: []bind.Statement{okBind("x")}}},
	}
	b := &bind.Binding{Statements: stmts}
	if err := b.Validate(); err != nil {
		t.Fatalf("a binding with skip on every statement kind was rejected: %v", err)
	}
	for i := range stmts {
		if stmts[i].Skip == "" {
			t.Errorf("statement %d lost its skip modifier", i)
		}
	}
}
