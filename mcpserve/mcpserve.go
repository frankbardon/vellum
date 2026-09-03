// Package mcpserve is the embedder entry point for Vellum's MCP surface: the
// one call that turns a [vellum.Options] into a working, listening server.
//
// It never imports modelcontextprotocol/go-sdk directly — CLAUDE.md's
// package map reserves that import to
// [github.com/frankbardon/vellum/mcp/gosdk] alone — and reaches every SDK
// type it needs to hold ([gosdk.Server], [gosdk.Transport]) through that
// package's own re-exports instead.
//
// # Wiring
//
//	server, err := mcpserve.New(mcpserve.Options{})
//	if err != nil { ... }
//	err = mcpserve.ServeStdio(ctx, server)
//
// [New] constructs a [vellum.Vellum] from opts.Vellum, builds the full tool
// catalog against it ([mcp.NewCatalog]), and mounts it on a freshly
// constructed SDK server ([mcp/gosdk.Register]) — it does not serve.
// [Serve] (or its stdio convenience, [ServeStdio]) is the separate call that
// starts listening, matching "Register mounts and never serves" one layer
// up: New itself neither mounts nor serves anything blocking, and the split
// between construction and serving is exactly [github.com/frankbardon/vellum/internal/cli]'s
// own mcp verb's shape — build the server once, then hand it whatever
// stdin/stdout (or, in a future transport, socket) the invocation supplies.
package mcpserve

import (
	"context"
	"io"
	"os"

	"github.com/frankbardon/vellum"
	"github.com/frankbardon/vellum/mcp"
	"github.com/frankbardon/vellum/mcp/gosdk"
)

// Options configures [New].
type Options struct {
	// Vellum configures the facade every tool call runs against — the same
	// seams [vellum.Options] documents (theme provider, asset resolver,
	// FEEL evaluator, source date epoch, producer). The zero value serves
	// the built-in theme and inline assets only, exactly as
	// internal/cli's own newFacade does.
	Vellum vellum.Options

	// Name and Version identify this server to a connecting MCP client.
	// Name defaults to "vellum" when empty; Version has no default — an
	// empty string is a legitimate, if uninformative, answer.
	Name    string
	Version string
}

// New constructs a working, unmounted-nowhere-yet MCP server: a
// [vellum.Vellum] from opts.Vellum, the full tool catalog built against it,
// and an SDK server with every tool already registered. It does not serve —
// see [Serve] and [ServeStdio].
func New(opts Options) (*gosdk.Server, error) {
	v, err := vellum.New(opts.Vellum)
	if err != nil {
		return nil, err
	}

	name := opts.Name
	if name == "" {
		name = "vellum"
	}

	server := gosdk.NewServer(&gosdk.Implementation{Name: name, Version: opts.Version}, nil)
	if err := gosdk.Register(server, mcp.NewCatalog(v)); err != nil {
		return nil, err
	}
	return server, nil
}

// Serve runs server against a transport built from r and w until the
// connection ends or ctx is cancelled — the embedder's actual "start
// listening" call, over any (reader, writer) pair rather than only real
// process stdio, which is what makes it callable against a test harness's
// own injected streams the same way every other internal/cli verb's Action
// is.
func Serve(ctx context.Context, server *gosdk.Server, r io.Reader, w io.Writer) error {
	return gosdk.Serve(ctx, server, &gosdk.IOTransport{
		Reader: io.NopCloser(r),
		Writer: nopWriteCloser{w},
	})
}

// ServeStdio is [Serve] against the process's real stdin/stdout — the
// production path an embedder or cmd/vellum's own mcp verb uses when there
// is no injected transport to prefer.
func ServeStdio(ctx context.Context, server *gosdk.Server) error {
	return Serve(ctx, server, os.Stdin, os.Stdout)
}

// nopWriteCloser adapts an io.Writer to io.WriteCloser: [gosdk.IOTransport]
// requires one, and neither cmd.Writer (internal/cli's own injected stream)
// nor os.Stdout's own type carries a meaningful Close beyond "do nothing" —
// closing stdout is the process's job on exit, not this server's.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }
