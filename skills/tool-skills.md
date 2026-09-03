---
name: tool-skills
description: Read a skill document from the embedded skill pack.
kind: skills
category: tool
type: skill
applies_to: ["all"]
examples_tags: [tool, skills, meta]
---

## What it does
Reads a skill document from this embedded pack by name (the filename stem,
e.g. `block-heading`, `fill-binding`). This is the tool an agent calls to
load the file being read right now.

## Input
```json
{ "name": "block-heading" }
```
An empty `name` is reserved for a future "list all" behaviour, not yet
defined.

## Output
```json
{ "content": "<raw markdown, frontmatter and body>" }
```

## See
- Not yet wired to this pack's content: calling this tool today reports
  `VELLUM_MCP_NOT_IMPLEMENTED`. Wiring it is tracked separately from this
  pack's own existence; see `skills/skills.go`'s `Get` for what a future
  handler reads.
- Every other `tool-*.md`
