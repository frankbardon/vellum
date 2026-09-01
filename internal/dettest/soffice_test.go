//go:build soffice

package dettest_test

import (
	"bytes"
	"context"
	stderrors "errors"
	"sort"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/internal/dettest"
	"github.com/frankbardon/vellum/internal/exttool"
	"github.com/frankbardon/vellum/internal/oovalidate"
)

// officeExpectation is what an installed office reader must be able to see in a
// golden artifact.
type officeExpectation struct {
	// TextFilter is the LibreOffice filter that extracts the artifact's text,
	// and TextExt the extension it writes. Empty means this format has no text
	// filter worth trusting and only the open check runs.
	TextFilter string
	TextExt    string

	// WantText are substrings that must survive the round trip into that text.
	//
	// Prose and headings rather than incidental formatting, because the claim
	// under test is "the content reached the reader", and an assertion on
	// LibreOffice's own layout choices would fail on its next release for no
	// reason of ours.
	WantText []string
}

// officeExpectations is the per-case expectation table.
//
// Every golden whose extension is an office format needs a row, enforced below.
// A format epic that registers a case and does not say what a reader should see
// in it has registered a file, not a check.
var officeExpectations = map[string]officeExpectation{
	"docx-skeleton": {
		TextFilter: "txt:Text",
		TextExt:    "txt",
		WantText: []string{
			"Walking Skeleton",
			"The substrate carries a real artifact end to end.",
			"Why this exists",
		},
	},
	"pptx-master": {
		// Flat ODP rather than a text filter. Impress has no text export that
		// accepts a file destination, and this one carries what the others
		// drop: the speaker notes, which live on a part of their own and are
		// the half of the deck most likely to arrive unreferenced.
		TextFilter: "fodp:OpenDocument Presentation Flat XML",
		TextExt:    "fodp",
		WantText: []string{
			"Authored From The Theme",
			"No template ships with this deck",
			"Three content models",
			// An outline level, which proves the body placeholder's list style
			// reached the slide rather than merely reaching the master.
			"theme by reference, values untyped",
			// The speaker note. It is in a separate part reached through a
			// separate relationship, so seeing it proves that graph resolved.
			"Nothing here was copied from a shipped package.",
			// The theme's own font, read back off the master rather than off
			// the run: a deck whose runs name no family and whose master names
			// the wrong one renders in the reader's default and looks fine.
			"Helvetica Neue",
		},
	},
	"pptx-compose": {
		TextFilter: "fodp:OpenDocument Presentation Flat XML",
		TextExt:    "fodp",
		WantText: []string{
			"Composed to a Deck",
			"Three content models",
			"spec is unresolved and hashable.",
			// After a page break, under the title carried over.
			"doc, sheet, deck and pdf are laid out.",
			// A level-two heading, which stays a title at its own smaller size
			// rather than being promoted.
			"Evidence",
			// The note, out of its own part.
			"Say why the fragment earns its place.",
		},
	},
	"pptx-table": {
		TextFilter: "fodp:OpenDocument Presentation Flat XML",
		TextExt:    "fodp",
		WantText: []string{
			"Awareness by band",
			// The first and last body rows, which is the claim the split
			// makes: every row landed somewhere, on one slide or the next.
			"Band 1",
			"Band 26",
			// The banner, which repeats on every part.
			"North",
			// The row-header stub, merged down the left edge.
			"Age",
			// The caption, which follows the table's last part rather than
			// occupying a slide of its own.
			"Percentages. Base: all adults.",
		},
	},
	"docx-profile": {
		TextFilter: "txt:Text",
		TextExt:    "txt",
		WantText: []string{
			"Findings",
			"Unmarked prose",
			"Crosstab",
			// A formatted number, deliberately. It is the only assertion here
			// that proves the numfmt engine's output reached a reader rather
			// than merely reached the bytes.
			"41.2%",
			"Figure",
		},
	},
}

// officeExts are the extensions an office reader is expected to open. A golden
// outside this set — the raw substrate package, for one — is not an office
// document and is excluded deliberately rather than by omission.
var officeExts = map[string]bool{
	"docx": true,
	"xlsx": true,
	"pptx": true,
}

