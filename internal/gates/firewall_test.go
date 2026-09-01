package gates

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoFontscanImport is the firewall against system font scanning.
//
// go-text/typesetting is a dependency Vellum needs for shaping and line
// breaking, and it ships fontscan, which enumerates the fonts installed on the
// machine. A single import of it anywhere reachable would make the same
// specification render differently on two machines — silently, and only for
// documents that happen to use a face the theme did not fully pin.
//
// Checked with `go list -deps` rather than by grepping this repository, because
// the danger is a transitive import: a package we add that innocently pulls it
// in is exactly the case a grep would miss.
func TestNoFontscanImport(t *testing.T) {
	assertNotInDependencyGraph(t, "github.com/go-text/typesetting/fontscan",
		"fonts come from the theme, only. A system font scan makes the same specification "+
			"render differently on two machines, which defeats byte-identical output and the "+
			"consumer dedupe that rests on it.")
}

// TestNoCgoImports pins that the module builds with CGO_ENABLED=0.
//
// There is no carve-out. A cgo dependency would make Vellum need a C toolchain
// to `go get`, and the whole point of owning the PDF writer and the subsetter
// rather than binding to an existing C library is that it does not.
func TestNoCgoImports(t *testing.T) {
	assertNotInDependencyGraph(t, "C",
		"nothing imports \"C\". The library must stay go-gettable without a C toolchain, "+
			"which is why the PDF writer and the subsetter are ours.")
}

// TestNoCollateImport keeps locale-aware ordering out.
//
// x/text is a direct dependency, for rendering schema validator faults. Its
// collate package sorts by locale, which is nondeterminism with extra steps: the
// same set of strings comes out in a different order depending on the collation
// tables in play. Bytewise sort.Strings, everywhere.
func TestNoCollateImport(t *testing.T) {
	assertNotInDependencyGraph(t, "golang.org/x/text/collate",
		"ordering is bytewise, always. Locale-aware ordering would make the same registry "+
			"enumerate differently on two machines.")
}

// assertNotInDependencyGraph fails when target appears anywhere in the module's
// transitive dependencies.
func assertNotInDependencyGraph(t *testing.T, target, why string) {
	t.Helper()

	out, err := exec.Command("go", "list", "-deps", "./...").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == target {
			t.Fatalf("%s is in the dependency graph.\n\n%s\n\n"+
				"Find the importer with:\n  go mod why %s", target, why, target)
		}
	}
}
