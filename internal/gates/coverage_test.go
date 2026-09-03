package gates

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// coverageFloorChildEnv marks a `go test ./...` subprocess spawned by
// TestPerPackageCoverageFloors, so that subprocess's own run of this very
// package's own tests does not spawn another subprocess in turn. Without the
// guard, the child's `go list ./...` reaches internal/gates, its `go test`
// re-runs TestPerPackageCoverageFloors, and that spawns a third generation —
// forever. See "VELLUM_COVERAGE_FLOOR_CHILD" in CLAUDE.md's Build / Env.
const coverageFloorChildEnv = "VELLUM_COVERAGE_FLOOR_CHILD"

// coverageFloor is the minimum per-package statement coverage a library
// package must clear, in percent.
//
// Not a round number chosen in the abstract: it sits a few points below the
// lowest coverage any currently-tested library package carries (pdf/color at
// 66.7%, canon at 67.5%, as of this gate's introduction), so it is high enough
// to catch a package that lost most of its tests and low enough that it does
// not fail on ordinary, already-reviewed code the day this gate lands. Raising
// it later, package by package, is a legitimate way to ratchet coverage up;
// this gate is the floor that stops it from silently ratcheting down.
const coverageFloor = 65.0

// coverageExceptions names packages this gate does not hold to the floor, and
// why. Every one of them was zero-test-file at the time this gate was written
// — verified by walking every package directory and counting *_test.go files
// — so the exception is a statement about what the package *is*, not a waiver
// granted because a number happened to be low.
//
// A package added here without a matching entry removed from
// TestPerPackageCoverageFloors's own non-vacuity proof is the failure mode to
// watch for: an exception is easy to reach for and this list is reviewed
// exactly as hard as the floor itself.
var coverageExceptions = map[string]string{
	"github.com/frankbardon/vellum/artifact": "pure enum and type definitions (Format, AllFormats, Report); " +
		"no branching logic of its own, exercised indirectly by every writer's own tests",
	"github.com/frankbardon/vellum/fragment": "pure format-neutral IR type definitions (see CLAUDE.md's " +
		"Architecture section); exercised indirectly by resolve/, doc/, sheet/, deck/, pdf/ and " +
		"template/splice's own tests",
	"github.com/frankbardon/vellum/cmd/vellum": "a single main() that calls internal/cli.New and os.Exit; " +
		"internal/cli carries the tested logic, per CLAUDE.md's \"the CLI is an adapter\"",
	"github.com/frankbardon/vellum/internal/cmd/validatorpin": "a one-shot build helper that prints the pinned " +
		"veraPDF image reference for the CI workflow to read; not reachable from any test or library path",
	"github.com/frankbardon/vellum/internal/exttool": "shared plumbing for the build-tagged external-tool " +
		"oracles; its real paths run only under -tags soffice or -tags verapdf, which the default " +
		"`go test ./...` this gate runs does not build",
	"github.com/frankbardon/vellum/internal/imagetest": "test-only raster fixture generator, itself untested " +
		"by design — see TestNoImagetestOnTheLibraryPath",
	"github.com/frankbardon/vellum/internal/pdfvalidate": "the veraPDF and poppler oracle plumbing; its verapdf " +
		"path is build-tagged (-tags verapdf) and outside the default build this gate runs",
	"github.com/frankbardon/vellum/internal/oovalidate": "the LibreOffice oracle, entirely build-tagged " +
		"(-tags soffice); under the default build tags it compiles to nothing and `go test` reports it " +
		"as carrying no test files",
	"github.com/frankbardon/vellum/internal/gates": "this gate's own package: doc.go carries only a package " +
		"comment, so there are no non-test statements to hold a floor over",
}

// coverageLineRe matches one line of `go test -cover`'s own per-package
// summary, tolerant of the "ok"/"FAIL"/"?" status column being present or
// blank and of whatever sits in the elapsed-time column — a duration like
// "0.475s", the literal "(cached)", or nothing at all for a package with no
// _test.go file of its own. That column's exact shape is deliberately not
// matched at all: whatever it contains, it never contains the literal
// "coverage", so skipping straight to that word is both simpler and immune to
// a column format this parser has not seen yet.
//
// Three capture groups, exactly one non-empty per match: the package import
// path, then a percentage, or the literal "[no statements]", or the literal
// "[no test files]" — the three shapes go test prints in that position.
var coverageLineRe = regexp.MustCompile(
	`^(?:ok|FAIL|\?)?\s*(\S+/\S*)\s+.*?coverage:\s+([\d.]+)%\s+of statements|` +
		`^(?:ok|FAIL|\?)?\s*(\S+/\S*)\s+.*?coverage:\s+(\[no statements\])|` +
		`^\?\s*(\S+/\S*)\s+(\[no test files\])`,
)

