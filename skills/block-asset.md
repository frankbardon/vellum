---
name: block-asset
description: Embeds an asset the host resolves; Vellum never renders one.
kind: asset
category: block
type: skill
applies_to: [docx, pptx, pdf]
examples_tags: [block, asset, image]
---

## What it is
Embeds an asset the host resolves. `spec.Asset` carries `Handle` (opaque,
host-interpreted), `Role` (a theme box; empty selects the default,
`asset.full`) and `AltText`. Renders in DOCX, PPTX and PDF; **rejected**
outright in XLSX (`block.asset`: `Rejects`, `VELLUM_CAPABILITY_REJECTED`) —
a workbook is where a reader goes for the numbers behind a chart, not the
chart.

## JSON shape
```json
{
  "kind": "asset",
  "asset": { "handle": "logo.png", "role": "asset.half", "alt_text": "Company logo" }
}
```
An empty `handle` is `VELLUM_SPEC_INVALID`.

## Gotchas
- Vellum never rasterises and never fetches bytes itself — the
  `asset.Resolver` seam does, keyed by `handle`.
- Accepted media (`asset.media.image/png`, `asset.media.image/jpeg`) render
  in DOCX/PPTX/PDF. `asset.media.image/svg+xml` degrades to a raster
  fallback with the vector embedded alongside it in DOCX/PPTX, and is
  **rejected** in PDF (`VELLUM_ASSET_MEDIA_UNSUPPORTED` — PDF has no SVG
  mechanism). XLSX rejects every one of these rows: it takes no assets at
  all, so an encoding-variant row like `asset.png.alpha` is a lookup answer,
  not an inference from a missing cell.
- PDF additionally rejects an interlaced PNG, a progressive JPEG and a CMYK
  JPEG — see `format-pdf.md`.
- `alt_text` degrades to "dropped" in PDF (`asset.alt_text`) — PDF/A-2b
  needs a structure tree Vellum does not write.
- `role` picks a theme box (`theme.BoxRole`) — see `theme-document.md`; a
  role the resolved layout does not declare is `VELLUM_THEME_BOX_NOT_FOUND`.

## See
- `format-docx.md`, `format-pptx.md`, `format-pdf.md`
- `theme-document.md`, `tool-boxes.md`
