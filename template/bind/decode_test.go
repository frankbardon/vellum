package bind_test

import (
	"strings"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/template/bind"
)

const validJSON = `{
  "format_version": "1.0",
  "statements": [
    {"kind": "bind", "bind": {"anchor": "customer_name", "expr": "data.customer.name"}},
    {"kind": "repeat", "repeat": {
      "over": "data.line_items",
      "as": "item",
      "target": "row",
      "body": [
        {"kind": "if", "if": {
          "when": "item.discounted",
          "then": [
            {"kind": "with", "with": {
              "as": "price",
              "value": "item.discounted_price",
              "body": [{"kind": "bind", "bind": {"anchor": "line_total", "expr": "price", "format": "#,##0.00"}}]
            }}
          ],
          "else": [{"kind": "bind", "bind": {"anchor": "line_total", "expr": "item.price"}}]
        }}
      ]
    }},
    {"kind": "bind", "skip": "not data.show_notes", "bind": {"anchor": "notes", "expr": "data.notes", "optional": true}}
  ]
}`

const validYAML = `
format_version: "1.0"
statements:
  - kind: bind
    bind:
      anchor: customer_name
      expr: data.customer.name
  - kind: repeat
    repeat:
      over: data.line_items
      as: item
      target: row
      body:
        - kind: if
          if:
            when: item.discounted
            then:
              - kind: with
                with:
                  as: price
                  value: item.discounted_price
                  body:
                    - kind: bind
                      bind: {anchor: line_total, expr: price, format: "#,##0.00"}
            else:
              - kind: bind
                bind: {anchor: line_total, expr: item.price}
  - kind: bind
    skip: not data.show_notes
    bind:
      anchor: notes
      expr: data.notes
      optional: true
`

func TestDecode_Valid(t *testing.T) {
	b, err := bind.Decode([]byte(validJSON))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(b.Statements) != 3 {
		t.Fatalf("unexpected shape: %d statements", len(b.Statements))
	}
	if b.Statements[0].Bind.Anchor != "customer_name" {
		t.Errorf("bind did not decode")
	}
	repeat := b.Statements[1].Repeat
	if repeat == nil || repeat.Target != bind.RepeatTargetRow || repeat.As != "item" {
		t.Fatalf("repeat did not decode: %+v", repeat)
	}
	nested := repeat.Body[0].If
	if nested == nil || nested.When != "item.discounted" {
		t.Fatalf("nested if did not decode: %+v", nested)
	}
	if b.Statements[2].Skip != "not data.show_notes" {
		t.Errorf("skip modifier did not decode")
	}
	if !b.Statements[2].Bind.Optional {
		t.Errorf("optional did not decode")
	}
}

// TestDecode_YAMLAndJSONAgree is the property that makes the authoring
// surface leave no trace, mirroring spec's own guarantee.
func TestDecode_YAMLAndJSONAgree(t *testing.T) {
	fromJSON, err := bind.Decode([]byte(validJSON))
	if err != nil {
		t.Fatalf("Decode JSON: %v", err)
	}
	fromYAML, err := bind.DecodeYAML([]byte(validYAML))
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
	a, err := bind.DecodeAuto([]byte(validJSON))
	if err != nil {
		t.Fatalf("DecodeAuto JSON: %v", err)
	}
	b, err := bind.DecodeAuto([]byte(validYAML))
	if err != nil {
		t.Fatalf("DecodeAuto YAML: %v", err)
	}
	if a.Hash() != b.Hash() {
		t.Error("DecodeAuto produced different results for the same document in two syntaxes")
	}
}

// TestDecode_RejectsUnknownFields is the reason there is no lenient mode.
func TestDecode_RejectsUnknownFields(t *testing.T) {
	tests := []struct {
		name  string
		input string
		field string
	}{
		{
			name:  "unknown top-level field",
			input: `{"statements": [], "extra": true}`,
			field: "extra",
		},
		{
			name:  "unknown field on a bind statement",
			input: `{"statements": [{"kind": "bind", "bind": {"anchor": "a", "expr": "x", "typo": 1}}]}`,
			field: "typo",
		},
		{
			name:  "unknown field on a repeat statement",
			input: `{"statements": [{"kind": "repeat", "repeat": {"over": "x", "as": "y", "target": "row", "body": [], "bogus": 1}}]}`,
			field: "bogus",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := bind.Decode([]byte(tt.input))
			if !verr.HasCode(err, verr.VELLUM_BIND_INVALID) {
				t.Fatalf("error = %v, want VELLUM_BIND_INVALID", err)
			}
			ce, _ := err.(*verr.CodedError)
			if ce == nil {
				t.Fatal("error is not a CodedError")
			}
			got, ok := ce.Detail("field")
			if !ok {
				t.Fatal("error does not name the offending field")
			}
			if !strings.Contains(got.(string), tt.field) {
				t.Errorf("field detail = %v, want it to contain %q", got, tt.field)
			}
		})
	}
}

func TestDecode_RejectsTrailingContent(t *testing.T) {
	_, err := bind.Decode([]byte(validJSON + `{}`))
	if !verr.HasCode(err, verr.VELLUM_BIND_INVALID) {
		t.Fatalf("error = %v, want VELLUM_BIND_INVALID", err)
	}
}

func TestDecode_RejectsMalformedJSON(t *testing.T) {
	_, err := bind.Decode([]byte(`{"statements": [`))
	if !verr.HasCode(err, verr.VELLUM_BIND_INVALID) {
		t.Fatalf("error = %v, want VELLUM_BIND_INVALID", err)
	}
}

func TestDecode_RejectsMalformedYAML(t *testing.T) {
	_, err := bind.DecodeYAML([]byte("statements:\n  - kind: bind\n  bind: {anchor: a}\n"))
	if !verr.HasCode(err, verr.VELLUM_BIND_INVALID) {
		t.Fatalf("error = %v, want VELLUM_BIND_INVALID", err)
	}
}

// TestDecode_StructurallyInvalidIsRejected checks that Decode calls Validate
// rather than merely parsing.
func TestDecode_StructurallyInvalidIsRejected(t *testing.T) {
	_, err := bind.Decode([]byte(`{"statements": []}`))
	if !verr.HasCode(err, verr.VELLUM_BIND_INVALID) {
		t.Fatalf("error = %v, want VELLUM_BIND_INVALID", err)
	}
}

func TestDecode_MissingRequiredBindField(t *testing.T) {
	_, err := bind.Decode([]byte(`{"statements": [{"kind": "bind", "bind": {"expr": "x"}}]}`))
	if !verr.HasCode(err, verr.VELLUM_BIND_INVALID) {
		t.Fatalf("error = %v, want VELLUM_BIND_INVALID", err)
	}
	ce, _ := err.(*verr.CodedError)
	if ce == nil {
		t.Fatal("error is not a CodedError")
	}
}