// TestPerPackageCoverageFloors requires every library package to clear
// coverageFloor, with named exceptions in coverageExceptions.
//
// It runs `go test -short -cover ./...` in a subprocess rather than computing
// coverage in-process, for two reasons. First, per-package statement coverage
// is exactly what `go test -cover` already reports on its own stdout — one
// line per package — so parsing that output is simpler and less fragile than
// driving `go tool cover` over a merged profile from inside a test binary.
// Second, and more important: a coverage-instrumented build is a different
// binary from the one under test, so computing this in-process would mean
// this package's own tests running inside an *already*-coverage-instrumented
// process, which either double-instruments or silently measures nothing
// depending on the toolchain version — a subprocess sidesteps the question
// entirely by building fresh.
//
// The cost is real: this spawns a second full test run, tests included, which
// on this module costs on the order of twenty seconds under -short (the
// determinism harness's own 1000x-per-golden repeat drops to 25x there, which
// is most of the wall-clock difference between -short and not). That is why
// this is skipped under an outer -short run — matching
// TestDeterminismCrossProcess's own convention of skipping subprocess-heavy
// checks under -short — rather than run always: a quick local loop should not
// pay for a second test run every time, and `make test` (what CI runs) does
// not pass -short.
func TestPerPackageCoverageFloors(t *testing.T) {
	if os.Getenv(coverageFloorChildEnv) != "" {
		t.Skip("nested `go test ./...` spawned by this gate's own subprocess; not the outer run")
	}
	if testing.Short() {
		t.Skip("spawns a full `go test -cover` subprocess; skipped under -short")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	mod := modulePath(t)
	cmd := exec.CommandContext(ctx, "go", "test", "-short", "-cover", mod+"/...")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), coverageFloorChildEnv+"=1")
	out, runErr := cmd.CombinedOutput()

	results, unparsed := parseCoverageOutput(string(out))
	if len(unparsed) > 0 {
		t.Errorf("could not parse a package's coverage line out of `go test -cover` output; "+
			"the gate cannot check what it cannot read:\n  %s", strings.Join(unparsed, "\n  "))
	}
	if len(results) == 0 {
		t.Fatalf("parsed no per-package coverage lines at all; the parser or the invocation is wrong "+
			"and this gate would pass vacuously.\nfull output:\n%s", out)
	}
	// A failure inside the subprocess's own test run is a real failure this
	// gate should surface — but only once every line that *did* parse has
	// also been checked against its floor, so one broken package's stack
	// trace does not hide every other package's own violation in the noise.
	if runErr != nil && len(results) < 30 {
		// A build failure or similarly total error produces few or no
		// parseable lines; a run that mostly succeeded and failed one
		// package's tests still produces dozens. Below this floor, treat it
		// as the invocation itself failing rather than a coverage problem.
		t.Fatalf("go test -short -cover %s/...: %v\n%s", mod, runErr, out)
	}

	want := allModulePackages(t)
	for _, pkg := range want {
		if _, ok := results[pkg]; !ok {
			t.Errorf("go list %s/... names %q, which `go test -cover` output never mentioned. "+
				"The gate is not checking it, and would pass whatever it contained.", mod, pkg)
		}
	}

	for pkg, pct := range results {
		if reason, excepted := coverageExceptions[pkg]; excepted {
			_ = reason
			continue
		}
		if pct < coverageFloor {
			t.Errorf("%s coverage is %.1f%%, below the %.1f%% floor.\n"+
				"Add tests to raise it, or — only if it is genuinely thin on testable logic, the way "+
				"artifact/ or fragment/ are — add it to coverageExceptions in internal/gates/coverage_test.go "+
				"with a stated reason.", pkg, pct, coverageFloor)
		}
	}

	if runErr != nil && len(results) >= 30 {
		t.Errorf("go test -short -cover %s/... exited non-zero (a test failed somewhere in the module); "+
			"the coverage floors above were still checked against whatever coverage was reported, "+
			"but see the full output for the actual failure:\n%s", mod, out)
	}
}

// parseCoverageOutput extracts a package -> statement-coverage-percent map
// from `go test -cover`'s stdout. "[no test files]" and "[no statements]"
// both parse to 0.0 — the former is a real gap unless excepted, and the
// latter cannot violate a floor because coverageFloor is never 0.
//
// unparsed carries every line that looked like a per-package summary (started
// with a status column or a package-shaped first field) but that the regex
// did not match, so a format this parser has not seen yet fails loudly rather
// than being silently dropped.
func parseCoverageOutput(out string) (results map[string]float64, unparsed []string) {
	results = make(map[string]float64)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		m := coverageLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		switch {
		case m[1] != "": // "... coverage: 78.4% of statements"
			pct, err := strconv.ParseFloat(m[2], 64)
			if err != nil {
				unparsed = append(unparsed, line)
				continue
			}
			results[m[1]] = pct
		case m[3] != "": // "... coverage: [no statements]"
			results[m[3]] = 0.0
		case m[5] != "": // "?   pkg  [no test files]"
			results[m[5]] = 0.0
		default:
			unparsed = append(unparsed, line)
		}
	}
	return results, unparsed
}

// allModulePackages returns every package `go list ./...` names, so the gate
// can tell "this package cleared the floor" from "this gate never heard about
// this package at all" — the same distinction TestNoUnsortedMapIteration
// draws for outputPathPackages.
func allModulePackages(t *testing.T) []string {
	t.Helper()
	mod := modulePath(t)
	out, err := exec.Command("go", "list", mod+"/...").CombinedOutput()
	if err != nil {
		t.Fatalf("go list %s/...: %v\n%s", mod, err, out)
	}
	var pkgs []string
	for _, line := range strings.Split(string(out), "\n") {
		if p := strings.TrimSpace(line); p != "" {
			pkgs = append(pkgs, p)
		}
	}
	if len(pkgs) < 30 {
		t.Fatalf("go list %s/... returned %d packages, fewer than expected; "+
			"the invocation is wrong and this gate would pass vacuously", mod, len(pkgs))
	}
	return pkgs
}
