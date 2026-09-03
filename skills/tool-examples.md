---
name: tool-examples
description: Read an example specification from the embedded example pack.
kind: examples
category: tool
type: skill
applies_to: ["all"]
examples_tags: [tool, examples, meta]
---

## What it does
Reads an example specification from the embedded example pack (`examples/`,
built by a later story) by name.

## Input
```json
{ "name": "<example name>" }
```

## Output
```json
{ "content": "<raw example spec, JSON or YAML>" }
```

## See
- Not yet wired: calling this tool today reports
  `VELLUM_MCP_NOT_IMPLEMENTED`. The `examples/` pack itself does not exist
  yet — it is a separate story from this one.
- Every `tool-*.md`
