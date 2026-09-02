//go:build verapdf

package dettest_test

import (
	"context"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/internal/dettest"
	"github.com/frankbardon/vellum/internal/exttool"
	"github.com/frankbardon/vellum/internal/pdfvalidate"
)

// Flavour is the PDF/A conformance level Vellum claims.
//
// 2b — "basic" — guarantees the visual appearance is reproducible in the long
// term: every font embedded, colour with a defined interpretation, metadata
// stated in XMP and agreeing with the information dictionary. Levels A and U
// additionally require tagging and reliable text extraction, which are separate
// commitments the block model is not yet in a position to make.
const Flavour = "2b"

// TestPDFAConformance validates every PDF golden against the flavour it claims.
//
// The file asserts its own conformance, in its own metadata, to any consumer
// that reads it. An assertion nothing checks is worse than no assertion: a
// consumer archiving on the strength of it has been told something false by the
// artifact itself. So the claim is verified against the reference validator
// rather than reasoned about.
//
// Behind a build tag because veraPDF is a JVM application nobody has installed
// by accident. CI provisions it and sets VELLUM_REQUIRE_OPTIONAL_GATES, so the
// gate cannot pass by never running.
func TestPDFAConformance(t *testing.T) {
	tool := locateVeraPDF(t)
	ctx := context.Background()

	checked := 0
	for _, c := range dettest.Cases() {
		if c.Ext != "pdf" {
			continue
		}
		checked++

		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			report, err := tool.Validate(ctx, writeGoldenFile(t, c), Flavour)
			if err != nil {
				t.Fatalf("the validator did not run: %v", err)
			}
			assertPinnedVersion(t, report)

			if !report.Compliant {
				t.Errorf("the document claims PDF/A-%s and does not conform.\n\n%s\n\n"+
					"Each failed clause names the requirement and the object it failed on. The usual "+
					"causes, in order of how often they are the answer: the XMP dates disagreeing with "+
					"the information dictionary, a property written into the packet without a PDF/A "+
					"extension schema describing it, a font descriptor missing a required key, and an "+
					"output intent whose profile the validator cannot parse.",
					Flavour, renderFailures(report))
			}
		})
	}

	if checked == 0 {
		t.Fatal("no PDF goldens were checked; this gate would pass vacuously")
	}
}

// assertPinnedVersion refuses a verdict from a validator nobody pinned.
//
// The version comes out of the report the validator just produced, not from a
// separate invocation, so the verdict and the version cannot describe two
// different programs.
//
// A mismatch fails rather than warns. A conformance verdict is only worth
// something if everyone reading the build knows which validator gave it: two
// releases disagree about clauses at the margins, and a corpus that passes
// under one and fails under the next has changed nothing about itself. Moving
// the pin is therefore a commit, with whatever the new validator says about the
// corpus in the same commit.
func assertPinnedVersion(t *testing.T, report pdfvalidate.Report) {
	t.Helper()

	if report.Version == "" {
		t.Fatalf("the validator's report states no version.\n\n" +
			"The verdict cannot be attributed to a release, so it is not a verdict this gate can accept.")
	}
	if report.Version != pdfvalidate.PinnedVersion {
		t.Fatalf("veraPDF %s produced this verdict; the corpus is pinned to %s.\n\n"+
			"Two releases disagree about clauses at the margins, so a corpus that passes under one "+
			"and fails under the next has changed nothing about itself. Move the pin deliberately: "+
			"update PinnedVersion and PinnedImage in internal/pdfvalidate/verapdf.go in one commit, "+
			"with whatever the new validator says about the corpus.",
			report.Version, pdfvalidate.PinnedVersion)
	}
}

// renderFailures lays out the failed clauses, one per requirement.
func renderFailures(report pdfvalidate.Report) string {
	if len(report.Failures) == 0 {
		// Non-compliant with nothing itemised means the report was not the
		// shape this reader expects, so its own text is all there is.
		return report.Output
	}
	var b strings.Builder
	for _, f := range report.Failures {
		b.WriteString(f.String())
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// locateVeraPDF finds the validator, skipping loudly when it is absent.
func locateVeraPDF(t *testing.T) pdfvalidate.VeraPDF {
	t.Helper()

	tool, err := pdfvalidate.FindVeraPDF()
	if err == nil {
		t.Logf("using veraPDF: %s", tool.Describe())
		return tool
	}

	var notFound *exttool.NotFoundError
	if !stderrors.As(err, &notFound) {
		t.Fatalf("%v", err)
	}
	if exttool.RequireOptional() {
		t.Fatalf("%v\n\n%s is set, so a missing external tool fails rather than skips.",
			err, exttool.EnvRequireOptional)
	}
	t.Skipf("SKIPPING the PDF/A conformance check: %v\n\n"+
		"The documents claim conformance in their own metadata and nothing here has verified it. "+
		"Set %s in CI so this cannot pass unnoticed forever.", err, exttool.EnvRequireOptional)
	return pdfvalidate.VeraPDF{}
}
