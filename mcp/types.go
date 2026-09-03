package mcp

import (
	"encoding/json"

	"github.com/frankbardon/vellum/capability"
	"github.com/frankbardon/vellum/descriptor"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/template"
	"github.com/frankbardon/vellum/theme"
)

// The In/Out pair below is minimal and JSON-tagged, mirroring what
// internal/cli's own verb handlers already parse and produce for the same
// operation — compose.go, validate.go, fill.go, inspect.go, boxes.go,
// capabilities.go and schema.go each do the CLI-flag-and-file-path version
// of exactly this decode. Binary payloads (a specification's own JSON, a
// template, a filled package) travel as []byte, which encoding/json already
// base64-encodes and decodes on the wire — no separate encoding scheme
// invented here.

// ComposeIn is [toolmeta.NameCompose]'s input: a specification (JSON or
// YAML — [spec.DecodeAuto] auto-detects, exactly as compose.go's CLI verb
// does) and the format to render it to.
type ComposeIn struct {
	Spec   json.RawMessage `json:"spec"`
	Format string          `json:"format"`
}

// ComposeOut is [toolmeta.NameCompose]'s output: the format actually
// written, the rendered artifact's bytes, and every warning resolution
// raised — [artifact.Report] flattened onto one struct rather than nested,
// so the wire shape names its own fields directly.
type ComposeOut struct {
	Format   string             `json:"format"`
	Artifact []byte             `json:"artifact"`
	Warnings []*verr.CodedError `json:"warnings"`
}

// ValidateIn is [toolmeta.NameValidate]'s input: the same (spec, format)
// pair ComposeIn carries, since Validate runs the same resolution Compose
// does and stops one step earlier.
type ValidateIn struct {
	Spec   json.RawMessage `json:"spec"`
	Format string          `json:"format"`
}

// ValidateOut is [toolmeta.NameValidate]'s output — see validateResult in
// internal/cli/validate.go, which this mirrors: Valid is carried explicitly
// rather than as "data": null, so a client can branch on it without
// inspecting the envelope's own top-level errors array.
type ValidateOut struct {
	Valid    bool               `json:"valid"`
	Warnings []*verr.CodedError `json:"warnings"`
}

// InspectIn is [toolmeta.NameInspect]'s input: an OOXML template's raw
// bytes.
type InspectIn struct {
	Template []byte `json:"template"`
}

// InspectOut is [toolmeta.NameInspect]'s output. It is exactly
// [template.InspectReport] — already JSON-tagged for this purpose, since
// internal/cli's own inspect verb writes it through the envelope unchanged
// — aliased rather than duplicated, so the two transports cannot drift
// apart on this shape.
type InspectOut = template.InspectReport

// FillIn is [toolmeta.NameFill]'s input: a template's raw bytes, a binding
// document (JSON or YAML — [bind.DecodeAuto] auto-detects), and the JSON
// data it evaluates against.
type FillIn struct {
	Template []byte          `json:"template"`
	Binding  json.RawMessage `json:"binding"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// FillOut is [toolmeta.NameFill]'s output: the filled package's raw bytes
// and [template.Result.Touched], the non-destructiveness receipt — every
// part outside Touched is byte-identical to Template.
type FillOut struct {
	Package []byte   `json:"package"`
	Touched []string `json:"touched"`
}

// CapabilitiesIn is [toolmeta.NameCapabilities]'s input: the format to
// report the matrix for.
type CapabilitiesIn struct {
	Format string `json:"format"`
}

// CapabilitiesOut is [toolmeta.NameCapabilities]'s output: the declared
// (feature, outcome) rows for the requested format.
type CapabilitiesOut struct {
	Capabilities capability.Matrix `json:"capabilities"`
}

// BoxesIn is [toolmeta.NameBoxes]'s input: a theme id (empty selects the
// built-in theme) and the format to report boxes for.
type BoxesIn struct {
	Theme  string `json:"theme,omitempty"`
	Format string `json:"format"`
}

// BoxesOut is [toolmeta.NameBoxes]'s output: the asset slots the theme
// offers the requested format.
type BoxesOut struct {
	Boxes theme.BoxSet `json:"boxes"`
}

// SchemaIn is [toolmeta.NameSchema]'s input. Empty: the published schema
// does not depend on anything a caller supplies, matching the CLI's own
// schema verb, which accepts no flags at all.
type SchemaIn struct{}

// SchemaOut is [toolmeta.NameSchema]'s output: the raw JSON Schema
// [descriptor.BuildPayloadSchema] publishes, carried as a field rather than
// as the tool's entire envelope data — CLAUDE.md's documented exception for
// vellum schema ("writes the raw JSON Schema unwrapped") is a CLI-transport
// carve-out for a stream that otherwise always carries an envelope; MCP has
// no unwrapped mode, so this tool's result is an envelope like every other
// tool's, with the schema as its data's one field.
type SchemaOut struct {
	Schema json.RawMessage `json:"schema"`
}

// ManifestIn is [toolmeta.NameManifest]'s input. Empty, for the same reason
// SchemaIn is: the manifest is a fixed description of the live registries.
type ManifestIn struct{}

// ManifestOut is [toolmeta.NameManifest]'s output: [descriptor.Manifest]
// carried as a field of this tool's data, matching SchemaOut's own shape
// rather than handing the manifest back as the envelope's entire data value
// — every tool's data is a struct with named fields, none bare.
type ManifestOut struct {
	Manifest *descriptor.Manifest `json:"manifest"`
}

// SkillsIn is [toolmeta.NameSkills]'s input: the skill document's name (a
// [skills.Doc.Stem] value, e.g. "block-heading"). Empty selects the listing
// form — see [SkillsOut].
type SkillsIn struct {
	Name string `json:"name,omitempty"`
}

// SkillsOut is [toolmeta.NameSkills]'s output. A non-empty [SkillsIn.Name]
// that matched a document returns Content — the whole file, frontmatter and
// body, exactly as embedded ([skills.Doc.Raw]) — and leaves Names empty. An
// empty Name returns every available document's [skills.Doc.Stem] in Names,
// sorted, and leaves Content empty. The two fields are mutually exclusive on
// the wire (both "omitempty") so a client can tell which form it got by
// which field is present, rather than by whether Name was empty on the way
// in.
type SkillsOut struct {
	Content string   `json:"content,omitempty"`
	Names   []string `json:"names,omitempty"`
}

// ExamplesIn is [toolmeta.NameExamples]'s input: the example specification's
// name (an [examples.Doc.Stem] value, e.g. "block-heading"). Empty selects
// the listing form — see [ExamplesOut].
type ExamplesIn struct {
	Name string `json:"name,omitempty"`
}

// ExamplesOut is [toolmeta.NameExamples]'s output, mirroring [SkillsOut]'s
// own shape: Content carries one document's raw bytes ([examples.Doc.Raw])
// as a string — encoding/json's own []byte-to-string conversion, since a
// spec.Spec or a bind.Binding is textual JSON, not a binary payload — found
// by name; Names carries every available document's [examples.Doc.Stem],
// sorted, when Name was empty. examples.Doc carries no description field
// (unlike skills.Doc.Frontmatter), so the listing is names only, keeping the
// two tools' listing shape identical rather than one carrying descriptions
// the other cannot.
type ExamplesOut struct {
	Content string   `json:"content,omitempty"`
	Names   []string `json:"names,omitempty"`
}
