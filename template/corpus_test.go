package template_test

// This file is E9-S5's real-corpus adapter: TestDefragCorpusComplete, named
// in CLAUDE.md's Non-Skippable CI Gates list. Per interview.md decision 6,
// the real Word-authored corpus does not exist yet; this gate's directory
// walk *is* the manifest, so dropping a real .docx in later requires no
// code change here, and the gate is live -- passing trivially, not
// skipped -- from day one, even while testdata/corpus/defrag/ is empty. See
// testdata/corpus/defrag/README.md for the schema and the gap in full.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/template"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/template/splice"
)

// defragCorpusDir is testdata/corpus/defrag/ relative to this package's own
// directory, which is where `go test` runs with its working directory set.
const defragCorpusDir = "../testdata/corpus/defrag"

// caseManifest is expect.json's schema. Deliberately minimal -- three
// fields -- and expected to grow once a real fixture reveals what else is
// actually needed; see testdata/corpus/defrag/README.md, which is the
// canonical description and must be kept in sync with this struct.
type caseManifest struct {
	// Description is free text: where the fixture came from, what it
	// exercises. Not checked, only carried, for a human reading the corpus.
	Description string `json:"description"`

	// Anchors is every anchor discovery is required to find in this
	// fixture.
	Anchors []anchorExpect `json:"anchors"`
}

// anchorExpect is one expected anchor.Anchor.
type anchorExpect struct {
	// Name is the anchor's binding key -- the w:tag value for a native
	// anchor, or the marker's own name for a {{marker}} anchor.
	Name string `json:"name"`

	// Kind is "native" or "marker", checked against anchor.Anchor.Kind.
	Kind string `json:"kind"`

	// Splice, when true, additionally requires template/splice.Splice to
	// succeed against this anchor with a trivial one-paragraph
	// fragment.Sequence. False (the default) means discovery alone is
	// checked -- a fixture may carry an anchor this manifest does not yet
	// assert splices cleanly, e.g. one recorded as a known gap.
	Splice bool `json:"splice,omitempty"`
}

// TestDefragCorpusComplete walks testdata/corpus/defrag/ for case
// directories and requires each to carry exactly one .docx fixture and one
// expect.json describing it. A directory carrying only one of the two fails
// the build; a directory carrying neither is not a case at all and is
// skipped (that is how testdata/corpus/defrag/README.md itself, which is
// not a directory, coexists with case directories).
//
// Zero case directories -- the corpus's current, honest state -- means zero
// cases to check, so this passes trivially rather than being skipped: the
// gate is live from day one, not "live once the corpus exists."
func TestDefragCorpusComplete(t *testing.T) {
	entries, err := os.ReadDir(defragCorpusDir)
	if err != nil {
		t.Fatalf("reading %s: %v\n\n"+
			"testdata/corpus/defrag/ must exist even while empty -- see its own README.md.", defragCorpusDir, err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			// testdata/corpus/defrag/README.md itself lives beside the case
			// directories, not inside one.
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			runDefragCorpusCase(t, filepath.Join(defragCorpusDir, name))
		})
	}
}

// runDefragCorpusCase checks one case directory's structural completeness
// and, when both halves are present, runs the fixture through discovery
// (and splice, where expect.json asks for it).
func runDefragCorpusCase(t *testing.T, dir string) {
	t.Helper()

	docxPaths, globErr := filepath.Glob(filepath.Join(dir, "*.docx"))
	if globErr != nil {
		t.Fatalf("globbing %s for a .docx fixture: %v", dir, globErr)
	}
	expectPath := filepath.Join(dir, "expect.json")
	_, statErr := os.Stat(expectPath)
	hasExpect := statErr == nil

	switch {
	case len(docxPaths) == 0 && !hasExpect:
		t.Fatalf("case directory %s has neither a .docx fixture nor expect.json; "+
			"remove the directory if it is not meant to be a case, or add both", dir)
	case len(docxPaths) == 0:
		t.Fatalf("case directory %s has expect.json but no .docx fixture; "+
			"a case needs both, or neither -- an expect.json with nothing to run it against fails the build", dir)
	case !hasExpect:
		t.Fatalf("case directory %s has a .docx fixture (%s) but no expect.json describing what discovery should "+
			"find in it; add one -- see testdata/corpus/defrag/README.md for the schema",
			dir, filepath.Base(docxPaths[0]))
	case len(docxPaths) > 1:
		t.Fatalf("case directory %s has more than one .docx fixture (%v); a case directory is exactly one template",
			dir, docxPaths)
	}

	raw, err := os.ReadFile(expectPath)
	if err != nil {
		t.Fatalf("reading %s: %v", expectPath, err)
	}
	var manifest caseManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("%s does not parse as JSON: %v", expectPath, err)
	}
	if len(manifest.Anchors) == 0 {
		t.Fatalf("%s declares zero expected anchors; a case that checks nothing is not testing anything -- "+
			"name at least one anchor discovery should find", expectPath)
	}
	for i, a := range manifest.Anchors {
		if a.Name == "" {
			t.Fatalf("%s: anchors[%d] has no name", expectPath, i)
		}
		if a.Kind != string(anchor.KindNative) && a.Kind != string(anchor.KindMarker) {
			t.Fatalf("%s: anchors[%d] (%q) has kind %q, want %q or %q",
				expectPath, i, a.Name, a.Kind, anchor.KindNative, anchor.KindMarker)
		}
	}

	docxBytes, err := os.ReadFile(docxPaths[0])
	if err != nil {
		t.Fatalf("reading %s: %v", docxPaths[0], err)
	}

	tmpl, err := template.Open(bytes.NewReader(docxBytes), int64(len(docxBytes)))
	if err != nil {
		t.Fatalf("template.Open(%s): %v", docxPaths[0], err)
	}
	if tmpl.Format() != artifact.FormatDOCX {
		t.Fatalf("%s: format = %v, want DOCX -- testdata/corpus/defrag/ is a DOCX-only corpus today", docxPaths[0], tmpl.Format())
	}

	inv, err := anchor.Discover(tmpl.Package(), tmpl.Format(), tmpl.MainPart())
	if err != nil {
		t.Fatalf("Discover(%s): %v", docxPaths[0], err)
	}
	discovered := make(map[string]anchor.Anchor, len(inv.Anchors))
	for _, a := range inv.Anchors {
		discovered[a.Name] = a
	}

	for _, want := range manifest.Anchors {
		got, ok := discovered[want.Name]
		if !ok {
			t.Errorf("%s: expected anchor %q was not discovered; discovery found %s",
				expectPath, want.Name, discoveredNames(inv))
			continue
		}
		if string(got.Kind) != want.Kind {
			t.Errorf("%s: anchor %q has kind %q, want %q", expectPath, want.Name, got.Kind, want.Kind)
		}
		if !want.Splice {
			continue
		}
		seq := fragment.Sequence{Blocks: []fragment.Block{
			{Kind: spec.BlockText, Paragraph: &fragment.Paragraph{
				Runs: []fragment.Run{{Text: "corpus test value"}},
			}},
		}}
		if _, err := splice.Splice(tmpl.Package(), got, seq); err != nil {
			t.Errorf("%s: anchor %q was expected to splice cleanly (\"splice\": true) but did not: %v",
				expectPath, want.Name, err)
		}
	}
}

func discoveredNames(inv *anchor.Inventory) string {
	if len(inv.Anchors) == 0 {
		return "no anchors at all"
	}
	var names []string
	for _, a := range inv.Anchors {
		names = append(names, string(a.Kind)+":"+a.Name)
	}
	return "[" + strings.Join(names, ", ") + "]"
}
