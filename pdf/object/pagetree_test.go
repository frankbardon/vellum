package object_test

import (
	"bytes"
	"strconv"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/object"
)

// samplePages returns n page dictionaries, each with a distinct content
// reference so they are distinguishable in the output.
func samplePages(n int) []object.Dict {
	out := make([]object.Dict, n)
	for i := range out {
		out[i] = object.NewDict(
			"MediaBox", object.Array{object.Int(0), object.Int(0), object.Int(612), object.Int(792)},
			"Resources", object.NewDict("ProcSet", object.Array{object.Name("PDF")}),
			"Contents", object.Ref{Number: 1000 + i},
		)
	}
	return out
}

func TestBuildPageTree_RejectsAnEmptyDocument(t *testing.T) {
	var doc object.Document
	_, err := object.BuildPageTree(&doc, nil, object.PageTreeOptions{})
	if !verr.HasCode(err, verr.VELLUM_PDF_OBJECT_UNRESOLVED) {
		t.Fatalf("got %v, want VELLUM_PDF_OBJECT_UNRESOLVED", err)
	}
}

func TestBuildPageTree_RejectsDegenerateBranching(t *testing.T) {
	var doc object.Document
	_, err := object.BuildPageTree(&doc, samplePages(4), object.PageTreeOptions{Branching: 1})
	if !verr.HasCode(err, verr.VELLUM_PDF_OBJECT_UNRESOLVED) {
		t.Fatalf("got %v, want VELLUM_PDF_OBJECT_UNRESOLVED", err)
	}
}

// TestBuildPageTree_CountsAreCorrectAtEveryDepth walks the emitted tree and
// checks each node against the leaves actually beneath it.
//
// /Count is the field a reader trusts to report the page total without
// descending, so a wrong one gives a document that opens and lies about its
// length. It is also exactly what an off-by-one in the fold corrupts, and only
// at the last, partly filled group of a level.
func TestBuildPageTree_CountsAreCorrectAtEveryDepth(t *testing.T) {
	for _, n := range []int{1, 2, 7, 8, 9, 20, 64, 65, 100} {
		t.Run(strconv.Itoa(n)+" pages", func(t *testing.T) {
			doc, root := buildTree(t, n, object.PageTreeOptions{})

			leaves, depth := walkTree(t, doc, root, 0)
			if leaves != n {
				t.Errorf("the tree holds %d pages, want %d", leaves, n)
			}
			if depth > 4 {
				t.Errorf("the tree is %d levels deep for %d pages, which is not balanced", depth, n)
			}
		})
	}
}

// TestBuildPageTree_HasNoSingleChildInteriorLevel pins that the fold does not
// leave a node holding exactly one node.
//
// A level like that is legal and means nothing: it adds an indirection every
// reader has to follow to reach the same set of pages. It appeared in the first
// version, which looped until one node remained and then made a root above it.
func TestBuildPageTree_HasNoSingleChildInteriorLevel(t *testing.T) {
	for _, n := range []int{1, 8, 9, 64, 65} {
		t.Run(strconv.Itoa(n)+" pages", func(t *testing.T) {
			doc, root := buildTree(t, n, object.PageTreeOptions{})

			var check func(ref object.Ref, isRoot bool)
			check = func(ref object.Ref, isRoot bool) {
				_, kids, ok := nodeAt(t, doc, ref)
				if !ok {
					return
				}
				if len(kids) == 1 {
					if _, _, interior := nodeAt(t, doc, kids[0]); interior {
						t.Errorf("object %d holds exactly one interior node, which is a level that means nothing", ref.Number)
					}
				}
				_ = isRoot
				for _, k := range kids {
					check(k, false)
				}
			}
			check(root, true)
		})
	}
}

// TestBuildPageTree_HoistsTheSharedMediaBox pins the size reduction.
func TestBuildPageTree_HoistsTheSharedMediaBox(t *testing.T) {
	doc, root := buildTree(t, 6, object.PageTreeOptions{})

	if _, ok := dictAt(t, doc, root)["MediaBox"]; !ok {
		t.Error("the shared media box was not lifted onto the root")
	}
	pages := pagesOf(t, doc)
	if len(pages) != 6 {
		t.Fatalf("found %d pages, want 6", len(pages))
	}
	for i, page := range pages {
		if _, ok := page["MediaBox"]; ok {
			t.Errorf("page %d still carries the media box the root now states", i)
		}
	}
}

