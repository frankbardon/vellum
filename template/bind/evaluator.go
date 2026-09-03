package bind

import (
	"fmt"
	"time"

	"github.com/pbinitiative/feel"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/numfmt"
)

// Scope is the evaluation context a FEEL expression is evaluated against:
// the caller-supplied binding data at the top level, extended with whatever
// a [Repeat]'s As or a [With]'s As has bound so far in nested evaluation —
// threading that extension through a whole statement tree is [Walk]'s and a
// later execution layer's job (E10-S3), not this file's.
//
// This is a type *alias* for map[string]any, not a distinct defined type,
// deliberately: pbinitiative/feel recognises nested scope data — the
// caller's own nested maps, for dotted-path access like "customer.name" —
// by asserting the exact type map[string]interface{} (feel.DotOp.Eval and
// feel.normalizeScope both do this). A named type with the same underlying
// shape satisfies that assertion at the top level, where this package hands
// the whole scope to feel, but *not* one level down: a nested value whose
// dynamic type is the named type rather than the bare map fails the
// assertion silently and reads as "not a map", breaking dotted access into
// exactly the caller-supplied data this scope exists to expose. Keeping
// Scope an alias means every level of caller-supplied nesting — and every
// value a [Repeat] or [With] binds from a FEEL result, which is itself
// already plain map[string]any because [Evaluate] never wraps it — is
// identically map[string]interface{} all the way down.
type Scope = map[string]any

// Evaluator evaluates a single FEEL expression against a [Scope].
//
// This is the seam CLAUDE.md's "Extension Points" section already names:
// bind.Evaluator, default FEEL. A host that wires nothing gets the default
// implementation in this file, [FEELEvaluator] — the same inert-default
// pattern as theme.Provider and asset.Resolver — so fill mode works without
// any evaluator configuration. A host substituting its own implementation
// (a different expression language, a sandboxed or metered evaluator) needs
// only satisfy this interface.
type Evaluator interface {
	// Evaluate parses and evaluates expr against scope and returns the raw
	// result, converted only as far as pbinitiative/feel's own public
	// EvalStringWithScope converts it — never further, because what shape
	// is expected next depends on which statement kind is asking, and that
	// is not this method's concern. The dynamic type of a non-nil, non-error
	// result is one of:
	//
	//   - nil                — FEEL null
	//   - bool                — FEEL boolean
	//   - string              — FEEL string
	//   - a numeric kind (float64 for anything built from a FEEL number
	//     literal or arithmetic; whatever numeric Go kind the caller's own
	//     scope data supplied, for a bare variable reference that was never
	//     touched by FEEL arithmetic)
	//   - time.Time           — a FEEL date, time-of-day or datetime. FEEL's
	//     three temporal kinds are not distinguished at this layer, which
	//     matches numfmt.Value's own single KindDate: the format code
	//     applied later decides which components are shown.
	//   - time.Duration        — a FEEL duration
	//   - []any                — a FEEL list, each element recursively one
	//     of these
	//   - map[string]any       — a FEEL context, same recursively
	//
	// Evaluate does not reject by shape. See ToScalar and ToList in this
	// file's "value modes" for the per-statement-kind coercion built on top.
	Evaluate(expr string, scope Scope) (any, error)

	// EvaluateBool evaluates expr for its truth value, deferring entirely to
	// FEEL's own boolean-coercion rule rather than Vellum inventing a
	// separate one. That rule (unexported inside pbinitiative/feel as
	// boolValue, and used internally by FEEL's own "if ... then ... else"
	// and "some/every ... satisfies") is, notably, not "must literally be
	// true or false": a nonzero number, a nonempty string, a nonempty list
	// or context, and — the one worth calling out explicitly because it is
	// easy to get backwards — FEEL's own null value are all *true*; only
	// zero, "", an empty list, an empty context and false are false. An
	// unresolved variable reference evaluates to null and is therefore
	// truthy under this rule, not falsy: a Skip or an If.When referencing a
	// misspelled or absent scope key is not silently treated as "skip
	// nothing" or "take the else branch" — it is treated as true. This is
	// FEEL's own documented-by-implementation behaviour, not a Vellum
	// choice, and is called out here because it is the single most
	// surprising thing about deferring to it.
	EvaluateBool(expr string, scope Scope) (bool, error)
}

// FEELEvaluator is the default [Evaluator], backed directly by
// pbinitiative/feel: it calls the library's own parse and evaluate
// functions plainly, with no suite wrapper reimplementing or narrowing
// FEEL's own semantics. Its zero value is ready to use.
type FEELEvaluator struct{}

// NewFEELEvaluator returns the default FEEL-backed [Evaluator].
func NewFEELEvaluator() FEELEvaluator {
	return FEELEvaluator{}
}

