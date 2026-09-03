---
name: fill-repeat
description: The repeat statement and its four per-format RepeatTarget realizations.
kind: repeat
category: fill
type: skill
applies_to: [docx, xlsx, pptx]
examples_tags: [fill, repeat, table]
---

## Semantics
`repeat` produces zero or more copies of `body`, once per item of a FEEL
list expression (`over`), each iteration's scope bound to `as`. `target`
(`bind.RepeatTarget`) says *how* the template's own format realizes that
repetition — declared at authoring time, never inferred by walking the
document, per "declared, not emergent":

- `row` splices N copies of a DOCX table row into the table it belongs to.
- `block` splices N copies of a DOCX native content control's whole
  content.
- `table_row` inserts N rows into an xlsx Excel Table's one sample data row
  and rewrites the table's own `ref`. Restricted to a table whose sample
  row is the sheet's own last content, or
  `VELLUM_TEMPLATE_TABLE_NOT_AT_SHEET_BOTTOM`.
- `slide` clones a whole pptx slide *part* N times — new relationships,
  `[Content_Types].xml` overrides and fresh `<p:sldId>` values, a real OPC
  structure mutation rather than a byte-span splice. A zero-item `slide`
  repeat targeting the deck's only slide is
  `VELLUM_TEMPLATE_SLIDE_REPEAT_EMPTIES_DECK`.

## Example
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

## Gotchas
- `target` has no default — the zero value is invalid
  (`VELLUM_BIND_REPEAT_TARGET_UNKNOWN`). Guessing between `row` and `block`
  from document shape is exactly what this field exists to prevent.
- Every anchor a repeat's body reaches must belong to one single splice
  container (the same table, the same content control, the same slide) —
  a body mixing containers fails reconciliation.
- `table_row` and `slide` are per-format: only xlsx has `table_row`, only
  pptx has `slide`. Naming a target the anchor's own format cannot realize
  is a template/binding mismatch caught at execution, not at `Validate`
  time (which checks vocabulary membership only).

## See
- `fill-binding.md`, `fill-anchors.md`
- `format-docx.md` (`row`/`block`), `format-xlsx.md` (`table_row`),
  `format-pptx.md` (`slide`)
