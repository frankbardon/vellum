package descriptor

import (
	"encoding/json"

	verr "github.com/frankbardon/vellum/errors"
)

// FormatVersion is the wire version of the envelope and of everything it
// carries.
//
// It appears in CLAUDE.md, where a hygiene test checks for it, and in the
// payload schema's identifier. The three move together or not at all.
const FormatVersion = "1.0"

// Envelope wraps every JSON payload Vellum emits.
//
// Errors and warnings are always present as arrays, never null and never
// omitted. A consumer should be able to write `for e of result.errors` without
// a guard, and an empty array says "nothing went wrong" where a missing key
// says only "this producer did not think about it".
type Envelope struct {
	// FormatVersion is the wire version.
	FormatVersion string `json:"format_version"`

	// Data is the payload.
	Data any `json:"data"`

	// Request echoes the request that produced the payload, when the caller
	// asked for it. Omitted otherwise, because echoing a large specification
	// back by default would double every response.
	Request any `json:"request,omitempty"`

	// Errors are the failures. A non-empty errors array means Data is not
	// trustworthy.
	Errors []*Entry `json:"errors"`

	// Warnings are the things that happened but did not fail: a font
	// substituted, a block degraded to a format's stated alternative. Every one
	// of them is a decision Vellum made on the caller's behalf, reported so
	// that none of them is silent.
	Warnings []*Entry `json:"warnings"`
}

// Entry is one error or warning.
type Entry struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// NewEnvelope wraps data.
//
// Errors and warnings start as empty non-nil slices so that a consumer never
// sees null where an array belongs.
func NewEnvelope(data any) *Envelope {
	return &Envelope{
		FormatVersion: FormatVersion,
		Data:          data,
		Errors:        []*Entry{},
		Warnings:      []*Entry{},
	}
}

// WithRequest attaches the originating request. Nil-safe, so it can be chained
// onto a constructor without a guard.
func (e *Envelope) WithRequest(request any) *Envelope {
	if e == nil {
		return nil
	}
	e.Request = request
	return e
}

// AddError appends an error entry.
func (e *Envelope) AddError(code, message string, details map[string]any) {
	if e == nil {
		return
	}
	e.Errors = append(e.Errors, &Entry{Code: code, Message: message, Details: details})
}

// AddWarning appends a warning entry.
func (e *Envelope) AddWarning(code, message string, details map[string]any) {
	if e == nil {
		return
	}
	e.Warnings = append(e.Warnings, &Entry{Code: code, Message: message, Details: details})
}

// AddCodedError appends a coded error, preserving its code and details rather
// than flattening it to a string.
//
// Flattening is the tempting shortcut and it destroys the thing that makes the
// error useful: a consumer branching on a code cannot branch on prose.
func (e *Envelope) AddCodedError(err error) {
	if e == nil || err == nil {
		return
	}
	var ce *verr.CodedError
	if asCoded(err, &ce) {
		e.AddError(string(ce.Code), ce.Message, ce.Details)
		return
	}
	e.AddError(string(verr.VELLUM_INTERNAL_INVARIANT), err.Error(), nil)
}

// AddCodedWarning appends a coded warning.
func (e *Envelope) AddCodedWarning(code verr.Code, message string, details map[string]any) {
	e.AddWarning(string(code), message, details)
}

// OK reports whether the envelope carries no errors.
func (e *Envelope) OK() bool { return e != nil && len(e.Errors) == 0 }

// MarshalJSON emits the envelope, guaranteeing the array invariant even for an
// envelope constructed as a bare struct literal rather than through
// NewEnvelope.
func (e *Envelope) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}
	// A local alias to avoid recursing into this method.
	type envelope Envelope
	out := envelope(*e)
	if out.FormatVersion == "" {
		out.FormatVersion = FormatVersion
	}
	if out.Errors == nil {
		out.Errors = []*Entry{}
	}
	if out.Warnings == nil {
		out.Warnings = []*Entry{}
	}
	return json.Marshal(out)
}

func asCoded(err error, target **verr.CodedError) bool {
	for err != nil {
		if ce, ok := err.(*verr.CodedError); ok {
			*target = ce
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
