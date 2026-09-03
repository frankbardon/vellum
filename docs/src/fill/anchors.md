# Templates and anchors

Fill mode binds data into an existing OOXML document — a real template
someone designed in Word, Excel or PowerPoint — rather than composing a new
one from a `spec.Spec`. `template.Open` reads the package; `template.Inspect`
(or `vellum_inspect` / `vellum inspect`) walks it read-only and reports every
place a binding could target: its **anchor inventory**. Discovery never edits
a part — it locates and reports, and every structural read goes through
`xmlcopy.Walk` rather than `encoding/xml`, because re-parsing and
re-marshalling a source part is precisely what would lose the formatting,
namespace prefixes and structure a fill is required to leave alone. See
[Non-destructiveness](non-destructive.md).

## The five anchor kinds

`anchor.Kind` names the mechanism a template's own format offers for a fill
site. Which kinds a template can carry depends entirely on its format — DOCX
offers the first two, XLSX the middle two, PPTX the last.

**`native`** — a DOCX `w:sdt` content control, named by its own `w:tag`
attribute. `Anchor.Span` is the whole `w:sdt` element, open tag through close
tag; a template author explicitly inserted this control (Word's Developer
tab), so its name is a deliberate binding key rather than something
PowerPoint or Word assigned by default.

**`marker`** — `{{name}}` text found inside a paragraph's own runs, possibly
fragmented across several of them by Word's own prior editing (a
spell-checker splitting a word mid-marker is the common real-world case).
`Anchor.Span` is deliberately the **whole enclosing paragraph**, not a run or
a byte offset, because resolving a fragmented marker down to its exact run
and position needs the flatten/match/position-map algorithm defragmentation
implements — discovery only establishes that a marker exists and which
paragraph it lives in.

**`defined_name`** — an xlsx workbook-scoped defined name resolving to
exactly one absolute cell on one sheet. `Anchor.Name` is the defined name's
own name, unchanged; `Anchor.Span` is the whole `<c>` cell element the
name's formula resolves to.

**`table_column`** — one column of an xlsx Excel Table, named
`"<displayName>.<columnName>"` (for example `"SalesTable.Region"`) — chosen
because it is what a binding author would write unprompted: the table names
the group, the column names the field, and the `.` reads the same way a
dotted FEEL path does everywhere else. It is only ever filled from inside a
`repeat` statement whose `target` is `table_row` — `Anchor.Span` is the whole
shared sample-data `<row>` for *every* column of that table, and
`Anchor.Table.Column` says which cell within it is this column's own.

**`shape`** — a pptx `<p:sp>` identified by its own `<p:cNvPr name="...">`
attribute, never by its alt text (`descr`, carried separately as
`Anchor.Alias`). Only a shape's own top-level text frame is discovered — a
picture, a table (`graphicFrame`), or a shape nested inside a group is out of
scope and simply absent from the inventory, not an error. Because PowerPoint
assigns every shape a non-empty default name whether or not a template
author ever renamed it (`"TextBox 3"`, `"Content Placeholder 2"`), a real
deck's discovered inventory for this kind is typically much larger than what
a binding actually references — narrowing it down is what
`Binding.OptionalAnchors` is for, not discovery's job.

## Reading an inventory

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

`Inventory.Anchors` is in document order — for a single part, ascending by
`Span.Start`; two markers sharing a paragraph carry the identical span and
sort adjacently, in the order they were found scanning the paragraph's text
left to right.

## Getting from here to a filled document

Inspection answers "what can I fill." Actually filling something needs a
**binding document** — see [Bindings and FEEL](bindings.md) — that names
these same anchors and supplies FEEL expressions and, for a `repeat`, a
`RepeatTarget` saying how the template's own format should realize
repetition over them. `Vellum.Inspect` and `Vellum.Fill` are both facade
methods over `template.Open`; see [Public facade](../library/facade.md).

## See also

- [Bindings and FEEL](bindings.md) — the statement model that actually
  targets these anchors.
- [Non-destructiveness](non-destructive.md) — the guarantee discovery and
  fill together make possible.
- `skills/fill-anchors.md` — the same vocabulary, terse, for an LLM
  inspecting a template at runtime.
