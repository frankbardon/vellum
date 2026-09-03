---
name: fill-binding
description: The binding document model — statements, scope, Skip and OptionalAnchors.
kind: binding
category: fill
type: skill
applies_to: [docx, xlsx, pptx]
examples_tags: [fill, bind, feel]
---

## Semantics
A `bind.Binding` is an ordered list of top-level `Statement`s, each a tagged
union over four `StatementKind`s: `bind` (the leaf — one anchor, one FEEL
expression), `repeat` (see `fill-repeat.md`), `if` (selects `then`/`else`
by a FEEL boolean) and `with` (narrows scope, binding a name to an
evaluated value for its nested body). Every statement of every kind also
carries an optional `skip`: a FEEL boolean evaluated first — true means the
whole statement is treated as absent, not an error, not spliced, and not a
candidate for anchor-reconciliation failure. `skip` says "this data does
not call for filling this run"; a leaf's own `optional: true` says
something different — "this anchor may legitimately not exist in the
template at all."

A `bind` statement's `expr` is opaque FEEL, evaluated against the caller's
data and the current scope; its optional `format` is an xlsx number-format
code, the same vocabulary `spec.Cell.Format` uses.
`Binding.OptionalAnchors` names template anchors the statement tree never
references at all and deliberately leaves unbound — the template-side half
of "error on a mismatch unless explicitly marked optional." `Bind.Optional`
is the statement-side half, for an anchor the tree *does* name.

Each combination of statement kind and target format is its own
capability-matrix row: `fill.bind.repeat`, `fill.bind.if`, `fill.bind.with`
and `fill.bind.skip` (the `skip` modifier every statement carries). DOCX,
XLSX and PPTX render all four; PDF rejects all four — see the relevant
`format-*.md`.

## Example
```json
{
  "format_version": "1.0",
  "statements": [
    { "kind": "bind", "bind": { "anchor": "title", "expr": "report.title" } },
    { "kind": "if", "if": { "when": "report.flagged", "then": [
      { "kind": "bind", "bind": { "anchor": "flag_note", "expr": "\"stale\"" } }
    ] } }
  ]
}
```

## Gotchas
- `Validate` checks shape only (kind in vocabulary, the matching arm
  present, its own required fields non-empty) — it never parses or
  evaluates FEEL. A malformed expression is `VELLUM_BIND_EXPR_MALFORMED`,
  caught by the FEEL-aware layer built on top of this one.
- `now()` and `today()` are banned builtins: both call `time.Now()`
  internally and would break byte-identical output. Put a reference instant
  in the data as an `as_of` field and compare against that instead. Using
  either is `VELLUM_BIND_NONDETERMINISTIC_EXPR`, caught by an AST walk,
  never by evaluating.
- A statement carrying content for an arm other than its own `kind` is
  `VELLUM_BIND_INVALID`, the same "stray arm" discipline `spec.Block`
  enforces.
- A `bind` expression evaluating to a FEEL list or context where a scalar
  was wanted is `VELLUM_BIND_VALUE_NOT_SCALAR` — that shape is a `repeat`,
  not a `bind`.

## See
- `fill-repeat.md`, `fill-anchors.md`
- `format-docx.md`, `format-xlsx.md`, `format-pptx.md`
