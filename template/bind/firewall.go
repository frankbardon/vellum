package bind

import (
	"strings"

	"github.com/pbinitiative/feel"

	verr "github.com/frankbardon/vellum/errors"
)

// bannedBuiltin is one entry in the nondeterministic-builtin firewall: a
// FEEL builtin function name whose result depends on wall-clock time, plus
// the reason and the alternative a binding author should reach for instead.
type bannedBuiltin struct {
	Name   string
	Reason string
}

// bannedBuiltins is the registry, hand-maintained, in the style of
// errors/codes.go's own registries: exported through [AllBannedBuiltins],
// never mutated after init.
//
// pbinitiative/feel's entire builtin surface was read (every prelude.Bind
// call across builtins.go, builtins_*.go, context.go, range_value.go and
// temporal.go, all reachable from feel.installBuiltinFunctions and its
// siblings) looking specifically for time.Now() and any other source of
// non-reproducible output — a random-number generator, a process- or
// counter-derived identifier, anything whose result would differ between
// two evaluations of the same expression against the same scope. now and
// today are the only two: they are the only builtins in the library that
// call time.Now() (temporal.go, installDatetimeFunctions). Nothing in the
// library reaches math/rand, crypto/rand, or any other nondeterministic
// source. date(), time(), "date and time"() and duration() all parse a
// caller-supplied string argument and are pure functions of it — banning
// them because they share a name prefix with now/today would be banning
// determinism itself, which CLAUDE.md's own guidance for this story warns
// against directly.
var bannedBuiltins = []bannedBuiltin{
	{
		Name:   "now",
		Reason: "now() binds directly to time.Now() (pbinitiative/feel, temporal.go): two evaluations of the same expression at different wall-clock moments disagree, which breaks byte-identical output. Put the reference instant in the binding data as an as_of field and compare against that instead.",
	},
	{
		Name:   "today",
		Reason: "today() binds directly to time.Now(), truncated to the calendar day (pbinitiative/feel, temporal.go): two evaluations on different calendar days disagree, which breaks byte-identical output. Put the reference date in the binding data as an as_of field and compare against that instead.",
	},
}

// AllBannedBuiltins returns the names of every FEEL builtin [Validate]
// rejects a call to, in registry order. Copy-returning, in the style of
// errors.AllCodes and this package's own AllStatementKinds/AllRepeatTargets.
func AllBannedBuiltins() []string {
	out := make([]string, len(bannedBuiltins))
	for i, b := range bannedBuiltins {
		out[i] = b.Name
	}
	return out
}

// Validate checks a binding both structurally — delegating to
// [Binding.Validate] — and for every FEEL expression it carries: that the
// expression parses, and that it calls no builtin in the nondeterministic
// registry ([AllBannedBuiltins]).
//
// It never evaluates an expression. A syntactically valid FEEL expression
// is parsed to its AST ([feel.ParseString]) and the AST is inspected for a
// call to a banned builtin; evaluating the expression to make that
// decision would call the very builtin this function exists to forbid, and
// would additionally require binding data this function does not have —
// Validate runs at binding-authoring time, before any fill and before any
// caller-supplied data exists.
func Validate(b *Binding) error {
	if err := b.Validate(); err != nil {
		return err
	}
	return WalkExprs(b.Statements, validateExpr)
}

// validateExpr parses expr and rejects a call to a banned builtin anywhere
// in it, at any nesting depth.
//
// The nesting-depth part is why this walks feel.Node.Repr() rather than the
// AST's own field structure node by node: pbinitiative/feel's FunCall.Args
// is a slice of an unexported struct whose own fields (the argument
// expression among them) are unexported, so a call nested inside another
// call's argument list — sum(now(), 1) — is not reachable through ordinary
// field access from outside the feel package at all. Repr() is exported on
// every feel.Node, is FEEL's own recursive serialisation of the full tree
// (implemented inside the feel package, where it has full access to those
// unexported fields), and produces an unambiguous, parenthesised form for a
// function call: "(call now [])", "(call sum [(call now []), 1])". Matching
// "(call <name> [" against it finds a call to name wherever in the tree it
// sits, and — because the form always has a delimiter immediately after
// the name — does not false-match a longer name sharing a prefix ("nowish"
// does not match the signature for "now"). This is a structural match
// against the AST's own canonical serialisation, not a regex over the raw
// source text: it does not depend on the author's whitespace, comment
// placement (FEEL has none) or quoting, only on feel.Node.Repr()'s stable,
// exported output shape.
func validateExpr(expr string) error {
	ast, err := feel.ParseString(expr)
	if err != nil {
		return malformedExprError(expr, err)
	}

	repr := ast.Repr()
	for _, bb := range bannedBuiltins {
		if strings.Contains(repr, callSignature(bb.Name)) {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_NONDETERMINISTIC_EXPR,
				"the expression calls a builtin whose result is not deterministic",
				map[string]any{
					"expr":        expr,
					"builtin":     bb.Name,
					"alternative": bb.Reason,
				})
		}
	}
	return nil
}

// callSignature returns the substring feel.Node.Repr() emits for a call to
// name, mirroring feel.Var.Repr()'s own quoting rule exactly (a name
// containing a space is backtick-quoted; every FEEL builtin's arguments are
// unary-word names like "now" and "today" today, but the registry is not
// restricted to those, so this stays general).
func callSignature(name string) string {
	if strings.Contains(name, " ") {
		return "(call `" + name + "` ["
	}
	return "(call " + name + " ["
}
