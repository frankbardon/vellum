package spec_test

import (
	"encoding/json"
	"strings"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/spec"
)

const validJSON = `{
  "format_version": "1.0",
  "title": "Quarterly Review",
  "sections": [
    {
      "id": "findings",
      "blocks": [
        {"kind": "heading", "heading": {"level": 1, "content": "Findings"}},
        {"kind": "text", "text": {"content": "Awareness rose."}, "marks": ["stale"]},
        {"kind": "table", "table": {
          "column_headers": [{"label": "Region", "children": [{"label": "North"}, {"label": "South"}]}],
          "body": [[{"value": {"kind": "number", "number": 10}}, {"value": {"kind": "number", "number": 20}}]]
        }},
        {"kind": "spacer", "spacer": {"height": {"value": 12, "unit": "pt"}}}
      ]
    }
  ]
}`

const validYAML = `
format_version: "1.0"
title: Quarterly Review
sections:
  - id: findings
    blocks:
      - kind: heading
        heading:
          level: 1
          content: Findings
      - kind: text
        text:
          content: Awareness rose.
        marks: [stale]
      - kind: table
        table:
          column_headers:
            - label: Region
              children:
                - label: North
                - label: South
          body:
            - - value: {kind: number, number: 10}
              - value: {kind: number, number: 20}
      - kind: spacer
        spacer:
          height: {value: 12, unit: pt}
`

func TestDecode_Valid(t *testing.T) {
	s, err := spec.Decode([]byte(validJSON))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if s.Title != "Quarterly Review" {
		t.Errorf("Title = %q", s.Title)
	}
	if len(s.Sections) != 1 || len(s.Sections[0].Blocks) != 4 {
		t.Fatalf("unexpected shape: %d sections", len(s.Sections))
	}
	if s.Sections[0].Blocks[0].Heading.Content != "Findings" {
		t.Errorf("heading did not decode")
	}
}

// TestDecode_YAMLAndJSONAgree is the property that makes the authoring surface
// leave no trace: the same logical specification hashes identically whichever
// way it was written.
func TestDecode_YAMLAndJSONAgree(t *testing.T) {
	fromJSON, err := spec.Decode([]byte(validJSON))
	if err != nil {
		t.Fatalf("Decode JSON: %v", err)
	}
	fromYAML, err := spec.DecodeYAML([]byte(validYAML))
	if err != nil {
		t.Fatalf("Decode YAML: %v", err)
	}

	if fromJSON.Hash() != fromYAML.Hash() {
		jj, _ := fromJSON.CanonicalJSON()
		yj, _ := fromYAML.CanonicalJSON()
		t.Errorf("YAML and JSON authoring produced different hashes\njson: %s\nyaml: %s", jj, yj)
	}
}

func TestDecodeAuto_HandlesBoth(t *testing.T) {
	a, err := spec.DecodeAuto([]byte(validJSON))
	if err != nil {
		t.Fatalf("DecodeAuto JSON: %v", err)
	}
	b, err := spec.DecodeAuto([]byte(validYAML))
	if err != nil {
		t.Fatalf("DecodeAuto YAML: %v", err)
	}
	if a.Hash() != b.Hash() {
		t.Error("DecodeAuto produced different results for the same document in two syntaxes")
	}
}

// TestDecode_RejectsUnknownFields is the reason there is no lenient mode. A
// silently dropped field gives a model no signal that its output was partially
// ignored: it sees a document that renders and a section that is missing, and
// cannot connect the two.
func TestDecode_RejectsUnknownFields(t *testing.T) {
	tests := []struct {
		name  string
		input string
		field string
	}{
		{
			name:  "top level",
			input: `{"sections":[{"blocks":[{"kind":"text","text":{"content":"x"}}]}],"colour_scheme":"dark"}`,
			field: "colour_scheme",
		},
		{
			name:  "inside a section",
			input: `{"sections":[{"blocks":[{"kind":"text","text":{"content":"x"}}],"columns":2}]}`,
			field: "columns",
		},
		{
			name:  "inside a block",
			input: `{"sections":[{"blocks":[{"kind":"text","text":{"content":"x"},"emphasis":true}]}]}`,
			field: "emphasis",
		},
		{
			name:  "inside a cell",
			input: `{"sections":[{"blocks":[{"kind":"table","table":{"body":[[{"text":"x","bold":true}]]}}]}]}`,
			field: "bold",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := spec.Decode([]byte(tt.input))
			if !verr.HasCode(err, verr.VELLUM_SPEC_INVALID) {
				t.Fatalf("error = %v, want VELLUM_SPEC_INVALID", err)
			}
			if !strings.Contains(err.Error()+detailString(err), tt.field) {
				t.Errorf("the error does not name the offending field %q: %v", tt.field, err)
			}
		})
	}
}

