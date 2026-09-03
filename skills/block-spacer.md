---
name: block-spacer
description: Vertical space.
kind: spacer
category: block
type: skill
applies_to: [docx, xlsx, pptx, pdf]
examples_tags: [block, spacer, layout]
---

## What it is
Vertical space. `spec.Spacer` carries `Height` (a `spec.Length`: value plus
unit, converted once to EMU at validation). Renders in DOCX, PPTX and PDF
(`block.spacer`: `Renders`); degrades in XLSX to a blank row
(`VELLUM_CAPABILITY_DEGRADED`) — a sheet has no arbitrary vertical measure,
only rows.

## JSON shape
```json
{ "kind": "spacer", "spacer": { "height": { "value": 12, "unit": "pt" } } }
```
Accepted units: `pt`, `mm`, `in`, `emu`. A non-finite value is
`VELLUM_SPEC_INVALID`.

## Gotchas
- In XLSX the requested height is not honoured proportionally: one spacer
  block is always exactly one blank row, whatever `height` says.
- `Length.EMU()` rounds half away from zero — a spacer measured in points
  that does not land on a whole EMU is rounded, never truncated.

## See
- `format-docx.md`, `format-xlsx.md`, `format-pptx.md`, `format-pdf.md`
