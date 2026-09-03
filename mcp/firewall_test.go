package mcp_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoGoSDKImport is the firewall CLAUDE.md's package map promises:
// "Only mcp/gosdk imports the SDK." This package — the SDK-free core — is
// the one every other rule in that sentence exists to protect: its typed
// contracts, its schema reflection and its facade-only handlers must be
// usable, and testable, without ever linking modelcontextprotocol/go-sdk.
//
// Checked with `go list -deps`, transitively — not merely a direct-import
// grep — in the same style internal/gates' own TestNoFontscanImport and
// TestNoCollateImport check their own firewalls: the danger worth guarding
// against is a transitive one, a future addition to this package that
// innocently pulls the SDK in through some other dependency. Transitive is
// the right question for this package specifically because it must never
// even indirectly depend on mcp/gosdk (that would be a package cycle, since
// gosdk itself depends on mcp) — unlike mcpserve, below, which legitimately
// does depend on the SDK transitively, through gosdk, and is checked for
// direct imports only.
func TestNoGoSDKImport(t *testing.T) {
	assertNotInTransitiveDeps(t, "github.com/frankbardon/vellum/mcp",
		"github.com/modelcontextprotocol/go-sdk")
}

// TestNoGoSDKImport_NonVacuous proves the check above can actually fail: it
// runs the identical `go list -deps` query against mcp/gosdk, which does —
// and must — import the SDK, so a broken query (a typo'd package path, a
// filter that silently matches nothing) would show up here as a spurious
// pass rather than being indistinguishable from a real firewall holding.
func TestNoGoSDKImport_NonVacuous(t *testing.T) {
	deps := depsOf(t, "github.com/frankbardon/vellum/mcp/gosdk")
	if !contains(deps, "github.com/modelcontextprotocol/go-sdk/mcp") {
		t.Fatalf("mcp/gosdk does not depend on the SDK at all — the query used by "+
			"TestNoGoSDKImport would pass vacuously against it.\ndeps: %v", deps)
	}
}

// TestNoGoSDKImport_DirectOnlyForToolmetaAndServe checks mcp/toolmeta and
// mcpserve for a *direct* import of the SDK, not a transitive dependency —
// the distinction matters here in a way it does not for mcp/ above.
// mcp/toolmeta has, by its own documented contract, zero dependencies at
// all. mcpserve legitimately depends on the SDK transitively (it calls
// mcp/gosdk, which is the one package that must reach the SDK), so a
// transitive check on mcpserve would fail for the expected, correct reason
// and prove nothing; what CLAUDE.md's "Only mcp/gosdk imports the SDK"
// actually promises about mcpserve is that its own source never writes
// `import "github.com/modelcontextprotocol/go-sdk/..."` — it reaches every
// SDK type it needs through gosdk's own re-exports (see mcp/gosdk's doc
// comment) instead. Checked the same way internal/gates'
// TestNoEncodingXMLInFill checks its own direct-import boundary: `go list
// -f '{{join .Imports}}'`, not `-deps`.
func TestNoGoSDKImport_DirectOnlyForToolmetaAndServe(t *testing.T) {
	for _, pkg := range []string{
		"github.com/frankbardon/vellum/mcp/toolmeta",
		"github.com/frankbardon/vellum/mcpserve",
	} {
		imports := directImportsOf(t, pkg)
		for _, imp := range imports {
			if imp == "github.com/modelcontextprotocol/go-sdk/mcp" || strings.HasPrefix(imp, "github.com/modelcontextprotocol/go-sdk/") {
				t.Errorf("%s directly imports %s; it must reach SDK types through mcp/gosdk's own re-exports instead", pkg, imp)
			}
		}
	}
}

func assertNotInTransitiveDeps(t *testing.T, pkg, target string) {
	t.Helper()
	deps := depsOf(t, pkg)
	if len(deps) == 0 {
		t.Fatalf("go list -deps %s returned nothing; the package path is wrong", pkg)
	}
	for _, d := range deps {
		if d == target || strings.HasPrefix(d, target+"/") {
			t.Errorf("%s is reachable from %s.\n\nfonts come from the theme, the SDK comes "+
				"from mcp/gosdk: this package must stay usable without linking a transport.\n\n"+
				"Find the importer with:\n  go mod why %s", target, pkg, target)
		}
	}
}

func depsOf(t *testing.T, pkg string) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-deps", pkg).CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
	}
	return nonEmptyLines(out)
}

func directImportsOf(t *testing.T, pkg string) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-f", `{{join .Imports "\n"}}`, pkg).CombinedOutput()
	if err != nil {
		t.Fatalf("go list %s: %v\n%s", pkg, err, out)
	}
	return nonEmptyLines(out)
}

func nonEmptyLines(out []byte) []string {
	var lines []string
	for _, line := range strings.Split(string(out), "\n") {
		if p := strings.TrimSpace(line); p != "" {
			lines = append(lines, p)
		}
	}
	return lines
}

func contains(list []string, target string) bool {
	for _, l := range list {
		if l == target {
			return true
		}
	}
	return false
}
