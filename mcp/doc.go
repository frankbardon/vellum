// Package mcp is the SDK-free core of Vellum's Model Context Protocol
// surface: typed request/response contracts over the same [vellum.Vellum]
// facade [internal/cli] wraps, translated to and from JSON at one seam.
//
// It is split four ways, per CLAUDE.md's package map:
//
//   - [github.com/frankbardon/vellum/mcp/toolmeta] — pure name/description
//     constants, no dependencies at all.
//   - This package — typed In/Out contracts, init-time schema reflection, a
//     facade-only handler per tool, and a type-erased catalog a transport
//     adapter can iterate without per-tool special-casing.
//   - [github.com/frankbardon/vellum/mcp/gosdk] — the only package
//     importing modelcontextprotocol/go-sdk. It translates this package's
//     uniform [Tool] shape into the SDK's own registration call and mounts
//     it; it never serves.
//   - [github.com/frankbardon/vellum/mcpserve] — the embedder entry point:
//     constructs a facade, builds the catalog, and exposes a Serve function
//     an embedder actually calls to start listening.
//
// # Facade-only handlers
//
// Every handler in handlers.go takes exactly one typed input, calls exactly
// one method on a *vellum.Vellum, and returns exactly one typed output. None
// of them touches a format writer, resolve, template, or any package below
// the facade directly — the same discipline internal/cli's own verb files
// already follow, applied here to a second transport rather than relitigated.
//
// # Schema reflection happens once
//
// Every tool's input and output JSON Schema is computed once, at package
// init, from its Go type via [github.com/google/jsonschema-go/jsonschema]
// — never per call. An MCP client asking a tool's contract (tools/list)
// receives exactly this reflected schema; nothing about it depends on how
// many times, or in what order, a client has asked before, which is what
// keeps the catalog itself deterministic: the same process answers
// tools/list identically on the first call and the thousandth.
//
// # Every tool's output is an envelope
//
// CLAUDE.md's Output Format Contract states plainly: "Every --json and MCP
// output path goes through descriptor.NewEnvelope; no fmt.Sprintf builds
// JSON." [Tool.Handle] holds that: its result is always a
// [descriptor.Envelope]-shaped JSON document, on both the success and the
// failure path — a tool call never returns a bare Go error string, and a
// client can read result.errors the same way a --json CLI invocation's
// stdout can.
package mcp
