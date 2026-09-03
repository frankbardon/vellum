// Package bind is the specification model for a fill-mode binding: the
// declarative, strictly-decoded, hashable shape an engineer authors to map a
// template's anchors onto FEEL expressions evaluated against caller-supplied
// data.
//
// "Templates declare slots; bindings declare logic." A template (a .docx, in
// v1) carries anchors — see [template/anchor] — and nothing else. A binding
// is a separate document, reviewed independently of the binary template it
// targets, naming which anchor each expression fills and the thin control
// layer (repeat, if, with, skip) that produces more than one leaf binding
// from one nested tree. It mirrors [spec.Spec] deliberately: strict JSON
// decode with unknown fields refused, YAML normalised to canonical JSON at
// the boundary, and a canonical content hash so two bindings that mean the
// same thing hash identically regardless of how they were typed.
//
// # Scope of this package
//
// This package is model, decode, hash, single-expression FEEL evaluation
// (E10-S2) and, as of E10-S3, execution: [Execute] threads a [Scope]
// through a whole statement tree exactly as repeat/if/with/skip's own doc
// comments describe, evaluating every [Bind] leaf and producing the
// [xmlcopy.Replacement] each one needs against a [Frame] — the anchor
// lookup and byte source execution runs against, which is either the whole
// opened template at the top of a fill or, inside a [Repeat]'s own body, a
// throwaway view built from one iteration's extracted, relocated container
// (see repeat.go). Execute has no opinion about how its output reaches a
// package on disk; composing that against one opened template and one data
// set is [template.Fill]'s job, one layer up.
//
// What this package guarantees structurally is that the tree [Execute] runs
// is well-formed: every statement's kind matches the one arm it carries,
// every required field is present, and every FEEL-bearing string is
// reachable through [Walk] and [WalkExprs]. [Validate] uses the latter to
// reject a nondeterministic builtin without needing to know anything about
// statement shape beyond "here is an expression" — and [Execute] assumes a
// binding has already been [Validate]d, re-checking neither.
//
// What this package guarantees about a single expression, once it has data
// to run against, is the [Evaluator] seam: [FEELEvaluator], the default,
// calls pbinitiative/feel directly — no suite wrapper reimplementing FEEL's
// own semantics — and the "value modes" documented on [EvaluateScalar],
// [EvaluateList] and [Evaluator.EvaluateBool] say precisely what shape each
// statement kind's expression is expected to produce and what happens when
// it produces something else.
package bind
