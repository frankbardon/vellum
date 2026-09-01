package spec

import (
	"encoding/json"
	"fmt"
)

// SchemaID is the published identifier of the specification schema. It doubles
// as the URL the schema is served from, and carries the format version so a
// consumer pinning one version is not silently handed another.
const SchemaID = "https://frankbardon.github.io/vellum/spec-schema-" + FormatVersion + ".json"

// Schema returns the JSON Schema for a specification, as draft 2020-12.
//
// Built from the registries rather than hand-maintained. Registering a new
// block kind, cell class, value kind, annotation position or unit changes this
// schema automatically and moves the committed golden, which is how the schema
// and the model are kept from drifting apart. A hand-written schema drifts
// silently, and this same schema drives the tool contract an LLM sees — where
// drift is not a documentation problem but a wrong answer.
//
// Maps are used freely here: encoding/json sorts object keys, which makes the
// output deterministic. That is the one place in this codebase where a map on
// an output path is safe, and it is safe only because of that guarantee.
func Schema() json.RawMessage {
	schema := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  SchemaID,
		"title":                "Vellum specification",
		"description":          "A declarative description of an artifact as ordered sections of generic blocks.",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"sections"},
		"properties": map[string]any{
			"format_version": map[string]any{
				"type":        "string",
				"const":       FormatVersion,
				"description": "The wire version this specification was authored against.",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "The document title, carried into format metadata.",
			},
			"theme": map[string]any{
				"type":        "string",
				"description": "The theme document to render against. Empty selects the built-in theme.",
			},
			"sections": map[string]any{
				"type":        "array",
				"minItems":    1,
				"description": "The ordered divisions of the document.",
				"items":       map[string]any{"$ref": "#/$defs/section"},
			},
		},
		"$defs": schemaDefs(),
	}

	raw, err := json.Marshal(schema)
	if err != nil {
		// Unreachable: the structure above is composed entirely of marshalable
		// values. Kept as a panic rather than a silent empty schema, because a
		// missing schema would disable validation everywhere at once.
		panic(fmt.Sprintf("vellum/spec: building the schema failed: %v", err))
	}
	return raw
}