// Evaluate implements [Evaluator].
func (FEELEvaluator) Evaluate(expr string, scope Scope) (result any, err error) {
	if _, perr := feel.ParseString(expr); perr != nil {
		return nil, malformedExprError(expr, perr)
	}

	// pbinitiative/feel's arbitrary-precision arithmetic panics rather than
	// erroring on a division by zero (math/big.(*Int).Div, reached from
	// feel's own Binop division path). A caller-supplied FEEL expression is
	// untrusted input exactly like a template's own bytes; a panic reaching
	// out of this package for that reason is treated the same as any other
	// malformed input, not as a Vellum bug.
	defer func() {
		if r := recover(); r != nil {
			result, err = nil, evalPanicError(expr, r)
		}
	}()

	v, evalErr := feel.EvalStringWithScope(expr, feel.Scope(scope))
	if evalErr != nil {
		return nil, evalFailedError(expr, evalErr)
	}
	return v, nil
}

// EvaluateBool implements [Evaluator].
//
// It parses expr once and wraps the resulting AST node in a synthetic
// "if <expr> then true else false", then evaluates that composed AST
// directly. This is not a reimplementation of FEEL's boolean coercion: the
// synthetic if/then/else is evaluated by feel.IfExpr.Eval, the same method
// FEEL's own parser produces for a literal "if" expression, so the
// coercion is entirely FEEL's, arrived at by composing two of its exported
// AST node types ([feel.IfExpr], [feel.BoolNode]) rather than by parsing a
// concatenated string (fragile against an expr that is itself a
// semicolon-separated expression list) or by duplicating feel's unexported
// boolValue logic (fragile against it changing without this package
// noticing).
func (FEELEvaluator) EvaluateBool(expr string, scope Scope) (result bool, err error) {
	ast, perr := feel.ParseString(expr)
	if perr != nil {
		return false, malformedExprError(expr, perr)
	}

	defer func() {
		if r := recover(); r != nil {
			result, err = false, evalPanicError(expr, r)
		}
	}()

	node := &feel.IfExpr{
		Cond:       ast,
		ThenBranch: &feel.BoolNode{Value: true},
		ElseBranch: &feel.BoolNode{Value: false},
	}
	intp := feel.NewIntepreter()
	intp.Push(feel.Scope(scope))

	v, evalErr := node.Eval(intp)
	if evalErr != nil {
		return false, evalFailedError(expr, evalErr)
	}
	b, ok := v.(bool)
	if !ok {
		// Unreachable by construction: both branches of the synthetic if
		// are BoolNode literals, whose Eval always returns a Go bool. Kept
		// as a checked error rather than a panic because "unreachable" is a
		// claim about this file's own code, not about pbinitiative/feel's,
		// and a library upgrade is exactly the kind of change that could
		// falsify it silently.
		return false, verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_EVAL_FAILED,
			"evaluating the expression's boolean coercion did not produce a boolean",
			map[string]any{"expr": expr, "got": fmt.Sprintf("%T", v)})
	}
	return b, nil
}

func malformedExprError(expr string, cause error) error {
	return verr.WrapCodedErrorWithDetails(cause, verr.VELLUM_BIND_EXPR_MALFORMED,
		"the FEEL expression does not parse",
		map[string]any{"expr": expr, "parse_error": cause.Error()})
}

func evalFailedError(expr string, cause error) error {
	return verr.WrapCodedErrorWithDetails(cause, verr.VELLUM_BIND_EVAL_FAILED,
		"the FEEL expression failed to evaluate against its scope",
		map[string]any{"expr": expr, "eval_error": cause.Error()})
}

func evalPanicError(expr string, recovered any) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_EVAL_FAILED,
		"evaluating the FEEL expression panicked",
		map[string]any{"expr": expr, "panic": fmt.Sprint(recovered)})
}

// Value modes
//
// A raw [Evaluator.Evaluate] result is FEEL-shaped, not Vellum-shaped, and
// what shape is *correct* depends on which statement kind produced the
// expression:
//
//   - Bind.Expr always expects a scalar: a bind statement fills exactly one
//     anchor with exactly one value. [EvaluateScalar] enforces this,
//     converting a scalar to numfmt.Value and rejecting a list or context
//     with VELLUM_BIND_VALUE_NOT_SCALAR.
//   - Repeat.Over always expects a list — the inverse of Bind.Expr, for the
//     inverse reason: the list's length is how many copies a repeat
//     produces, and a scalar or a context has no such count.
//     [EvaluateList] enforces this, rejecting anything else with
//     VELLUM_BIND_VALUE_NOT_LIST.
//   - If.When (and Skip, which is the same kind of expression) is not
//     scalar-typed at all: it defers to FEEL's own boolean coercion via
//     [Evaluator.EvaluateBool] rather than requiring a literal bool result
//     or inventing a Vellum-specific truthiness rule. See EvaluateBool's
//     own doc comment for exactly what that coercion does, including the
//     surprising case (null is truthy).
//   - With.Value has no dedicated value-mode helper. With's only job is to
//     bind a name to a value for statements nested inside it to reference
//     — a scalar to use directly, a context to dot into, or a list to
//     later repeat over — so any shape is legitimate and
//     [Evaluator.Evaluate]'s raw, unconverted result is exactly what gets
//     bound. Constraining it here would be inventing a restriction the
//     statement itself does not have.

