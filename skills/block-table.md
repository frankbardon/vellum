---
name: block-table
description: An analytical table with hierarchical headers, spans, annotations and marginal rows.
kind: table
category: block
type: skill
applies_to: [docx, xlsx, pptx, pdf]
examples_tags: [block, table]
---

## What it is
An analytical table — not a 2-D array of strings. `spec.Table` carries a
hierarchical `column_headers`/`row_headers` tree (`HeaderTree`, spanning
cells via `Span`), a row-major `Body [][]Cell` and a `Caption`. A `Cell`
carries a typed `Value` (`number`/`text`/`bool`/`date`), a display `Text`, an
xlsx-style `Format` code, `Annotations` (`superscript`/`suffix`/`prefix`/
`note`), `RowSpan`/`ColSpan`, and a `Class` (`""`/`margin`/`total`/
`header`). Renders in every format (`block.table`).

## JSON shape
```json
{
  "kind": "table",
  "table": {
    "column_headers": [{ "label": "2024", "children": [{"label":"Q1"},{"label":"Q2"}] }],
    "body": [[{ "value": { "kind": "number", "number": 42 }, "format": "0" }]]
  }
}
```

## Gotchas
- `table.hierarchical_headers`, `table.margins` and `table.cell_span` render
  in every format.
- `table.cell_annotation` degrades in XLSX only: a cell holds one typed
  value, so an annotation becomes a whole extra neighbouring column, added
  uniformly across every data column and only when at least one annotation
  exists anywhere in the table (`VELLUM_CAPABILITY_DEGRADED`).
- A PDF table draws in three passes over the whole table — every fill, then
  every hairline, then every cell's text — never one pass per cell.
- A PDF table's row capacity is *measured*, the one place PDF departs from
  the "theme-derived, not measured" rule, because Vellum lays PDF out
  itself.
- A PPTX row writes explicit cell insets and an exact `a:lnSpc` line box; an
  empty cell's paragraph mark still carries the table's own type size, or an
  unstated mark's 18pt default silently doubles every empty row's height.
- A DrawingML (PPTX) spanning cell keeps every covered cell present, marked
  `hMerge`/`vMerge`; WordprocessingML's `gridSpan` instead *replaces* the
  cells it covers. The two formats do not share a merge convention.
- `overflow.continue_repeat_headers` degrades to plain flow in DOCX (Word
  paginates itself) and to one continuous sheet in XLSX; it renders — a
  measured split with repeated headers — in PPTX and PDF.

## See
- `format-docx.md`, `format-xlsx.md`, `format-pptx.md`, `format-pdf.md`
- `fill-anchors.md` — the `table_column` anchor kind, xlsx-specific
