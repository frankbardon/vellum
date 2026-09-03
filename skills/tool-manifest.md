---
name: tool-manifest
description: Return the manifest describing what Vellum can do — formats, block kinds, capabilities and error codes.
kind: manifest
category: tool
type: skill
applies_to: ["all"]
examples_tags: [tool, manifest]
---

## What it does
Returns the manifest describing what Vellum can do: formats, block kinds,
capabilities, MCP tools and error codes — a machine-readable index over the
same registries CLAUDE.md's Update Demand table names.

## Input
None.

## Output
```json
{ "manifest": { "format_version": "1.0", "formats": [], "blocks": [] } }
```
`descriptor.Manifest`: format list, block-kind list, the capability matrix,
the MCP tool list, error-code names.

## See
- Every skill file this pack ships is one entry deeper than the manifest
  goes — the manifest says what exists, a skill file says how to use it
  correctly.
