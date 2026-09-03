---
name: format-docx
description: WordprocessingML, a flowing document — the richest target for prose and fill mode's richest template.
kind: docx
category: format
type: skill
applies_to: [docx]
examples_tags: [format, docx]
---

## What it emits
WordprocessingML: a flowing document. Every block kind renders (`block.*`:
`Renders`) except `block.notes`, which degrades to a footnote anchored where
the block sits (`VELLUM_CAPABILITY_DEGRADED`). `overflow.continue_repeat_headers`
also degrades, to plain flow — Word paginates itself, and a split Vellum
computed would disagree with the one Word performs.

## Capability notes
- Assets: `asset.media.image/png` and `asset.media.image/jpeg` render, with
  every encoding variant (`asset.png.alpha`, `asset.png.interlaced`,
  `asset.jpeg.progressive`, `asset.jpeg.cmyk`) passing through untouched.
  `asset.media.image/svg+xml` degrades to a raster fallback with the vector
  embedded alongside — Word 2016+ only reads an SVG when a raster blip
  accompanies it, and Vellum never rasterises, so the caller supplies the
  pair.
- Fonts: `font.embed.subset` and `font.embed.whole` both degrade to "the
  family referenced by name" — DOCX authors no font-embedding settings in
  v1. `font.embed.none` renders, the common case. `font.outlines.cff`
  doesn't change the answer: no font program is ever loaded here.
- Text: `text.bold` and `text.italic` render (the application selects or
  synthesises the cut). Every script renders — Vellum writes characters and
  Word shapes and breaks them, so `text.script.other` carries no
  restriction here the way it does in PDF.
- Fill: `fill` and every `fill.bind.repeat`/`fill.bind.if`/`fill.bind.with`/
  `fill.bind.skip` mode renders. DOCX is fill mode's richest target, with
  both `RepeatTargetRow` (table-row splicing) and `RepeatTargetBlock`
  (native content-control splicing) available — see `fill-repeat.md`.

## Gotchas
- Font embedding is genuinely absent here: a theme demanding `embed:
  "whole"` still only gets a name reference in DOCX. PDF is the only format
  that can honour an embed demand.
- A `block.notes` block always becomes a footnote here, never a margin
  comment (xlsx) or a speaker note (pptx).
- Fill mode: every part outside `Result.Touched` stays byte-identical. The
  non-destructiveness fixture carries tracked changes, a comment, a custom
  XML part, footnotes and an embedded OLE object, none of which fill mode
  disturbs.

## See
- `block-notes.md`, `fill-binding.md`, `fill-repeat.md`, `theme-document.md`
