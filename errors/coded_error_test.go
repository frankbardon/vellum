package errors_test

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
)

func TestCodedError_ErrorString(t *testing.T) {
	e := verr.NewCodedError(verr.VELLUM_ZIP_MALFORMED, "central directory is truncated")
	want := "VELLUM_ZIP_MALFORMED: central directory is truncated"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestCodedError_DetailsAreCopied(t *testing.T) {
	details := map[string]any{"part": "/word/document.xml"}
	e := verr.NewCodedErrorWithDetails(verr.VELLUM_OPC_PART_NOT_FOUND, "missing part", details)

	details["part"] = "/word/styles.xml"
	details["added_after"] = true

	got, ok := e.Detail("part")
	if !ok || got != "/word/document.xml" {
		t.Errorf("Detail(\"part\") = %v, %v; the constructor must copy defensively so a caller can reuse the map", got, ok)
	}
	if _, ok := e.Detail("added_after"); ok {
		t.Error("a key added to the caller's map after construction leaked into the error")
	}
}

func TestCodedError_UnwrapAndIs(t *testing.T) {
	base := stderrors.New("underlying")
	e := verr.WrapCodedError(base, verr.VELLUM_ZIP_MALFORMED, "bad archive")

	if !stderrors.Is(e, base) {
		t.Error("errors.Is could not reach the wrapped cause")
	}
	if got := e.Unwrap(); got != base {
		t.Errorf("Unwrap() = %v, want the wrapped cause", got)
	}

	var target *verr.CodedError
	if !stderrors.As(e, &target) {
		t.Fatal("errors.As failed on a CodedError")
	}
	if target.Code != verr.VELLUM_ZIP_MALFORMED {
		t.Errorf("Code = %q, want %q", target.Code, verr.VELLUM_ZIP_MALFORMED)
	}
}

func TestCodedError_IsMatchesOnCodeAlone(t *testing.T) {
	e := verr.NewCodedError(verr.VELLUM_ZIP_TOO_LARGE, "entry declares 4 GiB")
	probe := verr.NewCodedError(verr.VELLUM_ZIP_TOO_LARGE, "")

	if !stderrors.Is(e, probe) {
		t.Error("errors.Is must match on code alone, so a caller can test a category without reconstructing the message")
	}
	other := verr.NewCodedError(verr.VELLUM_ZIP_MALFORMED, "")
	if stderrors.Is(e, other) {
		t.Error("errors.Is matched two different codes")
	}
}

func TestHasCode_WalksTheChain(t *testing.T) {
	inner := verr.NewCodedError(verr.VELLUM_ZIP_MALFORMED, "truncated")
	middle := verr.WrapCodedError(inner, verr.VELLUM_OPC_INVALID, "cannot read package")
	outer := fmt.Errorf("opening template: %w", middle)

	tests := []struct {
		name string
		err  error
		code verr.Code
		want bool
	}{
		{"outermost coded", outer, verr.VELLUM_OPC_INVALID, true},
		{"innermost coded through fmt wrap", outer, verr.VELLUM_ZIP_MALFORMED, true},
		{"absent code", outer, verr.VELLUM_FONT_SUBSTITUTED, false},
		{"nil error", nil, verr.VELLUM_OPC_INVALID, false},
		{"plain error", stderrors.New("x"), verr.VELLUM_OPC_INVALID, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := verr.HasCode(tt.err, tt.code); got != tt.want {
				t.Errorf("HasCode = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCodeOf_ReturnsOutermost(t *testing.T) {
	inner := verr.NewCodedError(verr.VELLUM_ZIP_MALFORMED, "truncated")
	outer := verr.WrapCodedError(inner, verr.VELLUM_OPC_INVALID, "cannot read package")

	got, ok := verr.CodeOf(outer)
	if !ok {
		t.Fatal("CodeOf failed on a CodedError")
	}
	if got != verr.VELLUM_OPC_INVALID {
		t.Errorf("CodeOf = %q, want the outermost code %q", got, verr.VELLUM_OPC_INVALID)
	}
	if _, ok := verr.CodeOf(stderrors.New("plain")); ok {
		t.Error("CodeOf reported a code for a plain error")
	}
}

// TestCodedError_MarshalJSON pins the nil-versus-empty details distinction.
// omitempty cannot express it — it drops both — and the difference between
// "no context was gathered" and "context was gathered and was empty" is part
// of the wire contract.
func TestCodedError_MarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		err  *verr.CodedError
		want string
	}{
		{
			name: "nil details omits the key",
			err:  verr.NewCodedError(verr.VELLUM_ZIP_MALFORMED, "truncated"),
			want: `{"code":"VELLUM_ZIP_MALFORMED","message":"truncated"}`,
		},
		{
			name: "empty details emits an empty object",
			err:  verr.NewCodedErrorWithDetails(verr.VELLUM_ZIP_MALFORMED, "truncated", map[string]any{}),
			want: `{"code":"VELLUM_ZIP_MALFORMED","message":"truncated","details":{}}`,
		},
		{
			name: "populated details",
			err:  verr.NewCodedErrorWithDetails(verr.VELLUM_OPC_PART_NOT_FOUND, "missing part", map[string]any{"part_name": "/word/document.xml"}),
			want: `{"code":"VELLUM_OPC_PART_NOT_FOUND","message":"missing part","details":{"part_name":"/word/document.xml"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.err)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if got := string(b); got != tt.want {
				t.Errorf("Marshal = %s\nwant       %s", got, tt.want)
			}
		})
	}
}

// TestCodedError_MarshalJSONOmitsCause pins that a wrapped cause never reaches
// the wire. A cause is diagnostic context for a Go caller; serialising it
// would make output non-deterministic and could disclose a filesystem path.
func TestCodedError_MarshalJSONOmitsCause(t *testing.T) {
	e := verr.WrapCodedError(stderrors.New("open /home/someone/secret.docx: no such file"), verr.VELLUM_OPC_INVALID, "cannot read package")
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got := string(b); got != `{"code":"VELLUM_OPC_INVALID","message":"cannot read package"}` {
		t.Errorf("Marshal = %s; the cause must not be serialised", got)
	}
}

func TestCodedError_NilReceiverIsSafe(t *testing.T) {
	var e *verr.CodedError
	if got := e.Error(); got != "<nil>" {
		t.Errorf("Error() on nil = %q", got)
	}
	if got := e.Unwrap(); got != nil {
		t.Errorf("Unwrap() on nil = %v", got)
	}
	if _, ok := e.Detail("x"); ok {
		t.Error("Detail on nil reported a value")
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal on nil: %v", err)
	}
	if string(b) != "null" {
		t.Errorf("Marshal on nil = %s, want null", b)
	}
}
