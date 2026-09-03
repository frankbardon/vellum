# Tables

`spec.Table` is not a two-dimensional array of strings, and the model that
treats it as one is the model that eventually gets thrown away. An analytical
table — the kind a statistical report, a financial summary or a data
appendix actually needs — carries four properties Vellum treats as first
class:

- **Hierarchical headers with spanning cells on both axes**, because nested
  grouping variables ("2024" over "Q1", "Q2") produce multi-level banners, not
  a single flat row of labels.
- **Cell-level annotations that attach to a value rather than replacing it** —
  a significance letter sitting beside a number, not instead of it.
- **Margins and totals distinguishable from data**, so a renderer can style a
  total row without string-matching on the word "Total" (which breaks the
  moment the document is in another language, or a data row happens to be
  named that).
- **Per-cell marks**, so a consumer can flag one cell without Vellum ever
  learning what the flag means.

Vellum computes none of this. Significance letters, margins and low-base
flags all arrive in the specification already resolved — see CLAUDE.md's
"What NOT to Do": Vellum does not compute anything statistical, and there is
no `compute_totals` option and never will be one.

## Shape

```json
{
  "column_headers": [
    { "label": "2024", "children": [ { "label": "Q1" }, { "label": "Q2" } ] }
  ],
  "row_headers": [ { "label": "Americas" }, { "label": "EMEA" } ],
  "body": [
    [ { "value": { "kind": "number", "number": 812000 }, "format": "#,##0" } ]
  ],
  "caption": "Revenue by region"
}
```

`column_headers` and `row_headers` are each a `HeaderTree` — an ordered forest
of `HeaderNode`s, where a node's `span` is the number of leaf positions it
covers. Leaving `span` at zero means "derive it": one for a leaf, the sum of
the children for a parent. Stating it explicitly is allowed, and is checked
against the children rather than trusted — a banner whose stated span
disagrees with what its children actually tile is `VELLUM_TABLE_HEADER_SPAN_MISMATCH`,
because a mismatched banner renders as a table with a hole in it, and a hole
looks deliberate in a way a refusal does not.

`body` is row-major, `[][]Cell`. A `Cell` carries:

- `value` — a typed `Value` (`number`, `text`, `bool`, or `date` in RFC 3339
  form). Spreadsheet targets prefer this, with `format` applied.
- `text` — the rendered representation, preferred by flowing targets when the
  consumer has already formatted the value or the cell is purely textual.
- `format` — an xlsx number-format code. This is the *one* formatting
  vocabulary shared across every target — see `numfmt/` — so there is no
  second dialect to learn or to drift from the first.
- `annotations` — a list of `Annotation`, each with its own `text` and a
  `position` (`superscript` — the default — `suffix`, `prefix`, or `note`,
  which renders away from the value entirely, as a footnote or a comment
  depending on format).
- `row_span` / `col_span` — counts, so the natural, common value (one) needs
  no field at all; zero means one.
- `class` — `""` (ordinary data, the zero value), `margin`, `total`, or
  `header` (a header rendered inside the body grid, for a stub column that is
  structurally part of it).
- `marks` — consumer-defined style hooks, same discipline as everywhere else.

## Validation: occupancy, not just arithmetic

`Table.Validate` does more than sum spans. It actually places every cell onto
a grid, left to right into the first free column of its row — which is how a
table with row spans genuinely reads: a cell spanning several rows occupies
the position beneath it, and the next row's cells flow around it — and reports
the first collision it finds. That needs a real occupancy walk rather than an
arithmetic check, because a row span reaches into rows that have not been
read yet. The errors this walk can raise:

- `VELLUM_TABLE_SPAN_OVERLAP` — a cell extends past the table's own right
  edge, past its last row, or two cells claim the same grid position.
- `VELLUM_TABLE_ROW_ARITY` — after placement, a row does not cover exactly
  the table's declared width.
- `VELLUM_TABLE_HEADER_SPAN_MISMATCH` — a header node's stated `span`
  disagrees with the sum of its children's.

A table with no column headers at all is legal — its width is derived from
the widest row instead, so a caption-only or header-less table is still
expressible.

## Annotations in practice

The motivating case for `annotations` is a significance letter or a
low-base-size marker sitting beside a number without replacing it, and how it
renders depends heavily on the target: DOCX, PPTX and PDF all support the
positions natively as inline or footnote-style markup, but XLSX has no
concept of a value carrying extra decoration — a cell holds one typed value.
`table.cell_annotation` therefore degrades in XLSX specifically: the
annotation becomes a whole extra neighbouring column, added uniformly across
every data column and only when the table carries at least one annotation
anywhere in it — never on a per-cell basis, which would double some columns
and not others depending on which row happened to carry a flag. See
[XLSX](../formats/xlsx.md).

## What each format actually does with this structure

Every format renders `block.table`, `table.hierarchical_headers`,
`table.margins` and `table.cell_span` — but the mechanics differ sharply
enough to be worth reading per format:

- **PDF** draws in three full passes over the table — every cell's fill,
  then every cell's hairline, then every cell's text — never one pass per
  cell, because PDF paints strictly in operator order and interleaving would
  let a later cell's rule cover an earlier cell's fill. Its row capacity is
  *measured* rather than theme-derived, the one place PDF departs from the
  rule the rest of the emitter follows, because Vellum lays PDF out
  completely itself.
- **PPTX** writes explicit cell insets and an exact line box on every
  paragraph, including an empty cell's — an empty paragraph left unstated
  takes DrawingML's 18pt default paragraph-mark size, silently doubling every
  empty row's height in a crosstab that is mostly merged corners and
  continued stub cells.
- **PPTX**'s spanning cells keep every covered cell present, marked
  `hMerge`/`vMerge`, the opposite of WordprocessingML's `gridSpan`, which
  *replaces* the cells it covers. The two OOXML dialects do not share a merge
  convention, and code that assumes one breaks silently against the other.
- `overflow.continue_repeat_headers` — a table too long for one page, slide
  or sheet continuing onto the next with its header band repeated — degrades
  to plain flow in DOCX (Word paginates itself; a split Vellum computed would
  disagree with the one Word performs) and to one continuous sheet in XLSX
  (a sheet has no page at all); it fully **renders** in PPTX and PDF, both of
  which Vellum paginates itself.

See [Capability matrix](../formats/capabilities.md) for the complete outcome
table, and `skills/block-table.md` for the same material written for an LLM
composing a table at runtime.
