---
name: format-pptx
description: PresentationML, a slide deck — the only format with a native speaker-note channel.
kind: pptx
category: format
type: skill
applies_to: [pptx]
examples_tags: [format, pptx]
---

## What it emits
PresentationML: a slide deck. Every block renders, including `block.notes`
— the only format with a native speaker-note channel (`Renders`, not
degraded). `block.page_break` degrades to a new slide.

## Capability notes
- Assets: `asset.media.image/png`/`asset.media.image/jpeg` render, with
  every encoding variant (`asset.png.alpha`, `asset.png.interlaced`,
  `asset.jpeg.progressive`, `asset.jpeg.cmyk`). `asset.media.image/svg+xml`
  degrades to raster-plus-vector, same as DOCX.
- `overflow.continue_repeat_headers` **renders**: a table longer than a
  slide continues onto the next with headers repeated, capacity
  theme-derived rather than measured so the split stays reproducible.
- Fonts: as DOCX — `font.embed.subset`/`font.embed.whole` degrade to a name
  reference. A deck is where a missing face is most visible (large type,
  fixed layout), so the substitution is warned rather than assumed
  harmless.
- Fill: `fill.bind.repeat` gains `RepeatTargetSlide` — N new slide *parts*
  are cloned with copied relationships, `[Content_Types].xml` overrides,
  and fresh `<p:sldId>` values. A zero-item repeat targeting the deck's
  only slide is rejected (`VELLUM_TEMPLATE_SLIDE_REPEAT_EMPTIES_DECK`)
  rather than emptying the deck. See `fill-repeat.md`.

## Gotchas
- `sldId`/`sldMasterId`/`sldLayoutId` occupy disjoint, bounded ranges
  (`sldId` in [256, 2^31); master/layout ids at or above it) — a deck
  mixing them is one PowerPoint repairs on open.
- Shape identifiers start at 2 (identifier 1 belongs to the shape tree's
  own group shape) and are assigned by tree position, so two slides
  carrying the same shapes get the same ids.
- A title placeholder carries no `idx`; every other placeholder does — the
  asymmetry is the schema's, not a bug.
- Every colour a master, layout or slide states is a scheme reference
  (`srgbClr` lives only in `theme1.xml`) — a literal elsewhere cannot be
  restyled and nothing reports it.
- Masters, layouts and the theme part are authored from the theme, never
  shipped as a fixed template — a `.pptx` with none of those is a file
  PowerPoint refuses to open.

## See
- `block-notes.md`, `fill-repeat.md`, `theme-document.md`
