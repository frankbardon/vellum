// Package gosdk is the only package in the module that imports
// github.com/modelcontextprotocol/go-sdk — CLAUDE.md's package map states
// this as a hard rule ("Only mcp/gosdk imports the SDK"), and this package
// is where that rule is made true rather than merely documented.
//
// Its job is narrow: translate [github.com/frankbardon/vellum/mcp]'s
// SDK-free, type-erased [mcp.Tool] catalog into the SDK's own registration
// call ([Register]), and re-export exactly the SDK types a caller outside
// this package needs a name for ([Server], [ServerOptions],
// [Implementation], [Transport], [IOTransport], [StdioTransport]) so that
// caller — [github.com/frankbardon/vellum/mcpserve], today; potentially an
// embedder directly — never needs its own import of the SDK module just to
// hold a value of one of its types. Type aliases, not wrapper types: a
// *Server here is the exact same value an SDK caller would construct, so
// nothing about the SDK's own behaviour or documentation stops applying.
//
// [Register] mounts every tool onto a server and returns without serving.
// Starting the transport — [Serve] — is a separate call, so a caller that
// wants to register tools without immediately blocking on a connection can.
package gosdk

import (
	"context"
	"encoding/json"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frankbardon/vellum/mcp"
)

// Server, ServerOptions, Implementation, Transport, IOTransport and
// StdioTransport are the SDK types this package's own callers need to name.
// Aliases, not new types: assigning through one of these names is assigning
// the SDK's own type, so mcpserve (or an embedder) can pass a *Server it got
// from [NewServer] straight into any SDK-documented API that also happens to
// be reachable — there is nothing this package's boundary hides about how
// the type behaves, only which package must import the module to spell its
// name.
type (
	Server         = sdkmcp.Server
	ServerOptions  = sdkmcp.ServerOptions
	Implementation = sdkmcp.Implementation
	Transport      = sdkmcp.Transport
	IOTransport    = sdkmcp.IOTransport
	StdioTransport = sdkmcp.StdioTransport
)

// NewServer constructs an SDK server named impl, with no tools mounted yet.
// A thin pass-through to [sdkmcp.NewServer], so a caller outside this
// package never needs the SDK's own import to reach it.
func NewServer(impl *Implementation, opts *ServerOptions) *Server {
	return sdkmcp.NewServer(impl, opts)
}

// Register mounts every tool in catalog onto server, translating between
// [mcp.Tool]'s uniform shape and the SDK's own [sdkmcp.Server.AddTool] call.
// It only registers: starting the transport belongs to [Serve], never to
// this function — "Register mounts and never serves," per CLAUDE.md.
//
// It returns an error today for the same reason [vellum.New] does even
// though nothing here can yet fail: catalog is built by
// [mcp.NewCatalog] from a fixed, hand-maintained registry with no duplicate
// names and every schema already validated against the SDK's own
// requirements by mcp's own tests, so there is no failure mode this
// function discovers today. The signature stays fallible so a future one
// (the SDK rejecting a tool name, say) does not need every call site
// rewritten to notice it.
func Register(server *Server, catalog []mcp.Tool) error {
	for _, t := range catalog {
		tool := &sdkmcp.Tool{
			Name:         t.Meta.Name,
			Description:  t.Meta.Description,
			InputSchema:  t.InputSchema,
			OutputSchema: t.OutputSchema,
		}
		handle := t.Handle
		server.AddTool(tool, func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			raw, herr := handle(ctx, req.Params.Arguments)
			// herr is never returned as this closure's own error: per
			// ToolHandler's own doc comment, a non-nil error here would be
			// treated as an MCP *protocol* error rather than a tool
			// failure. handle already turned any tool failure into a
			// descriptor.Envelope carrying an error entry (see
			// mcp.Tool.Handle's own contract), so the correct translation
			// is IsError on a normal result, not a returned error — the
			// same distinction CLAUDE.md draws between VELLUM_CLI_USAGE
			// (exit 2, nothing attempted) and an operation that ran and
			// reported failure (exit 1).
			return &sdkmcp.CallToolResult{
				IsError:           herr != nil,
				Content:           []sdkmcp.Content{&sdkmcp.TextContent{Text: string(raw)}},
				StructuredContent: json.RawMessage(raw),
			}, nil
		})
	}
	return nil
}

// Serve runs server against t until the connection ends or ctx is
// cancelled. A thin pass-through to [sdkmcp.Server.Run], so
// [github.com/frankbardon/vellum/mcpserve] (and, transitively, any
// embedder) never needs its own import of the SDK just to start listening.
func Serve(ctx context.Context, server *Server, t Transport) error {
	return server.Run(ctx, t)
}
