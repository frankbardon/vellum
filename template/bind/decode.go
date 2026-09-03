package bind

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
	"sigs.k8s.io/yaml"
)

// Decode parses a binding from canonical JSON.
//
// Decoding is strict: an unknown field is an error naming the field, not a
// value quietly ignored. There is no lenient mode and there will not be
// one — the same reason [spec.Decode] gives applies unchanged here: a
// binding half-understood and half-dropped is a document that fills and is
// silently wrong, discovered by whoever reads the rendered output rather
// than by whoever authored the mistake.
func Decode(data []byte) (*Binding, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var b Binding
	if err := dec.Decode(&b); err != nil {
		return nil, decodeError(err)
	}
	// A second value after the document is a sign of concatenated input,
	// which would otherwise decode the first and silently discard the rest.
	if dec.More() {
		return nil, verr.NewCodedError(verr.VELLUM_BIND_INVALID,
			"input contains more than one JSON document")
	}

	if err := b.Validate(); err != nil {
		return nil, err
	}
	return &b, nil
}

// DecodeYAML parses a binding from YAML.
//
// It routes through JSON immediately, the same convention [spec.DecodeYAML]
// and [theme.DecodeYAML] both follow: a binding authored in YAML and the
// same one authored in JSON decode to the same tree and therefore hash
// identically, so the authoring surface leaves no trace.
func DecodeYAML(data []byte) (*Binding, error) {
	asJSON, err := yaml.YAMLToJSON(data)
	if err != nil {
		return nil, verr.WrapCodedError(err, verr.VELLUM_BIND_INVALID,
			"the binding document is not well-formed YAML")
	}
	return Decode(asJSON)
}

// DecodeAuto parses either JSON or YAML, choosing by inspecting the input.
//
// The sniff is deliberately crude — a leading brace means JSON — because
// YAML is a superset of JSON and the conversion is lossless either way; see
// [spec.DecodeAuto].
func DecodeAuto(data []byte) (*Binding, error) {
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return Decode(data)
	}
	return DecodeYAML(data)
}

const unknownFieldPrefix = "json: unknown field "

// decodeError turns a Go decoder fault into a coded error, recovering the
// offending field name from the standard library's message, the same
// technique [spec] and [theme] both use and both pin with a test against the
// standard library's message shape.
func decodeError(err error) error {
	msg := err.Error()
	if rest, ok := strings.CutPrefix(msg, unknownFieldPrefix); ok {
		name := strings.TrimSpace(rest)
		if unquoted, uerr := strconv.Unquote(name); uerr == nil {
			name = unquoted
		}
		return verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_INVALID,
			"the binding document carries a field Vellum does not recognise",
			map[string]any{
				"field": name,
				"hint":  "Unknown fields are errors rather than being ignored, so a partially understood binding cannot be trusted to bind what its author intended.",
			})
	}
	return verr.WrapCodedError(err, verr.VELLUM_BIND_INVALID,
		"the binding document could not be decoded")
}

// CanonicalJSON renders the binding as canonical JSON — the form Vellum
// hashes, and the form every downstream layer sees regardless of whether the
// author wrote JSON or YAML.
func (b *Binding) CanonicalJSON() ([]byte, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, verr.WrapCodedError(err, verr.VELLUM_BIND_INVALID,
			"the binding could not be encoded")
	}
	return raw, nil
}
