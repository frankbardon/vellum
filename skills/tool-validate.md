---
name: tool-validate
description: Check a specification against a format's capability matrix, without rendering.
kind: validate
category: tool
type: skill
applies_to: [docx, xlsx, pptx, pdf]
examples_tags: [tool, validate]
---

## What it does
Checks a specification against a format's capability matrix without
rendering — the same resolution `vellum_compose` performs (theme, font and
asset resolution, capability enforcement), stopped one step short of
lowering and writing.

## Input
```json
{ "spec": { "...": "a spec.Spec, JSON or YAML" }, "format": "docx" }
```
Identical shape to `vellum_compose`'s input.

## Output
```json
{ "valid": true, "warnings": [] }
```
`valid` is carried explicitly, rather than as a bare envelope `data: null`,
so a caller can branch without inspecting the envelope's own `errors`
array. A specification that would be rejected surfaces through the
envelope's `errors`, exactly as `vellum_compose` would report it.

## See
- `tool-compose.md`
- `tool-capabilities.md`
