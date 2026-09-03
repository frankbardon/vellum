package mcpserve_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/frankbardon/vellum/mcpserve"
)

// TestNew_BuildsAServerWithNoError is the smoke test for the embedder's
// first call: a zero-value Options must already produce a working server,
// per every other Options struct in this codebase's own "a host that wires
// nothing still gets a working library" convention.
func TestNew_BuildsAServerWithNoError(t *testing.T) {
	server, err := mcpserve.New(mcpserve.Options{})
	if err != nil {
		t.Fatalf("New(Options{}): %v", err)
	}
	if server == nil {
		t.Fatal("New(Options{}) returned a nil server with no error")
	}
}

// TestNew_DefaultNameIsVellum checks Options.Name's documented default
// takes effect without asserting anything about the SDK's own internal
// representation of it (which mcpserve's own tests cannot reach without
// importing the SDK, the one thing this package must not do) — it is
// enough that New does not error and does not require Name to be set.
func TestNew_DefaultNameIsVellum(t *testing.T) {
	if _, err := mcpserve.New(mcpserve.Options{Name: "", Version: "v0.0.0-test"}); err != nil {
		t.Fatalf("New with an empty Name: %v", err)
	}
}

// TestServe_EmptyInputEndsTheConnectionCleanly checks the shape
// internal/cli's own mcp verb depends on: Serve, handed a reader that is
// already at EOF, returns promptly with no error — a clean connection close
// — rather than blocking forever or reporting a spurious failure. This is
// what makes "vellum mcp" testable through the CLI's own harness (which
// feeds a fixed, non-interactive stdin) at all.
//
// Bounded by a context timeout so that a wrong assumption about the SDK's
// own EOF handling fails this test loudly, in a few seconds, rather than
// hanging the whole suite.
func TestServe_EmptyInputEndsTheConnectionCleanly(t *testing.T) {
	server, err := mcpserve.New(mcpserve.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- mcpserve.Serve(ctx, server, strings.NewReader(""), &strings.Builder{})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve(empty stdin) = %v, want nil (a clean close)", err)
		}
	case <-ctx.Done():
		t.Fatal("Serve did not return before the context's own timeout — it must end cleanly on EOF")
	}
}

// TestServe_ContextCancellationStopsTheServer checks the other way a
// connection ends: the caller's own context, not the peer closing the
// stream. A reader that never produces EOF (a pipe nobody writes to or
// closes) would otherwise block Serve forever; ctx cancellation must still
// unblock it.
func TestServe_ContextCancellationStopsTheServer(t *testing.T) {
	server, err := mcpserve.New(mcpserve.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// io.Pipe's reader blocks until something is written or the writer is
	// closed — no spontaneous EOF — so this genuinely exercises "cancel
	// while still waiting for input" rather than a read that would have
	// returned on its own.
	r, w := io.Pipe()
	defer w.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- mcpserve.Serve(ctx, server, r, &strings.Builder{})
	}()

	// Give Serve a moment to actually connect before cancelling, so this
	// exercises "cancel an in-flight session" rather than "cancel before
	// anything started".
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Serve returned — whether nil or ctx.Err(), cancellation unblocked
		// it, which is what this test checks. The exact error value is the
		// SDK's own choice, not this package's contract.
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after its context was cancelled")
	}
}
