# MCP Integration

`vellum mcp` runs a Model Context Protocol server over stdin/stdout,
newline-delimited JSON-RPC, for an MCP client to launch as a subprocess and
connect to. It runs until the connection ends, and it is the only CLI verb
with no `--json` mode — MCP is itself the wire protocol this verb speaks, so
there is no second output shape to opt into.

## Embedding the server directly

A Go embedder does not need the CLI at all:

```go
server, err := mcpserve.New(mcpserve.Options{})
if err != nil {
    // ...
}
if err := mcpserve.ServeStdio(ctx, server); err != nil {
    // ...
}
```

`mcpserve.Options` carries the same `vellum.Options` seams described in
[Seams](../library/seams.md) — a host embedding the MCP server configures
its own theme provider, asset resolver and FEEL evaluator exactly as it
would for direct library use. `mcpserve.New` constructs the facade, builds
the full tool catalog against it, and mounts it on a freshly built SDK
server; it does not itself start listening. `Serve` (or its stdio
convenience `ServeStdio`) is the separate call that blocks and actually
serves — construction and serving are kept apart so an embedder can build
the server once and hand it whatever transport an invocation supplies.

## The SDK-free architecture

The MCP surface is deliberately split four ways, so that
`modelcontextprotocol/go-sdk` is reachable from exactly one narrow package
rather than threaded through the facade:

- **`mcp/toolmeta`** — pure `(name, description)` constants, zero
  dependencies at all, not even on Vellum's own `errors` package. Importable
  from anywhere a tool's name needs to be known without pulling in schema
  reflection or the SDK.
- **`mcp`** — the SDK-free core: typed request/response contracts, one
  facade-only handler per tool (each touches exactly one `*vellum.Vellum`
  method and nothing below the facade directly), and a type-erased catalog a
  transport adapter can iterate without per-tool special-casing.
- **`mcp/gosdk`** — the *only* package that imports
  `modelcontextprotocol/go-sdk`. It translates `mcp`'s uniform tool shape
  into the SDK's own registration call.
- **`mcpserve`** — the embedder entry point described above: builds a
  facade, builds the catalog, and exposes the `Serve`/`ServeStdio` functions
  that actually start listening.

A Go embedder pulling in `mcpserve` gets the SDK transitively, of course —
but nothing about `mcp`'s own request/response contracts or handler logic
depends on it, which keeps the tool behaviour testable and reasoned about
without an MCP client in the loop at all.

## Every tool's output is an envelope

Per CLAUDE.md's Output Format Contract, every tool call's result is a
`descriptor.Envelope`-shaped JSON document on both the success and the
failure path — a tool call never returns a bare error string, and a client
reads `result.errors` the same way a `--json` CLI invocation's stdout can.
See [The output envelope](../contract/envelope.md).

Every tool's input and output JSON Schema is reflected once, at process
init, from its Go type — never per call — so `tools/list` answers
identically on the first call and the thousandth in one running process.

## The ten tools

| Tool | What it does | Doc page |
|---|---|---|
| `vellum_compose` | Render a specification to an artifact. | [Formats](../formats/capabilities.md) |
| `vellum_validate` | Check a specification against a format's capability matrix, without rendering. | [Formats](../formats/capabilities.md) |
| `vellum_inspect` | Report a template's anchor inventory and font requirements. | [Templates and anchors](../fill/anchors.md) |
| `vellum_fill` | Bind data into a template, leaving untouched parts byte-identical. | [Bindings and FEEL](../fill/bindings.md) |
| `vellum_capabilities` | Return the declared (feature, format) outcome matrix. | [Capability matrix](../formats/capabilities.md) |
| `vellum_boxes` | Return the asset slots a theme offers a format. | [Themes](../spec/themes.md#layouts-pages-and-box-roles) |
| `vellum_schema` | Return the published JSON Schema for a specification. | [Payload schema](../contract/payload-schema.md) |
| `vellum_manifest` | Return the manifest describing what Vellum can do. | [Payload schema](../contract/payload-schema.md) |
| `vellum_skills` | Read a skill document from the embedded skill pack by name, or list every available name. | — |
| `vellum_examples` | Read an example specification from the embedded example pack by name, or list every available name. | [Quickstart](../getting-started/quickstart.md) |

All ten tools are fully wired today. Eight of them — `compose` through
`manifest` — are thin, tested handlers over exactly one `*vellum.Vellum`
method, following the identical shape internal/cli's own verb files use.
`vellum_skills` and `vellum_examples` read the embedded `skills/` and
`examples/` packs directly instead, since neither pack's content depends on
a `*vellum.Vellum` instance: a non-empty `name` looks a document up by its
filename stem (e.g. `block-heading`) and returns the whole file; an empty
`name` returns every available stem instead, sorted. A `name` that matches
nothing in the pack is `VELLUM_MCP_INVALID_INPUT`, naming the value it was
given and the full set of names that would have matched.

## See also

- [The output envelope](../contract/envelope.md) — the exact JSON shape
  every tool call's result carries.
- [Seams](../library/seams.md) — what `mcpserve.Options` configures.
- `skills/tool-*.md` — one skill file per tool, each documenting that
  tool's exact input and output shape for an LLM calling it at runtime.
