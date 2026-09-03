---
name: tool-inspect
description: Report an OOXML template's anchor inventory and font requirements, without modifying it.
kind: inspect
category: tool
type: skill
applies_to: [docx, xlsx, pptx]
examples_tags: [tool, inspect, fill, anchors]
---

## What it does
Reports an OOXML template's anchor inventory, without modifying it.
Read-only discovery over DOCX's native content controls and `{{marker}}`
text, xlsx's defined names and Excel Table columns, and pptx's shape names
— see `fill-anchors.md` for the full kind vocabulary.

## Input
```json
{ "template": "<base64 bytes>" }
```
The raw OOXML package.

## Output
`template.InspectReport`, aliased directly onto this tool's output (no
separate wire shape): an `Inventory` of discovered `anchor.Anchor` entries,
each carrying `Name`, `Kind`, `Alias`, `Part` and `Span`.

## See
- `fill-anchors.md`
- `tool-fill.md`
