package dettest_test

import (
	"bytes"
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/internal/dettest"
	"github.com/frankbardon/vellum/internal/exttool"
	"github.com/frankbardon/vellum/internal/pdfvalidate"
	"github.com/frankbardon/vellum/opc/zipdet"
)

// pdfExpectation is what an independent reader must see in a PDF golden.
type pdfExpectation struct {
	// WantText are substrings the extracted text must contain.
	//
	// Extraction is the check worth making. It exercises the whole text path at
	// once: the composite font's Identity-H encoding, the ToUnicode CMap and the
	// glyph identifiers in the content stream all have to agree for one word to
	// come back correct, and each of the three is written by different code.
	WantText []string

	// WantInfo are information-dictionary fields and their required values.
	WantInfo [][2]string
}

// pdfExpectations is the per-case expectation table. Every PDF golden needs a
// row, enforced below.
var pdfExpectations = map[string]pdfExpectation{
	"pdf-spike": {
		WantText: []string{
			"Hello, PDF/A",
			"Composed by Vellum. Every glyph here is embedded as a subset.",
		},
		WantInfo: [][2]string{
			{"Title", "Hello, PDF/A"},
			{"Producer", "Vellum 0.0.0-golden"},
			{"Pages", "1"},
			{"Encrypted", "no"},
			{"PDF version", "1.7"},
			// PDF/A requires the XMP packet. A reader reporting no metadata
			// stream means the conformance claim is not even present, let alone
			// true.
			{"Metadata Stream", "yes"},
		},
	},
	"pdf-pages": {
		WantText: []string{
			"Page 1 of 20",
			"Page 8 of 20",
			// The first page of the last, partly filled group at the deepest
			// level: the one the fold is most likely to get wrong.
			"Page 17 of 20",
			"Page 20 of 20",
		},
		WantInfo: [][2]string{
			{"Title", "Twenty Pages"},
			{"Pages", "20"},
			{"Encrypted", "no"},
			{"Metadata Stream", "yes"},
		},
	},
	"pdf-prose": {
		WantText: []string{
			"Justified",
			"Flush left",
			// A phrase that spans a line break in the wrapped output, so
			// extraction proves the lines were drawn in order and the
			// justification adjustments did not reorder or drop anything.
			"harfbuzz",
			"two-byte encoding.",
		},
		WantInfo: [][2]string{
			{"Title", "Justified Prose"},
			{"Pages", "1"},
			{"Metadata Stream", "yes"},
		},
	},
}

// TestPDFReaderSeesTheContent checks every PDF golden with poppler.
//
// It runs in the ordinary suite rather than behind a tag, because poppler is
// small, fast and commonly installed, and because this is the only assertion in
// the project that establishes a PDF is readable at all. Everything else
// compares Vellum's bytes against Vellum's bytes.
//
// It does not check PDF/A conformance. TestPDFAConformance does, with veraPDF.
func TestPDFReaderSeesTheContent(t *testing.T) {
	tool := locatePoppler(t)
	ctx := context.Background()

	for _, c := range dettest.Cases() {
		if c.Ext != "pdf" {
			continue
		}
		exp, ok := pdfExpectations[c.Name]
		if !ok {
			t.Errorf("golden %q is a PDF with no entry in pdfExpectations.\n"+
				"A registered case with no stated expectation checks nothing; add a row saying what a reader should see in it.", c.Name)
			continue
		}

		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			path := writeGoldenFile(t, c)

			info, err := tool.Info(ctx, path)
			if err != nil {
				t.Fatalf("an independent reader could not open the document.\n%v\n\n"+
					"The determinism suite cannot catch this: it compares our bytes against our bytes, "+
					"and a file can be byte-identical across a thousand runs and still be one no reader accepts.", err)
			}
			for _, want := range exp.WantInfo {
				if got := info.Get(want[0]); got != want[1] {
					t.Errorf("%s is %q, want %q. What the reader reported:\n%s",
						want[0], got, want[1], info)
				}
			}

			text, err := tool.Text(ctx, path)
			if err != nil {
				t.Fatalf("extracting text failed.\n%v", err)
			}
			for _, want := range exp.WantText {
				if !strings.Contains(text, want) {
					t.Errorf("a reader does not see %q in the document.\n\n"+
						"The glyphs may be drawn correctly and still not extract: that happens when the ToUnicode "+
						"CMap disagrees with the glyph identifiers the content stream shows, which is the one "+
						"failure a rendered page looks perfectly fine for.\n\nWhat was extracted:\n%s",
						want, indentText(text))
				}
			}
		})
	}

	assertNoOrphanPDFExpectations(t)
}

// locatePoppler finds the reader, skipping loudly when it is absent.
func locatePoppler(t *testing.T) pdfvalidate.Poppler {
	t.Helper()

	tool, err := pdfvalidate.FindPoppler()
	if err == nil {
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
	t.Skipf("SKIPPING the PDF reader check: %v\n\n"+
		"Nothing else in this suite establishes that these documents open or that their text extracts. "+
		"Set %s in CI so this cannot pass unnoticed forever.", err, exttool.EnvRequireOptional)
	return pdfvalidate.Poppler{}
}

// assertNoOrphanPDFExpectations fails when the table names a case that no
// longer exists.
func assertNoOrphanPDFExpectations(t *testing.T) {
	t.Helper()

	live := make(map[string]bool)
	for _, c := range dettest.Cases() {
		live[c.Name] = true
	}
	names := make([]string, 0, len(pdfExpectations))
	for name := range pdfExpectations {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if !live[name] {
			t.Errorf("pdfExpectations names %q, which is not a registered case. "+
				"A stale expectation checks nothing while appearing to.", name)
		}
	}
}

// writeGoldenFile materialises a case's committed golden into a temp file.
//
// The committed golden rather than a fresh emission, deliberately: the artifact
// checked here is the one under review in the diff. Emitting afresh would let a
// stale golden pass while the file a reader would actually receive differs.
func writeGoldenFile(t *testing.T, c dettest.Case) string {
	t.Helper()

	m, err := dettest.LoadManifest(dettest.GoldenRoot)
	if err != nil {
		t.Fatalf("%v", err)
	}
	raw, err := dettest.ReadGolden(dettest.GoldenRoot, c, m)
	if err != nil {
		t.Fatalf("%v", err)
	}
	got, err := c.Bytes(zipdet.PinnedEpoch)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	if !bytes.Equal(raw, got) {
		t.Skipf("the golden and the current output differ; TestGoldensNotHandEdited owns that failure. " +
			"Checking a stale artifact against a reader would report on a file nobody ships.")
	}

	path := filepath.Join(t.TempDir(), c.Name+"."+c.Ext)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("writing the artifact: %v", err)
	}
	return path
}

// indentText renders extracted text for a failure message, bounded so a long
// document does not bury the assertion that failed.
func indentText(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	const max = 40
	truncated := false
	if len(lines) > max {
		lines, truncated = lines[:max], true
	}
	for i := range lines {
		lines[i] = "    " + lines[i]
	}
	out := strings.Join(lines, "\n")
	if truncated {
		out += "\n    ... (truncated)"
	}
	return out
}
