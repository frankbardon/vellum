package gates

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/internal/pdfvalidate"
)

// ciWorkflowPath is the workflow the conformance gate runs in.
const ciWorkflowPath = repoRoot + "/.github/workflows/ci.yml"

// TestNoUnpinnedValidatorImage keeps the conformance gate's verdict
// attributable.
//
// The gate checks a claim each artifact makes about itself in its own metadata.
// A validator that can change without a commit makes that claim's verification
// unattributable: the job goes red one morning for a reason nobody can find,
// and — the half that matters more — it can go green the same way.
//
// Two conditions, and the second is the one that rots. The reference has to be
// a digest rather than a tag, because a tag is a name its publisher may
// repoint. And it has to be stated once: a digest written both in Go and in the
// workflow is a digest that will eventually disagree with itself, and the
// disagreement shows up as a job quietly running something else.
func TestNoUnpinnedValidatorImage(t *testing.T) {
	if pdfvalidate.PinnedImage == "" || pdfvalidate.PinnedVersion == "" {
		t.Fatalf("the validator pins are empty: image=%q version=%q",
			pdfvalidate.PinnedImage, pdfvalidate.PinnedVersion)
	}
	if !strings.Contains(pdfvalidate.PinnedImage, "@sha256:") {
		t.Errorf("PinnedImage is %q, which names the validator by tag.\n\n"+
			"A tag is a name its publisher may repoint, so the gate would report a verdict from "+
			"whatever the registry held at the moment it ran.", pdfvalidate.PinnedImage)
	}

	raw, err := os.ReadFile(ciWorkflowPath)
	if err != nil {
		t.Fatalf("reading %s: %v", ciWorkflowPath, err)
	}
	workflow := string(raw)

	if !strings.Contains(workflow, "validatorpin") {
		t.Errorf("%s does not go through internal/cmd/validatorpin to learn the validator "+
			"reference.\n\nThe reference is stated once, in Go. A workflow that names its own is "+
			"a second place for it to be wrong.", ciWorkflowPath)
	}
	for _, m := range digestPattern.FindAllString(workflow, -1) {
		t.Errorf("%s names a content digest of its own: %s\n\n"+
			"The conformance gate's validator is pinned in internal/pdfvalidate/pin.go and read "+
			"from there. A digest in the workflow too is a digest that will eventually disagree "+
			"with itself.", ciWorkflowPath, m)
	}
}

// digestPattern matches an OCI content digest.
//
// Deliberately not anchored to the veraPDF image. Any hard-coded digest in the
// workflow is a second source of truth for something, and the next one to
// appear will not be this one.
var digestPattern = regexp.MustCompile(`sha256:[0-9a-f]{64}`)
