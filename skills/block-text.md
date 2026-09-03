---
name: block-text
description: A paragraph of prose.
kind: text
category: block
type: skill
applies_to: [docx, xlsx, pptx, pdf]
examples_tags: [block, text]
---

## What it is
A paragraph of prose. `spec.Text` carries only `Content` (string). Renders
natively in DOCX, PPTX and PDF (`block.text`: `Renders`); XLSX degrades it to
a wrapped cell (`VELLUM_CAPABILITY_DEGRADED`).

## JSON shape
```json
{ "kind": "text", "text": { "content": "Body prose goes here." } }
```

## Gotchas
- No inline run-level styling lives on this block — bold and italic
  (`text.bold`, `text.italic`) are resolver-applied, not fields here.
- Empty `content` is legal; `Validate` only checks that the `text` arm is
  present, not that it carries anything.
- PDF lays itself out and only promises `text.script.latin`,
  `text.script.greek` and `text.script.cyrillic` — anything falling under
  `text.script.other`, or two supported scripts mixed in one string, is
  rejected (`VELLUM_PDF_SCRIPT_UNSUPPORTED`) rather than drawn wrong. The
  three OOXML formats impose no such restriction: Word/Excel/PowerPoint do
  their own shaping and line breaking.

## See
- `format-docx.md`, `format-xlsx.md`, `format-pptx.md`, `format-pdf.md`
