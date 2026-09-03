# Bindings and FEEL

A `bind.Binding` document says what a fill should do: an ordered list of
top-level `Statement`s, each a tagged union over one of four
`StatementKind`s. Every expression in a binding is
[FEEL](https://kiegroup.github.io/dmn-feel-handbook/) — the Friendly Enough
Expression Language from DMN — evaluated through the `bind.Evaluator` seam
(default: `pbinitiative/feel`, used directly with no wrapper). See
[Seams](../library/seams.md#bindevaluator) for substituting a different
evaluator entirely — a sandboxed one, a metered one, or a different
expression language altogether.

## The four statement kinds

**`bind`** — the leaf. One anchor, one FEEL expression: `{ "anchor":
"title", "expr": "report.title" }`. An optional `format` is an xlsx
number-format code, the identical vocabulary `spec.Cell.Format` uses — one
formatting dialect across the whole library, composed or filled. An
expression evaluating to a FEEL list or context where a scalar was expected
is `VELLUM_BIND_VALUE_NOT_SCALAR` — that shape is a `repeat`, not a `bind`.

**`repeat`** — see below.

**`if`** — selects `then` or `else`, each a nested statement list, by a FEEL
boolean `when`.

**`with`** — narrows scope: binds a name to an evaluated value for its
nested `body`, so a deeply repeated structure does not need to re-spell a
long path at every leaf.

Every statement of every kind, whatever its own `kind`, also carries an
optional `skip`: a FEEL boolean evaluated first. `skip: true` means the
*whole statement* is treated as though it were never in the binding at all —
not an error, not spliced, and not a candidate for anchor-reconciliation
failure. This is a genuinely different concept from a leaf's own `optional:
true`: `skip` says "this data does not call for filling this run at all";
`optional` says "this anchor may legitimately not exist in the template,
whether or not this data calls for filling it."

A statement carrying content for an arm other than the one its own `kind`
names is `VELLUM_BIND_INVALID` — the identical "stray arm" discipline
`spec.Block` enforces for a mismatched block kind.

## `repeat` and its four `RepeatTarget`s

```json
{
  "kind": "repeat",
  "repeat": {
    "over": "report.rows",
    "as": "row",
    "target": "row",
    "body": [ { "kind": "bind", "bind": { "anchor": "name", "expr": "row.name" } } ]
  }
}
```

`repeat` produces zero or more copies of `body`, once per item of a FEEL list
expression (`over`), with each iteration's item bound to the name `as` for
that copy's own scope. `target` (`bind.RepeatTarget`) says **how** the
template's own format should physically realize that repetition —
**declared at authoring time, never inferred by walking the document**,
which is the same "declared, not emergent" discipline the capability matrix
follows for output formats. `target` has no default; the zero value is
rejected (`VELLUM_BIND_REPEAT_TARGET_UNKNOWN`), because guessing between
`row` and `block` from the document's shape is exactly what this field
exists to make unnecessary.

Four targets, three formats:

- **`row`** (DOCX) splices N copies of a table row into the table it
  belongs to.
- **`block`** (DOCX) splices N copies of a native content control's whole
  content.
- **`table_row`** (XLSX) inserts N rows into an Excel Table's one sample
  data row and rewrites the table's own `ref`. Restricted to a table whose
  sample row is the sheet's own last content — inserting anywhere else
  invalidates every absolute reference below it — and a template that fails
  the check is rejected up front with
  `VELLUM_TEMPLATE_TABLE_NOT_AT_SHEET_BOTTOM` before any splicing happens.
- **`slide`** (PPTX) clones a whole slide **part** N times: new
  relationships, `[Content_Types].xml` overrides, and fresh `<p:sldId>`
  values — a real OPC structure mutation, not a byte-span splice the way the
  other three targets are. A zero-item `slide` repeat targeting the deck's
  only slide is rejected (`VELLUM_TEMPLATE_SLIDE_REPEAT_EMPTIES_DECK`)
  rather than silently emptying the deck.

Every anchor a repeat's `body` reaches must belong to one single splice
container — the same table, the same content control, the same slide — a
body mixing containers fails reconciliation. `target` naming a mechanism the
anchor's own format cannot realize (asking for `slide` against a DOCX
anchor, say) is caught at execution against the real inventory, not at
`Validate` time, which checks only that the value is a member of the
`RepeatTarget` vocabulary.

## What `Validate` checks, and what it deliberately does not

`bind.Binding.Validate` checks shape only: that every statement's `kind` is
in the four-member vocabulary, that the matching arm is present, and that
its own required fields are non-empty. It **never parses or evaluates FEEL**
— a malformed expression surfaces later, as `VELLUM_BIND_EXPR_MALFORMED`,
from the FEEL-aware layer built on top of this one, at the point an
expression actually needs evaluating.

## Banned builtins: determinism reaches into FEEL too

`now()` and `today()` are banned — both call `time.Now()` internally inside
`pbinitiative/feel`, and either would silently break byte-identical output
the same way a stray `time.Now()` anywhere else in Vellum would. Using
either is `VELLUM_BIND_NONDETERMINISTIC_EXPR`, caught by walking the parsed
FEEL AST before evaluation ever runs, never by observing what evaluating it
produced. The fix is always the same: put a reference instant in the
caller's own data, as an `as_of` field, and compare against that instead of
asking FEEL for the current time. The banned-builtin registry
(`bind.AllBannedBuiltins()`) has its own completeness gate
(`TestBindBannedBuiltinsComplete`), so a future banned builtin cannot be
added to the check without a corresponding, documented entry.

## Every combination is its own capability-matrix row

`fill.bind.repeat`, `fill.bind.if`, `fill.bind.with` and `fill.bind.skip`
(the `skip` modifier every statement carries) are each declared,
independently, per format — DOCX, XLSX and PPTX all render every one of
them; PDF rejects all four outright, because a PDF carries no OPC structure
to splice into in the first place. See [Capability
matrix](../formats/capabilities.md) and each format's own page.

## See also

- [Templates and anchors](anchors.md) — what a `bind` or `repeat` statement
  actually targets.
- [Non-destructiveness](non-destructive.md) — what happens to everything a
  binding does *not* mention.
- [Seams](../library/seams.md#bindevaluator) — substituting a non-default
  `bind.Evaluator`.
- `skills/fill-binding.md`, `skills/fill-repeat.md` — the same material,
  terse, for an LLM authoring a binding document at runtime.
