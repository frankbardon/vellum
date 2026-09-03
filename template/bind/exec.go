package bind

// This file is E10-S3's execution layer: the tree [Walk] only visits, this
// actually runs — threading a [Scope] through nested repeat/if/with,
// evaluating every [Bind] leaf and producing the [xmlcopy.Replacement] each
// one needs, exactly as the package doc's own "Scope of this package"
// section says is layered on top once it exists.
//
// Execute is deliberately not itself a template-level entry point: it has
// no opinion about how its output is written back to a package, and it
// takes every piece of context it needs (the [Evaluator], the anchor
// lookup and byte source it splices against, the package new assets land
// in) as explicit parameters rather than package-level state, so the same
// function serves both the top level of a fill (against the whole opened
// template) and a [Repeat]'s own body (against one iteration's extracted,
// relocated container) without needing two implementations.

import (
	"sort"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/numfmt"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/template/splice"
	"github.com/frankbardon/vellum/xmlcopy"
)

// Frame is the anchor-resolution and byte-source context [Execute] runs
// against.
//
// At the top level of a fill this is the whole opened template's own
// package and its discovered [anchor.Inventory], by name. Inside a
// [Repeat]'s own body it is a throwaway, single-part package holding one
// iteration's extracted container slice, and every anchor the body reaches
// relocated into that slice's own coordinate space — see execRepeat for
// exactly how. A Frame never carries the package new assets land in: that is
// always the real output package, threaded through [Execute] separately as
// assetPkg, precisely so a repeated row's own throwaway view can be
// discarded once its bytes are captured without discarding a picture one of
// its iterations embedded.
type Frame struct {
	// SrcPkg supplies the bytes an anchor's own Part is spliced against.
	SrcPkg *opc.Package

	// Anchors resolves a binding statement's anchor name to the
	// [anchor.Anchor] a splice targets, at this frame's level.
	Anchors map[string]anchor.Anchor
}

// ReplacementSet accumulates every [xmlcopy.Replacement] a binding's
// execution produces, grouped by the real OPC part name it targets.
//
// Replacements are recorded in whatever order [Execute] visits the
// statements that produced them, not in the template's own document order —
// a binding's authors are not required to write their statements in the
// same order the anchors happen to sit in the document. [For] sorts by
// [xmlcopy.Replacement.Start] before handing a part's replacements to a
// caller, which is the order [xmlcopy.Apply] requires; [PartNames] is
// bytewise sorted, never map iteration order, for the same determinism
// reason every other ordered output in this codebase is.
type ReplacementSet struct {
	byPart map[string][]xmlcopy.Replacement
}

// NewReplacementSet returns an empty set.
func NewReplacementSet() *ReplacementSet {
	return &ReplacementSet{byPart: make(map[string][]xmlcopy.Replacement)}
}

// Add records r against part.
func (rs *ReplacementSet) Add(part string, r xmlcopy.Replacement) {
	rs.byPart[part] = append(rs.byPart[part], r)
}

