---
name: format-xlsx
description: SpreadsheetML, a workbook of presentation tables — not a spreadsheet.
kind: xlsx
category: format
type: skill
applies_to: [xlsx]
examples_tags: [format, xlsx]
---

## What it emits
SpreadsheetML: a workbook of presentation tables, not a spreadsheet — no
formulas, no pivot tables. `block.table` is the only block that renders
natively; every other block degrades or rejects.

## Capability notes
- `block.heading` -> a styled cell above the table; `block.text` -> a
  wrapped cell; `block.page_break` -> a new sheet; `block.notes` -> a cell
  comment; `block.spacer` -> a blank row. All `VELLUM_CAPABILITY_DEGRADED`.
- `block.asset` **rejects** (`VELLUM_CAPABILITY_REJECTED`) — a workbook
  carries no assets at all, and every `asset.*` row (including the
  encoding-variant rows like `asset.png.alpha`) rejects for the same
  reason, declared as rows so the answer is a lookup, not an inference from
  a missing cell.
- `table.cell_annotation` degrades: text appended to the cell, with the
  typed value preserved in a neighbouring column — added only when the
  table carries at least one annotation, uniformly across every data
  column.
- `overflow.continue_repeat_headers` degrades to one continuous sheet — a
  sheet has no page.
- Fill: `fill.bind.repeat` uses `RepeatTargetTableRow` against a
  `table_column` or `defined_name` anchor. Restricted to a table whose
  sample row is the sheet's own last content — row insertion invalidates
  every absolute reference below it — a template failing that check is
  rejected with `VELLUM_TEMPLATE_TABLE_NOT_AT_SHEET_BOTTOM` before any
  splicing happens. See `fill-anchors.md`.

## Gotchas
- `xl/styles.xml`'s fixed preamble keeps builtin fill indices 0 and 1
  reserved — getting this wrong makes Excel refuse to open the file.
- A legacy cell comment is two parts, not one: `xl/comments1.xml` plus
  `xl/drawings/vmlDrawing1.vml`, referenced by the worksheet's own
  `<legacyDrawing r:id="…"/>`. Threaded comments are never written.
- A row-header stub spanning several rows is anchored to the *top* of its
  merge, explicitly — Excel's own default is bottom, which hides a group
  label until the reader scrolls past every row it names.
- The table's own `ref` attribute is rewritten on every fill, including a
  zero-item repeat, which shrinks it back to the header row alone.

## See
- `block-table.md`, `fill-anchors.md`, `fill-repeat.md`
