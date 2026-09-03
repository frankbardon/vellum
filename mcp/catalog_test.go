package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/frankbardon/vellum"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/mcp"
	"github.com/frankbardon/vellum/mcp/toolmeta"
)

func testVellum(t *testing.T) *vellum.Vellum {
	t.Helper()
	v, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("vellum.New: %v", err)
	}
	return v
}

// envelope mirrors descriptor.Envelope's wire shape independently, the same
// deliberate choice internal/cli's own test harness makes: decoding against
// a second, hand-written struct checks the wire shape rather than trusting
// the producer's own struct tags.
type envelope struct {
	FormatVersion string          `json:"format_version"`
	Data          json.RawMessage `json:"data"`
	Errors        []entry         `json:"errors"`
	Warnings      []entry         `json:"warnings"`
}

type entry struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

func decodeEnvelope(t *testing.T, raw json.RawMessage) envelope {
	t.Helper()
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("output is not a valid envelope: %v\nraw:\n%s", err, raw)
	}
	if env.Errors == nil || env.Warnings == nil {
		t.Errorf("envelope has a nil errors or warnings array: %+v", env)
	}
	return env
}

// assertOneError checks that env carries exactly one error, under code. code
// is a typed [verr.Code] rather than a bare string literal so this
// comparison cannot be mistaken by TestClaudeMdMentionsAllEnvVars for a
// VELLUM_-prefixed environment variable name — that gate walks the AST for
// string literals matching that shape, and an error code compared as a
// literal string in a test file outside errors/ looks identical to one. The
// same convention internal/cli's own test harness uses.
func assertOneError(t *testing.T, env envelope, code verr.Code) {
	t.Helper()
	if len(env.Errors) != 1 {
		t.Fatalf("errors = %+v, want exactly one", env.Errors)
	}
	if env.Errors[0].Code != string(code) {
		t.Errorf("error code = %q, want %q", env.Errors[0].Code, code)
	}
}

func TestNewCatalog_CoversEveryRegisteredToolInOrder(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	want := toolmeta.AllTools()
	if len(catalog) != len(want) {
		t.Fatalf("catalog has %d tools, toolmeta registers %d", len(catalog), len(want))
	}
	for i, w := range want {
		if catalog[i].Meta != w {
			t.Errorf("catalog[%d].Meta = %+v, want %+v", i, catalog[i].Meta, w)
		}
	}
}

func TestNewCatalog_EverySchemaIsValidObjectJSON(t *testing.T) {
	for _, tool := range mcp.NewCatalog(testVellum(t)) {
		t.Run(tool.Meta.Name, func(t *testing.T) {
			var in map[string]any
			if err := json.Unmarshal(tool.InputSchema, &in); err != nil {
				t.Fatalf("InputSchema is not valid JSON: %v\n%s", err, tool.InputSchema)
			}
			if in["type"] != "object" {
				t.Errorf("InputSchema type = %v, want %q (the SDK requires an object-rooted input schema)", in["type"], "object")
			}

			var out map[string]any
			if err := json.Unmarshal(tool.OutputSchema, &out); err != nil {
				t.Fatalf("OutputSchema is not valid JSON: %v\n%s", err, tool.OutputSchema)
			}
		})
	}
}

// TestNewCatalog_SchemasAreStableAcrossCalls checks that the reflected
// schemas do not change between two catalogs built against two different
// facades — they are computed once, at package init, from the Go types
// alone, so a second NewCatalog call must reuse the identical bytes.
func TestNewCatalog_SchemasAreStableAcrossCalls(t *testing.T) {
	c1 := mcp.NewCatalog(testVellum(t))
	c2 := mcp.NewCatalog(testVellum(t))
	for i := range c1 {
		if string(c1[i].InputSchema) != string(c2[i].InputSchema) {
			t.Errorf("%s: InputSchema changed between catalog builds", c1[i].Meta.Name)
		}
		if string(c1[i].OutputSchema) != string(c2[i].OutputSchema) {
			t.Errorf("%s: OutputSchema changed between catalog builds", c1[i].Meta.Name)
		}
	}
}

func TestLookup_FindsARegisteredTool(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	tool, ok := mcp.Lookup(catalog, toolmeta.NameCompose)
	if !ok {
		t.Fatal("Lookup did not find vellum_compose")
	}
	if tool.Meta.Name != toolmeta.NameCompose {
		t.Errorf("Lookup returned %q", tool.Meta.Name)
	}
}

func TestLookup_UnknownNameNotFound(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	if _, ok := mcp.Lookup(catalog, "vellum_does_not_exist"); ok {
		t.Fatal("Lookup found a tool that was never registered")
	}
}

func TestDispatch_UnknownToolIsCodedNotPanic(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	raw, err := mcp.Dispatch(context.Background(), catalog, "vellum_does_not_exist", nil)
	if err == nil {
		t.Fatal("Dispatch(unknown tool) = nil error, want VELLUM_MCP_UNKNOWN_TOOL")
	}
	env := decodeEnvelope(t, raw)
	assertOneError(t, env, verr.VELLUM_MCP_UNKNOWN_TOOL)
}

// TestToolHandle_ErrorReturnMatchesEnvelopeErrors is the contract
// [mcp.Tool.Handle] documents: the returned error is non-nil exactly when
// the returned envelope carries at least one error entry. Checked over
// every tool, both on a call that succeeds and one that fails, so the
// invariant cannot quietly drift for one tool while holding for the rest.
func TestToolHandle_ErrorReturnMatchesEnvelopeErrors(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))

	check := func(t *testing.T, toolName string, args json.RawMessage) {
		t.Helper()
		tool, ok := mcp.Lookup(catalog, toolName)
		if !ok {
			t.Fatalf("tool %q not registered", toolName)
		}
		raw, err := tool.Handle(context.Background(), args)
		env := decodeEnvelope(t, raw)
		hasErrors := len(env.Errors) > 0
		if hasErrors != (err != nil) {
			t.Errorf("%s: Handle error = %v, envelope errors = %+v; these must agree", toolName, err, env.Errors)
		}
	}

	t.Run("capabilities_ok", func(t *testing.T) {
		check(t, toolmeta.NameCapabilities, json.RawMessage(`{"format":"docx"}`))
	})
	t.Run("capabilities_bad_format", func(t *testing.T) {
		check(t, toolmeta.NameCapabilities, json.RawMessage(`{"format":"bogus"}`))
	})
	t.Run("compose_malformed_json", func(t *testing.T) {
		check(t, toolmeta.NameCompose, json.RawMessage(`not json`))
	})
	t.Run("skills_stub", func(t *testing.T) {
		check(t, toolmeta.NameSkills, json.RawMessage(`{}`))
	})
	t.Run("manifest_empty_args", func(t *testing.T) {
		check(t, toolmeta.NameManifest, nil)
	})
}
