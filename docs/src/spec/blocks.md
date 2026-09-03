# The block model

A [`spec.Spec`](https://pkg.go.dev/github.com/frankbardon/vellum/spec#Spec) is
a title, a theme reference, and an ordered list of `Section`s, each an ordered
list of `Block`s. That is the whole shape a caller — human or model — authors
against. Everything else in Vellum exists to turn this into bytes.

```json
{
  "format_version": "1.0",
  "title": "…",
  "theme": "",
  "sections": [
    { "id": "…", "layout": "", "blocks": [ /* Block, Block, … */ ] }
  ]
}
```

`theme` empty selects the built-in theme (`spec.DefaultThemeID`); `layout`
empty on a section selects that theme's default master layout for the target
format. See [Themes](themes.md) for what a layout is.

## Why blocks, not sections with meaning

Vellum ships no semantic vocabulary. There is no `"cover"` block, no
`"executive summary"` section type, no built-in idea of what a report is made
of. That vocabulary belongs entirely to whoever is composing the document —
which might be a human author, a template designer, or a language model
generating a specification from a prompt. A library that shipped one product's
section types would make the next consumer fight it, and a *closed* set of
seven generic kinds is what keeps every consumer's document expressible
without ever needing a new kind added on their behalf. Compose "the cover
page" out of a `heading`, an `asset` and a `spacer`; Vellum never learns that
is what you did.

## Marks: the one escape hatch

Nearly every block, table cell, header node and annotation carries an optional
`marks: []string` field — consumer-defined style hooks. A block's mark is a
name a theme's own `Marks` table maps to a style (bold, a colour role, an
underline); Vellum resolves the mapping but never learns what the name
*means*. The motivating case in CLAUDE.md is a document whose underlying data
has moved and must be visibly flagged as stale — Vellum renders the flag
without ever knowing what "stale" is. See [Themes](themes.md#mark-styles).

## The seven kinds

Each of the seven `spec.BlockKind` values is documented at length, with its
capability-matrix outcome per format, in the skill pack under
`skills/block-<kind>.md` — that material is written for an LLM composing a
spec at runtime and is worth reading directly rather than duplicated here. In
prose, connected rather than flat:

**`heading`** (`spec.Heading`) is a titled division: an integer `level`
(1 is most prominent, and there is no upper bound — a theme's own type scale
clamps to its deepest declared size for anything beyond it) and `content`
text. It renders natively everywhere except XLSX, where a sheet has no
heading construct and it becomes a styled cell above the table instead.

**`text`** (`spec.Text`) is a paragraph of prose: `content` only, with no
inline run-level styling on the block itself — bold and italic are theme-mark
concerns, not fields here. It renders natively in DOCX, PPTX and PDF; XLSX
wraps it into a cell. PDF is the one format that lays itself out, and it only
promises to shape and break Latin, Greek and Cyrillic text correctly — a
script outside that set, or two of those scripts mixed in one string, is a
rejection rather than a silent mis-render, because Vellum's own line breaker
has not established it handles the mix. The three OOXML formats impose no
such restriction, because Word, Excel and PowerPoint do their own shaping.

**`asset`** (`spec.Asset`) embeds an asset the host resolves: a `handle`
Vellum does not interpret, an optional `role` naming a theme box (empty
selects `asset.full`, the full content width), and `alt_text`. It renders in
DOCX, PPTX and PDF and is flatly rejected in XLSX — a workbook is where a
reader goes for the numbers behind a chart, not the chart itself, and that is
declared as a matrix row rather than discovered when a render silently drops
a picture. See [Seams](../library/seams.md#assetresolver) for how a handle
becomes bytes, and [Themes](themes.md#layouts-pages-and-box-roles) for what a role selects.

**`table`** (`spec.Table`) is the block with the most structure — see
[Tables](tables.md) for its own page. It renders in every format.

**`page_break`** (`spec.PageBreak`) starts a new page, slide or sheet
depending on format. It carries no fields today — a struct rather than a bare
kind, so a break *type* could be added later without reshaping every other
block — but the empty `page_break: {}` arm is still required; a block naming
the kind with no arm present is `VELLUM_SPEC_INVALID`. It renders natively in
DOCX and PDF; XLSX degrades it to a literal new worksheet (there is no
print-layout concept in this model at all) and PPTX degrades it to a new
slide.

**`notes`** (`spec.Notes`) is speaker-note or annotation content: `content`
only. PPTX is the only format with a real speaker-note channel and renders it
natively; DOCX and PDF both degrade it to a footnote anchored at the block's
own position, and XLSX degrades it to a legacy cell comment (paired with its
own VML drawing part — see [XLSX](../formats/xlsx.md)).

**`spacer`** (`spec.Spacer`) is vertical space: a `Length` (value plus unit —
`pt`, `mm`, `in` or `emu` — converted once to EMU at validation, rounding half
away from zero). It renders in DOCX, PPTX and PDF; XLSX degrades it to exactly
one blank row regardless of the requested height, because a sheet has no
arbitrary vertical measure.

## Strict decoding

`spec.Decode` (and its YAML-detecting counterpart, `spec.DecodeAuto`) rejects
an unrecognized field rather than ignoring it. There is deliberately no
"lenient" or "best effort" mode, not even behind a flag: an LLM authoring a
specification gets no signal at all that part of its output was silently
dropped if decoding is forgiving, and silent partial acceptance is worse than
a loud rejection a caller can act on. `spec.Validate` runs after decoding and
checks shape only — that every section has blocks, that a block's kind is in
the seven-member vocabulary, that the arm matching its kind is present and
that no *other* arm is also set (a block whose `kind` is `"text"` but which
also carries a populated `heading` field is rejected as a "stray arm", not
silently resolved by picking one). Whether a given kind can actually be
*rendered* in a given target format is a separate question entirely, answered
by the capability matrix — see [Capability matrix](../formats/capabilities.md).

## See also

- [Tables](tables.md) — the one block with enough internal structure to earn
  its own page.
- [Themes](themes.md) — what a mark, a box role and a layout are.
- [Capability matrix](../formats/capabilities.md) — what each block kind
  becomes in each of the four formats.
- `skills/block-*.md` in the repository — the same vocabulary, written
  tersely for an LLM composing a specification at runtime.
