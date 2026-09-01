package descriptor

import (
	"encoding/json"
	"fmt"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/spec"
)

// PayloadSchemaID is the published identifier of the payload schema, carrying
// the format version so a consumer pinning one version is not handed another.
const PayloadSchemaID = "https://frankbardon.github.io/vellum/payload-schema-" + FormatVersion + ".json"

// BuildPayloadSchema returns the JSON Schema for everything on Vellum's wire:
// the envelope, the specification, and the request shapes.
//
// It embeds the specification schema rather than restating it, so there is one
// definition of a block and not two that can disagree. The same document drives
// runtime validation and the tool contract an agent authors against — where a
// drift is not a documentation problem but a wrong answer.
func BuildPayloadSchema() json.RawMessage {
	var specSchema map[string]any
	if err := json.Unmarshal(spec.Schema(), &specSchema); err != nil {
		panic(fmt.Sprintf("vellum/descriptor: the specification schema is not valid JSON: %v", err))
	}

	// The embedded copy keeps its own $defs but drops the $schema and $id
	// keywords, which belong to the document that carries it rather than to a
	// definition inside one.
	specDefs, _ := specSchema["$defs"].(map[string]any)
	delete(specSchema, "$schema")
	delete(specSchema, "$id")
	delete(specSchema, "$defs")

	defs := map[string]any{
		"spec": specSchema,

		"envelope": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"format_version", "data", "errors", "warnings"},
			"description": "Every JSON payload Vellum emits. Errors and warnings are always present as arrays, " +
				"never null and never omitted, so a consumer can iterate them without a guard.",
			"properties": map[string]any{
				"format_version": map[string]any{"type": "string", "const": FormatVersion},
				"data":           map[string]any{"description": "The payload. Not trustworthy when errors is non-empty."},
				"request":        map[string]any{"description": "The originating request, echoed only when asked for."},
				"errors": map[string]any{
					"type":  "array",
					"items": map[string]any{"$ref": "#/$defs/entry"},
				},
				"warnings": map[string]any{
					"type":        "array",
					"items":       map[string]any{"$ref": "#/$defs/entry"},
					"description": "Decisions Vellum made on the caller's behalf — a font substituted, a block degraded to a format's stated alternative. Reported so that none of them is silent.",
				},
			},
		},

		"entry": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"code", "message"},
			"properties": map[string]any{
				"code":    map[string]any{"type": "string", "pattern": "^VELLUM_[A-Z0-9_]+$"},
				"message": map[string]any{"type": "string"},
				"details": map[string]any{"type": "object", "description": "Structured context — the offending block, part, anchor or coordinate."},
			},
		},

		"format": map[string]any{
			"enum":        enumOf(artifact.AllFormats()),
			"description": "An output format.",
		},

		"composeRequest": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"spec", "format"},
			"properties": map[string]any{
				"spec":   map[string]any{"$ref": "#/$defs/spec"},
				"format": map[string]any{"$ref": "#/$defs/format"},
				"theme":  map[string]any{"type": "string", "description": "Overrides the theme the specification names."},
			},
		},

		"validateRequest": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"spec", "format"},
			"properties": map[string]any{
				"spec":   map[string]any{"$ref": "#/$defs/spec"},
				"format": map[string]any{"$ref": "#/$defs/format"},
			},
		},

		"validateResult": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"ok"},
			"properties": map[string]any{
				"ok":           map[string]any{"type": "boolean"},
				"rejections":   map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/capabilityFault"}},
				"degradations": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/capabilityFault"}},
			},
		},

		"capabilityFault": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"feature", "outcome", "code", "section_index", "block_index", "kind"},
			"properties": map[string]any{
				"feature":       map[string]any{"type": "string"},
				"outcome":       map[string]any{"type": "string"},
				"degrade":       map[string]any{"type": "string"},
				"code":          map[string]any{"type": "string"},
				"section_index": map[string]any{"type": "integer"},
				"section_id":    map[string]any{"type": "string"},
				"block_index":   map[string]any{"type": "integer"},
				"kind":          map[string]any{"type": "string"},
			},
		},
	}

	// Merge the specification's own definitions in, so its internal $ref
	// targets still resolve inside this document.
	for name, def := range specDefs {
		if _, clash := defs[name]; clash {
			panic(fmt.Sprintf("vellum/descriptor: schema definition %q is declared by both the payload schema and the specification schema", name))
		}
		defs[name] = def
	}

	schema := map[string]any{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"$id":         PayloadSchemaID,
		"title":       "Vellum payload contract",
		"description": "The envelope, the specification, and the request and result shapes on Vellum's wire.",
		"$defs":       defs,
	}

	raw, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Sprintf("vellum/descriptor: building the payload schema failed: %v", err))
	}
	return raw
}

func enumOf[T ~string](values []T) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}
