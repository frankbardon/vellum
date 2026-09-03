package descriptor

import (
	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/capability"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/mcp/toolmeta"
	"github.com/frankbardon/vellum/spec"
)

// Manifest describes what Vellum can do.
//
// It is assembled from the live registries, never hand-maintained, so
// registering a new block kind or format changes it automatically and moves
// the committed golden. That is the whole mechanism by which the manifest
// cannot drift from the code it describes.
//
// It is also the first thing an agent reads. The prose here is written for
// that reader.
type Manifest struct {
	FormatVersion string `json:"format_version"`

	// Description is a one-paragraph account of what the library is, for a
	// reader who has only this document.
	Description string `json:"description"`

	// Formats are the output formats.
	Formats []FormatInfo `json:"formats"`

	// Blocks are the block kinds a specification is composed from.
	Blocks []BlockInfo `json:"blocks"`

	// Capabilities is the full feature-by-format matrix.
	Capabilities capability.Matrix `json:"capabilities"`

	// Vocabularies are the closed enumerations a specification draws on.
	Vocabularies Vocabularies `json:"vocabularies"`

	// Operations are the things a caller can ask for.
	Operations []OperationInfo `json:"operations"`

	// MCPTools are the tools the MCP surface registers.
	MCPTools []MCPToolInfo `json:"mcp_tools"`

	// ErrorDomains and ErrorCodes describe the failure surface. Only names
	// here: the prose for a code is fetched on demand, so the manifest an
	// agent loads at the start of a session stays small.
	ErrorDomains    []string `json:"error_domains"`
	ErrorCodes      []string `json:"error_codes"`
	ErrorCodesCount int      `json:"error_codes_count"`

	// SchemaID is where the payload schema is published.
	SchemaID string `json:"schema_id"`
}

// FormatInfo describes one output format.
type FormatInfo struct {
	Name      artifact.Format `json:"name"`
	Extension string          `json:"extension"`
	OOXML     bool            `json:"ooxml"`
	Renders   []string        `json:"renders"`
	Note      string          `json:"note,omitempty"`
}

// BlockInfo describes one block kind.
type BlockInfo struct {
	Kind    spec.BlockKind `json:"kind"`
	Purpose string         `json:"purpose"`
}

// Vocabularies are the closed enumerations.
type Vocabularies struct {
	CellClasses         []spec.CellClass          `json:"cell_classes"`
	ValueKinds          []spec.ValueKind          `json:"value_kinds"`
	AnnotationPositions []spec.AnnotationPosition `json:"annotation_positions"`
	Units               []spec.Unit               `json:"units"`
	Outcomes            []capability.Outcome      `json:"capability_outcomes"`
}

// MCPToolInfo describes one registered MCP tool.
type MCPToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// OperationInfo describes one thing a caller can ask Vellum to do.
type OperationInfo struct {
	Name        string `json:"name"`
	Summary     string `json:"summary"`
	NeedsRender bool   `json:"needs_render"`
}

