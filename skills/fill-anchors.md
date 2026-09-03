---
name: fill-anchors
description: The five anchor kinds a template's own format offers, and what Discover reports for each.
kind: anchors
category: fill
type: skill
applies_to: [docx, xlsx, pptx]
examples_tags: [fill, anchors, discover]
---

## Semantics
`anchor.Kind` names the mechanism a template format offers for a fill site:

- `native` — a DOCX `w:sdt` content control, named by its `w:tag`.
- `marker` — `{{name}}` text inside a paragraph's runs, possibly fragmented
  across several by Word's own editing. Defragmentation happens later; this
  package only locates the enclosing paragraph.
- `defined_name` — an xlsx workbook-scoped defined name resolving to one
  absolute cell.
- `table_column` — one column of an xlsx Excel Table, named
  `"<displayName>.<columnName>"`, only ever filled from inside a
  `table_row` repeat.
- `shape` — a pptx `<p:sp>` identified by its own `<p:cNvPr name="...">`,
  its `descr` carried separately as `Alias`, never as a name fallback.

Discovery is read-only: `anchor.Discover` locates and reports, never edits
a part.

## Example
```json
{ "anchors": [
  { "name": "customer_name", "kind": "native", "part": "/word/document.xml",
    "span": { "start": 120, "end": 240 } },
  { "name": "SalesTable.Region", "kind": "table_column",
    "part": "/xl/worksheets/sheet1.xml",
    "table": { "display_name": "SalesTable", "column": 2,
               "table_part": "/xl/tables/table1.xml", "header_row": 1,
               "from_column": 1, "to_column": 4 } }
] }
```

## Gotchas
- A `marker` anchor's `Span` is the whole enclosing `w:p`, not a run or
  byte offset — Word can fragment `{{name}}` across runs, and resolving
  that needs the flatten/match/position-map defrag pass, not this package.
- A `shape` anchor's `Name` is always the shape's own name attribute.
  PowerPoint auto-names every shape, so a real deck's discovered inventory
  is typically larger than what a binding actually references — narrowing
  it down is `Binding.OptionalAnchors`' job, not discovery's. A shape with
  a genuinely empty name is simply not discovered, not an error.
- Only a `<p:sp>` shape's own text frame is discovered — a picture, a table
  (graphicFrame) or a shape nested in a group is out of scope and silently
  absent from the inventory.
- `table_column`'s `Span` is the whole shared sample-data `<row>` for every
  column in that table — `Table.Column` says which cell within it is this
  column's own.
- `vellum_fill`'s non-destructiveness guarantee (`Result.Touched`) is
  checked part-for-part with byte equality against real per-format
  fixtures: tracked changes, a comment, custom XML, footnotes and an OLE
  object for DOCX; a second untouched worksheet and a legacy comment paired
  with its own VML drawing for xlsx; a second untouched slide with its own
  speaker notes and embedded media for pptx.

## See
- `fill-binding.md`, `fill-repeat.md`
- `tool-inspect.md`, `tool-fill.md`
