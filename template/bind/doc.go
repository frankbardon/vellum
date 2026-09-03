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
// This package is model, decode and hash only. It does not evaluate FEEL —
// every expr, when, value and over field is stored as an opaque string — and
// it does not execute control flow: repeat does not iterate, if does not
// branch, with does not narrow a scope. Those are later stories layered on
// top of the tree this package builds and validates structurally. What this
// package guarantees is that the tree it hands to those later stories is
// well-formed: every statement's kind matches the one arm it carries, every
// required field is present, and every FEEL-bearing string is reachable
// through [Walk] and [WalkExprs] — which is what lets a later validator walk
// the whole tree once, in one place, to reject a nondeterministic builtin
// without this package having to know what FEEL is.
package bind
