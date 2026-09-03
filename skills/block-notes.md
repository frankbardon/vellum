---
name: block-notes
description: Speaker-note or annotation content, native in pptx and degraded everywhere else.
kind: notes
category: block
type: skill
applies_to: [docx, xlsx, pptx, pdf]
examples_tags: [block, notes]
---

## What it is
Speaker-note or annotation content. `spec.Notes` carries `Content` (string).
What it becomes is declared by the capability matrix, not discovered at
render time: PPTX renders it natively (`block.notes`: `Renders`) as the only
format with a real speaker-note channel. DOCX and PDF both degrade it to a
footnote anchored at the block's own position; XLSX degrades it to a cell
comment. All three non-PPTX outcomes are `VELLUM_CAPABILITY_DEGRADED`.

## JSON shape
```json
{ "kind": "notes", "notes": { "content": "Only mention this if asked." } }
```

## Gotchas
- The DOCX and PDF footnotes anchor at the block's own position in the
  section, not in a document-wide end-note list.
- The XLSX cell comment is a *legacy* comment: `xl/comments1.xml` paired
  with `xl/drawings/vmlDrawing1.vml`, referenced by the worksheet's own
  `<legacyDrawing r:id="…"/>`. The newer threaded-comment schema, which
  needs an author identity this library has no source for, is not written.
- A note does not reach into the section's own `marks` — it renders with
  whatever the format's native note styling is.

## See
- `format-docx.md`, `format-xlsx.md`, `format-pptx.md`, `format-pdf.md`