// TestBuildPageTree_DoesNotHoistResources is the regression pin on ISO 19005-2
// clause 6.2.2.
//
// Resources is inheritable under ISO 32000-1 and lifting it is an obviously
// correct saving. PDF/A forbids relying on it: a content stream referencing a
// font must have an explicitly associated resource dictionary, and veraPDF
// reports "A content stream refers to resource(s) F1 not defined in an
// explicitly associated Resources dictionary".
//
// The first version of this builder hoisted it and every PDF golden failed
// validation. This test exists so re-adding it fails here, in a second, rather
// than in the conformance gate, which needs a JVM and a validator install.
func TestBuildPageTree_DoesNotHoistResources(t *testing.T) {
	doc, root := buildTree(t, 6, object.PageTreeOptions{})

	if _, ok := dictAt(t, doc, root)["Resources"]; ok {
		t.Error("Resources was lifted onto the root. PDF/A requires every content stream " +
			"to have an explicitly associated resource dictionary; an inherited one does not satisfy " +
			"ISO 19005-2 clause 6.2.2, and every PDF golden will fail validation.")
	}
	pages := pagesOf(t, doc)
	if len(pages) != 6 {
		t.Fatalf("found %d pages, want 6", len(pages))
	}
	for i, page := range pages {
		if _, ok := page["Resources"]; !ok {
			t.Errorf("page %d has no Resources of its own", i)
		}
	}
}

// TestBuildPageTree_NoHoistLeavesPagesSelfDescribing pins the escape hatch.
func TestBuildPageTree_NoHoistLeavesPagesSelfDescribing(t *testing.T) {
	doc, root := buildTree(t, 4, object.PageTreeOptions{NoHoist: true})

	if _, ok := dictAt(t, doc, root)["MediaBox"]; ok {
		t.Error("NoHoist still lifted the media box")
	}
	for i, page := range pagesOf(t, doc) {
		if _, ok := page["MediaBox"]; !ok {
			t.Errorf("page %d lost its media box", i)
		}
	}
}

// TestBuildPageTree_DoesNotMutateTheCallersPages is the aliasing pin.
//
// A Dict is a value holding a slice, so the pages handed in share their entries
// with the caller's. Hoisting deletes keys from them; without a clone, asking
// for a page tree would silently strip the media box from dictionaries the
// caller still holds.
func TestBuildPageTree_DoesNotMutateTheCallersPages(t *testing.T) {
	pages := samplePages(4)
	before := make([]string, len(pages))
	for i, p := range pages {
		before[i] = string(p.AppendPDF(nil))
	}

	var doc object.Document
	if _, err := object.BuildPageTree(&doc, pages, object.PageTreeOptions{}); err != nil {
		t.Fatalf("BuildPageTree: %v", err)
	}

	for i, p := range pages {
		if got := string(p.AppendPDF(nil)); got != before[i] {
			t.Errorf("page %d was modified:\n before %s\n after  %s", i, before[i], got)
		}
	}
}

// TestBuildPageTree_EveryChildNamesItsParent checks the back references.
func TestBuildPageTree_EveryChildNamesItsParent(t *testing.T) {
	doc, root := buildTree(t, 20, object.PageTreeOptions{})

	if _, ok := dictAt(t, doc, root)["Parent"]; ok {
		t.Error("the root names a parent")
	}

	var check func(parent object.Ref)
	check = func(parent object.Ref) {
		_, kids, ok := nodeAt(t, doc, parent)
		if !ok {
			return
		}
		for _, k := range kids {
			got, ok := dictAt(t, doc, k)["Parent"]
			if !ok {
				t.Fatalf("object %d names no parent", k.Number)
			}
			if got != (pref{Num: parent.Number}) {
				t.Errorf("object %d names parent %v, want object %d", k.Number, got, parent.Number)
			}
			check(k)
		}
	}
	check(root)
}

// TestBuildPageTree_IsDeterministic emits the same tree repeatedly.
func TestBuildPageTree_IsDeterministic(t *testing.T) {
	build := func() []byte {
		var doc object.Document
		root, err := object.BuildPageTree(&doc, samplePages(37), object.PageTreeOptions{})
		if err != nil {
			t.Fatalf("BuildPageTree: %v", err)
		}
		doc.Root = doc.Add(object.NewDict("Type", object.Name("Catalog"), "Pages", root))

		var buf bytes.Buffer
		if err := doc.Write(&buf); err != nil {
			t.Fatalf("Write: %v", err)
		}
		return buf.Bytes()
	}

	first := build()
	for range 25 {
		if !bytes.Equal(first, build()) {
			t.Fatal("two identical page trees produced different bytes")
		}
	}
}