// PartNames returns every part with at least one accumulated replacement,
// bytewise sorted.
func (rs *ReplacementSet) PartNames() []string {
	out := make([]string, 0, len(rs.byPart))
	for name := range rs.byPart {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// For returns part's replacements in ascending Start order — the order
// [xmlcopy.Apply] requires — leaving the set itself untouched. The sort is
// stable, so two replacements that legitimately share a Start keep the
// order they were added in rather than being reordered again on a second
// call.
func (rs *ReplacementSet) For(part string) []xmlcopy.Replacement {
	src := rs.byPart[part]
	out := make([]xmlcopy.Replacement, len(src))
	copy(out, src)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

// Execute runs stmts against scope, in order, threading scope through
// nested repeat/if/with exactly as each statement's own doc comment
// describes, and accumulates every splice it produces into repls.
//
// ev evaluates every FEEL expression. frame resolves an anchor name to the
// [anchor.Anchor] and byte source a leaf splices against. assetPkg is the
// package a Bind statement's own Asset content (reached through the
// fragment.Sequence its Expr's evaluated value builds — see execBind) would
// register a new media part and relationship against; for every call this
// package's own Fill orchestration makes at the top level it is the same
// package frame.SrcPkg reads from, and only inside a Repeat's own body do
// the two diverge (see execRepeat).
//
// A binding is assumed already [Validate]d: this does not re-parse or
// re-check any expression for a banned builtin, only evaluates it.
func Execute(stmts []Statement, scope Scope, ev Evaluator, frame Frame, assetPkg *opc.Package, repls *ReplacementSet) error {
	for i := range stmts {
		if err := execStatement(&stmts[i], scope, ev, frame, assetPkg, repls); err != nil {
			return err
		}
	}
	return nil
}

func execStatement(s *Statement, scope Scope, ev Evaluator, frame Frame, assetPkg *opc.Package, repls *ReplacementSet) error {
	if s.Skip != "" {
		skip, err := ev.EvaluateBool(s.Skip, scope)
		if err != nil {
			return err
		}
		if skip {
			// Skip means this statement — leaf or subtree — is treated as
			// absent: nothing under it is visited, nothing spliced. See
			// Statement.Skip's own doc comment.
			return nil
		}
	}

	switch s.Kind {
	case StatementBind:
		return execBind(s.Bind, scope, ev, frame, assetPkg, repls)
	case StatementIf:
		return execIf(s.If, scope, ev, frame, assetPkg, repls)
	case StatementWith:
		return execWith(s.With, scope, ev, frame, assetPkg, repls)
	case StatementRepeat:
		return execRepeat(s.Repeat, scope, ev, frame, assetPkg, repls)
	default:
		// Unreachable given a Validate()d binding: ValidStatementKind already
		// rejected anything outside the four-kind vocabulary before
		// execution ever starts.
		return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"a statement reached execution with a kind outside the vocabulary Validate already checks",
			map[string]any{"kind": string(s.Kind)})
	}
}

// execBind evaluates b.Expr to a single [numfmt.Value], formats it through
// b.Format, wraps the formatted text as a one-block, one-paragraph,
// one-run [fragment.Sequence] and splices it into b.Anchor's own location.
func execBind(b *Bind, scope Scope, ev Evaluator, frame Frame, assetPkg *opc.Package, repls *ReplacementSet) error {
	a, ok := frame.Anchors[b.Anchor]
	if !ok {
		return unknownAnchorErr(b.Anchor)
	}

	val, err := EvaluateScalar(ev, b.Expr, scope)
	if err != nil {
		return err
	}

	format, err := numfmt.Parse(b.Format)
	if err != nil {
		return verr.WrapCodedErrorWithDetails(err, verr.VELLUM_TABLE_FORMAT_INVALID,
			"a bind statement's number-format code does not parse",
			map[string]any{"anchor": b.Anchor, "format": b.Format})
	}
	text := format.Apply(val)

	seq := fragment.Sequence{Blocks: []fragment.Block{{
		Kind:      spec.BlockText,
		Paragraph: &fragment.Paragraph{Runs: []fragment.Run{{Text: text}}},
	}}}

	repl, err := splice.SpliceInto(frame.SrcPkg, assetPkg, a, seq)
	if err != nil {
		return err
	}
	repls.Add(a.Part, repl)
	return nil
}

// execIf evaluates iv.When and runs Then or Else with the same scope — an
// if statement never narrows anything.
func execIf(iv *If, scope Scope, ev Evaluator, frame Frame, assetPkg *opc.Package, repls *ReplacementSet) error {
	ok, err := ev.EvaluateBool(iv.When, scope)
	if err != nil {
		return err
	}
	if ok {
		return Execute(iv.Then, scope, ev, frame, assetPkg, repls)
	}
	return Execute(iv.Else, scope, ev, frame, assetPkg, repls)
}

// execWith evaluates w.Value once, binds it to w.As in a scope that extends
// (never mutates) the current one, and runs Body against the extended
// scope.
func execWith(w *With, scope Scope, ev Evaluator, frame Frame, assetPkg *opc.Package, repls *ReplacementSet) error {
	v, err := ev.Evaluate(w.Value, scope)
	if err != nil {
		return err
	}
	return Execute(w.Body, extendScope(scope, w.As, v), ev, frame, assetPkg, repls)
}

// extendScope returns a new [Scope] carrying every entry of parent plus
// key/val, never mutating parent — a child statement tree's scope must not
// leak back into a sibling's, and [Scope]'s own doc comment requires the
// result to stay a bare map[string]any, never a wrapped or nested structure,
// for pbinitiative/feel's own type assertions to see through it.
func extendScope(parent Scope, key string, val any) Scope {
	out := make(Scope, len(parent)+1)
	for k, v := range parent {
		out[k] = v
	}
	out[key] = val
	return out
}

func unknownAnchorErr(name string) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_ANCHOR_UNKNOWN,
		"a binding statement names an anchor the template does not discover",
		map[string]any{"anchor": name})
}
