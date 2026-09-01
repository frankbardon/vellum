package opc_test

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
)

const seedDir = "testdata/seeds"

// seedOutcome is what opening a seed must produce.
type seedOutcome struct {
	// openCode is the code Open must return, or the empty string when the
	// package must open successfully.
	openCode verr.Code

	// validateCode is the code Validate must return once opened. Only
	// meaningful when openCode is empty.
	validateCode verr.Code

	// maxPartBytes bounds the read, for seeds that exist to exercise the
	// bound.
	maxPartBytes int64
}

// seedExpectations pins the outcome of every committed seed.
//
// A file in the seed directory with no entry here fails the build, so a case
// cannot be added and quietly left unexercised.
var seedExpectations = map[string]seedOutcome{
	"valid-minimal.zip":         {},
	"dangling-relationship.zip": {validateCode: verr.VELLUM_OPC_RELATIONSHIP_INVALID},
	"traversal-entry-name.zip":  {openCode: verr.VELLUM_ZIP_ENTRY_NAME_INVALID},
	"missing-content-types.zip": {openCode: verr.VELLUM_OPC_INVALID},
	"malformed-rels.zip":        {openCode: verr.VELLUM_OPC_RELATIONSHIP_INVALID},
	"rels-missing-target.zip":   {openCode: verr.VELLUM_OPC_RELATIONSHIP_INVALID},
	"bomb.zip":                  {openCode: verr.VELLUM_ZIP_TOO_LARGE, maxPartBytes: 1 << 20},
	"truncated.zip":             {openCode: verr.VELLUM_ZIP_MALFORMED},
	"not-a-zip.zip":             {openCode: verr.VELLUM_ZIP_MALFORMED},
	"empty.zip":                 {openCode: verr.VELLUM_ZIP_MALFORMED},
}

func TestSeeds_ExpectedOutcomes(t *testing.T) {
	for _, name := range seedFiles(t) {
		want, ok := seedExpectations[name]
		if !ok {
			t.Errorf("seed %q has no entry in seedExpectations; add one, and a row to testdata/seeds/README.md", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			raw := readSeed(t, name)

			var opts []opc.OpenOption
			if want.maxPartBytes > 0 {
				opts = append(opts, opc.WithMaxPartBytes(want.maxPartBytes))
			}
			p, err := opc.Open(bytes.NewReader(raw), int64(len(raw)), opts...)

			if want.openCode != "" {
				if !verr.HasCode(err, want.openCode) {
					t.Fatalf("Open error = %v, want %s", err, want.openCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("Open failed on a seed expected to open: %v", err)
			}

			valErr := p.Validate()
			if want.validateCode == "" {
				if valErr != nil {
					t.Errorf("Validate failed on a seed expected to be consistent: %v", valErr)
				}
				return
			}
			if !verr.HasCode(valErr, want.validateCode) {
				t.Errorf("Validate error = %v, want %s", valErr, want.validateCode)
			}
		})
	}
}

// TestSeeds_ExpectationsHaveFiles catches the reverse omission: an expectation
// for a seed that has been deleted, which would otherwise sit here asserting
// nothing.
func TestSeeds_ExpectationsHaveFiles(t *testing.T) {
	present := make(map[string]bool)
	for _, name := range seedFiles(t) {
		present[name] = true
	}
	var orphans []string
	for name := range seedExpectations {
		if !present[name] {
			orphans = append(orphans, name)
		}
	}
	sort.Strings(orphans)
	if len(orphans) > 0 {
		t.Errorf("seedExpectations names files that do not exist: %v", orphans)
	}
}

// TestSeeds_NeverPanic is the blunt version of the guarantee: whatever a seed
// is, opening it terminates and returns.
func TestSeeds_NeverPanic(t *testing.T) {
	for _, name := range seedFiles(t) {
		raw := readSeed(t, name)
		for _, size := range []int64{int64(len(raw)), int64(len(raw)) / 2, 0, -1} {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("Open panicked on seed %q with declared size %d: %v", name, size, r)
					}
				}()
				//nolint:errcheck // the return value is irrelevant; not panicking is the assertion
				_, _ = opc.Open(bytes.NewReader(raw), size, opc.WithMaxPartBytes(1<<20))
			}()
		}
	}
}

func seedFiles(t testing.TB) []string {
	t.Helper()
	entries, err := os.ReadDir(seedDir)
	if err != nil {
		t.Fatalf("reading the seed corpus: %v", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".zip" {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	if len(out) == 0 {
		t.Fatal("the seed corpus is empty")
	}
	return out
}

func readSeed(t testing.TB, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(seedDir, name))
	if err != nil {
		t.Fatalf("reading seed %q: %v", name, err)
	}
	return raw
}
