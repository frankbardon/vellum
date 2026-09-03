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
// This package is model, decode and hash — plus, as of E10-S2, single-
// expression FEEL evaluation and the nondeterminism firewall in front of it.
// It still does not execute control flow: repeat does not iterate, if does
// not branch, with does not narrow a scope across a whole tree. That
// orchestration — threading a [Scope] through nested repeat/if/with as
// [Walk] visits them — is E10-S3's job, layered on top of what is here.
//
// What this package guarantees structurally is that the tree it hands to
// that later layer is well-formed: every statement's kind matches the one
// arm it carries, every required field is present, and every FEEL-bearing
// string is reachable through [Walk] and [WalkExprs]. [Validate] uses the
// latter to reject a nondeterministic builtin without needing to know
// anything about statement shape beyond "here is an expression".
//
// What this package guarantees about a single expression, once it has data
// to run against, is the [Evaluator] seam: [FEELEvaluator], the default,
// calls pbinitiative/feel directly — no suite wrapper reimplementing FEEL's
// own semantics — and the "value modes" documented on [EvaluateScalar],
// [EvaluateList] and [Evaluator.EvaluateBool] say precisely what shape each
// statement kind's expression is expected to produce and what happens when
// it produces something else.
package bind
