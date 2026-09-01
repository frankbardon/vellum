package errors

import (
	"encoding/json"
	stderrors "errors"
	"maps"
)

// CodedError is Vellum's structured error type. It pairs a machine-readable
// [Code] with a human-readable message and an open detail map, and optionally
// wraps an underlying cause.
//
// It is the only error type Vellum's domain packages return. There are no
// per-package error types, because a consumer switching on behaviour should
// switch on a code rather than on a type assertion that changes when a package
// is refactored.
type CodedError struct {
	// Code identifies the error category and is stable public API.
	Code Code

	// Message is a human-readable description, lowercase and without a
	// trailing period, in the style of the standard library.
	Message string

	// Details carries structured context — the part name, the block index,
	// the anchor, the coordinates. Keys are snake_case. A nil map and an empty
	// map are distinguished on the wire: nil omits the key entirely, empty
	// emits {}. That distinction lets a caller tell "no context was gathered"
	// from "context was gathered and was empty".
	Details map[string]any

	// Cause is the underlying error, if any. It participates in errors.Is and
	// errors.As through Unwrap.
	Cause error
}

// NewCodedError returns a CodedError with no details and no cause.
func NewCodedError(code Code, message string) *CodedError {
	return &CodedError{Code: code, Message: message}
}

// NewCodedErrorWithDetails returns a CodedError carrying structured context.
// This is the dominant constructor: an error a consumer cannot locate is an
// error they cannot act on, so most call sites have something worth naming.
//
// details is copied defensively, so a caller may reuse or mutate the map it
// passed without changing the error.
func NewCodedErrorWithDetails(code Code, message string, details map[string]any) *CodedError {
	e := &CodedError{Code: code, Message: message}
	if details != nil {
		e.Details = make(map[string]any, len(details))
		maps.Copy(e.Details, details)
	}
	return e
}

// WrapCodedError wraps err with a code and message. The cause is preserved and
// reachable through errors.Is, errors.As and Unwrap.
func WrapCodedError(err error, code Code, message string) *CodedError {
	return &CodedError{Code: code, Message: message, Cause: err}
}

// WrapCodedErrorWithDetails wraps err with a code, message and structured
// context. details is copied defensively.
func WrapCodedErrorWithDetails(err error, code Code, message string, details map[string]any) *CodedError {
	e := WrapCodedError(err, code, message)
	if details != nil {
		e.Details = make(map[string]any, len(details))
		maps.Copy(e.Details, details)
	}
	return e
}

// Error implements error. The form is "CODE: message" — the code first,
// because it is the part a reader scans for.
func (e *CodedError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return string(e.Code) + ": " + e.Message
}

// Unwrap returns the underlying cause, or nil.
func (e *CodedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Is reports whether target is a CodedError with the same code. This makes
// errors.Is(err, errors.NewCodedError(VELLUM_ZIP_MALFORMED, "")) work as a
// category test without the caller having to construct a matching message.
func (e *CodedError) Is(target error) bool {
	if e == nil {
		return target == nil
	}
	var t *CodedError
	if !stderrors.As(target, &t) || t == nil {
		return false
	}
	return e.Code == t.Code
}

// Detail returns the named detail value. The second result reports presence,
// so a caller can distinguish a missing key from a stored nil.
func (e *CodedError) Detail(key string) (any, bool) {
	if e == nil || e.Details == nil {
		return nil, false
	}
	v, ok := e.Details[key]
	return v, ok
}

// codedErrorJSONWithDetails and codedErrorJSONNoDetails exist as two shapes
// rather than one shape with omitempty because omitempty cannot distinguish a
// nil map from an empty one — it drops both — and that distinction is part of
// the wire contract.
type codedErrorJSONWithDetails struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

type codedErrorJSONNoDetails struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// MarshalJSON emits the envelope-entry shape. Note that the cause is
// deliberately not serialised: it is diagnostic context for a Go caller, and
// leaking an arbitrary wrapped error's text into a wire payload would make the
// output non-deterministic and potentially disclose a filesystem path.
func (e *CodedError) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}
	if e.Details == nil {
		return json.Marshal(codedErrorJSONNoDetails{
			Code:    string(e.Code),
			Message: e.Message,
		})
	}
	return json.Marshal(codedErrorJSONWithDetails{
		Code:    string(e.Code),
		Message: e.Message,
		Details: e.Details,
	})
}

// HasCode reports whether err, or any error it wraps, is a CodedError with the
// given code.
func HasCode(err error, code Code) bool {
	for err != nil {
		var ce *CodedError
		if stderrors.As(err, &ce) {
			if ce.Code == code {
				return true
			}
			err = ce.Cause
			continue
		}
		err = stderrors.Unwrap(err)
	}
	return false
}

// CodeOf returns the code of the outermost CodedError in err's chain. The
// second result reports whether the chain contained one at all.
func CodeOf(err error) (Code, bool) {
	var ce *CodedError
	if stderrors.As(err, &ce) && ce != nil {
		return ce.Code, true
	}
	return "", false
}

// WithDetails returns a copy of the error carrying additional details.
//
// It exists for the layer that knows *where* a failure happened but not *what*
// it was. A section resolver knows the section index; the length parser that
// actually failed knows only that a length was bad. Re-wrapping would restate
// the message and nest the cause one deeper for no gain, so the position is
// merged into the original instead.
//
// Existing keys are not overwritten. The inner layer was closer to the fault
// and its account of a shared key is the better one.
//
// The receiver is not modified: an error value may be held by a caller, and a
// second annotation of the same error must not change what the first one said.
func (e *CodedError) WithDetails(details map[string]any) *CodedError {
	if e == nil {
		return nil
	}
	merged := make(map[string]any, len(e.Details)+len(details))
	for k, v := range details {
		merged[k] = v
	}
	for k, v := range e.Details {
		merged[k] = v
	}
	return &CodedError{Code: e.Code, Message: e.Message, Details: merged, Cause: e.Cause}
}

// Annotate adds details to err when it is a coded error, and returns it
// unchanged when it is not.
//
// The plain-error case is deliberately a passthrough rather than a wrap. An
// error that is not coded came from outside this library or from a layer that
// has not adopted the vocabulary, and attaching a code to it here would invent
// a classification nobody made.
func Annotate(err error, details map[string]any) error {
	if err == nil {
		return nil
	}
	var coded *CodedError
	if stderrors.As(err, &coded) {
		return coded.WithDetails(details)
	}
	return err
}
