---
name: tool-compose
description: Render a specification to an artifact (DOCX, XLSX, PPTX or PDF/A-2b).
kind: compose
category: tool
type: skill
applies_to: [docx, xlsx, pptx, pdf]
examples_tags: [tool, compose, render]
---

## What it does
Renders a specification to an artifact. Wraps `Vellum.Compose`:
decodes/validates the spec, resolves it against the target format's theme
and capability matrix, lowers into the format's own model, and writes it.
Nothing is written until resolution succeeds — a rejected feature or an
unrenderable block returns an error before any bytes are produced.

## Input
```json
{ "spec": { "...": "a spec.Spec, JSON or YAML" }, "format": "docx" }
```
`spec` is raw JSON (`spec.DecodeAuto` auto-detects YAML, the same as the
CLI's own compose verb). `format` is one of `docx`, `xlsx`, `pptx`, `pdf`.

## Output
```json
{ "format": "docx", "artifact": "<base64 bytes>", "warnings": [] }
```
`artifact.Report` flattened onto this tool's own fields. `warnings` names
every degradation the render performed — read them before trusting an
unattended batch.

## See
- `tool-validate.md` — the same resolution, no bytes written
- `tool-capabilities.md` — the declared outcome matrix
- Every `block-*.md` and `format-*.md`