func schemaDefs() map[string]any {
	return map[string]any{
		"marks": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Consumer-defined style hooks. Vellum never interprets a mark; the theme maps it to a style.",
		},

		"section": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"blocks"},
			"properties": map[string]any{
				"id":     map[string]any{"type": "string"},
				"layout": map[string]any{"type": "string"},
				"marks":  map[string]any{"$ref": "#/$defs/marks"},
				"blocks": map[string]any{
					"type":     "array",
					"minItems": 1,
					"items":    map[string]any{"$ref": "#/$defs/block"},
				},
			},
		},

		"block": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"kind"},
			"description":          "A tagged union. The kind names which arm carries content, and the arm for that kind must be present.",
			"properties": map[string]any{
				"kind":       map[string]any{"enum": enumOf(AllBlockKinds())},
				"heading":    map[string]any{"$ref": "#/$defs/heading"},
				"text":       map[string]any{"$ref": "#/$defs/text"},
				"asset":      map[string]any{"$ref": "#/$defs/asset"},
				"table":      map[string]any{"$ref": "#/$defs/table"},
				"page_break": map[string]any{"$ref": "#/$defs/pageBreak"},
				"notes":      map[string]any{"$ref": "#/$defs/notes"},
				"spacer":     map[string]any{"$ref": "#/$defs/spacer"},
				"marks":      map[string]any{"$ref": "#/$defs/marks"},
			},
			// The discriminator is enforced structurally rather than only by
			// Go validation, so an agent authoring against the schema alone is
			// told which arm its kind requires.
			"allOf": blockArmConstraints(),
		},

		"heading": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"level", "content"},
			"properties": map[string]any{
				"level":   map[string]any{"type": "integer", "minimum": 1},
				"content": map[string]any{"type": "string"},
			},
		},

		"text": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"content"},
			"properties":           map[string]any{"content": map[string]any{"type": "string"}},
		},

		"asset": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"handle"},
			"description":          "References an asset by handle. Vellum never renders one and never fetches one; the host's resolver supplies the bytes.",
			"properties": map[string]any{
				"handle":   map[string]any{"type": "string", "minLength": 1},
				"role":     map[string]any{"type": "string", "description": "The theme box this asset fills, so its size comes from the theme."},
				"alt_text": map[string]any{"type": "string"},
			},
		},

		"pageBreak": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           map[string]any{},
		},

		"notes": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"content"},
			"properties":           map[string]any{"content": map[string]any{"type": "string"}},
		},

		"spacer": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"height"},
			"properties":           map[string]any{"height": map[string]any{"$ref": "#/$defs/length"}},
		},

		"length": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"value", "unit"},
			"description":          "A measurement with an explicit unit. A bare number is the classic source of a document that is right on one machine and wrong on another.",
			"properties": map[string]any{
				"value": map[string]any{"type": "number"},
				"unit":  map[string]any{"enum": enumOf(AllUnits())},
			},
		},

		"table": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"body"},
			"description":          "An analytical table: hierarchical headers on both axes, annotations attached to values, and margins distinguishable from data.",
			"properties": map[string]any{
				"column_headers": map[string]any{"$ref": "#/$defs/headerTree"},
				"row_headers":    map[string]any{"$ref": "#/$defs/headerTree"},
				"caption":        map[string]any{"type": "string"},
				"marks":          map[string]any{"$ref": "#/$defs/marks"},
				"body": map[string]any{
					"type":     "array",
					"minItems": 1,
					"items": map[string]any{
						"type":  "array",
						"items": map[string]any{"$ref": "#/$defs/cell"},
					},
				},
			},
		},

		"headerTree": map[string]any{
			"type":  "array",
			"items": map[string]any{"$ref": "#/$defs/headerNode"},
		},

		"headerNode": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"label"},
			"properties": map[string]any{
				"label":    map[string]any{"type": "string"},
				"span":     map[string]any{"type": "integer", "minimum": 0, "description": "Omit to derive from the tree shape. Stating it is checked against the children."},
				"children": map[string]any{"$ref": "#/$defs/headerTree"},
				"marks":    map[string]any{"$ref": "#/$defs/marks"},
			},
		},

		"cell": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"value":       map[string]any{"$ref": "#/$defs/value"},
				"text":        map[string]any{"type": "string"},
				"format":      map[string]any{"type": "string", "description": "An xlsx number-format code — one formatting vocabulary across every target."},
				"annotations": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/annotation"}},
				"row_span":    map[string]any{"type": "integer", "minimum": 0},
				"col_span":    map[string]any{"type": "integer", "minimum": 0},
				"class":       map[string]any{"enum": enumOf(AllCellClasses())},
				"marks":       map[string]any{"$ref": "#/$defs/marks"},
			},
		},

		"value": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"kind"},
			"properties": map[string]any{
				"kind":   map[string]any{"enum": enumOf(AllValueKinds())},
				"number": map[string]any{"type": "number"},
				"text":   map[string]any{"type": "string"},
				"bool":   map[string]any{"type": "boolean"},
				"date":   map[string]any{"type": "string", "format": "date-time"},
			},
		},

		"annotation": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"text"},
			"description":          "Attaches to a value rather than replacing it — a significance letter beside a number, not instead of it.",
			"properties": map[string]any{
				"text":     map[string]any{"type": "string"},
				"position": map[string]any{"enum": enumOf(AllAnnotationPositions())},
				"marks":    map[string]any{"$ref": "#/$defs/marks"},
			},
		},
	}
}

// blockArmConstraints expresses the discriminator: each kind requires its own
// arm. Generated from the registry so a new kind cannot be added without the
// schema demanding its content.
func blockArmConstraints() []any {
	arms := map[BlockKind]string{
		BlockHeading:   "heading",
		BlockText:      "text",
		BlockAsset:     "asset",
		BlockTable:     "table",
		BlockPageBreak: "page_break",
		BlockNotes:     "notes",
		BlockSpacer:    "spacer",
	}

	out := make([]any, 0, len(allBlockKinds))
	for _, kind := range allBlockKinds {
		arm, ok := arms[kind]
		if !ok {
			// A kind with no arm mapping is a bug that would otherwise produce
			// a schema quietly missing a constraint.
			panic(fmt.Sprintf("vellum/spec: block kind %q has no schema arm", kind))
		}
		out = append(out, map[string]any{
			"if":   map[string]any{"properties": map[string]any{"kind": map[string]any{"const": string(kind)}}, "required": []any{"kind"}},
			"then": map[string]any{"required": []any{arm}},
		})
	}
	return out
}

// enumOf renders a registry as a JSON Schema enum. Generic over the string
// kinds so a registry cannot be surfaced with the wrong element type.
func enumOf[T ~string](values []T) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}