// BuildManifest assembles the manifest from the live registries.
func BuildManifest() *Manifest {
	m := &Manifest{
		FormatVersion: FormatVersion,
		Description: "Vellum is a declarative artifact emitter: a specification of generic blocks in, a document out. " +
			"It emits DOCX, XLSX, PPTX and PDF/A-2b from one model, and fills existing OOXML templates without disturbing " +
			"the parts it does not understand. Identical inputs produce byte-identical outputs. Vellum never renders a " +
			"chart, never performs network I/O, and never runs an external binary.",
		SchemaID:     spec.SchemaID,
		Capabilities: capability.All(),
		Vocabularies: Vocabularies{
			CellClasses:         spec.AllCellClasses(),
			ValueKinds:          spec.AllValueKinds(),
			AnnotationPositions: spec.AllAnnotationPositions(),
			Units:               spec.AllUnits(),
			Outcomes:            capability.AllOutcomes(),
		},
		Operations: operations(),
	}

	for _, f := range artifact.AllFormats() {
		renders := make([]string, 0, len(capability.AllFeatures()))
		for _, feat := range capability.Profile(f) {
			renders = append(renders, string(feat))
		}
		m.Formats = append(m.Formats, FormatInfo{
			Name:      f,
			Extension: f.Extension(),
			OOXML:     f.IsOOXML(),
			Renders:   renders,
			Note:      formatNotes[f],
		})
	}

	for _, k := range spec.AllBlockKinds() {
		m.Blocks = append(m.Blocks, BlockInfo{Kind: k, Purpose: blockPurposes[k]})
	}

	m.ErrorDomains = verr.AllDomains()
	for _, c := range verr.AllCodes() {
		m.ErrorCodes = append(m.ErrorCodes, string(c))
	}
	m.ErrorCodesCount = len(m.ErrorCodes)

	for _, t := range toolmeta.AllTools() {
		m.MCPTools = append(m.MCPTools, MCPToolInfo{Name: t.Name, Description: t.Description})
	}

	return m
}

// formatNotes carries the one thing a reader most needs to know about each
// format that the capability rows do not already say.
var formatNotes = map[artifact.Format]string{
	artifact.FormatDOCX: "A flowing document. Word paginates it, so Vellum does not compute page breaks.",
	artifact.FormatXLSX: "Presentation tables, not a spreadsheet: no formulas, no pivot tables, no macros. A consumer wanting a live model builds it elsewhere.",
	artifact.FormatPPTX: "A deck. The only format with a native speaker-note channel, and the only one where Vellum computes the overflow split itself.",
	artifact.FormatPDF:  "PDF/A-2b, emitted directly rather than converted. Every font is embedded, because the archival profile requires it.",
}

// blockPurposes describes each kind in the terms a caller composes in.
var blockPurposes = map[spec.BlockKind]string{
	spec.BlockHeading:   "A titled division of the content. Level 1 is the most prominent.",
	spec.BlockText:      "A paragraph of prose.",
	spec.BlockAsset:     "An artifact the host resolves by handle. Vellum embeds it; it never renders one. Ask Boxes what size to render at.",
	spec.BlockTable:     "An analytical table: hierarchical headers on both axes, annotations attached to values, margins distinguishable from data.",
	spec.BlockPageBreak: "Starts a new page, slide or sheet, depending on the format.",
	spec.BlockNotes:     "Annotation content. Native in a deck; a footnote or a cell comment elsewhere, as the capability matrix declares.",
	spec.BlockSpacer:    "Vertical space.",
}

// operations lists what a caller can ask for, and — importantly — which
// answers need no render at all.
func operations() []OperationInfo {
	return []OperationInfo{
		{Name: "validate", Summary: "Check a specification against a format's capability matrix. Reports every rejection at once.", NeedsRender: false},
		{Name: "capabilities", Summary: "Return the feature-by-format matrix, so a scheduled job can learn what will happen before it runs.", NeedsRender: false},
		{Name: "schema", Summary: "Return the published JSON Schema for a specification.", NeedsRender: false},
		{Name: "manifest", Summary: "Return this document.", NeedsRender: false},
		{Name: "hash", Summary: "Return a specification's content hash. Derived from inputs, so an artifact's identity is knowable before it is produced.", NeedsRender: false},
		{Name: "boxes", Summary: "Return the asset slots a theme declares for a format, so a host knows what size to render an asset at. Answerable before a specification exists.", NeedsRender: false},
		{Name: "compose", Summary: "Render a specification to a format.", NeedsRender: true},
		{Name: "inspect", Summary: "Report a template's anchor inventory and font requirements without modifying it.", NeedsRender: false},
		{Name: "fill", Summary: "Bind data into an OOXML template, leaving every part it did not touch byte-identical.", NeedsRender: true},
	}
}
