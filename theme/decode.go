package theme

import (
	"bytes"
	"encoding/json"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
	"sigs.k8s.io/yaml"
)

// Decode reads a theme document from JSON, strictly.
//
// Unknown fields are refused. There is no lenient mode and there will not be
// one: silent tolerance of an unrecognised key is poison when a theme is
// authored by hand or by a model, because the author gets no signal that half
// their document was ignored — and a theme that was half-ignored still renders,
// just wrongly.
func Decode(data []byte) (*Theme, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var t Theme
	if err := dec.Decode(&t); err != nil {
		return nil, decodeError(err)
	}
	// A second token must not follow: two concatenated documents is a mistake
	// worth reporting rather than a first document silently winning.
	if dec.More() {
		return nil, verr.NewCodedError(verr.VELLUM_THEME_INVALID,
			"trailing content after the theme document")
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return &t, nil
}

// DecodeYAML reads a theme document from YAML.
//
// It routes through JSON rather than decoding YAML directly, which is the
// reason for the dependency rather than an accident of it: the two forms must
// mean exactly the same thing, and the only way to guarantee that is for one to
// become the other before anything interprets it.
//
// One inherited trap, stated here because it is easier to read about than to
// debug: YAML 1.1 resolves bare y, n, yes, no, on and off to booleans, so an id
// written unquoted as `id: y` arrives as `true` and fails as a type mismatch.
// Quote the scalar. Vellum does not paper over it — a loud failure teaches the
// rule and a silent coercion does not. TestDecode_YAMLBareScalarsAreYAML11
// pins the behaviour.
func DecodeYAML(data []byte) (*Theme, error) {
	asJSON, err := yaml.YAMLToJSON(data)
	if err != nil {
		return nil, verr.WrapCodedError(err, verr.VELLUM_THEME_INVALID,
			"the theme document is not valid YAML")
	}
	return Decode(asJSON)
}

// DecodeAuto reads a theme document in either form, choosing by content.
func DecodeAuto(data []byte) (*Theme, error) {
	if isJSON(data) {
		return Decode(data)
	}
	return DecodeYAML(data)
}

// isJSON reports whether the first non-space byte opens a JSON object. YAML is
// a superset of JSON, so this decides which error prose a malformed document
// gets, not whether it can be read.
func isJSON(data []byte) bool {
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	return len(trimmed) > 0 && trimmed[0] == '{'
}

// decodeError converts a json decode failure into a coded one, lifting the
// offending field out of the standard library's prose so a caller can route on
// details rather than parse a message.
func decodeError(err error) error {
	const unknownPrefix = `json: unknown field `
	msg := err.Error()
	if i := strings.Index(msg, unknownPrefix); i >= 0 {
		field := strings.Trim(msg[i+len(unknownPrefix):], `"`)
		return verr.WrapCodedErrorWithDetails(err, verr.VELLUM_THEME_INVALID,
			"the theme document contains an unknown field",
			map[string]any{"field": field})
	}
	return verr.WrapCodedError(err, verr.VELLUM_THEME_INVALID,
		"the theme document is not valid JSON")
}
