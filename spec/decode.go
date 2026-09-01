package spec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"sigs.k8s.io/yaml"
)

// faultPrinter renders validator messages.
//
// The library's LocalizedString takes a printer and dereferences it without a
// nil check, so passing nil panics. One English printer is built here rather
// than per call: Vellum does not localise its diagnostics, and what a consumer
// actually branches on is the coded error and its machine-readable path, not
// the prose.
var faultPrinter = message.NewPrinter(language.English)

// Decode parses a specification from canonical JSON.
//
// Decoding is strict: an unknown field is an error naming the field and its
// JSON path, not a value quietly ignored. There is no lenient mode and there
// will not be one — it would be used.
//
// The reason is not fastidiousness. When a model authors the specification, a
// silently dropped field gives it no signal that its output was partially
// ignored; it sees a document that renders and a section that is missing, and
// has no way to connect the two. Strictness is what makes the MCP surface
// usable at all.
func Decode(data []byte) (*Spec, error) {
	if err := validateAgainstSchema(data); err != nil {
		return nil, err
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var s Spec
	if err := dec.Decode(&s); err != nil {
		return nil, decodeError(err)
	}
	// A second value after the document is a sign of concatenated input, which
	// would otherwise decode the first and silently discard the rest.
	if dec.More() {
		return nil, verr.NewCodedError(verr.VELLUM_SPEC_INVALID,
			"input contains more than one JSON document")
	}

	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

// DecodeYAML parses a specification from YAML.
//
// YAML is an authoring convenience, converted to canonical JSON immediately so
// that everything downstream — validation, hashing, error paths — sees exactly
// one representation. That conversion is why the YAML library is chosen for
// its JSON round trip rather than its feature set: routing through JSON is the
// requirement, not an implementation detail.
//
// A consequence worth stating: a specification authored in YAML and the same
// one authored in JSON produce identical bytes and therefore an identical
// hash. The authoring surface leaves no trace.
func DecodeYAML(data []byte) (*Spec, error) {
	asJSON, err := yaml.YAMLToJSON(data)
	if err != nil {
		return nil, verr.WrapCodedError(err, verr.VELLUM_SPEC_INVALID,
			"the input is not well-formed YAML")
	}
	return Decode(asJSON)
}

// DecodeAuto parses either JSON or YAML, choosing by inspecting the input.
//
// The sniff is deliberately crude — a leading brace means JSON — because YAML
// is a superset of JSON and the conversion is lossless either way. The
// distinction exists only to keep JSON error messages pointing at the JSON the
// caller actually wrote.
func DecodeAuto(data []byte) (*Spec, error) {
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return Decode(data)
	}
	return DecodeYAML(data)
}

// compiledSchema is built once. Compilation is pure and the schema is a
// constant of the package, so doing it per decode would be waste.
var compiledSchema = func() *jsonschema.Schema {
	raw := Schema()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		panic(fmt.Sprintf("vellum/spec: the generated schema is not valid JSON: %v", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(SchemaID, doc); err != nil {
		panic(fmt.Sprintf("vellum/spec: adding the schema resource failed: %v", err))
	}
	compiled, err := c.Compile(SchemaID)
	if err != nil {
		panic(fmt.Sprintf("vellum/spec: the generated schema does not compile: %v", err))
	}
	return compiled
}()

// validateAgainstSchema checks the document before it reaches the Go decoder,
// so the caller gets a message with a path rather than a Go unmarshal fault.
//
// Faults are collected rather than reported one at a time. An author fixing a
// specification one error per run is an author having a bad afternoon, and a
// model doing the same is a model burning a turn per typo.
func validateAgainstSchema(data []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return verr.WrapCodedError(err, verr.VELLUM_SPEC_INVALID,
			"the input is not well-formed JSON")
	}

	err = compiledSchema.Validate(doc)
	if err == nil {
		return nil
	}

	var ve *jsonschema.ValidationError
	if !asValidationError(err, &ve) {
		return verr.WrapCodedError(err, verr.VELLUM_SPEC_INVALID,
			"the specification does not satisfy the schema")
	}

	faults := collectFaults(ve)
	sort.Slice(faults, func(i, j int) bool {
		if faults[i]["path"] != faults[j]["path"] {
			return faults[i]["path"].(string) < faults[j]["path"].(string)
		}
		return faults[i]["problem"].(string) < faults[j]["problem"].(string)
	})

	return verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_INVALID,
		"the specification does not satisfy the schema",
		map[string]any{"faults": faults, "fault_count": len(faults)})
}

// collectFaults flattens the validator's cause tree into a list of leaves,
// which is where the actionable detail lives — the interior nodes only say
// that something below them failed.
func collectFaults(ve *jsonschema.ValidationError) []map[string]any {
	if len(ve.Causes) == 0 {
		return []map[string]any{{
			"path":    jsonPointer(ve.InstanceLocation),
			"problem": ve.ErrorKind.LocalizedString(faultPrinter),
		}}
	}
	var out []map[string]any
	for _, c := range ve.Causes {
		out = append(out, collectFaults(c)...)
	}
	return out
}

func jsonPointer(loc []string) string {
	if len(loc) == 0 {
		return "/"
	}
	var b strings.Builder
	for _, seg := range loc {
		b.WriteByte('/')
		b.WriteString(seg)
	}
	return b.String()
}

func asValidationError(err error, target **jsonschema.ValidationError) bool {
	for err != nil {
		if ve, ok := err.(*jsonschema.ValidationError); ok {
			*target = ve
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

const unknownFieldPrefix = "json: unknown field "

// decodeError turns a Go decoder fault into a coded error, recovering the
// offending field name from the standard library's message.
//
// Parsing an error string is unpleasant, and it is done here rather than
// avoided because the alternative — reporting "the document could not be
// decoded" — tells the author nothing they can act on. The prefix is stable
// across Go releases and a test pins the behaviour, so a change would surface
// as a failure rather than as a degraded message.
func decodeError(err error) error {
	msg := err.Error()
	if rest, ok := strings.CutPrefix(msg, unknownFieldPrefix); ok {
		name := strings.TrimSpace(rest)
		if unquoted, uerr := strconv.Unquote(name); uerr == nil {
			name = unquoted
		}
		return verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_INVALID,
			"the specification carries a field Vellum does not recognise",
			map[string]any{
				"field": name,
				"hint":  "Unknown fields are errors rather than being ignored, so a partially understood document cannot render as if it were fully understood.",
			})
	}
	return verr.WrapCodedError(err, verr.VELLUM_SPEC_INVALID,
		"the specification could not be decoded")
}

// CanonicalJSON renders the specification as canonical JSON — the form Vellum
// hashes and the form every downstream layer sees, regardless of whether the
// author wrote JSON or YAML.
func (s *Spec) CanonicalJSON() ([]byte, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, verr.WrapCodedError(err, verr.VELLUM_SPEC_INVALID,
			"the specification could not be encoded")
	}
	return raw, nil
}
