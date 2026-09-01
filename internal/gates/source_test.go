package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot is where the walk starts. The gates package sits one level down from
// internal, so the module root is two above it.
const repoRoot = "../.."

// TestNoTimeNow fails when non-test source outside the sanctioned opt-in calls
// time.Now.
//
// Every date Vellum writes comes from a SourceDateEpoch. A single time.Now on a
// write path does not fail any behavioural test: it produces a document that is
// correct, that opens, and that differs from the one produced a second earlier.
// The determinism harness would catch it eventually — but only for a golden that
// happens to exercise that path, and only once somebody wondered why a digest
// moved.
//
// provenance is the one opt-in, because recording when something was generated
// is the one place a real clock is the point. Setting a real epoch there is a
// deliberate departure from byte-identical output and is recorded as such.
func TestNoTimeNow(t *testing.T) {
	var offences []string

	walkSource(t, func(path string, file *ast.File, fset *token.FileSet) {
		if strings.HasPrefix(path, "provenance/") {
			return
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "time" {
				return true
			}
			if sel.Sel.Name == "Now" {
				offences = append(offences,
					path+":"+positionOf(fset, call)+" calls time.Now")
			}
			return true
		})
	})

	if len(offences) > 0 {
		t.Fatalf("time.Now on a non-provenance path:\n  %s\n\n"+
			"Timestamps come from SourceDateEpoch. A clock read here produces a document "+
			"that is correct, opens, and differs from the one produced a second earlier.",
			strings.Join(offences, "\n  "))
	}
}

// outputPathPackages are the packages whose emission order is part of the output
// bytes.
//
// The list is deliberately explicit rather than "everything". A map ranged in a
// validator that reports the first fault it finds is a real defect too, but it
// is a different one — it makes an error message unstable, not a document — and
// conflating them would make this gate something people suppress rather than
// obey.
var outputPathPackages = []string{
	"opc", "opc/zipdet", "doc", "sheet", "deck", "pdf",
	"pdf/object", "pdf/content", "pdf/font", "pdf/font/sfnt", "pdf/image",
	"pdf/text", "pdf/shape", "pdf/color", "pdf/xmp", "pdf/pdfa",
	"fragment", "resolve", "theme", "descriptor", "provenance",
}

// TestNoUnsortedMapIteration fails when an output-path package ranges a map
// without an evident ordering step.
//
// Ranging a Go map yields its entries in a different order on every run, by
// design. On an output path that is a document whose parts are in a different
// order each time it is built — which no behavioural test notices, because every
// ordering is valid, and which the determinism harness reports as a digest that
// moves for no reason anybody can see.
//
// The check is deliberately shallow and deliberately loud: any `range` over a
// value that is syntactically a map fails, and a legitimate case is fixed by
// collecting keys into a slice and sorting it — never by suppressing the gate.
// Where an API could accept a map on an ordered path it takes an ordered slice
// instead, so most of these never arise.
func TestNoUnsortedMapIteration(t *testing.T) {
	var offences []string
	visited := make(map[string]bool)

	walkSource(t, func(path string, file *ast.File, fset *token.FileSet) {
		if !onOutputPath(path) {
			return
		}
		visited[filepath.ToSlash(filepath.Dir(path))] = true
		// Names are collected per function rather than per file, plus the
		// file's package-level declarations. Collecting per file made any local
		// share a verdict with every same-named local elsewhere in the file —
		// which produced two false positives in a row before this was
		// tightened, and a gate that cries wolf is a gate people learn to
		// suppress.
		fileScope := mapValuedNames(file, false)

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			scope := make(map[string]bool, len(fileScope))
			for k := range fileScope {
				scope[k] = true
			}
			for k := range mapValuedNamesIn(fn.Body) {
				scope[k] = true
			}
			// A parameter that is not a map shadows a package-level map of the
			// same name, so it is removed from the scope rather than trusted.
			for _, name := range nonMapParams(fn) {
				delete(scope, name)
			}

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				rng, ok := n.(*ast.RangeStmt)
				if !ok {
					return true
				}
				ident, ok := rng.X.(*ast.Ident)
				if !ok || !scope[ident.Name] {
					return true
				}
				// Ranging for keys alone, to sort them, is the sanctioned shape
				// and is what the accumulate-then-sort idiom looks like.
				// Ranging for values is the one that puts map order into
				// output. Copying one map into another is likewise safe: the
				// destination is a map, so the order it was filled in cannot be
				// observed.
				if rng.Value == nil && collectsIntoSlice(rng) {
					return true
				}
				if copiesIntoMap(rng) {
					return true
				}
				offences = append(offences,
					path+":"+positionOf(fset, rng)+" ranges the map "+ident.Name)
				return true
			})
		}
	})

	if len(offences) > 0 {
		t.Fatalf("map iteration on an output path:\n  %s\n\n"+
			"Collect the keys into a slice, sort them bytewise, and range that. "+
			"A map ranged here puts a different order into the bytes on every run.",
			strings.Join(offences, "\n  "))
	}

	// A package that exists and was never reached is a package this gate
	// silently stopped covering — a renamed directory, or a name that never
	// matched in the first place. It would keep passing, which is the one way a
	// gate fails that nothing else reports.
	//
	// A name with no directory is not a fault: several of these are packages a
	// later epic builds, and naming them now is how the rule arrives before the
	// code rather than after it.
	for _, p := range outputPathPackages {
		if visited[p] {
			continue
		}
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(p))); err != nil {
			continue
		}
		t.Errorf("outputPathPackages names %q, which exists and which the source walk never reached. "+
			"The gate is not checking it, and would pass whatever it contained.", p)
	}
}

