---
name: tool-boxes
description: Return the asset slots a theme offers a format, answerable before any specification exists.
kind: boxes
category: tool
type: skill
applies_to: [docx, xlsx, pptx, pdf]
examples_tags: [tool, boxes, theme, asset]
---

## What it does
Returns the asset slots a theme offers a format — the size a host should
render a chart or image at — answerable before any specification exists.
This is the query behind `spec.Asset.Role` and its `asset.full`/
`asset.half`/`asset.quarter`/`logo` roles.

## Input
```json
{ "theme": "", "format": "pptx" }
```
`theme` empty selects the built-in theme.

## Output
```json
{ "boxes": [ { "role": "asset.full", "width": { "value": 6, "unit": "in" } } ] }
```
Every box the format's layouts declare, unioned and sorted by role. A zero
`height` means the box takes its height from the asset's own aspect ratio.

## See
- `theme-document.md`, `block-asset.md`
