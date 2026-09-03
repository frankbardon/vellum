package gosdk_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frankbardon/vellum"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/mcp"
	"github.com/frankbardon/vellum/mcp/gosdk"
	"github.com/frankbardon/vellum/mcp/toolmeta"
)

// This file is the one place in the module allowed to import the SDK
// directly, other than mcp/gosdk's own source — it is testing that source,
// so the test binary linking the SDK is exactly the exception CLAUDE.md's
// package-map rule anticipates.

func testCatalog(t *testing.T) []mcp.Tool {
	t.Helper()
	v, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("vellum.New: %v", err)
	}
	return mcp.NewCatalog(v)
}

// connectedServerAndClient builds a server carrying catalog, mounts it with
// [gosdk.Register], connects it and a client over an in-memory transport
// pair (the SDK's own [sdkmcp.NewInMemoryTransports], its documented
// testing surface for exactly this — an in-process client/server pair
// needing no real subprocess or socket), and returns the connected client
// session. t.Cleanup closes both ends.
func connectedServerAndClient(t *testing.T, catalog []mcp.Tool) *sdkmcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	server := gosdk.NewServer(&gosdk.Implementation{Name: "vellum-test", Version: "test"}, nil)
	if err := gosdk.Register(server, catalog); err != nil {
		t.Fatalf("Register: %v", err)
	}

	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- gosdk.Serve(ctx, server, serverTransport)
	}()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() {
		cs.Close()
		select {
		case <-serverDone:
		case <-time.After(5 * time.Second):
			t.Error("server did not shut down after the client closed the connection")
		}
	})
	return cs
}

// TestRegister_MountsEveryCatalogTool checks that a client listing tools
// over a real (in-memory) MCP connection sees every tool [mcp.NewCatalog]
// built, under its own registered name, with a non-empty description and an
// object-typed input schema — end to end through the SDK's own wire
// encoding, not just through Register's Go-level bookkeeping.
func TestRegister_MountsEveryCatalogTool(t *testing.T) {
	catalog := testCatalog(t)
	cs := connectedServerAndClient(t, catalog)

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	want := toolmeta.AllTools()
	if len(res.Tools) != len(want) {
		t.Fatalf("ListTools returned %d tools, want %d", len(res.Tools), len(want))
	}

	seen := make(map[string]*sdkmcp.Tool, len(res.Tools))
	for _, tool := range res.Tools {
		seen[tool.Name] = tool
	}
	for _, w := range want {
		got, ok := seen[w.Name]
		if !ok {
			t.Errorf("ListTools did not include %q", w.Name)
			continue
		}
		if got.Description != w.Description {
			t.Errorf("%s: description = %q, want %q", w.Name, got.Description, w.Description)
		}
		var schema map[string]any
		raw, err := json.Marshal(got.InputSchema)
		if err != nil {
			t.Errorf("%s: marshalling input schema: %v", w.Name, err)
			continue
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Errorf("%s: input schema is not valid JSON: %v", w.Name, err)
			continue
		}
		if schema["type"] != "object" {
			t.Errorf("%s: input schema type = %v, want object", w.Name, schema["type"])
		}
	}
}

// TestRegister_CallToolRoundTripsThroughTheSDK calls vellum_manifest over
// the wire and checks the structured content is the manifest's own JSON —
// proving the whole path (SDK request -> Register's wrapper closure ->
// mcp.Tool.Handle -> descriptor.Envelope -> SDK response) round-trips
// correctly, not just that registration succeeded.
func TestRegister_CallToolRoundTripsThroughTheSDK(t *testing.T) {
	catalog := testCatalog(t)
	cs := connectedServerAndClient(t, catalog)

	res, err := cs.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      toolmeta.NameManifest,
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool(vellum_manifest) reported IsError; content: %+v", res.Content)
	}

	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshalling structured content: %v", err)
	}
	var env struct {
		Data struct {
			Manifest struct {
				FormatVersion string `json:"format_version"`
			} `json:"manifest"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decoding structured content as an envelope: %v\nraw: %s", err, raw)
	}
	if len(env.Errors) != 0 {
		t.Errorf("errors = %+v, want none", env.Errors)
	}
	if env.Data.Manifest.FormatVersion == "" {
		t.Error("manifest carries no format_version")
	}
}

// TestRegister_ToolFailureSetsIsErrorNotAProtocolError checks the
// translation Register's own doc comment describes: a facade failure (here,
// an unrecognised format) comes back as a normal CallTool result with
// IsError set, never as a transport-level error — exactly the distinction
// ToolHandler's own doc comment draws between a protocol error and a tool
// error.
func TestRegister_ToolFailureSetsIsErrorNotAProtocolError(t *testing.T) {
	catalog := testCatalog(t)
	cs := connectedServerAndClient(t, catalog)

	res, err := cs.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      toolmeta.NameCapabilities,
		Arguments: map[string]any{"format": "bogus"},
	})
	if err != nil {
		t.Fatalf("CallTool returned a protocol-level error for a tool failure: %v", err)
	}
	if !res.IsError {
		t.Fatal("CallTool(bad format) did not set IsError")
	}

	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshalling structured content: %v", err)
	}
	var env struct {
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decoding structured content: %v\nraw: %s", err, raw)
	}
	// code is a typed verr.Code, not a bare string literal, so this
	// comparison cannot be mistaken by TestClaudeMdMentionsAllEnvVars for a
	// VELLUM_-prefixed environment variable name.
	if wantCode := verr.VELLUM_MCP_INVALID_INPUT; len(env.Errors) != 1 || env.Errors[0].Code != string(wantCode) {
		t.Fatalf("errors = %+v, want exactly one %s", env.Errors, wantCode)
	}
}

// TestRegister_NeverServes is the other half of "Register mounts and never
// serves": it must return as soon as every tool is added, without needing a
// connected transport at all — a hang here would mean Register is blocking
// on something transport-shaped, which is exactly the responsibility
// CLAUDE.md assigns to Serve instead.
func TestRegister_NeverServes(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		server := gosdk.NewServer(&gosdk.Implementation{Name: "vellum-test", Version: "test"}, nil)
		done <- gosdk.Register(server, testCatalog(t))
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Register: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Register did not return; it must mount tools and return without serving")
	}
}
