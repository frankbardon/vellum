---
name: tool-schema
description: Return the published JSON Schema for a specification.
kind: schema
category: tool
type: skill
applies_to: ["all"]
examples_tags: [tool, schema]
---

## What it does
Returns the published JSON Schema for a specification — the same schema
`vellum schema` writes unwrapped on the CLI, carried here as one field of
the tool's own envelope data, because MCP has no unwrapped transport mode.

## Input
None. The schema does not depend on anything a caller supplies.

## Output
```json
{ "schema": { "$schema": "...", "$id": "...", "...": "..." } }
```
Self-contained: carries its own `$schema` and `$id`, and embeds the `spec`
definitions rather than referencing them out of band.

## See
- Every `block-*.md`
