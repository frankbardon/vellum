---
name: block-page-break
description: Starts a new page, slide or sheet, depending on the target format.
kind: page_break
category: block
type: skill
applies_to: [docx, xlsx, pptx, pdf]
examples_tags: [block, page-break]
---

## What it is
Starts a new page, slide or sheet depending on the target format.
`spec.PageBreak` carries no fields today — a struct rather than a bare kind,
so it can gain a break type later without reshaping every other block.
Renders in DOCX and PDF (`block.page_break`: `Renders`); degrades in XLSX to
a new sheet and in PPTX to a new slide, both `VELLUM_CAPABILITY_DEGRADED`.

## JSON shape
```json
{ "kind": "page_break", "page_break": {} }
```

## Gotchas
- The empty arm (`{}`) is still required. A block with `kind: "page_break"`
  and no `page_break` field is `VELLUM_SPEC_INVALID` ("missing arm").
- In XLSX "a new sheet" is a literal new worksheet, not a print-area page
  break — there is no print-layout concept here at all.
- In PPTX "a new slide" is independent of `overflow.continue_repeat_headers`,
  which also produces new slides when a table overflows one; the two
  mechanisms can interact within one long section.

## See
- `format-docx.md`, `format-xlsx.md`, `format-pptx.md`, `format-pdf.md`
