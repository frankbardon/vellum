# XLSX

SpreadsheetML — a workbook of presentation tables, deliberately **not** a
spreadsheet. There are no formulas, no pivot tables, no macros, no array
formulas and no external references anywhere in what Vellum writes; a
consumer wanting a live, editable model builds that elsewhere. `block.table`
is the only block kind that renders natively; every other kind either
degrades to something a worksheet can express, or is rejected outright.

## What degrades, and what is rejected

- `heading` becomes a styled cell above the table.
- `text` becomes a wrapped cell.
- `page_break` becomes a literal new worksheet — not a print-area page break;
  there is no print-layout concept in this model at all.
- `notes` becomes a legacy cell comment.
- `spacer` becomes exactly one blank row, whatever height was requested.
- `asset` **rejects** outright (`VELLUM_CAPABILITY_REJECTED`), and so does
  every `asset.*` row including the encoding-variant ones like
  `asset.png.alpha` — a workbook is where a reader goes for the numbers
  behind a chart, not the chart, and every one of those rows is declared so
  the answer is a lookup rather than an inference from a missing cell.

## Table annotations become a column

`table.cell_annotation` is the one table feature that degrades rather than
renders here, because a cell holds exactly one typed value and appending
annotation text to a number would silently turn it into a string — defeating
the entire reason to export a workbook in the first place. The degradation
instead adds a whole extra neighbouring column, and only when the table
carries at least one annotation anywhere in it; an unannotated table gets no
extra columns, and the doubling is applied uniformly across every data column
rather than per cell, so the doubled structure looks the same shape on every
table that has one at all. See [Tables](../spec/tables.md#annotations-in-practice).

## Overflow

`overflow.continue_repeat_headers` degrades to one continuous sheet — a
sheet, unlike a page or a slide, has no boundary to continue across.

## Fill mode

`fill.bind.repeat` uses `RepeatTargetTableRow` against a `table_column` or
`defined_name` anchor, inserting rows into an Excel Table's one sample data
row and rewriting the table's own `ref`. This is restricted to a table whose
sample row is the sheet's own last content — inserting rows anywhere else
would invalidate every absolute cell reference below it — and a template
that fails that check is rejected with
`VELLUM_TEMPLATE_TABLE_NOT_AT_SHEET_BOTTOM` before any splicing happens at
all. The table's own `ref` attribute is rewritten on every fill, including a
zero-item repeat, which correctly shrinks it back to the header row alone.
See [Bindings and FEEL](../fill/bindings.md) and [Templates and
anchors](../fill/anchors.md).

## Byte-layout notes worth knowing

- `xl/styles.xml` keeps a fixed preamble intact, with builtin fill indices 0
  and 1 reserved by the OOXML spec itself — getting this wrong makes Excel
  simply refuse to open the file, with no more specific diagnosis than "this
  file is corrupt."
- A legacy cell comment is genuinely **two** parts, not one:
  `xl/comments1.xml` states the text, and `xl/drawings/vmlDrawing1.vml`
  states the shape that draws it, referenced by the worksheet's own
  `<legacyDrawing r:id="…"/>`. A comments part with no paired VML drawing is
  a comment Excel shows the indicator triangle for and nothing behind it.
  The newer threaded-comment schema needs an author-identity part this
  library has no source for and is never written; the legacy form is the one
  every reader back to Excel 2007 draws correctly.
- A row-header stub spanning several rows is anchored to the **top** of its
  merge explicitly, via `vertical="top"` — Excel's own default is bottom,
  which leaves a ten-row group label invisible until a reader has already
  scrolled past every row it names.

## See also

- [Tables](../spec/tables.md) — the model this format's one native block
  kind draws its structure from.
- [Templates and anchors](../fill/anchors.md) — `table_column` and
  `defined_name` anchor kinds.
- `skills/format-xlsx.md` — the same material, terse, for an LLM composing
  against this target.
