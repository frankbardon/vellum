package bind_test

import (
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/template/bind"
)

func bindingWithExpr(expr string) *bind.Binding {
	return &bind.Binding{
		FormatVersion: bind.FormatVersion,
		Statements: []bind.Statement{
			{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "a", Expr: expr}},
		},
	}
}

// TestBindBannedBuiltinsComplete is the completeness gate CLAUDE.md's Update
// Demand table names: the registry [bind.AllBannedBuiltins] and
// [bind.Validate]'s actual rejection behaviour cannot drift apart.
//
// Direction one: every registered name, called as a trivial function call,
// is actually rejected by Validate with VELLUM_BIND_NONDETERMINISTIC_EXPR —
// so a name added to the registry with no matching rejection (or the
// reverse) fails here. Direction two, "nothing rejected by Validate is
// missing from the registry", is true by construction rather than checked
// by a second scan: Validate's own rejection logic (validateExpr) has no
// path to VELLUM_BIND_NONDETERMINISTIC_EXPR other than a name match against
// this same registry, so the two cannot disagree about a name Validate
// doesn't otherwise recognise as banned. What direction two is really
// standing in for — whether the registry itself covers every
// nondeterministic builtin pbinitiative/feel actually ships — is not
// mechanically checkable from outside the library (its Prelude has no
// exported enumeration), and is instead documented at the registry
// (bannedBuiltins in firewall.go): every prelude.Bind call site in the
// library's source was read looking for time.Now(), and now/today are the
// only two found.
func TestBindBannedBuiltinsComplete(t *testing.T) {
	names := bind.AllBannedBuiltins()
	if len(names) == 0 {
		t.Fatal("AllBannedBuiltins returned no names — the registry should not be empty")
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			expr := name + "()"
			err := bind.Validate(bindingWithExpr(expr))
			if err == nil {
				t.Fatalf("Validate(%q) = nil, want VELLUM_BIND_NONDETERMINISTIC_EXPR", expr)
			}
			if !verr.HasCode(err, verr.VELLUM_BIND_NONDETERMINISTIC_EXPR) {
				t.Fatalf("Validate(%q) error = %v, want code VELLUM_BIND_NONDETERMINISTIC_EXPR", expr, err)
			}
		})
	}

	// Non-vacuity: this completeness check only means something if Validate
	// does *not* reject everything indiscriminately. A deterministic
	// builtin call, from outside the registry, must pass Validate cleanly —
	// otherwise a rejection of every registered name above would be
	// consistent with a Validate that rejects all function calls, and the
	// gate would prove nothing about the registry specifically.
	t.Run("non_vacuous", func(t *testing.T) {
		err := bind.Validate(bindingWithExpr(`sum([1,2,3])`))
		if err != nil {
			t.Fatalf("Validate(deterministic builtin call) = %v, want nil", err)
		}
	})
}

func TestValidate_EachBannedBuiltinIndividually(t *testing.T) {
	for _, name := range bind.AllBannedBuiltins() {
		expr := name + "()"
		err := bind.Validate(bindingWithExpr(expr))
		if !verr.HasCode(err, verr.VELLUM_BIND_NONDETERMINISTIC_EXPR) {
			t.Errorf("Validate(%q) = %v, want VELLUM_BIND_NONDETERMINISTIC_EXPR", expr, err)
		}
		var coded *verr.CodedError
		if ce, ok := err.(*verr.CodedError); ok {
			coded = ce
		}
		if coded == nil {
			t.Fatalf("Validate(%q) error is not *errors.CodedError: %v", expr, err)
		}
		if got, _ := coded.Detail("builtin"); got != name {
			t.Errorf("Validate(%q) detail[builtin] = %v, want %q", expr, got, name)
		}
		if got, _ := coded.Detail("expr"); got != expr {
			t.Errorf("Validate(%q) detail[expr] = %v, want %q", expr, got, expr)
		}
	}
}

func TestValidate_BannedBuiltinNestedInsideAnotherCall(t *testing.T) {
	err := bind.Validate(bindingWithExpr(`sum(now(), 1)`))
	if !verr.HasCode(err, verr.VELLUM_BIND_NONDETERMINISTIC_EXPR) {
		t.Fatalf("Validate(nested now()) error = %v, want VELLUM_BIND_NONDETERMINISTIC_EXPR", err)
	}
}

func TestValidate_BannedBuiltinDoesNotFalseMatchLongerName(t *testing.T) {
	// "nowish" is not a real FEEL builtin, but it would fail to parse as a
	// call anyway (unknown function) at evaluation time, not at Validate —
	// Validate only rejects a *banned* call, an unrecognised call is not
	// its concern. What matters here is that the substring match for "now"
	// does not spuriously fire on "nowish".
	err := bind.Validate(bindingWithExpr(`nowish(1)`))
	if verr.HasCode(err, verr.VELLUM_BIND_NONDETERMINISTIC_EXPR) {
		t.Fatalf("Validate(nowish(1)) incorrectly flagged as nondeterministic: %v", err)
	}
}

func TestValidate_DeterministicDateFunctionAccepted(t *testing.T) {
	err := bind.Validate(bindingWithExpr(`date("2024-01-01")`))
	if err != nil {
		t.Fatalf("Validate(date(...)) = %v, want nil", err)
	}
}

func TestValidate_MalformedSyntaxIsDistinctFromBannedBuiltin(t *testing.T) {
	err := bind.Validate(bindingWithExpr("1 + "))
	if !verr.HasCode(err, verr.VELLUM_BIND_EXPR_MALFORMED) {
		t.Fatalf("error = %v, want VELLUM_BIND_EXPR_MALFORMED", err)
	}
	if verr.HasCode(err, verr.VELLUM_BIND_NONDETERMINISTIC_EXPR) {
		t.Fatalf("malformed expression incorrectly also carries VELLUM_BIND_NONDETERMINISTIC_EXPR: %v", err)
	}
}

func TestValidate_BannedBuiltinReachedThroughSkipRepeatIfWith(t *testing.T) {
	b := &bind.Binding{
		FormatVersion: bind.FormatVersion,
		Statements: []bind.Statement{
			{Kind: bind.StatementRepeat, Skip: "today() = today()", Repeat: &bind.Repeat{
				Over: "items", As: "item", Target: bind.RepeatTargetRow,
				Body: []bind.Statement{
					{Kind: bind.StatementIf, If: &bind.If{
						When: "item.active",
						Then: []bind.Statement{
							{Kind: bind.StatementWith, With: &bind.With{
								As: "scoped", Value: `now()`,
								Body: []bind.Statement{
									{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "a", Expr: "scoped"}},
								},
							}},
						},
					}},
				},
			}},
		},
	}
	err := bind.Validate(b)
	if !verr.HasCode(err, verr.VELLUM_BIND_NONDETERMINISTIC_EXPR) {
		t.Fatalf("error = %v, want VELLUM_BIND_NONDETERMINISTIC_EXPR (via Skip)", err)
	}
}

func TestValidate_StructuralFailureStillCaught(t *testing.T) {
	err := bind.Validate(&bind.Binding{FormatVersion: bind.FormatVersion})
	if !verr.HasCode(err, verr.VELLUM_BIND_INVALID) {
		t.Fatalf("error = %v, want VELLUM_BIND_INVALID", err)
	}
}
