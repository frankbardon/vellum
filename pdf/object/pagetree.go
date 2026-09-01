package object

import (
	"bytes"

	verr "github.com/frankbardon/vellum/errors"
)

// InheritableKeys are the page attributes a Pages node may hold on behalf of
// its descendants, per ISO 32000-1.
//
// A page that does not carry one takes its nearest ancestor's value, which is
// what lets a five hundred page document state its media box once.
var InheritableKeys = []Name{"Resources", "MediaBox", "CropBox", "Rotate"}

// HoistableKeys are the attributes [BuildPageTree] will actually lift onto the
// root.
//
// It is [InheritableKeys] without Resources, and the omission is not an
// oversight. ISO 19005-2 clause 6.2.2 requires a content stream referencing a
// font or an image to have an *explicitly associated* resource dictionary — an
// inherited one does not satisfy it, and veraPDF reports:
//
//	A content stream refers to resource(s) F1 not defined in an explicitly
//	associated Resources dictionary
//
// So the two specifications disagree about whether this is a good idea, and
// PDF/A wins because that is the conformance Vellum claims. Every page carries
// its own Resources; the cost is one small dictionary per page, which is
// nothing beside the content it names.
//
// This was found by the validator rather than by reading, which is the whole
// argument for having the validator in the loop: hoisting the resources was an
// obviously correct optimisation right up until something checked it.
var HoistableKeys = []Name{"MediaBox", "CropBox", "Rotate"}

// DefaultBranching is how many children a page tree node holds.
//
// A flat list of kids is legal at any length and is what a naive writer emits.
// It also makes a reader scan linearly to reach page four hundred, which is the
// behaviour the specification's recommendation of a balanced tree exists to
// avoid. Eight is within the range producers conventionally use; the exact
// value matters less than its being fixed, because the tree's shape reaches the
// output bytes and a shape that varied with anything would be nondeterminism.
const DefaultBranching = 8

// PageTreeOptions configures the tree. The zero value is the default.
type PageTreeOptions struct {
	// Branching is the maximum number of children per node. Zero selects
	// [DefaultBranching]. A value below two is rejected, because a node with
	// one child never reduces and the fold would not terminate.
	Branching int

	// NoHoist keeps inheritable attributes on the pages that declared them
	// rather than lifting the ones they all share onto the root.
	//
	// Named so the zero value hoists, which is what almost every document
	// wants. It exists for a caller who needs each page dictionary to be
	// self-describing — a consumer extracting single pages, most plausibly.
	NoHoist bool
}

// BuildPageTree writes a balanced page tree over pages and returns its root.
//
// References are reserved and filled rather than added directly, because the
// tree is cyclic: a page names its parent and the parent names its kids, so one
// of the two must exist before the object it points at.
//
// The emission walk is fixed: every page is numbered first, in the order given,
// then the interior nodes level by level from the leaves up. Object numbers
// appear inside every reference in the file, so a walk that varied would change
// the bytes without changing the document.
func BuildPageTree(doc *Document, pages []Dict, opts PageTreeOptions) (Ref, error) {
	if len(pages) == 0 {
		return Ref{}, verr.NewCodedError(verr.VELLUM_PDF_OBJECT_UNRESOLVED,
			"a page tree needs at least one page")
	}
	branching := opts.Branching
	if branching == 0 {
		branching = DefaultBranching
	}
	if branching < 2 {
		return Ref{}, verr.NewCodedErrorWithDetails(verr.VELLUM_PDF_OBJECT_UNRESOLVED,
			"a page tree node must be able to hold at least two children",
			map[string]any{"branching": branching})
	}

	// Cloned rather than copied. A Dict shares its entries when assigned, and
	// hoisting removes keys from the pages it was handed — without this, asking
	// for a page tree would silently strip the media box from the caller's own
	// dictionaries.
	leaves := make([]Dict, len(pages))
	for i, p := range pages {
		leaves[i] = p.Clone()
	}

	var inherited Dict
	if !opts.NoHoist {
		inherited = hoist(leaves)
	}

	b := &pageTree{doc: doc, parent: map[int]Ref{}}

	// Pages first, so page one is the lowest-numbered page object in the file
	// and a reader triaging a hex dump finds them where they expect.
	pageRefs := make([]Ref, len(leaves))
	for i := range leaves {
		pageRefs[i] = doc.Reserve()
	}

	root := b.fold(pageRefs, branching)

	// The root carries what every page had in common, and is the only node that
	// does. An interior node holding a value its siblings did not share would
	// be a value some pages inherit and others do not, which is a distinction
	// nothing else in the document expresses.
	for _, key := range HoistableKeys {
		if v, ok := inherited.Get(key); ok {
			b.nodeDict(root).Set(key, v)
		}
	}

	for i, ref := range pageRefs {
		page := leaves[i]
		page.Set("Type", Name("Page"))
		page.Set("Parent", b.parent[ref.Number])
		if err := doc.Fill(ref, page); err != nil {
			return Ref{}, err
		}
	}
	for _, n := range b.nodes {
		d := n.dict
		if p, ok := b.parent[n.ref.Number]; ok {
			d.Set("Parent", p)
		}
		if err := doc.Fill(n.ref, d); err != nil {
			return Ref{}, err
		}
	}
	return root, nil
}