func onOutputPath(path string) bool {
	dir := filepath.ToSlash(filepath.Dir(path))
	for _, p := range outputPathPackages {
		if dir == p {
			return true
		}
	}
	return false
}

// mapValuedNames collects identifiers whose declared type is syntactically a
// map.
//
// Syntactic rather than type-checked, on purpose. A full type check would need
// the package to load, which makes the gate slow and makes it fail for reasons
// that are not the gate's business. The benefit is that the common shape — a
// local or a package-level map — is caught in milliseconds.
//
// The cost, accepted: a map reached through a call or through a selector is not
// seen, so this gate is a floor rather than a proof.
//
// Struct fields are deliberately not collected. They were, and every field name
// then made every same-named local look like a map — which is a false positive
// on a variable the gate never had grounds to suspect, since a field is reached
// through a selector and a selector is not an Ident.
func mapValuedNames(file *ast.File, includeLocals bool) map[string]bool {
	out := make(map[string]bool)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			if vs, ok := spec.(*ast.ValueSpec); ok {
				recordMapNames(out, vs.Names, vs.Type, vs.Values)
			}
		}
	}
	if includeLocals {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				for k := range mapValuedNamesIn(fn.Body) {
					out[k] = true
				}
			}
		}
	}
	return out
}

// mapValuedNamesIn collects map-typed locals declared within one node.
func mapValuedNamesIn(node ast.Node) map[string]bool {
	out := make(map[string]bool)
	ast.Inspect(node, func(n ast.Node) bool {
		switch d := n.(type) {
		case *ast.ValueSpec:
			recordMapNames(out, d.Names, d.Type, d.Values)
		case *ast.AssignStmt:
			if d.Tok != token.DEFINE {
				return true
			}
			names := make([]*ast.Ident, 0, len(d.Lhs))
			for _, l := range d.Lhs {
				if id, ok := l.(*ast.Ident); ok {
					names = append(names, id)
				}
			}
			recordMapNames(out, names, nil, d.Rhs)
		}
		return true
	})
	return out
}

// nonMapParams lists a function's parameters whose type is not a map, so a
// parameter shadowing a package-level map is not mistaken for it.
func nonMapParams(fn *ast.FuncDecl) []string {
	var out []string
	if fn.Type == nil || fn.Type.Params == nil {
		return nil
	}
	for _, field := range fn.Type.Params.List {
		if _, isMap := field.Type.(*ast.MapType); isMap {
			continue
		}
		for _, name := range field.Names {
			out = append(out, name.Name)
		}
	}
	return out
}

func recordMapNames(out map[string]bool, names []*ast.Ident, typ ast.Expr, values []ast.Expr) {
	isMap := false
	if _, ok := typ.(*ast.MapType); ok {
		isMap = true
	}
	for _, v := range values {
		switch e := v.(type) {
		case *ast.CompositeLit:
			if _, ok := e.Type.(*ast.MapType); ok {
				isMap = true
			}
		case *ast.CallExpr:
			// make(map[k]v, n)
			if id, ok := e.Fun.(*ast.Ident); ok && id.Name == "make" && len(e.Args) > 0 {
				if _, ok := e.Args[0].(*ast.MapType); ok {
					isMap = true
				}
			}
		}
	}
	if !isMap {
		return
	}
	for _, n := range names {
		out[n.Name] = true
	}
}

// copiesIntoMap reports whether a range body does nothing but assign into an
// index expression — the map-to-map copy idiom.
//
// Safe because the destination is a map: the order it was filled in is not
// observable, so the source's iteration order cannot reach the output.
func copiesIntoMap(rng *ast.RangeStmt) bool {
	if rng.Body == nil || len(rng.Body.List) == 0 {
		return false
	}
	for _, stmt := range rng.Body.List {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 {
			return false
		}
		if _, ok := assign.Lhs[0].(*ast.IndexExpr); !ok {
			return false
		}
	}
	return true
}

// collectsIntoSlice reports whether a range body does nothing but append.
//
// This is the accumulate-then-sort idiom: range the map for its keys, append
// them, sort the slice, and range that. The order the appends happen in does not
// reach the output because the sort erases it.
func collectsIntoSlice(rng *ast.RangeStmt) bool {
	if rng.Body == nil || len(rng.Body.List) == 0 {
		return false
	}
	for _, stmt := range rng.Body.List {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 {
			return false
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			return false
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok || id.Name != "append" {
			return false
		}
	}
	return true
}

// walkSource visits every non-test .go file in the module.
func walkSource(t *testing.T, visit func(path string, file *ast.File, fset *token.FileSet)) {
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
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
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

func positionOf(fset *token.FileSet, n ast.Node) string {
	pos := fset.Position(n.Pos())
	return itoa(pos.Line)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
