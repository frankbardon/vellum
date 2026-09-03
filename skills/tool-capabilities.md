---
name: tool-capabilities
description: Return the declared (feature, format) outcome matrix — renders, degrades, or rejects.
kind: capabilities
category: tool
type: skill
applies_to: [docx, xlsx, pptx, pdf]
examples_tags: [tool, capabilities, matrix]
---

## What it does
Returns the declared (feature, format) outcome matrix: renders, degrades
(naming the alternative and the warning code), or rejects (naming the error
code). Answerable before any specification exists — the same matrix
`vellum_compose` and `vellum_validate` enforce.

## Input
```json
{ "format": "docx" }
```
One of `docx`, `xlsx`, `pptx`, `pdf`.

## Output
```json
{ "capabilities": [
  { "feature": "block.notes", "format": "docx", "outcome": "degrades",
    "degrade": "footnote", "code": "VELLUM_CAPABILITY_DEGRADED", "note": "..." }
] }
```
Every declared entry for the requested format, in feature declaration
order.

## See
- Every `block-*.md`/`format-*.md` "Gotchas" section restates the rows
  relevant to it.
- `tool-compose.md`
