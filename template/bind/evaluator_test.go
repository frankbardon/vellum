package bind_test

import (
	"testing"
	"time"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/numfmt"
	"github.com/frankbardon/vellum/template/bind"
)

func TestFEELEvaluator_ScalarKinds(t *testing.T) {
	ev := bind.NewFEELEvaluator()

	cases := []struct {
		name string
		expr string
		want numfmt.Value
	}{
		{"string", `"hello"`, numfmt.Value{Kind: numfmt.KindText, Text: "hello"}},
		{"number", `1 + 2`, numfmt.Value{Kind: numfmt.KindNumber, Number: 3}},
		{"bool_true", `true`, numfmt.Value{Kind: numfmt.KindBool, Bool: true}},
		{"bool_false", `1 > 2`, numfmt.Value{Kind: numfmt.KindBool, Bool: false}},
		{"date", `date("2024-06-01")`, numfmt.Value{Kind: numfmt.KindDate, Time: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)}},
		{"null", `null`, numfmt.Value{Kind: numfmt.KindEmpty}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := bind.EvaluateScalar(ev, c.expr, nil)
			if err != nil {
				t.Fatalf("EvaluateScalar(%q) error: %v", c.expr, err)
			}
			if got.Kind != c.want.Kind {
				t.Fatalf("Kind = %v, want %v", got.Kind, c.want.Kind)
			}
			switch c.want.Kind {
			case numfmt.KindText:
				if got.Text != c.want.Text {
					t.Errorf("Text = %q, want %q", got.Text, c.want.Text)
				}
			case numfmt.KindNumber:
				if got.Number != c.want.Number {
					t.Errorf("Number = %v, want %v", got.Number, c.want.Number)
				}
			case numfmt.KindBool:
				if got.Bool != c.want.Bool {
					t.Errorf("Bool = %v, want %v", got.Bool, c.want.Bool)
				}
			case numfmt.KindDate:
				if !got.Time.Equal(c.want.Time) {
					t.Errorf("Time = %v, want %v", got.Time, c.want.Time)
				}
			}
		})
	}
}

func TestFEELEvaluator_NestedScopeDottedPath(t *testing.T) {
	ev := bind.NewFEELEvaluator()
	scope := bind.Scope{
		"customer": map[string]any{
			"name": "Jane Doe",
			"address": map[string]any{
				"city": "Springfield",
			},
		},
	}

	got, err := bind.EvaluateScalar(ev, "customer.name", scope)
	if err != nil {
		t.Fatalf("EvaluateScalar error: %v", err)
	}
	if got.Kind != numfmt.KindText || got.Text != "Jane Doe" {
		t.Fatalf("got %+v, want text \"Jane Doe\"", got)
	}

	got, err = bind.EvaluateScalar(ev, "customer.address.city", scope)
	if err != nil {
		t.Fatalf("EvaluateScalar (nested) error: %v", err)
	}
	if got.Kind != numfmt.KindText || got.Text != "Springfield" {
		t.Fatalf("got %+v, want text \"Springfield\"", got)
	}
}

