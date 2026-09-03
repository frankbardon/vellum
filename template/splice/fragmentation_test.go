package splice_test

// This file is E9-S5's fragmentation-generator coverage: systematic
// discover -> splice tests (splice.Splice drives template/defrag's own
// Flatten/Locate internally for a marker anchor) over the five run-
// fragmentation shapes internal/fragtest models, complementing (not
// replacing) template/defrag's and template/splice's own hand-written unit
// tests from E9-S3/S4.

import (
	"bytes"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/internal/fragtest"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/template/splice"
	"github.com/frankbardon/vellum/xmlcopy"
)

func TestSplice_FragmentationStrategies(t *testing.T) {
	const markerName = "customer_name"
	const literal = "{{customer_name}}"

	cases := []struct {
		strategy fragtest.Strategy
		check    func(t *testing.T, out []byte)
	}{
		{
			strategy: fragtest.MidWordSpellCheck,
			check: func(t *testing.T, out []byte) {
				// Both source runs carried no formatting at all, so the
				// substituted run must not have picked one up from nowhere.
				if !bytes.Contains(out, []byte(`<w:r><w:t>Acme</w:t></w:r>`)) {
					t.Errorf("expected an unformatted replacement run: %s", out)
				}
			},
		},
		{
			strategy: fragtest.LanguageMarkBoundary,
			check: func(t *testing.T, out []byte) {
				// The formatting basis is the *first* touched run's own
				// rPr (en-US), never the second run's (en-GB), even though
				// the match consumes both runs entirely.
				if !bytes.Contains(out, []byte(`<w:rPr><w:lang w:val="en-US"/></w:rPr><w:t>Acme</w:t>`)) {
					t.Errorf("expected the first touched run's own w:lang as the formatting basis: %s", out)
				}
				if bytes.Contains(out, []byte(`en-GB`)) {
					t.Errorf("the discarded second run's w:lang leaked into the output: %s", out)
				}
			},
		},
		{
			strategy: fragtest.RevisionSaveIDSplit,
			check: func(t *testing.T, out []byte) {
				// The formatting basis is the first run's bold rPr. Neither
				// run's own w:rsidR/w:rsidRPr attributes -- carried on the
				// w:r element itself, not on w:rPr -- are read by
				// template/defrag at all (see the package-level finding
				// recorded in this file's own doc comment below), so their
				// presence must not prevent a correct splice.
				if !bytes.Contains(out, []byte(`<w:rPr><w:b/></w:rPr><w:t>Acme</w:t>`)) {
					t.Errorf("expected the first touched run's own bold rPr as the formatting basis: %s", out)
				}
				if bytes.Contains(out, []byte("<w:i/>")) {
					t.Errorf("the discarded second run's italic formatting leaked into the output: %s", out)
				}
			},
		},
		{
			strategy: fragtest.PasteBoundary,
			check: func(t *testing.T, out []byte) {
				// "mid" and "tail" are the two boundary runs' own
				// non-marker text and must survive as Prefix/Suffix
				// pieces. The two fully-consumed middle runs' bold and
				// italic formatting must not leak into anything.
				if !bytes.Contains(out, []byte(">mid<")) {
					t.Errorf("expected the prefix boundary run's own text ('mid') to survive: %s", out)
				}
				if !bytes.Contains(out, []byte("tail")) {
					t.Errorf("expected the suffix boundary run's own text ('tail') to survive: %s", out)
				}
				if !bytes.Contains(out, []byte("Acme")) {
					t.Errorf("expected the new value to appear between the two boundary runs: %s", out)
				}
				if bytes.Contains(out, []byte("<w:b/>")) || bytes.Contains(out, []byte("<w:i/>")) {
					t.Errorf("the two discarded fully-consumed middle runs' formatting leaked into the output: %s", out)
				}
			},
		},
		{
			strategy: fragtest.AcceptedTrackedChangeResidue,
			check: func(t *testing.T, out []byte) {
				// The empty w:ins shell sits outside the match's own
				// Affected span (it contributes no run for Flatten to
				// find) and must survive completely untouched, right next
				// to the correctly spliced marker.
				if !bytes.Contains(out, []byte(`<w:ins w:id="900" w:author="Reviewer" w:date="2020-01-01T00:00:00Z"/>`)) {
					t.Errorf("expected the adjacent empty w:ins shell to survive untouched: %s", out)
				}
				if !bytes.Contains(out, []byte("Acme")) {
					t.Errorf("expected the new value to appear: %s", out)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.strategy.String(), func(t *testing.T) {
			para := fragtest.Fragment(c.strategy, literal)
			src := wordDoc(string(para))
			pkg := buildPackage(t, src)
			a := markerAnchor(t, src, markerName)

			seq := fragment.Sequence{Blocks: []fragment.Block{
				textBlock(run("Acme", fragment.TextStyle{})),
			}}
			repl, err := splice.Splice(pkg, a, seq)
			if err != nil {
				t.Fatalf("Splice: %v", err)
			}
			out := mustApply(t, src, []xmlcopy.Replacement{repl})

			if bytes.Contains(out, []byte(literal)) {
				t.Errorf("the raw marker text survived the splice: %s", out)
			}
			c.check(t, out)
		})
	}
}

// TestSplice_FragmentationStrategiesAreDiscoverable proves the earlier
// stage of the pipeline too: anchor.Discover finds exactly one marker
// anchor named correctly, for every fragmentation shape, independent of
// splice.Splice succeeding.
func TestSplice_FragmentationStrategiesAreDiscoverable(t *testing.T) {
	for _, strategy := range fragtest.All() {
		t.Run(strategy.String(), func(t *testing.T) {
			para := fragtest.Fragment(strategy, "{{customer_name}}")
			src := wordDoc(string(para))
			pkg := buildPackage(t, src)

			inv, err := anchor.Discover(pkg, artifact.FormatDOCX, partDocument)
			if err != nil {
				t.Fatalf("Discover: %v", err)
			}
			if len(inv.Anchors) != 1 {
				t.Fatalf("got %d anchors, want 1: %+v", len(inv.Anchors), inv.Anchors)
			}
			if inv.Anchors[0].Kind != anchor.KindMarker {
				t.Errorf("kind = %q, want marker", inv.Anchors[0].Kind)
			}
			if inv.Anchors[0].Name != "customer_name" {
				t.Errorf("name = %q, want customer_name", inv.Anchors[0].Name)
			}
		})
	}
}

// A finding surfaced while building this file, recorded here rather than
// silently worked around: template/defrag's [defrag.Piece.RPr] and
// [defrag.Site.RunRPr] clone only a run's <w:rPr>...</w:rPr> child, never
// the <w:r> element's own attributes. A real Word-authored run legitimately
// carries w:rsidR / w:rsidRPr / w:rsidDel directly on <w:r ...>, stamped by
// Word whenever a save touches that run -- see the RevisionSaveIDSplit and
// AcceptedTrackedChangeResidue cases above, both of which put such
// attributes on the source runs. defrag.RenderRun writes a literal "<w:r>"
// with no attributes at all, so those revision-save-ID attributes do not
// survive onto a newly rendered run.
//
// This is not a functional bug: w:rsidR and its siblings are Word's own
// internal bookkeeping for its track-changes merge and "who last touched
// this run" tooling, carry no visible formatting, and are not required for
// a document to open or read correctly. No CLAUDE.md byte-layout invariant
// mentions them. Piece.RPr's own doc comment already states the
// verbatim-clone rule is scoped to rPr ("never reconstructed: a run's rPr
// can hold properties Vellum's own style model does not represent at all"),
// which reads naturally as being about rPr specifically, not about the run
// element's own attribute list -- so this is a real, if narrow, scope gap in
// what "verbatim-clone" carries forward, surfaced here per E9-S5's
// instructions rather than silently patched, and left unfixed: carrying
// w:r's own attributes through would touch template/defrag's runInfo and
// Piece shapes and RenderRun's output shape, which is more than this
// story's stated scope (testdata/corpus/defrag plus this story's own test
// infrastructure) should take on for a cosmetic-metadata gap nothing in
// this codebase's own conformance story currently asks for.