// TestBuildPageTree_PagesAreNumberedFirst pins the emission walk.
//
// Object numbers appear inside every reference in the file, so the walk is part
// of the output. Pages before interior nodes, in the order given, means page
// one is the lowest-numbered page object — which is where somebody reading a
// hex dump looks for it.
func TestBuildPageTree_PagesAreNumberedFirst(t *testing.T) {
	const n = 20
	doc, _ := buildTree(t, n, object.PageTreeOptions{})

	all := readObjects(t, doc)
	for i := range n {
		d, ok := all[i+1].(pdict)
		if !ok {
			t.Fatalf("object %d is not a dictionary", i+1)
		}
		if d["Type"] != pname("Page") {
			t.Fatalf("object %d is not a page; the pages were not numbered first", i+1)
		}
		if got := d["Contents"]; got != (pref{Num: 1000 + i}) {
			t.Fatalf("object %d holds %v, want the reference for page %d; the pages are out of order",
				i+1, got, i+1)
		}
	}
}

// TestDocument_UncompressedStoresStreamsVerbatim pins the escape hatch for
// callers who need byte-identity across Go toolchain versions.
func TestDocument_UncompressedStoresStreamsVerbatim(t *testing.T) {
	raw := bytes.Repeat([]byte("BT /F1 12 Tf (hello) Tj ET\n"), 40)

	var doc object.Document
	doc.Uncompressed = true
	ref, err := doc.AddStream(object.Dict{}, raw)
	if err != nil {
		t.Fatalf("AddStream: %v", err)
	}
	doc.Root = doc.Add(object.NewDict("Type", object.Name("Catalog")))
	_ = ref

	var buf bytes.Buffer
	if err := doc.Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), raw) {
		t.Error("the stream is not stored verbatim")
	}
	if bytes.Contains(buf.Bytes(), []byte("/Filter")) {
		t.Error("an uncompressed stream declares a filter")
	}
}

// buildTree builds a page tree and returns the written document.
func buildTree(t *testing.T, pages int, opts object.PageTreeOptions) ([]byte, object.Ref) {
	t.Helper()

	var doc object.Document
	root, err := object.BuildPageTree(&doc, samplePages(pages), opts)
	if err != nil {
		t.Fatalf("BuildPageTree: %v", err)
	}
	doc.Root = doc.Add(object.NewDict("Type", object.Name("Catalog"), "Pages", root))

	var buf bytes.Buffer
	if err := doc.Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return buf.Bytes(), root
}

// dictAt returns the parsed dictionary for a reference.
func dictAt(t *testing.T, raw []byte, ref object.Ref) pdict {
	t.Helper()
	v, ok := readObjects(t, raw)[ref.Number]
	if !ok {
		t.Fatalf("the document has no object %d", ref.Number)
	}
	d, ok := v.(pdict)
	if !ok {
		t.Fatalf("object %d is not a dictionary: %v", ref.Number, v)
	}
	return d
}

// nodeAt returns a Pages node's kids. The final result reports whether the
// object is an interior node at all.
func nodeAt(t *testing.T, raw []byte, ref object.Ref) (pdict, []object.Ref, bool) {
	t.Helper()

	d := dictAt(t, raw, ref)
	if d["Type"] != pname("Pages") {
		return d, nil, false
	}
	kids, ok := d["Kids"].(parr)
	if !ok {
		t.Fatalf("object %d is a Pages node with no Kids array", ref.Number)
	}
	out := make([]object.Ref, 0, len(kids))
	for _, k := range kids {
		r, ok := k.(pref)
		if !ok {
			t.Fatalf("object %d has a kid that is not a reference: %v", ref.Number, k)
		}
		out = append(out, object.Ref{Number: r.Num})
	}
	return d, out, true
}

// walkTree returns the leaf count beneath a node and the depth of the subtree,
// checking each node's declared count against what is actually below it.
func walkTree(t *testing.T, raw []byte, ref object.Ref, depth int) (int, int) {
	t.Helper()
	if depth > 12 {
		t.Fatal("the tree is deeper than any balanced tree of this size; the fold does not terminate")
	}

	d, kids, isNode := nodeAt(t, raw, ref)
	if !isNode {
		return 1, depth
	}

	total, deepest := 0, depth
	for _, k := range kids {
		n, dd := walkTree(t, raw, k, depth+1)
		total += n
		deepest = max(deepest, dd)
	}

	// /Count is what a reader trusts to report the page total without
	// descending, so a wrong one gives a document that opens and lies about its
	// length.
	if got := d["Count"]; got != pnum(strconv.Itoa(total)) {
		t.Errorf("object %d declares Count %v, but %d pages are beneath it", ref.Number, got, total)
	}
	return total, deepest
}

// pagesOf returns every page dictionary in the document, in object order.
func pagesOf(t *testing.T, raw []byte) []pdict {
	t.Helper()

	all := readObjects(t, raw)
	var out []pdict
	for n := 1; n <= len(all); n++ {
		d, ok := all[n].(pdict)
		if ok && d["Type"] == pname("Page") {
			out = append(out, d)
		}
	}
	return out
}
