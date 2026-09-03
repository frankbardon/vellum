package bind_test

import (
	"errors"
	"testing"

	"github.com/frankbardon/vellum/template/bind"
)

// deepTree is a repeat containing an if containing a with containing a
// bind, with a skip modifier on every level, plus an else branch — every
// shape a later walk needs to reach.
func deepTree() []bind.Statement {
	return []bind.Statement{
		{Kind: bind.StatementBind, Skip: "skip.top", Bind: &bind.Bind{Anchor: "a", Expr: "expr.a"}},
		{Kind: bind.StatementRepeat, Skip: "skip.repeat", Repeat: &bind.Repeat{
			Over: "expr.over", As: "item", Target: bind.RepeatTargetRow,
			Body: []bind.Statement{
				{Kind: bind.StatementIf, Skip: "skip.if", If: &bind.If{
					When: "expr.when",
					Then: []bind.Statement{
						{Kind: bind.StatementWith, Skip: "skip.with", With: &bind.With{
							As: "scoped", Value: "expr.value",
							Body: []bind.Statement{
								{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "nested", Expr: "expr.nested"}},
							},
						}},
					},
					Else: []bind.Statement{
						{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "else_leaf", Expr: "expr.else"}},
					},
				}},
			},
		}},
	}
}

func TestWalk_VisitsEveryStatement(t *testing.T) {
	var kinds []bind.StatementKind
	err := bind.Walk(deepTree(), func(s *bind.Statement) error {
		kinds = append(kinds, s.Kind)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	want := []bind.StatementKind{
		bind.StatementBind,   // top-level "a"
		bind.StatementRepeat, // the repeat
		bind.StatementIf,     // the if inside its body
		bind.StatementWith,   // the with inside "then"
		bind.StatementBind,   // "nested" inside the with's body
		bind.StatementBind,   // "else_leaf" inside "else"
	}
	if len(kinds) != len(want) {
		t.Fatalf("Walk visited %d statements, want %d: %v", len(kinds), len(want), kinds)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("visit %d kind = %q, want %q", i, kinds[i], want[i])
		}
	}
}

func TestWalk_PropagatesVisitorError(t *testing.T) {
	sentinel := errors.New("stop")
	calls := 0
	err := bind.Walk(deepTree(), func(s *bind.Statement) error {
		calls++
		if s.Kind == bind.StatementIf {
			return sentinel
		}
		return nil
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Walk error = %v, want sentinel", err)
	}
	// The if statement is the third one visited in document order; nothing
	// after it (with, or either bind) should have been reached.
	if calls != 3 {
		t.Errorf("Walk made %d visits before stopping, want 3", calls)
	}
}

func TestWalk_EmptyTreeVisitsNothing(t *testing.T) {
	called := false
	if err := bind.Walk(nil, func(*bind.Statement) error { called = true; return nil }); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if called {
		t.Error("Walk visited a statement in an empty tree")
	}
}

// TestWalkExprs_ReachesEveryExpressionString proves the property S2 depends
// on: every FEEL-bearing string authored anywhere in a deeply nested tree —
// bind's expr, repeat's over, if's when, with's value, and every skip
// modifier — is reachable through WalkExprs.
func TestWalkExprs_ReachesEveryExpressionString(t *testing.T) {
	var got []string
	err := bind.WalkExprs(deepTree(), func(expr string) error {
		got = append(got, expr)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkExprs: %v", err)
	}

	want := []string{
		"skip.top", "expr.a", // the top bind: skip then expr
		"skip.repeat", "expr.over", // the repeat: skip then over
		"skip.if", "expr.when", // the if: skip then when
		"skip.with", "expr.value", // the with: skip then value
		"expr.nested", // the bind nested in the with's body
		"expr.else",   // the bind in the else branch
	}
	if len(got) != len(want) {
		t.Fatalf("WalkExprs visited %d expressions, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("expression %d = %q, want %q", i, got[i], want[i])
		}
	}

	// Every string is reachable — the stronger, order-independent form of
	// the same assertion.
	wantSet := make(map[string]bool, len(want))
	for _, w := range want {
		wantSet[w] = true
	}
	gotSet := make(map[string]bool, len(got))
	for _, g := range got {
		gotSet[g] = true
	}
	for w := range wantSet {
		if !gotSet[w] {
			t.Errorf("expression %q was never reached by WalkExprs", w)
		}
	}
	for g := range gotSet {
		if !wantSet[g] {
			t.Errorf("WalkExprs visited unexpected string %q", g)
		}
	}
}

func TestWalkExprs_SkipsUnsetSkipModifier(t *testing.T) {
	stmts := []bind.Statement{
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "a", Expr: "expr.a"}},
	}
	var got []string
	if err := bind.WalkExprs(stmts, func(expr string) error { got = append(got, expr); return nil }); err != nil {
		t.Fatalf("WalkExprs: %v", err)
	}
	if len(got) != 1 || got[0] != "expr.a" {
		t.Errorf("WalkExprs visited %v, want [\"expr.a\"] — an unset skip must not be visited", got)
	}
}

func TestWalkExprs_PropagatesVisitorError(t *testing.T) {
	sentinel := errors.New("stop")
	err := bind.WalkExprs(deepTree(), func(expr string) error {
		if expr == "expr.when" {
			return sentinel
		}
		return nil
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("WalkExprs error = %v, want sentinel", err)
	}
}
