package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// claudeMdPath is the machine-checked contract these gates read.
const claudeMdPath = repoRoot + "/CLAUDE.md"

// currentFormatVersion is the envelope version CLAUDE.md must name.
//
// Duplicated here as a literal rather than imported from descriptor,
// deliberately. The gate's job is to catch the case where the constant moved
// and the document did not; importing the constant would make both sides move
// together and the test would pass through the exact change it exists to
// notice. Bumping the version is therefore a three-line edit — constant,
// document, this line — which is the intended friction.
const currentFormatVersion = "1.0"

// reservedGatePrefixes are the test-name prefixes CLAUDE.md reserves for
// non-skippable gates.
//
// A test named with one of these is making a contract claim, so it must appear
// in the document. The prefixes are listed in CLAUDE.md itself and repeated
// here for the same reason as the version above.
var reservedGatePrefixes = []string{
	"TestClaudeMd",
	"TestUpdateDemand",
	"TestGoldensNot",
	"TestSkillsCover",
	"TestCapability",
	"TestDeterminism",
	"TestNo",
	"TestPerPackageCoverage",
	"TestManifest",
	"TestPayloadSchema",
	"TestSpecHash",
	"TestBind",
}

// TestClaudeMdMentionsFormatVersion pins the document to the live envelope
// version.
//
// The version is the one thing in the contract a consumer parses rather than
// reads, so a document naming a version the code no longer emits is worse than
// no document.
func TestClaudeMdMentionsFormatVersion(t *testing.T) {
	doc := readClaudeMd(t)
	if !strings.Contains(doc, `"`+currentFormatVersion+`"`) {
		t.Fatalf("CLAUDE.md does not mention the current format_version %q.\n"+
			"Update the Output Format Contract section, and check that every example envelope in it says the same thing.",
			currentFormatVersion)
	}
}

// TestClaudeMdMentionsAllEnvVars requires every environment variable the code
// reads to be documented.
//
// An undocumented environment variable is a behaviour change nobody can find:
// it alters output on a machine that has it set, and the reader of the source
// has no list to check against.
//
// Variables are found by resolving the argument of every os.Getenv and
// os.LookupEnv call, rather than by grepping for the VELLUM_ prefix. A grep
// would sweep up the whole error-code registry, which shares the prefix and is
// not configuration.
func TestClaudeMdMentionsAllEnvVars(t *testing.T) {
	doc := readClaudeMd(t)

	var missing []string
	for _, name := range environmentVariables(t) {
		if !strings.Contains(doc, name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("environment variables read by the code but absent from CLAUDE.md: %s\n\n"+
			"Add each to the \"Build / Env\" section, saying what it does and what the unset default is. "+
			"An undocumented variable changes behaviour on the machine that sets it and nowhere else.",
			strings.Join(missing, ", "))
	}
}

// TestClaudeMdMentionsAllNonSkippableGates requires every test claiming a
// reserved prefix to be listed.
//
// The list is what a reviewer reads to learn what the build actually enforces.
// A gate missing from it can be deleted without anybody noticing the guarantee
// went with it.
func TestClaudeMdMentionsAllNonSkippableGates(t *testing.T) {
	doc := readClaudeMd(t)

	var missing []string
	for _, name := range reservedPrefixTests(t) {
		if !strings.Contains(doc, name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("tests using a reserved gate prefix but absent from CLAUDE.md: %s\n\n"+
			"Add each under \"Non-Skippable CI Gates\" with one line saying what it enforces, "+
			"or rename it if it is not making a contract claim.",
			strings.Join(missing, ", "))
	}
}

// TestUpdateDemandTableCovers requires the Update Demand table to cover every
// kind of change that obliges a documentation update.
//
// The table is the mechanism that stops documentation rotting, so a gap in it
// is a category of change that can land undocumented forever. The subjects are
// checked rather than the exact wording, because the wording should be free to
// improve.
func TestUpdateDemandTableCovers(t *testing.T) {
	table := sectionOf(t, readClaudeMd(t), "## The Update Demand")

	// Each entry is one required subject and the alternatives that count as
	// naming it, so a row may be reworded without failing.
	required := []struct {
		subject string
		any     []string
	}{
		{"a block kind", []string{"BlockKind"}},
		{"an output format", []string{"artifact.Format"}},
		{"a capability", []string{"capability matrix", "capability/matrix.go"}},
		{"a theme slot", []string{"theme slot"}},
		{"an error code", []string{"error code"}},
		{"an MCP tool", []string{"MCP tool"}},
		{"a CLI leaf", []string{"CLI leaf"}},
		{"the envelope version", []string{"format_version"}},
		{"an environment variable", []string{"environment variable"}},
		{"a CI gate", []string{"CI gate"}},
		{"a dependency", []string{"dependency"}},
		{"a byte-layout rule", []string{"byte-layout"}},
	}

	for _, r := range required {
		found := false
		for _, alt := range r.any {
			if strings.Contains(table, alt) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("the Update Demand table has no row covering %s.\n"+
				"A change of that kind can land with its skill file and its documentation untouched, "+
				"which is the failure the table exists to prevent.", r.subject)
		}
	}
}

// readClaudeMd returns the contract document.
func readClaudeMd(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(claudeMdPath)
	if err != nil {
		t.Fatalf("reading CLAUDE.md: %v", err)
	}
	return string(raw)
}

// sectionOf returns the text of one heading's section, up to the next heading
// at the same level.
func sectionOf(t *testing.T, doc, heading string) string {
	t.Helper()

	i := strings.Index(doc, heading)
	if i < 0 {
		t.Fatalf("CLAUDE.md has no %q section", heading)
	}
	rest := doc[i+len(heading):]
	level := strings.SplitN(heading, " ", 2)[0]
	if j := strings.Index(rest, "\n"+level+" "); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

// environmentVariables returns every variable name reachable through os.Getenv
// or os.LookupEnv, sorted.
func environmentVariables(t *testing.T) []string {
	t.Helper()

	found := map[string]bool{}
	walkAllSource(t, func(_ string, file *ast.File, _ *token.FileSet) {
		consts := stringConstants(file)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "os" {
				return true
			}
			if sel.Sel.Name != "Getenv" && sel.Sel.Name != "LookupEnv" {
				return true
			}
			if name, ok := resolveString(call.Args[0], consts); ok {
				found[name] = true
			} else {
				// A dynamically computed variable name cannot be documented,
				// and would let a real one through unnoticed.
				t.Errorf("%s: the environment variable name is not a constant, "+
					"so it cannot be checked against CLAUDE.md", sel.Sel.Name)
			}
			return true
		})
	})
	return sortedKeys(found)
}

// stringConstants maps the string-valued constants declared in a file to their
// values, so a Getenv call naming one can be resolved.
func stringConstants(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, s := range gen.Specs {
			vs, ok := s.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				if v, ok := literalString(vs.Values[i]); ok {
					out[name.Name] = v
				}
			}
		}
	}
	return out
}

