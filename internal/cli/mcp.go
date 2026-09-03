package cli

import (
	"context"
	"fmt"

	"github.com/frankbardon/vellum/mcpserve"
	"github.com/urfave/cli/v3"
)

// newMCPCommand builds the mcp verb: it starts a Model Context Protocol
// server exposing every vellum_* tool, over cmd.Reader/cmd.Writer — the same
// injected streams every other verb in this package reads from and writes
// to, rather than the process's own os.Stdin/os.Stdout directly. That is
// what makes this verb testable through runCLI's own fixed, non-interactive
// stdin the same way compose or fill are: a client that writes nothing (or
// closes its write end) causes the server to end the connection cleanly and
// this Action to return, rather than blocking forever.
//
// There is no --json mode: MCP is itself the wire protocol this verb speaks
// on cmd.Writer, and wrapping it in a second, incompatible envelope format
// would not be a --json variant of the same output, it would be a different
// protocol.
func newMCPCommand() *cli.Command {
	return &cli.Command{
		Name:  "mcp",
		Usage: "run the MCP server, exposing every vellum_* tool over stdin/stdout",
		Description: "Starts a Model Context Protocol server speaking newline-delimited JSON-RPC\n" +
			"over stdin/stdout, for an MCP client (an agent host) to launch as a\n" +
			"subprocess and connect to. Runs until the connection ends — the client\n" +
			"closes its stream, or this process is signalled.",
		Action: runMCP,
	}
}

func runMCP(ctx context.Context, cmd *cli.Command) error {
	server, err := mcpserve.New(mcpserve.Options{Name: "vellum", Version: cmd.Root().Version})
	if err != nil {
		return reportError(cmd, false, failErr(err))
	}

	if err := mcpserve.Serve(ctx, server, cmd.Reader, cmd.Writer); err != nil {
		// A non-nil return from Serve is the ordinary way an MCP stdio
		// session ends, not a failed operation: the client closing its end
		// of the connection is by far the most common cause, and the SDK's
		// own reference servers (examples/server/hello, in
		// modelcontextprotocol/go-sdk) log this the same way — informational,
		// not fatal — rather than exiting non-zero. CLAUDE.md's exit code 1
		// means "the operation the facade was asked to perform failed"; a
		// session that ran until its peer disconnected did not fail, it
		// completed.
		fmt.Fprintf(cmd.ErrWriter, "mcp: session ended: %v\n", err)
	}
	return nil
}