// EvaluateScalar evaluates expr against scope via ev and converts a FEEL
// scalar result to a [numfmt.Value] — the value mode [Bind.Expr] always
// uses. See "Value modes" above.
func EvaluateScalar(ev Evaluator, expr string, scope Scope) (numfmt.Value, error) {
	v, err := ev.Evaluate(expr, scope)
	if err != nil {
		return numfmt.Value{}, err
	}
	return toScalar(expr, v)
}

// EvaluateList evaluates expr against scope via ev and requires a FEEL list
// result — the value mode [Repeat.Over] always uses. See "Value modes"
// above.
func EvaluateList(ev Evaluator, expr string, scope Scope) ([]any, error) {
	v, err := ev.Evaluate(expr, scope)
	if err != nil {
		return nil, err
	}
	list, ok := v.([]any)
	if !ok {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_VALUE_NOT_LIST,
			"the repeat's over expression did not evaluate to a list",
			map[string]any{"expr": expr, "got": feelKindOf(v)})
	}
	return list, nil
}

// toScalar converts a raw FEEL result to a numfmt.Value, or reports why it
// cannot: VELLUM_BIND_VALUE_NOT_SCALAR for a list or context (the shape
// mismatch a caller can fix by restructuring the expression or switching to
// a repeat), VELLUM_BIND_VALUE_UNSUPPORTED_TYPE for a scalar FEEL produces
// that numfmt.Value simply has no variant for (a duration, a function, a
// range).
func toScalar(expr string, v any) (numfmt.Value, error) {
	switch vv := v.(type) {
	case nil:
		return numfmt.Value{Kind: numfmt.KindEmpty}, nil
	case bool:
		return numfmt.Value{Kind: numfmt.KindBool, Bool: vv}, nil
	case string:
		return numfmt.Value{Kind: numfmt.KindText, Text: vv}, nil
	case time.Time:
		return numfmt.Value{Kind: numfmt.KindDate, Time: vv}, nil
	case float64:
		return numfmt.Value{Kind: numfmt.KindNumber, Number: vv}, nil
	case float32:
		return numfmt.Value{Kind: numfmt.KindNumber, Number: float64(vv)}, nil
	case int:
		return numfmt.Value{Kind: numfmt.KindNumber, Number: float64(vv)}, nil
	case int8:
		return numfmt.Value{Kind: numfmt.KindNumber, Number: float64(vv)}, nil
	case int16:
		return numfmt.Value{Kind: numfmt.KindNumber, Number: float64(vv)}, nil
	case int32:
		return numfmt.Value{Kind: numfmt.KindNumber, Number: float64(vv)}, nil
	case int64:
		return numfmt.Value{Kind: numfmt.KindNumber, Number: float64(vv)}, nil
	case uint:
		return numfmt.Value{Kind: numfmt.KindNumber, Number: float64(vv)}, nil
	case uint8:
		return numfmt.Value{Kind: numfmt.KindNumber, Number: float64(vv)}, nil
	case uint16:
		return numfmt.Value{Kind: numfmt.KindNumber, Number: float64(vv)}, nil
	case uint32:
		return numfmt.Value{Kind: numfmt.KindNumber, Number: float64(vv)}, nil
	case uint64:
		return numfmt.Value{Kind: numfmt.KindNumber, Number: float64(vv)}, nil
	case []any:
		return numfmt.Value{}, verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_VALUE_NOT_SCALAR,
			"the bind expression evaluated to a list, not a single value",
			map[string]any{"expr": expr, "got": "list"})
	case map[string]any:
		return numfmt.Value{}, verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_VALUE_NOT_SCALAR,
			"the bind expression evaluated to a context, not a single value",
			map[string]any{"expr": expr, "got": "context"})
	default:
		return numfmt.Value{}, verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_VALUE_UNSUPPORTED_TYPE,
			"the bind expression evaluated to a scalar type numfmt.Value has no variant for",
			map[string]any{"expr": expr, "got": feelKindOf(v)})
	}
}

// feelKindOf names a raw Evaluate result's shape for an error detail, in the
// same vocabulary [Evaluator.Evaluate]'s own doc comment uses.
func feelKindOf(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case string:
		return "string"
	case time.Time:
		return "date"
	case time.Duration:
		return "duration"
	case []any:
		return "list"
	case map[string]any:
		return "context"
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "number"
	default:
		return fmt.Sprintf("%T", v)
	}
}