func TestDecode_RejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ``},
		{"not JSON", `this is not JSON at all`},
		{"truncated", `{"sections":[`},
		{"two documents", `{"sections":[{"blocks":[{"kind":"text","text":{"content":"x"}}]}]}{"sections":[]}`},
		{"no sections", `{}`},
		{"empty sections", `{"sections":[]}`},
		{"unknown kind", `{"sections":[{"blocks":[{"kind":"sidebar"}]}]}`},
		{"heading level zero", `{"sections":[{"blocks":[{"kind":"heading","heading":{"level":0,"content":"x"}}]}]}`},
		{"missing arm", `{"sections":[{"blocks":[{"kind":"heading"}]}]}`},
		{"bad unit", `{"sections":[{"blocks":[{"kind":"spacer","spacer":{"height":{"value":1,"unit":"furlong"}}}]}]}`},
		{"wrong format version", `{"format_version":"9.9","sections":[{"blocks":[{"kind":"text","text":{"content":"x"}}]}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := spec.Decode([]byte(tt.input)); !verr.HasCode(err, verr.VELLUM_SPEC_INVALID) {
				t.Errorf("error = %v, want VELLUM_SPEC_INVALID", err)
			}
		})
	}
}

// TestDecode_CollectsEveryFault checks that an author fixing a specification
// is not made to do it one error per run.
func TestDecode_CollectsEveryFault(t *testing.T) {
	input := `{"sections":[{"blocks":[
		{"kind":"heading","heading":{"level":0,"content":"x"}},
		{"kind":"nonsense"},
		{"kind":"spacer","spacer":{"height":{"value":1,"unit":"furlong"}}}
	]}]}`

	_, err := spec.Decode([]byte(input))
	if err == nil {
		t.Fatal("Decode succeeded on a document with three faults")
	}
	ce, _ := err.(*verr.CodedError)
	if ce == nil {
		t.Fatalf("error is not a CodedError: %v", err)
	}
	count, ok := ce.Detail("fault_count")
	if !ok {
		t.Fatalf("the error reports no fault count: %v", ce.Details)
	}
	if n, _ := count.(int); n < 3 {
		t.Errorf("fault_count = %v, want at least 3; faults must be collected rather than reported one per run", count)
	}
}

// TestDecode_RejectsYAMLThatCannotRoundTrip covers the YAML features that have
// no JSON equivalent. Accepting them would mean the canonical form silently
// differed from what the author wrote.
func TestDecode_RejectsYAMLThatCannotRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "duplicate keys",
			input: "sections:\n  - blocks: []\nsections:\n  - blocks: []\n",
		},
		{
			name:  "non-string key",
			input: "sections:\n  ? [a, b]\n  : c\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := spec.DecodeYAML([]byte(tt.input)); err == nil {
				t.Error("DecodeYAML accepted input that cannot round-trip through JSON")
			}
		})
	}
}

func TestCanonicalJSON_IsAFixedPoint(t *testing.T) {
	first, err := spec.Decode([]byte(validJSON))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	raw, err := first.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}

	second, err := spec.Decode(raw)
	if err != nil {
		t.Fatalf("re-decoding canonical JSON failed: %v\n%s", err, raw)
	}
	again, err := second.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	if string(raw) != string(again) {
		t.Errorf("canonical JSON is not a fixed point:\nfirst  %s\nsecond %s", raw, again)
	}
	if first.Hash() != second.Hash() {
		t.Error("a canonical-JSON round trip moved the hash")
	}
}

func TestSchema_IsValidAndPublished(t *testing.T) {
	raw := spec.Schema()

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("the schema is not valid JSON: %v", err)
	}
	if doc["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Errorf("$schema = %v", doc["$schema"])
	}
	if doc["$id"] != spec.SchemaID {
		t.Errorf("$id = %v, want %q", doc["$id"], spec.SchemaID)
	}
	if !strings.Contains(spec.SchemaID, spec.FormatVersion) {
		t.Error("the schema identifier does not carry the format version; a consumer pinning one version could be handed another")
	}
}

// TestSchema_EnumsMatchTheRegistries is what keeps the schema and the model
// from drifting. The schema drives the tool contract an agent sees, where
// drift is not a documentation problem but a wrong answer.
func TestSchema_EnumsMatchTheRegistries(t *testing.T) {
	var doc struct {
		Defs map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(spec.Schema(), &doc); err != nil {
		t.Fatal(err)
	}

	check := func(def, field string, want []string) {
		t.Helper()
		var d struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(doc.Defs[def], &d); err != nil {
			t.Fatalf("parsing $defs/%s: %v", def, err)
		}
		got := d.Properties[field].Enum
		if len(got) != len(want) {
			t.Fatalf("$defs/%s.%s enum has %d entries, want %d: %v", def, field, len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("$defs/%s.%s enum[%d] = %q, want %q", def, field, i, got[i], want[i])
			}
		}
	}

	check("block", "kind", stringsOf(spec.AllBlockKinds()))
	check("cell", "class", stringsOf(spec.AllCellClasses()))
	check("value", "kind", stringsOf(spec.AllValueKinds()))
	check("annotation", "position", stringsOf(spec.AllAnnotationPositions()))
	check("length", "unit", stringsOf(spec.AllUnits()))
}

// TestSchema_EveryBlockKindHasAnArmConstraint proves the discriminator is
// expressed in the schema itself, so an agent authoring against the schema
// alone is told which arm its kind requires.
func TestSchema_EveryBlockKindHasAnArmConstraint(t *testing.T) {
	var doc struct {
		Defs struct {
			Block struct {
				AllOf []struct {
					If struct {
						Properties struct {
							Kind struct {
								Const string `json:"const"`
							} `json:"kind"`
						} `json:"properties"`
					} `json:"if"`
					Then struct {
						Required []string `json:"required"`
					} `json:"then"`
				} `json:"allOf"`
			} `json:"block"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(spec.Schema(), &doc); err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]bool)
	for _, c := range doc.Defs.Block.AllOf {
		if len(c.Then.Required) != 1 {
			t.Errorf("kind %q requires %v, want exactly one arm", c.If.Properties.Kind.Const, c.Then.Required)
		}
		seen[c.If.Properties.Kind.Const] = true
	}
	for _, kind := range spec.AllBlockKinds() {
		if !seen[string(kind)] {
			t.Errorf("block kind %q has no arm constraint in the schema", kind)
		}
	}
}

func stringsOf[T ~string](in []T) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}

func detailString(err error) string {
	ce, _ := err.(*verr.CodedError)
	if ce == nil {
		return ""
	}
	raw, _ := json.Marshal(ce.Details)
	return string(raw)
}