func TestFEELEvaluator_ListValuedExpression(t *testing.T) {
	ev := bind.NewFEELEvaluator()
	scope := bind.Scope{"items": []any{"a", "b", "c"}}

	got, err := bind.EvaluateList(ev, "items", scope)
	if err != nil {
		t.Fatalf("EvaluateList error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("got %v", got)
	}
}

func TestEvaluateScalar_RejectsList(t *testing.T) {
	ev := bind.NewFEELEvaluator()
	_, err := bind.EvaluateScalar(ev, "[1,2,3]", nil)
	if !verr.HasCode(err, verr.VELLUM_BIND_VALUE_NOT_SCALAR) {
		t.Fatalf("error = %v, want VELLUM_BIND_VALUE_NOT_SCALAR", err)
	}
}

func TestEvaluateScalar_RejectsContext(t *testing.T) {
	ev := bind.NewFEELEvaluator()
	_, err := bind.EvaluateScalar(ev, `{a: 1}`, nil)
	if !verr.HasCode(err, verr.VELLUM_BIND_VALUE_NOT_SCALAR) {
		t.Fatalf("error = %v, want VELLUM_BIND_VALUE_NOT_SCALAR", err)
	}
}

func TestEvaluateList_RejectsScalar(t *testing.T) {
	ev := bind.NewFEELEvaluator()
	_, err := bind.EvaluateList(ev, `"a"`, nil)
	if !verr.HasCode(err, verr.VELLUM_BIND_VALUE_NOT_LIST) {
		t.Fatalf("error = %v, want VELLUM_BIND_VALUE_NOT_LIST", err)
	}
}

func TestEvaluateScalar_RejectsDuration(t *testing.T) {
	ev := bind.NewFEELEvaluator()
	_, err := bind.EvaluateScalar(ev, `duration("P1DT2H")`, nil)
	if !verr.HasCode(err, verr.VELLUM_BIND_VALUE_UNSUPPORTED_TYPE) {
		t.Fatalf("error = %v, want VELLUM_BIND_VALUE_UNSUPPORTED_TYPE", err)
	}
}

func TestFEELEvaluator_Evaluate_MalformedSyntax(t *testing.T) {
	ev := bind.NewFEELEvaluator()
	_, err := ev.Evaluate("1 + ", nil)
	if !verr.HasCode(err, verr.VELLUM_BIND_EXPR_MALFORMED) {
		t.Fatalf("error = %v, want VELLUM_BIND_EXPR_MALFORMED", err)
	}
}

func TestFEELEvaluator_Evaluate_RuntimeFailureIsDistinctFromMalformed(t *testing.T) {
	ev := bind.NewFEELEvaluator()
	// "missing" resolves to null (unresolved var), and a dotted access on
	// null is a genuine FEEL type-mismatch error, not a parse error.
	_, err := ev.Evaluate("missing.name", bind.Scope{})
	if !verr.HasCode(err, verr.VELLUM_BIND_EVAL_FAILED) {
		t.Fatalf("error = %v, want VELLUM_BIND_EVAL_FAILED", err)
	}
	if verr.HasCode(err, verr.VELLUM_BIND_EXPR_MALFORMED) {
		t.Fatalf("runtime failure incorrectly also carries VELLUM_BIND_EXPR_MALFORMED: %v", err)
	}
}

func TestFEELEvaluator_Evaluate_RecoversPanicFromDivisionByZero(t *testing.T) {
	ev := bind.NewFEELEvaluator()
	_, err := ev.Evaluate("1/0", nil)
	if err == nil {
		t.Fatal("expected an error, got nil (panic escaped uncaught?)")
	}
	if !verr.HasCode(err, verr.VELLUM_BIND_EVAL_FAILED) {
		t.Fatalf("error = %v, want VELLUM_BIND_EVAL_FAILED", err)
	}
}

func TestFEELEvaluator_EvaluateBool(t *testing.T) {
	ev := bind.NewFEELEvaluator()

	cases := []struct {
		expr string
		want bool
	}{
		{"true", true},
		{"false", false},
		{"1 = 1", true},
		{`""`, false},
		{`"x"`, true},
		{"0", false},
		{"1", true},
		// FEEL's own null-is-truthy rule, deferred to rather than
		// reimplemented — see Evaluator.EvaluateBool's doc comment.
		{"null", true},
		{"[]", false},
		{"[1]", true},
	}
	for _, c := range cases {
		got, err := ev.EvaluateBool(c.expr, nil)
		if err != nil {
			t.Fatalf("EvaluateBool(%q) error: %v", c.expr, err)
		}
		if got != c.want {
			t.Errorf("EvaluateBool(%q) = %v, want %v", c.expr, got, c.want)
		}
	}
}

func TestFEELEvaluator_EvaluateBool_DeterministicDateFunctionAccepted(t *testing.T) {
	ev := bind.NewFEELEvaluator()
	// pbinitiative/feel's own comparison operator does not implement
	// *feel.FEELDate < *feel.FEELDate (only *Number and *FEELDatetime are
	// handled in eval_binop.go's compareValues) — a library limitation, not
	// a Vellum one. "date and time(...)" (a FEELDatetime) compares fine and
	// is exactly as deterministic: both are pure functions of their literal
	// string argument, not of the clock.
	got, err := ev.EvaluateBool(
		`date and time("2024-01-01T00:00:00") < date and time("2024-06-01T00:00:00")`, nil)
	if err != nil {
		t.Fatalf("EvaluateBool error: %v", err)
	}
	if !got {
		t.Fatal("expected true")
	}
}
