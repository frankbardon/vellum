---
name: theme-document
description: The theme document — font, colour and box roles, master layouts, and the Boxes() query.
kind: document
category: theme
type: skill
applies_to: [docx, xlsx, pptx, pdf]
examples_tags: [theme, fonts, colors, boxes, layout]
---

## Font roles
Three typographic slots, `theme.FontRole`: `body` (running prose),
`heading` (title and heading text) and `mono` (fixed-width text). Each
`theme.Font` names a `Family`, carries an `EmbedMode` (`""`/auto, `subset`,
`whole`) stating a licence condition on *how* the program may be embedded,
and a handle the asset resolver serves the program bytes from.
`font.embed.none` — a face with no embed mode set, referenced by name only
— is the default every OOXML format gets in v1. PDF/A-2b requires every
font embedded, so a theme with no embeddable face (the built-in theme)
cannot compose to PDF at all (`VELLUM_FONT_EMBED_UNSUPPORTED`).

## Color roles
Ten semantic slots, `theme.ColorRole`, and a fragment carries the theme's
*whole* palette rather than only the colours the content happened to use —
a document with no body paragraph still needs an answer for `background`.
The roles: `background`, `text`, `text_muted`, `heading`, `accent`,
`accent_text`, `rule`, `table_header_background`, `table_header_text`,
`table_stripe`. A block or cell's `marks` field maps to a colour (and a
font, a weight) through the theme's own mark-style table — never through a
value Vellum interprets itself.

## Box roles
Four asset slots, `theme.BoxRole`: `asset.full` (spans the content width —
the default an asset block with no `role` selects), `asset.half` (a two-up
row), `asset.quarter`, and `logo` (the brand slot in a header, footer or
master). Each `theme.Box` states a `Width` and, for a fixed-geometry format
(PPTX), a `Height` too; a zero `Height` means "follow the asset's own
aspect ratio," the normal answer in a flowing format (DOCX, PDF) where only
the column width is fixed. `Theme.Boxes(format)` answers this before any
specification exists — see `tool-boxes.md`.

## Gotchas
- A theme carries one or more `Layout` per format, each with its own `Page`
  geometry (width, height, four margins) — a document's landscape section
  is a different layout, not a flag on one.
- Exactly one layout per format must be marked `Default`. A section naming
  no layout gets that one; a named layout the theme does not declare for
  the target format is `VELLUM_THEME_LAYOUT_NOT_FOUND`, not a silent
  fallback.
- An asset block's `role` naming a box the resolved layout does not declare
  is `VELLUM_THEME_BOX_NOT_FOUND`.
- In PPTX, every colour a master, layout or slide states must be a scheme
  reference (`theme1.xml`'s own slots), never a literal `srgbClr` — a
  literal elsewhere cannot be restyled and nothing reports it.
- Font embedding is a licence condition, not a hint: `embed: "subset"` on a
  CFF-outline face in PDF is `VELLUM_FONT_EMBED_UNSUPPORTED` (CFF cannot be
  subset), while an *unspecified* mode on the same face degrades quietly
  to whole-program embedding.

## See
- `block-asset.md`, `block-heading.md`
- `format-pdf.md` — the one format where `font.embed.none` is fatal
- `tool-boxes.md`