// TestOfficeReaderOpensGoldens checks every office golden against an installed
// LibreOffice.
//
// It is the answer to a gap the rest of the suite cannot close: every other
// assertion here compares our bytes against our bytes, which proves determinism
// and proves nothing about whether the file opens. Three defects have reached a
// human that way. See [oovalidate] for what this does and does not establish —
// LibreOffice is not Word, and a pass is evidence rather than proof.
//
// Run it with:
//
//	make test-office
//
// It is not part of `make test`, because it needs a LibreOffice installation
// and takes seconds per case rather than milliseconds.
func TestOfficeReaderOpensGoldens(t *testing.T) {
	tool := locateOffice(t)
	ctx := context.Background()

	for _, c := range dettest.Cases() {
		if !officeExts[c.Ext] {
			continue
		}
		exp, ok := officeExpectations[c.Name]
		if !ok {
			// Reported per case rather than in one completeness test, so the
			// failure names the case and appears next to its siblings.
			t.Errorf("golden %q is an office artifact with no entry in officeExpectations.\n"+
				"A registered case with no stated expectation checks nothing; add a row saying what a reader should see in it.", c.Name)
			continue
		}

		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			src := writeGoldenFile(t, c)

			// The open check. Converting to PDF is the cheapest way to make
			// LibreOffice parse, lay out and re-serialise the whole document:
			// it has to understand the file to do it. What comes out is never
			// compared, only required to exist and to be a PDF — its bytes vary
			// with the LibreOffice version and the installed fonts, which is
			// exactly the nondeterminism Vellum refuses to depend on.
			pdf, err := tool.Convert(ctx, src, "pdf", "pdf")
			if err != nil {
				t.Fatalf("LibreOffice could not open the %s golden.\n%v\n\n"+
					"The determinism suite cannot catch this class: it compares our bytes against our bytes, "+
					"and a file can be byte-identical across a thousand runs and still be one no reader accepts.", c.Ext, err)
			}
			if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
				t.Fatalf("LibreOffice wrote %d bytes that are not a PDF; the conversion did not do what it reported", len(pdf))
			}

			if exp.TextFilter == "" {
				return
			}

			raw, err := tool.Convert(ctx, src, exp.TextFilter, exp.TextExt)
			if err != nil {
				t.Fatalf("extracting text with the %q filter failed.\n%v", exp.TextFilter, err)
			}
			text := string(raw)
			for _, want := range exp.WantText {
				if !strings.Contains(text, want) {
					t.Errorf("a reader does not see %q in the artifact.\n\n"+
						"Either the content did not reach the file, or it reached it somewhere a reader does not look — "+
						"an element in a part nothing references, or one emitted outside the schema order its parent requires.\n\n"+
						"What LibreOffice extracted:\n%s", want, indentText(text))
				}
			}
		})
	}

	assertNoOrphanExpectations(t)
}

// assertNoOrphanExpectations fails when the table names a case that no longer
// exists.
//
// A renamed case would otherwise leave its expectation behind, still listed,
// still passing, and checking nothing at all — the same failure mode
// OrphanGoldens exists to prevent for the artifacts themselves.
func assertNoOrphanExpectations(t *testing.T) {
	t.Helper()

	live := make(map[string]bool)
	for _, c := range dettest.Cases() {
		live[c.Name] = true
	}
	for _, name := range sortedCaseNames(officeExpectations) {
		if !live[name] {
			t.Errorf("officeExpectations names %q, which is not a registered case. "+
				"A stale expectation checks nothing while appearing to.", name)
		}
	}
}

// locateOffice finds LibreOffice, skipping loudly when it is absent unless the
// environment demands it be present.
func locateOffice(t *testing.T) oovalidate.Tool {
	t.Helper()

	tool, err := oovalidate.Find()
	if err == nil {
		t.Logf("using LibreOffice at %s", tool.Path())
		return tool
	}

	var notFound *exttool.NotFoundError
	if !stderrors.As(err, &notFound) {
		// A misconfigured override is a real failure. Skipping it would hide
		// the fact that the tool the runner asked for is not the tool that ran.
		t.Fatalf("%v", err)
	}
	if exttool.RequireOptional() {
		t.Fatalf("%v\n\n%s is set, so a missing external tool fails rather than skips.",
			err, exttool.EnvRequireOptional)
	}
	t.Skipf("SKIPPING the office reader check: %v\n\n"+
		"Nothing else in this suite establishes that these artifacts open. "+
		"Set %s in CI so this cannot pass unnoticed forever.", err, exttool.EnvRequireOptional)
	return oovalidate.Tool{}
}

// sortedCaseNames is used by the diagnostics above; kept so the failure text is
// stable rather than map-ordered.
func sortedCaseNames(m map[string]officeExpectation) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
