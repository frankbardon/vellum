package cli

import (
	"encoding/json"
	"io"

	"github.com/frankbardon/vellum/descriptor"
)

// writeEnvelope is the one place in this package that marshals a
// [descriptor.Envelope] to JSON — the choke point CLAUDE.md's "every --json
// output goes through descriptor.NewEnvelope; no fmt.Sprintf builds JSON"
// requires. Every --json code path in every verb file calls this or
// [writeEnvelopeError], never json.Marshal directly.
//
// HTML escaping is disabled: the output is a file or a terminal, never
// embedded in an HTML document, and a coded error's details carrying a
// path with "&" or "<" in it should read as written.
func writeEnvelope(w io.Writer, data any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(descriptor.NewEnvelope(data))
}

// writeEnvelopeError writes an envelope whose only content is err, added
// through [descriptor.Envelope.AddCodedError] so a *errors.CodedError keeps
// its code and structured details rather than being flattened to a string.
//
// This is what lets a --json invocation's failure path still satisfy the
// Output Format Contract: errors go to stdout, inside the envelope, exactly
// as a success does — never to stderr in --json mode, and never a bare
// error string outside the envelope shape.
func writeEnvelopeError(w io.Writer, err error) error {
	env := descriptor.NewEnvelope(nil)
	env.AddCodedError(err)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(env)
}
