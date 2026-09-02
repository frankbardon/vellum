//go:build verapdf

package pdfvalidate

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/frankbardon/vellum/internal/exttool"
)

// veraPDFSpec describes how to find a local installation.
var veraPDFSpec = exttool.Spec{
	Name:     "veraPDF",
	Env:      EnvVeraPDF,
	Commands: []string{"verapdf"},
	Paths: map[string][]string{
		"darwin": {"/Applications/veraPDF/verapdf"},
		"":       {"/opt/verapdf/verapdf", "/usr/local/verapdf/verapdf"},
	},
	Install: "https://verapdf.org/software/, or set " + EnvVeraPDF + "=" +
		ContainerPrefix + PinnedImage + " to run the pinned container",
}

// containerSpec describes how to find a container runtime.
var containerSpec = exttool.Spec{
	Name:     "a container runtime",
	Commands: []string{"docker", "podman"},
	Install:  "install Docker or Podman, or point " + EnvVeraPDF + " at a veraPDF installation",
}

// VeraPDF is a located validator.
type VeraPDF struct {
	inner exttool.Tool

	// image is the container reference when the validator runs from one, and
	// empty when it is a local installation.
	image string
}

// Path is the located executable: the validator itself, or the container
// runtime that runs it.
func (v VeraPDF) Path() string { return v.inner.Path }

// Image is the container reference, or empty for a local installation.
func (v VeraPDF) Image() string { return v.image }

// Describe names what will actually run, for a log line.
func (v VeraPDF) Describe() string {
	if v.image != "" {
		return v.image + " via " + v.inner.Path
	}
	return v.inner.Path
}

// FindVeraPDF locates the validator.
//
// A container reference is honoured before anything else, because a runner that
// asked for one must not silently fall back to whatever happens to be installed
// beside it — the whole point of pinning is that the verdict comes from a known
// validator.
func FindVeraPDF() (VeraPDF, error) {
	ref := strings.TrimSpace(os.Getenv(EnvVeraPDF))
	if strings.HasPrefix(ref, ContainerPrefix) {
		return findContainer(strings.TrimPrefix(ref, ContainerPrefix))
	}

	inner, err := exttool.Find(veraPDFSpec)
	if err != nil {
		return VeraPDF{}, err
	}
	return VeraPDF{inner: inner}, nil
}

// findContainer prepares a containerised validator.
func findContainer(ref string) (VeraPDF, error) {
	if ref == "" {
		return VeraPDF{}, fmt.Errorf("%s names a container and gives no reference", EnvVeraPDF)
	}
	if !strings.Contains(ref, "@sha256:") {
		return VeraPDF{}, fmt.Errorf(
			"%s names the container %q by tag rather than by digest.\n\n"+
				"A tag is a name someone else may repoint, so the gate would report a verdict from "+
				"whatever the registry held at the moment it ran — and a corpus that passed yesterday "+
				"and fails today would have changed nothing. Use %s%s",
			EnvVeraPDF, ref, ContainerPrefix, PinnedImage)
	}

	inner, err := exttool.Find(containerSpec)
	if err != nil {
		return VeraPDF{}, err
	}
	return VeraPDF{inner: inner, image: ref}, nil
}

// Report is a validation outcome.
type Report struct {
	// Compliant reports whether the file met the flavour it was checked
	// against.
	Compliant bool

	// Version is the validator's core release, read out of the report it just
	// produced rather than from a separate invocation. The verdict and the
	// version therefore come from one run and cannot describe two validators.
	Version string

	// Failures are the clauses that failed, in the order reported.
	Failures []Failure

	// Output is the validator's own text, for a message when the report could
	// not be parsed.
	Output string
}

// Failure is one failed clause.
type Failure struct {
	// Clause and Test locate the requirement in ISO 19005.
	Clause, Test string

	// Description is the requirement, in the standard's own words.
	Description string

	// Context is the object the check ran against, as a path through the
	// document. This is the half that says *where*, and it is the half a
	// summary usually drops.
	Context string

	// Message is what the validator said about it.
	Message string
}

func (f Failure) String() string {
	out := "  " + f.Clause + "-" + f.Test + ": " + f.Description
	if f.Context != "" {
		out += "\n    at " + f.Context
	}
	if f.Message != "" {
		out += "\n    " + f.Message
	}
	return out
}

