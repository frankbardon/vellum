package template

// Fill is fill mode's public orchestration entry point: E9 built discovery,
// defragmentation and splicing; E10-S1/S2 built the binding model and
// single-expression FEEL evaluation; E10-S3's bind.Execute threads a scope
// through the whole statement tree and produces every splice a binding's
// repeat/if/with/bind statements need. Fill is what composes those pieces
// against one opened Template and one binding data set, exactly as the
// architecture diagram in CLAUDE.md names it.

import (
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/template/bind"
	"github.com/frankbardon/vellum/xmlcopy"
)

// Result is what filling a template produced.
type Result struct {
	// Touched is the part names Fill actually rewrote, bytewise sorted.
	// CLAUDE.md's own architecture diagram names this the non-destructiveness
	// receipt: every part in Package not named here is byte-identical to
	// the Template's own source part, the same property
	// TestNonDestructiveCorpus already proves at the lower splice/xmlcopy
	// layer, now asserted again at this entry point by the caller who
	// actually uses it.
	Touched []string

	// Package is the filled package, ready for [opc.Package.WriteTo]. It is
	// a clone of the Template's own package (see [opc.Package.Clone]): Fill
	// never mutates the *Template it was called against, so the same opened
	// template can be filled again with different data.
	Package *opc.Package
}

// FillOption configures [Fill].
type FillOption func(*fillConfig)

type fillConfig struct {
	ev bind.Evaluator
}

// WithEvaluator substitutes the [bind.Evaluator] every FEEL expression in
// the binding is evaluated through. The default is [bind.FEELEvaluator],
// bind's own default per CLAUDE.md's "Extension Points" section.
func WithEvaluator(ev bind.Evaluator) FillOption {
	return func(c *fillConfig) { c.ev = ev }
}

// Fill binds data into t according to b and returns the filled package.
//
// t is never mutated: every read against its own package happens through
// t.Package() directly, and every write lands in a clone taken once at the
// start, so the same opened *Template can be filled again with different
// data and the two fills do not interfere with each other or with t.
//
// The steps are exactly the ones CLAUDE.md's architecture diagram names for
// fill mode's Fill stage:
//
//  1. anchor.Discover finds every anchor in t, once.
//  2. bind.Execute runs b.Statements against data as the initial scope,
//     threading it through every nested repeat/if/with, and accumulates
//     the xmlcopy.Replacement each Bind or Repeat statement produces —
//     grouped by the real OPC part name it targets — into a
//     bind.ReplacementSet. Any new media part a spliced Asset block needs
//     is registered directly against the clone from step 3, not against
//     t's own package and not against any throwaway package a Repeat's own
//     body execution used internally.
//  3. A clone of t's own package is what step 2's asset writes, and every
//     part this function touches, actually land in.
//  4. Every touched part's accumulated replacements are applied in one
//     xmlcopy.Apply pass, in ascending order, and the result replaces that
//     part in the clone.
//  5. The clone and the sorted list of touched part names are returned as
//     a *Result.
//
// Before any execution happens, bind.Reconcile checks the discovered
// inventory against the whole of b's statement tree and returns every
// mismatch it finds in one pass — FR-F6's "error on anchors present in the
// template but absent from the binding, and the reverse, unless explicitly
// marked optional" — so a binding author learns about every problem at
// once rather than one at a time as execution happens to trip over them. A
// reconciliation failure returns immediately: no clone is mutated, no
// statement is executed, and the returned *Result is nil.
//
// A binding statement naming an anchor t's inventory does not discover, or
// a repeat whose body's own anchors do not reconcile to one splice
// container, still fails loudly with a coded error rather than silently
// splicing nothing if it somehow reaches execution regardless — see
// bind.Execute's own errors, verr.VELLUM_BIND_ANCHOR_UNKNOWN and
// verr.VELLUM_TEMPLATE_REPEAT_CONTAINER_INVALID — but bind.Reconcile is
// expected to catch every VELLUM_BIND_ANCHOR_UNKNOWN case first.
func Fill(t *Template, b *bind.Binding, data bind.Scope, opts ...FillOption) (*Result, error) {
	if t == nil {
		return nil, verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT, "nil template")
	}
	if b == nil {
		return nil, verr.NewCodedError(verr.VELLUM_BIND_INVALID, "nil binding")
	}

	cfg := fillConfig{ev: bind.NewFEELEvaluator()}
	for _, o := range opts {
		o(&cfg)
	}

	inv, err := anchor.Discover(t.pkg, t.format, t.mainPart)
	if err != nil {
		return nil, err
	}
	if err := bind.Reconcile(inv, b); err != nil {
		return nil, err
	}

	anchors := make(map[string]anchor.Anchor, len(inv.Anchors))
	for _, a := range inv.Anchors {
		anchors[a.Name] = a
	}

	out := t.pkg.Clone()

	repls := bind.NewReplacementSet()
	frame := bind.Frame{SrcPkg: t.pkg, Anchors: anchors}
	if err := bind.Execute(b.Statements, data, cfg.ev, frame, out, repls); err != nil {
		return nil, err
	}

	touched := repls.PartNames()
	for _, name := range touched {
		part, ok := out.Get(name)
		if !ok {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
				"an accumulated replacement names a part the output package does not contain",
				map[string]any{"part": name})
		}
		src, err := part.Bytes()
		if err != nil {
			return nil, err
		}
		applied, err := xmlcopy.Apply(src, repls.For(name))
		if err != nil {
			return nil, err
		}
		if err := out.Put(&opc.Part{Name: name, ContentType: part.ContentType, Data: applied}); err != nil {
			return nil, err
		}
	}

	return &Result{Touched: touched, Package: out}, nil
}
