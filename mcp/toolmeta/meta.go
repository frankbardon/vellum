// Package toolmeta names every tool the MCP surface registers: one constant
// pair (name, description) per tool, plus a small ordered registry. It has
// zero dependencies — not even on "github.com/frankbardon/vellum/errors" —
// so it is importable from anywhere a tool's name needs to be known, without
// pulling in schema reflection, the facade, or the SDK. That includes
// skills/, once E13 builds a coverage gate ("every vellum_* tool has a
// skills/tool-<kebab>.md") that needs this registry to check against.
//
// Per CLAUDE.md's Update Demand table, a tool name carries the "vellum_"
// prefix; the skill file that documents it does not. AllTools returns the
// full set, copy-returning and in declaration order, in the style every
// other registry in this codebase already uses (errors.AllCodes,
// capability.AllFeatures, bind.AllBannedBuiltins).
package toolmeta

// ToolMeta is one tool's name and one-line description — the contract shape
// every MCP client sees from tools/list before it ever calls the tool.
type ToolMeta struct {
	// Name is the tool's registered name, always "vellum_" prefixed.
	Name string

	// Description is a one-line, human- and LLM-readable summary of what the
	// tool does — the "hint" a client shows an operator or feeds a model
	// deciding whether to call it.
	Description string
}

// The registered tool names. Per FR-U3: "Tools covering compose, validate,
// inspect, fill, capabilities, boxes, schema, manifest, skills and
// examples" — ten tools, one per name below, in that same order.
const (
	NameCompose      = "vellum_compose"
	NameValidate     = "vellum_validate"
	NameInspect      = "vellum_inspect"
	NameFill         = "vellum_fill"
	NameCapabilities = "vellum_capabilities"
	NameBoxes        = "vellum_boxes"
	NameSchema       = "vellum_schema"
	NameManifest     = "vellum_manifest"
	NameSkills       = "vellum_skills"
	NameExamples     = "vellum_examples"
)

// Compose, Validate, Inspect, Fill, Capabilities, Boxes, Schema, Manifest,
// Skills and Examples are the registered [ToolMeta] values, in FR-U3's own
// order — the same order [AllTools] returns them in.
//
// Skills and Examples serve the go:embed packs under skills/ and examples/
// directly: a name looks one document up by its stem, and an empty name
// lists every document the pack carries.
var (
	Compose = ToolMeta{
		Name:        NameCompose,
		Description: "Render a specification to an artifact (DOCX, XLSX, PPTX or PDF/A-2b).",
	}
	Validate = ToolMeta{
		Name:        NameValidate,
		Description: "Check a specification against a format's capability matrix, without rendering.",
	}
	Inspect = ToolMeta{
		Name:        NameInspect,
		Description: "Report an OOXML template's anchor inventory and font requirements, without modifying it.",
	}
	Fill = ToolMeta{
		Name:        NameFill,
		Description: "Bind data into an OOXML template, leaving every part it did not touch byte-identical.",
	}
	Capabilities = ToolMeta{
		Name:        NameCapabilities,
		Description: "Return the declared (feature, format) outcome matrix: renders, degrades, or rejects.",
	}
	Boxes = ToolMeta{
		Name:        NameBoxes,
		Description: "Return the asset slots a theme offers a format, answerable before any specification exists.",
	}
	Schema = ToolMeta{
		Name:        NameSchema,
		Description: "Return the published JSON Schema for a specification.",
	}
	Manifest = ToolMeta{
		Name:        NameManifest,
		Description: "Return the manifest describing what Vellum can do: formats, block kinds, capabilities and error codes.",
	}
	Skills = ToolMeta{
		Name:        NameSkills,
		Description: "Read a skill document from the embedded skill pack by name, or list every available name when none is given.",
	}
	Examples = ToolMeta{
		Name:        NameExamples,
		Description: "Read an example specification from the embedded example pack by name, or list every available name when none is given.",
	}
)

// registry is the ordered set every AllTools call copies from. Declared
// separately from the named vars above (rather than deriving the vars from
// it) so each tool keeps a stable, directly-referenceable identifier
// ([Compose], [Fill], and so on) for a caller building a catalog entry by
// name, the same shape [ToolMeta] pairs it with.
var registry = []ToolMeta{
	Compose,
	Validate,
	Inspect,
	Fill,
	Capabilities,
	Boxes,
	Schema,
	Manifest,
	Skills,
	Examples,
}

// AllTools returns every registered tool, in declaration order. Copy-
// returning, in the style of errors.AllCodes and capability.AllFeatures: the
// registry backs the MCP catalog, so a caller that could mutate it could
// change what a client sees between calls.
func AllTools() []ToolMeta {
	out := make([]ToolMeta, len(registry))
	copy(out, registry)
	return out
}
