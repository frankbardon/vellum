//go:build verapdf

package pdfvalidate

import (
	"context"
	"strings"

	"github.com/frankbardon/vellum/internal/exttool"
)

// EnvVeraPDF names the environment variable holding an explicit veraPDF path.
const EnvVeraPDF = "VELLUM_VERAPDF"

// veraPDFSpec describes how to find the validator.
var veraPDFSpec = exttool.Spec{
	Name:     "veraPDF",
	Env:      EnvVeraPDF,
	Commands: []string{"verapdf"},
	Paths: map[string][]string{
		"darwin": {"/Applications/veraPDF/verapdf"},
		"":       {"/opt/verapdf/verapdf", "/usr/local/verapdf/verapdf"},
	},
	Install: "https://verapdf.org/software/",
}

// VeraPDF is a located validator.
type VeraPDF struct {
	inner exttool.Tool
}

// Path is the located validator.
func (v VeraPDF) Path() string { return v.inner.Path }

// FindVeraPDF locates the validator.
func FindVeraPDF() (VeraPDF, error) {
	inner, err := exttool.Find(veraPDFSpec)
	if err != nil {
		return VeraPDF{}, err
	}
	return VeraPDF{inner: inner}, nil
}

// Report is a validation outcome.
type Report struct {
	// Compliant reports whether the file met the flavour it was checked against.
	Compliant bool

	// Output is the validator's own text, for a failure message. It names the
	// clauses that failed, which is the part worth reading.
	Output string
}

// Validate checks a file against a PDF/A flavour such as "2b".
//
// A non-zero exit status means non-compliance rather than a failure of the
// validator, so it is read as a verdict rather than raised as an error. The
// distinction matters: a validator that could not run and a file that did not
// conform must not be reported the same way, because the first is a broken
// environment and the second is a broken artifact.
func (v VeraPDF) Validate(ctx context.Context, path, flavour string) (Report, error) {
	res, err := v.inner.Run(ctx, nil, "--format", "text", "--flavour", flavour, path)
	if err != nil {
		return Report{}, err
	}

	out := res.Combined()
	compliant := res.ExitCode == 0 && !strings.Contains(out, "FAIL")
	return Report{Compliant: compliant, Output: out}, nil
}
