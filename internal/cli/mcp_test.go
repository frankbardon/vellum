package cli_test

import (
	"testing"
	"time"

	vellumcli "github.com/frankbardon/vellum/internal/cli"
)

// TestMCP_EmptyStdinEndsCleanly is the CLI-level counterpart to
// mcpserve's own TestServe_EmptyInputEndsTheConnectionCleanly: an MCP
// client that connects and immediately closes its write end (which is what
// an empty fixed stdin looks like to the server) ends the session cleanly,
// and the mcp verb exits 0 rather than hanging the test run.
//
// Bounded implicitly by go test's own timeout rather than an explicit one
// here — runCLI is synchronous, so a real hang in mcpserve.Serve would show
// up as this test (and the whole package) timing out, which is exactly what
// mcpserve's own bounded tests already guard against directly. This test's
// job is narrower: prove the CLI verb actually reaches mcpserve.Serve over
// cmd.Reader/cmd.Writer rather than the stub it used to be.
func TestMCP_EmptyStdinEndsCleanly(t *testing.T) {
	done := make(chan struct{})
	var res cliResult
	go func() {
		res = runCLI(t, "", "mcp")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("vellum mcp did not return on empty stdin within 10s")
	}

	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d, want %d\nstderr=%s", res.ExitCode, vellumcli.ExitOK, res.Stderr)
	}
}
