---
name: tool-fill
description: Bind data into an OOXML template, leaving every part it did not touch byte-identical.
kind: fill
category: tool
type: skill
applies_to: [docx, xlsx, pptx]
examples_tags: [tool, fill, bind]
---

## What it does
Binds data into an OOXML template, leaving every untouched part
byte-identical. Wraps `Vellum.Fill`: decodes a binding document
(`bind`/`repeat`/`if`/`with` statements — see `fill-binding.md`), evaluates
every FEEL expression against the supplied data, and splices the result
back into the template's own parts.

## Input
```json
{
  "template": "<base64 bytes>",
  "binding": { "...": "a bind.Binding, JSON or YAML" },
  "data": { "...": "arbitrary JSON, optional" }
}
```

## Output
```json
{ "package": "<base64 filled bytes>", "touched": ["/word/document.xml"] }
```
`touched` is the non-destructiveness receipt: every part outside it is
byte-identical to `template`.

## See
- `fill-binding.md`, `fill-repeat.md`, `fill-anchors.md`
- `tool-inspect.md`
