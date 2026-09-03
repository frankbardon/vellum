package mcp

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/frankbardon/vellum"
	"github.com/frankbardon/vellum/descriptor"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/mcp/toolmeta"
)

// Tool is one registered tool's type-erased shape: everything a transport
// adapter needs to mount it, with the typed In/Out both erased to raw JSON
// at the boundary. This is the uniform shape [github.com/frankbardon/vellum/mcp/gosdk]
// iterates to register the whole catalog with the SDK server without any
// per-tool special-casing — the "type-erased catalog" CLAUDE.md's bootstrap
// plan names.
type Tool struct {
	// Meta is the tool's registered name and description.
	Meta toolmeta.ToolMeta

	// InputSchema and OutputSchema are this tool's reflected JSON Schema —
	// computed once, at package init (see schema.go), never per call.
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage

	// Handle runs the tool against raw, JSON-encoded arguments and returns
	// raw, JSON-encoded output.
	//
	// The returned json.RawMessage is always a [descriptor.Envelope]-shaped
	// document — on success and on failure alike, per CLAUDE.md's "every MCP
	// output path goes through descriptor.NewEnvelope". The returned error
	// is non-nil exactly when that envelope carries at least one error entry
	// (mirrors [descriptor.Envelope.OK]), which is what lets a caller decide
	// whether to flag the result as a tool error without re-parsing the JSON
	// it already has.
	Handle func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error)
}

// NewCatalog builds the full tool catalog bound to v: one [Tool] per
// registered [toolmeta.ToolMeta], in [toolmeta.AllTools]'s own order, so two
// calls against the same facade produce identically ordered catalogs and a
// client listing tools twice sees the same order both times.
//
// v is threaded through once, here, rather than looked up per call: every
// handler in handlers.go is a pure function of (ctx, *vellum.Vellum, In), and
// binding it to v is this function's entire job.
func NewCatalog(v *vellum.Vellum) []Tool {
	return []Tool{
		newTool(toolmeta.Compose, composeInputSchema, composeOutputSchema, v, handleCompose),
		newTool(toolmeta.Validate, validateInputSchema, validateOutputSchema, v, handleValidate),
		newTool(toolmeta.Inspect, inspectInputSchema, inspectOutputSchema, v, handleInspect),
		newTool(toolmeta.Fill, fillInputSchema, fillOutputSchema, v, handleFill),
		newTool(toolmeta.Capabilities, capabilitiesInputSchema, capabilitiesOutputSchema, v, handleCapabilities),
		newTool(toolmeta.Boxes, boxesInputSchema, boxesOutputSchema, v, handleBoxes),
		newTool(toolmeta.Schema, schemaInputSchema, schemaOutputSchema, v, handleSchema),
		newTool(toolmeta.Manifest, manifestInputSchema, manifestOutputSchema, v, handleManifest),
		newTool(toolmeta.Skills, skillsInputSchema, skillsOutputSchema, v, handleSkills),
		newTool(toolmeta.Examples, examplesInputSchema, examplesOutputSchema, v, handleExamples),
	}
}

// newTool is the type-erasure boundary: it closes a typed handler function
// over v and wraps it into [Tool.Handle]'s uniform (raw json in, raw json
// out) shape, instantiated once per tool by [NewCatalog].
func newTool[In, Out any](
	meta toolmeta.ToolMeta,
	inputSchema, outputSchema json.RawMessage,
	v *vellum.Vellum,
	fn func(ctx context.Context, v *vellum.Vellum, in In) (Out, error),
) Tool {
	return Tool{
		Meta:         meta,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		Handle:       bindHandler(meta.Name, v, fn),
	}
}

// bindHandler decodes raw against In, calls fn, and wraps the result — or
// any failure along the way, decode included — in a [descriptor.Envelope],
// per [Tool.Handle]'s own contract.
func bindHandler[In, Out any](
	name string,
	v *vellum.Vellum,
	fn func(ctx context.Context, v *vellum.Vellum, in In) (Out, error),
) func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	return func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
		payload := raw
		if len(bytes.TrimSpace(payload)) == 0 {
			payload = []byte("{}")
		}

		var in In
		if err := json.Unmarshal(payload, &in); err != nil {
			return envelopeError(verr.NewCodedErrorWithDetails(verr.VELLUM_MCP_INVALID_INPUT,
				"the tool input does not decode against its schema",
				map[string]any{"tool": name, "error": err.Error()}))
		}

		out, err := fn(ctx, v, in)
		if err != nil {
			return envelopeError(err)
		}
		return envelopeOK(out)
	}
}

// envelopeOK wraps data as a successful envelope.
func envelopeOK(data any) (json.RawMessage, error) {
	raw, err := json.Marshal(descriptor.NewEnvelope(data))
	if err != nil {
		return envelopeError(verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"tool output failed to marshal", map[string]any{"error": err.Error()}))
	}
	return raw, nil
}

// envelopeError wraps err as a failed envelope and returns err alongside it,
// per [Tool.Handle]'s contract that the error return is non-nil exactly when
// the envelope carries one.
func envelopeError(err error) (json.RawMessage, error) {
	env := descriptor.NewEnvelope(nil)
	env.AddCodedError(err)
	raw, merr := json.Marshal(env)
	if merr != nil {
		// Unreachable in practice: env's fields are all plain, already-JSON-
		// safe values (strings, a details map descriptor.Entry already
		// marshals elsewhere). Guarded rather than panicking, because a tool
		// call is exactly the place a library must not panic — a hand-built
		// minimal envelope is a truthful degraded answer, not a crash.
		raw = []byte(`{"format_version":"` + descriptor.FormatVersion +
			`","data":null,"errors":[{"code":"VELLUM_INTERNAL_INVARIANT","message":"the error envelope itself failed to marshal"}],"warnings":[]}`)
	}
	return raw, err
}

// Lookup returns the tool named name from catalog, and whether the catalog
// registers one.
func Lookup(catalog []Tool, name string) (Tool, bool) {
	for _, t := range catalog {
		if t.Meta.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

// Dispatch looks up name in catalog and runs it against raw, or returns a
// VELLUM_MCP_UNKNOWN_TOOL envelope when no tool in catalog is registered
// under that name. It is not on the [mcp/gosdk] hot path — the SDK routes a
// call to the handler [github.com/frankbardon/vellum/mcp/gosdk.Register]
// mounted it under, so an unknown tool name never reaches this package
// through that route at all — but it is the one place VELLUM_MCP_UNKNOWN_TOOL
// is actually raised, for a caller (a test, a future direct-dispatch
// transport) that has a catalog and a bare name rather than an SDK server
// already wired to one.
func Dispatch(ctx context.Context, catalog []Tool, name string, raw json.RawMessage) (json.RawMessage, error) {
	t, ok := Lookup(catalog, name)
	if !ok {
		names := make([]string, len(catalog))
		for i, c := range catalog {
			names[i] = c.Meta.Name
		}
		return envelopeError(verr.NewCodedErrorWithDetails(verr.VELLUM_MCP_UNKNOWN_TOOL,
			"the named tool is not in the registered catalog",
			map[string]any{"tool": name, "registered": names}))
	}
	return t.Handle(ctx, raw)
}
