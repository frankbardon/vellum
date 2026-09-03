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
Reads an example specification from the embedded example pack (`examples/`)
by name.

## Input
```json
{ "name": "<example name>" }
```
An empty `name` (or an omitted one) lists every available name instead of
fetching one document.

## Output
Fetching one document by name:
```json
{ "content": "<raw example spec or binding, JSON>" }
```
Listing (empty `name`):
```json
{ "names": ["block-heading", "format-docx", "fill-bind", "..."] }
```
`content` and `names` are mutually exclusive on the wire — a response
carries whichever one the request asked for, never both. A `name` that
matches nothing in the pack is `VELLUM_MCP_INVALID_INPUT`, naming the value
given and the full set of names that would have matched.

## See
- `examples/examples.go`'s `Get` and `All` — what this tool's handler
  (`mcp/handlers.go`'s `handleExamples`) actually calls.
- Every `tool-*.md`
