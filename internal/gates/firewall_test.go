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

// TestNoExternalToolingOnTheLibraryPath keeps the test oracles out of the
// shipped library.
//
// internal/exttool and the packages built on it drive installed programs —
// LibreOffice, poppler, veraPDF — which means they shell out, the one thing
// CLAUDE.md's "do not shell out to anything" forbids. That rule governs the
// library: a consumer embedding Vellum gets a pure-Go writer that runs no
// subprocess, performs no I/O it was not handed, and behaves identically
// whether or not any of those programs is installed. The oracles are test
// fixtures that read artifacts Vellum already wrote, and they stay that.
//
// Two mechanisms hold the line and this is the second. The first is the build
// tags on the heavier oracles, which keep them from linking into a build that
// did not ask for them. That alone would be enough today and would stop being
// enough the moment somebody removed a tag for convenience, so the import graph
// is checked as well — and the poppler oracle deliberately carries no tag, so
// for that one this test is the only thing holding it.
func TestNoExternalToolingOnTheLibraryPath(t *testing.T) {
	assertNotReachableFromShipped(t,
		[]string{
			"github.com/frankbardon/vellum/internal/exttool",
			"github.com/frankbardon/vellum/internal/oovalidate",
			"github.com/frankbardon/vellum/internal/pdfvalidate",
		},
		"these packages run subprocesses. The library runs none: a consumer embedding Vellum "+
			"must get the same bytes whether or not an office suite or a PDF validator is installed.")
}

// TestNoTestFontOnTheLibraryPath keeps the test face out of the shipped
// library.
//
// The determinism fixtures embed Go Regular, which is a font program compiled
// into the module. A shipped package reaching it would have a font available
// without the theme having supplied one — and the moment a fallback face is
// reachable, some code path will eventually reach for it. Fonts come from the
// theme, only: a face that arrives any other way makes the same specification
// render differently depending on what happened to be linked in.
//
// This is the same invariant TestNoFontscanImport defends from the other side.
// That one keeps the machine's fonts out; this one keeps ours out.
func TestNoTestFontOnTheLibraryPath(t *testing.T) {
	assertNotReachableFromShipped(t,
		[]string{
			"golang.org/x/image/font/gofont/goregular",
			"golang.org/x/image/font/sfnt",
		},
		"fonts come from the theme, only. A face reachable from the library is a fallback waiting "+
			"to be used, and a fallback makes the same specification render differently on two machines.")
}

// TestNoImagetestOnTheLibraryPath keeps the generated image fixtures out of the
// shipped library.
//
// internal/imagetest draws gradients and encodes them, which is one step from
// rasterising — the thing Vellum does not do. A shipped package reaching it
// would have a source of pictures the host did not supply, and the whole asset
// seam exists so that every image in a document came from the host.
//
// It is the same argument as the test face, applied to the other kind of asset:
// what makes a fallback dangerous is that it works.
//
// The check is on this package and not on image/png and image/jpeg, which would
// be the stronger statement and is not available: go-text/typesetting's font
// package reads colour bitmap glyph tables, so both encoders are already in the
// graph transitively, through the dependency Vellum takes for shaping. Vellum
// asks for neither. That the firewall cannot be drawn at the stdlib boundary is
// worth knowing rather than worth pretending — the boundary that is actually
// enforceable is this one.
func TestNoImagetestOnTheLibraryPath(t *testing.T) {
	assertNotReachableFromShipped(t,
		[]string{"github.com/frankbardon/vellum/internal/imagetest"},
		"images come from the host, only. Vellum embeds bytes it is handed: it does not draw them "+
			"and does not re-encode them.")
}

// assertNotReachableFromShipped fails when any target is in the dependency
// graph of a package outside internal/.
func assertNotReachableFromShipped(t *testing.T, targets []string, why string) {
	t.Helper()

	shipped := shippedPackages(t)
	args := append([]string{"list", "-deps"}, shipped...)
	out, err := exec.Command("go", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}

	reachable := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		reachable[strings.TrimSpace(line)] = true
	}
	for _, target := range targets {
		if reachable[target] {
			t.Errorf("%s is reachable from a shipped package.\n\n%s\n\n"+
				"Find the importer with:\n  go mod why %s", target, why, target)
		}
	}
}

// shippedPackages lists the module's packages that are not under internal/.
//
// The check runs over these rather than over ./... because ./... includes the
// test tooling itself, which is allowed to reach the oracle and is the whole
// reason it exists.
func shippedPackages(t *testing.T) []string {
	t.Helper()

	out, err := exec.Command("go", "list", modulePath(t)+"/...").CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	var pkgs []string
	for _, line := range strings.Split(string(out), "\n") {
		p := strings.TrimSpace(line)
		if p == "" || strings.Contains(p, "/internal/") || strings.HasSuffix(p, "/internal") {
			continue
		}
		pkgs = append(pkgs, p)
	}
	if len(pkgs) == 0 {
		t.Fatal("no shipped packages found; the filter is wrong and this gate would pass vacuously")
	}
	return pkgs
}

// assertNotInDependencyGraph fails when target appears anywhere in the module's
// transitive dependencies.
func assertNotInDependencyGraph(t *testing.T, target, why string) {
	t.Helper()

	for _, dep := range moduleDeps(t) {
		if dep == target {
			t.Fatalf("%s is in the dependency graph.\n\n%s\n\n"+
				"Find the importer with:\n  go mod why %s", target, why, target)
		}
	}
}

// moduleDeps returns the transitive dependencies of every package in the
// module.
//
// The pattern is the module path rather than "./...", because a test runs with
// its own package directory as the working directory, where "./..." means this
// one package. Every firewall in this file was scanning a graph of exactly one
// entry and passing on all of them — which is the failure mode a firewall has:
// it cannot report the absence of something it never looked for. The floor
// check below is the guard against that recurring.
func moduleDeps(t *testing.T) []string {
	t.Helper()

	out, err := exec.Command("go", "list", "-deps", modulePath(t)+"/...").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}

	var deps []string
	for _, line := range strings.Split(string(out), "\n") {
		if p := strings.TrimSpace(line); p != "" {
			deps = append(deps, p)
		}
	}

	// A firewall that scans an empty or near-empty graph passes for the wrong
	// reason. The floor sits well below the real count and well above what a
	// mis-scoped pattern yields, so it fires on the mistake rather than on
	// ordinary growth.
	const floor = 25
	if len(deps) < floor {
		t.Fatalf("the dependency graph has %d entries, fewer than the floor of %d. "+
			"The pattern is wrong and every firewall in this file would pass vacuously", len(deps), floor)
	}
	return deps
}

// modulePath returns the module under test.
func modulePath(t *testing.T) string {
	t.Helper()

	out, err := exec.Command("go", "list", "-m").Output()
	if err != nil {
		t.Fatalf("go list -m: %v", err)
	}
	module := strings.TrimSpace(string(out))
	if module == "" {
		t.Fatal("go list -m returned no module path")
	}
	return module
}