// pageTree accumulates the interior nodes while the tree is folded.
//
// Parents are recorded rather than written as they are discovered, because a
// child's dictionary is not in the document yet when its parent is created.
// Filling everything at the end is what lets the walk stay one pass.
type pageTree struct {
	doc    *Document
	nodes  []treeNode
	parent map[int]Ref
}

type treeNode struct {
	ref  Ref
	dict Dict
}

// nodeDict returns the interior node's dictionary for modification.
func (b *pageTree) nodeDict(ref Ref) *Dict {
	for i := range b.nodes {
		if b.nodes[i].ref == ref {
			return &b.nodes[i].dict
		}
	}
	// Unreachable: every ref handed here came from add.
	panic("object: no such page tree node")
}

// add records an interior node over the given children.
func (b *pageTree) add(children []Ref, count int) Ref {
	ref := b.doc.Reserve()

	kids := make(Array, len(children))
	for i, c := range children {
		kids[i] = c
		b.parent[c.Number] = ref
	}

	b.nodes = append(b.nodes, treeNode{
		ref: ref,
		dict: NewDict(
			"Type", Name("Pages"),
			"Kids", kids,
			"Count", Int(count),
		),
	})
	return ref
}

// fold reduces a level of references to a single root.
//
// Bottom-up rather than top-down, so the leaves stay in the order they were
// given and the shape is a pure function of the page count and the branching
// factor.
func (b *pageTree) fold(refs []Ref, branching int) Ref {
	counts := make([]int, len(refs))
	for i := range counts {
		counts[i] = 1
	}
	// Whether each reference is already an interior node. The first level holds
	// pages, so none of them is.
	interior := make([]bool, len(refs))

	// The condition is "more than one node, or none made yet". The second half
	// covers a single-page document, where the fold has nothing to reduce and a
	// Pages node is still required: the catalogue's /Pages must name a node,
	// not a page.
	for made := false; len(refs) > 1 || !made; made = true {
		next := make([]Ref, 0, (len(refs)+branching-1)/branching)
		nextCounts := make([]int, 0, cap(next))
		nextInterior := make([]bool, 0, cap(next))

		for i := 0; i < len(refs); i += branching {
			end := min(i+branching, len(refs))

			// A remainder group of one interior node is carried up rather than
			// wrapped. Wrapping it produces a Pages node whose only child is a
			// Pages node — legal, and an indirection every reader follows to
			// reach the same pages. It arises whenever the page count leaves a
			// remainder of one at a level above the leaves, which is why the
			// test covers 65 pages and not only the round numbers.
			if end-i == 1 && interior[i] {
				next = append(next, refs[i])
				nextCounts = append(nextCounts, counts[i])
				nextInterior = append(nextInterior, true)
				continue
			}

			total := 0
			for _, c := range counts[i:end] {
				total += c
			}
			next = append(next, b.add(refs[i:end], total))
			nextCounts = append(nextCounts, total)
			nextInterior = append(nextInterior, true)
		}
		refs, counts, interior = next, nextCounts, nextInterior
	}
	return refs[0]
}

// hoist removes the inheritable attributes every page shares and returns them.
//
// Only an attribute present on every page with the same value moves. A key most
// pages agree on is left alone: lifting it would mean writing an override onto
// the pages that disagree, and an inherited value plus an override is two
// places carrying the same fact.
func hoist(pages []Dict) Dict {
	var out Dict
	for _, key := range HoistableKeys {
		first, ok := pages[0].Get(key)
		if !ok {
			continue
		}
		want := first.AppendPDF(nil)

		shared := true
		for _, p := range pages[1:] {
			v, ok := p.Get(key)
			if !ok || !bytes.Equal(v.AppendPDF(nil), want) {
				shared = false
				break
			}
		}
		if !shared {
			continue
		}

		out.Set(key, first)
		for i := range pages {
			pages[i].Delete(key)
		}
	}
	return out
}