// Validate checks a file against a PDF/A flavour such as "2b".
//
// A non-zero exit status means non-compliance rather than a failure of the
// validator, so it is read as a verdict rather than raised as an error. The
// distinction matters: a validator that could not run and a file that did not
// conform must not be reported the same way, because the first is a broken
// environment and the second is a broken artifact.
//
// The XML report is asked for rather than the text one. The text report says
// "FAIL <path> 2b" and nothing else, which names neither the clause nor the
// object — so a failure costs a second, manual invocation before anyone can
// begin. The XML carries the clause, the requirement's own wording, the object
// it failed on and the validator's message, which is the whole of what a person
// needs.
func (v VeraPDF) Validate(ctx context.Context, path, flavour string) (Report, error) {
	args, err := v.args(path, flavour)
	if err != nil {
		return Report{}, err
	}

	res, err := v.inner.Run(ctx, nil, args...)
	if err != nil {
		return Report{}, err
	}

	report, perr := parseReport(res.Stdout)
	report.Output = res.Combined()
	if perr != nil {
		return report, fmt.Errorf("the validator's report did not parse: %w", perr)
	}
	return report, nil
}

// args builds the invocation, which differs by where the validator lives.
//
// The container form mounts the file's directory read-only and gives the
// validator a path inside it. Mounting the directory rather than the file
// itself is what lets a runtime that cannot bind-mount a single file — several
// cannot — run this unchanged.
func (v VeraPDF) args(path, flavour string) ([]string, error) {
	if v.image == "" {
		return []string{"--format", "xml", "--flavour", flavour, path}, nil
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return []string{
		"run", "--rm",
		// Nothing here needs the network, and a validator that reached for one
		// would be fetching a profile whose contents nobody pinned.
		"--network", "none",
		"-v", filepath.Dir(abs) + ":/data:ro",
		v.image,
		"--format", "xml", "--flavour", flavour,
		"/data/" + filepath.Base(abs),
	}, nil
}

// xmlReport is the shape of veraPDF's XML output, reduced to what is read.
type xmlReport struct {
	XMLName xml.Name `xml:"report"`
	Build   struct {
		Releases []struct {
			ID      string `xml:"id,attr"`
			Version string `xml:"version,attr"`
		} `xml:"releaseDetails"`
	} `xml:"buildInformation"`
	Jobs struct {
		Jobs []struct {
			Validation struct {
				Compliant bool `xml:"isCompliant,attr"`
				Details   struct {
					Rules []xmlRule `xml:"rule"`
				} `xml:"details"`
			} `xml:"validationReport"`
		} `xml:"job"`
	} `xml:"jobs"`
}

type xmlRule struct {
	Clause      string `xml:"clause,attr"`
	Test        string `xml:"testNumber,attr"`
	Status      string `xml:"status,attr"`
	Description string `xml:"description"`
	Checks      []struct {
		Status  string `xml:"status,attr"`
		Context string `xml:"context"`
		Message string `xml:"errorMessage"`
	} `xml:"check"`
}

// parseReport reads the validator's XML.
func parseReport(raw []byte) (Report, error) {
	var doc xmlReport
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return Report{}, err
	}
	if len(doc.Jobs.Jobs) == 0 {
		return Report{}, fmt.Errorf("the report describes no job")
	}

	out := Report{Compliant: doc.Jobs.Jobs[0].Validation.Compliant}
	for _, r := range doc.Build.Releases {
		// "core" is the validation engine. The GUI and model components carry
		// their own versions and are not what produced the verdict.
		if r.ID == "core" {
			out.Version = r.Version
		}
	}

	for _, rule := range doc.Jobs.Jobs[0].Validation.Details.Rules {
		if rule.Status != "failed" {
			continue
		}
		f := Failure{Clause: rule.Clause, Test: rule.Test,
			Description: strings.TrimSpace(rule.Description)}
		for _, c := range rule.Checks {
			if c.Status == "failed" {
				f.Context = strings.TrimSpace(c.Context)
				f.Message = strings.TrimSpace(c.Message)
				break
			}
		}
		out.Failures = append(out.Failures, f)
	}
	return out, nil
}