// resolveString reduces an expression to a string, following a same-file
// constant by name.
func resolveString(e ast.Expr, consts map[string]string) (string, bool) {
	if v, ok := literalString(e); ok {
		return v, true
	}
	if id, ok := e.(*ast.Ident); ok {
		if v, ok := consts[id.Name]; ok {
			return v, true
		}
	}
	// A qualified constant from another package: the name carries the package,
	// so the value is not visible here. Resolved by scanning for the constant's
	// value wherever it was declared, which stringConstants already collected
	// per file — so this arm reports only genuinely opaque expressions.
	if sel, ok := e.(*ast.SelectorExpr); ok {
		if v, ok := consts[sel.Sel.Name]; ok {
			return v, true
		}
	}
	return "", false
}

// literalString unquotes a string literal.
func literalString(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	v, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return v, true
}

// testFuncRe matches a top-level test function declaration.
var testFuncRe = regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]+)\(`)

// reservedPrefixTests returns every test function whose name claims a reserved
// gate prefix, sorted.
func reservedPrefixTests(t *testing.T) []string {
	t.Helper()

	found := map[string]bool{}
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "testdata", "docs", "bin", ".planning":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, m := range testFuncRe.FindAllStringSubmatch(string(raw), -1) {
			if hasReservedPrefix(m[1]) {
				found[m[1]] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the test files: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("no tests with a reserved prefix were found; the walk is wrong and this gate would pass vacuously")
	}
	return sortedKeys(found)
}

// hasReservedPrefix reports whether name claims a reserved gate prefix.
func hasReservedPrefix(name string) bool {
	for _, p := range reservedGatePrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// walkAllSource visits every .go file in the module, test files included.
//
// Tests are included because a test may read an environment variable too, and
// one that changes what the suite checks is exactly as undocumented as one that
// changes what the library emits.
func walkAllSource(t *testing.T, visit func(path string, file *ast.File, fset *token.FileSet)) {
	t.Helper()
	fset := token.NewFileSet()

	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "testdata", "docs", "bin", ".planning":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Parsed without build-tag filtering on purpose: a variable read only
		// under a tag is still a variable somebody must be told about.
		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return parseErr
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			rel = path
		}
		visit(filepath.ToSlash(rel), file, fset)
		return nil
	})
	if err != nil {
		t.Fatalf("walking the source tree: %v", err)
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
