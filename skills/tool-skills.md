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
An empty `name` (or an omitted one) lists every available name instead of
fetching one document.

## Output
Fetching one document by name:
```json
{ "content": "<raw markdown, frontmatter and body>" }
```
Listing (empty `name`):
```json
{ "names": ["block-heading", "fill-binding", "..."] }
```
`content` and `names` are mutually exclusive on the wire — a response
carries whichever one the request asked for, never both. A `name` that
matches nothing in the pack is `VELLUM_MCP_INVALID_INPUT`, naming the value
given and the full set of names that would have matched.

## See
- `skills/skills.go`'s `Get` and `All` — what this tool's handler
  (`mcp/handlers.go`'s `handleSkills`) actually calls.
- Every other `tool-*.md`
