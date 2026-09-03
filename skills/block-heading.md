---
name: block-heading
description: A titled division of content.
kind: heading
category: block
type: skill
applies_to: [docx, xlsx, pptx, pdf]
examples_tags: [block, heading]
---

## What it is
A titled division of content. `spec.BlockHeading` carries `Level` (int, >= 1,
1 is most prominent) and `Content` (string). Renders in every format
(`block.heading`) except XLSX, which degrades it to a styled cell above the
table (`VELLUM_CAPABILITY_DEGRADED`) — a sheet has no heading construct.

## JSON shape
```json
{
  "kind": "heading",
  "heading": { "level": 1, "content": "Overview" },
  "marks": ["stale"]
}
```
`level < 1` is rejected (`VELLUM_SPEC_INVALID`). `marks` are consumer-defined
style hooks Vellum never interprets — see `theme-document.md` for how a mark
maps to a style.

## Gotchas
- XLSX's degraded form is a plain cell, not a merged banner — do not expect
  a workbook heading to visually span columns.
- `level` is not clamped to the theme's own heading depth; a level 6 heading
  against a theme with two heading styles reuses the deepest one.
- A block must carry exactly one arm matching its `kind`. Setting `heading`
  on a block whose `kind` is `"text"` is `VELLUM_SPEC_INVALID` ("stray arm"),
  not silently ignored.

## See
- `format-docx.md`, `format-xlsx.md`, `format-pptx.md`, `format-pdf.md`
- `theme-document.md` — the `heading` font role and mark styles
