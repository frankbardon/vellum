package mcp

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"

	verr "github.com/frankbardon/vellum/errors"
)

// schemaOptions corrects three places where [jsonschema.For]'s default
// struct-field reflection disagrees with what encoding/json actually puts on
// the wire, for every In/Out type in types.go:
//
//   - json.RawMessage is reflected as "any valid JSON value" rather than as
//     its underlying []byte representation — an array of byte-range
//     integers, which is what the default would otherwise produce.
//     ComposeIn.Spec, FillIn.Binding, FillIn.Data and SchemaOut.Schema all
//     carry embedded JSON with exactly that "arbitrary document" meaning.
//   - []byte (a binary payload: ComposeOut.Artifact, InspectIn.Template,
//     FillIn.Template, FillOut.Package) is reflected as a base64-encoded
//     string, matching what encoding/json's own []byte marshalling actually
//     produces, rather than as an array of integers.
//   - [errors.CodedError] (ComposeOut.Warnings, ValidateOut.Warnings) is
//     reflected as its actual wire shape — {"code","message","details"},
//     lowercase, no "cause" — rather than its Go field names, because
//     [errors.CodedError.MarshalJSON] is a hand-written custom marshaller
//     jsonschema.For's struct-field walk has no way to see through: it
//     would otherwise describe {"Code","Message","Details","Cause"}, which
//     is not the JSON this package (or CLAUDE.md's own envelope contract)
//     ever actually emits.
//
// Applied package-wide, once, rather than per type: every correction here is
// true of every occurrence of the corrected type in this package's
// contracts.
var schemaOptions = &jsonschema.ForOptions{
	TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[json.RawMessage](): {},
		reflect.TypeFor[[]byte]():          {Type: "string", ContentEncoding: "base64"},
		reflect.TypeFor[verr.CodedError](): {
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"code":    {Type: "string"},
				"message": {Type: "string"},
				"details": {Type: "object"},
			},
			Required: []string{"code", "message"},
		},
	},
}

// reflectSchema infers a JSON Schema for T and marshals it once. Called only
// from the package-level var block below — at init, never per call — which
// is what makes a tool's published inputSchema/outputSchema stable across
// repeated tools/list calls: the bytes are computed a single time and reused
// for the life of the process.
//
// Panicking on failure is deliberate and confined to init: every In/Out type
// in types.go is a plain, JSON-tagged struct — no maps with a non-string key,
// no channel, no function, nothing jsonschema.For rejects — so a failure here
// can only mean a future edit introduced a type it cannot reflect, and that
// is a build-time programming error, not a runtime condition any caller can
// recover from.
func reflectSchema[T any]() json.RawMessage {
	s, err := jsonschema.For[T](schemaOptions)
	if err != nil {
		panic(fmt.Sprintf("mcp: reflecting schema for %T: %v", *new(T), err))
	}
	raw, err := json.Marshal(s)
	if err != nil {
		panic(fmt.Sprintf("mcp: marshalling reflected schema for %T: %v", *new(T), err))
	}
	return raw
}

// Every tool's input and output schema, reflected once at package init. The
// var block's own declaration order matches [toolmeta.AllTools]'s, purely
// for a reader's convenience — Go initialises package-level vars in
// dependency order regardless, so this ordering carries no behavioural
// weight.
var (
	composeInputSchema   = reflectSchema[ComposeIn]()
	composeOutputSchema  = reflectSchema[ComposeOut]()
	validateInputSchema  = reflectSchema[ValidateIn]()
	validateOutputSchema = reflectSchema[ValidateOut]()
	inspectInputSchema   = reflectSchema[InspectIn]()
	inspectOutputSchema  = reflectSchema[InspectOut]()
	fillInputSchema      = reflectSchema[FillIn]()
	fillOutputSchema     = reflectSchema[FillOut]()

	capabilitiesInputSchema  = reflectSchema[CapabilitiesIn]()
	capabilitiesOutputSchema = reflectSchema[CapabilitiesOut]()
	boxesInputSchema         = reflectSchema[BoxesIn]()
	boxesOutputSchema        = reflectSchema[BoxesOut]()
	schemaInputSchema        = reflectSchema[SchemaIn]()
	schemaOutputSchema       = reflectSchema[SchemaOut]()
	manifestInputSchema      = reflectSchema[ManifestIn]()
	manifestOutputSchema     = reflectSchema[ManifestOut]()

	skillsInputSchema    = reflectSchema[SkillsIn]()
	skillsOutputSchema   = reflectSchema[SkillsOut]()
	examplesInputSchema  = reflectSchema[ExamplesIn]()
	examplesOutputSchema = reflectSchema[ExamplesOut]()
)
