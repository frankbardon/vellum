// Package anchor discovers the places a fill-mode binding will splice data
// into: native content controls and {{marker}} text, today, for DOCX.
//
// Discovery is read-only. It locates anchors and reports their location —
// enough for a later story (defrag, splice) to find its way back to the
// exact bytes — but it never edits a part. Every structural read goes
// through xmlcopy.Walk, never encoding/xml directly: see the xmlcopy package
// doc for why re-parsing a source part any other way is the one thing this
// subtree does not do.
package anchor

import "github.com/frankbardon/vellum/xmlcopy"

// Kind distinguishes the two anchor mechanisms DOCX offers. Format-specific
// kinds — an xlsx ListObject range, a defined name, a pptx shape name — are
// E11's job and are deliberately not anticipated here.
type Kind string

const (
	// KindNative is a w:sdt content control, named by its w:tag.
	KindNative Kind = "native"

	// KindMarker is {{name}} text found inside a paragraph's runs, possibly
	// fragmented across several of them by Word's own editing (the
	// spell-checker splitting a word mid-marker is the common case). Run-level
	// fragmentation is resolved by defrag, a later story; this package only
	// establishes that a marker exists and which paragraph it lives in.
	KindMarker Kind = "marker"
)

// Anchor is one fillable location discovered in a template.
type Anchor struct {
	// Name is the anchor's binding key: the content control's w:tag value for
	// a native anchor, or the text between {{ and }} for a marker.
	Name string `json:"name"`

	// Kind is which mechanism this anchor uses.
	Kind Kind `json:"kind"`

	// Alias is the content control's human-readable w:alias, when the
	// template's author set one. Empty for a marker anchor, which carries no
	// separate label.
	Alias string `json:"alias,omitempty"`

	// Part is the OPC part name the anchor was found in, e.g.
	// "/word/document.xml".
	Part string `json:"part"`

	// Span locates the element a fill needs to target.
	//
	// For a native anchor this is the whole w:sdt element, open tag through
	// close tag — splicing a native anchor replaces (or edits within) that
	// whole span.
	//
	// For a marker anchor this is deliberately the whole enclosing w:p
	// paragraph, not a run or a text node. A {{marker}} can be fragmented
	// across several w:r runs by Word's own editing, and resolving that down
	// to the exact run and byte offset needs the flatten/match/position-map
	// algorithm defrag (E9-S3) implements — re-walking this same paragraph
	// span is exactly how that story is expected to pick the marker back up.
	// Recording anything finer-grained here would be a position this package
	// has no reliable way to compute yet.
	Span xmlcopy.Span `json:"span"`
}

// Inventory is every anchor discovered in a template.
type Inventory struct {
	// Anchors is in document order: for a single part, ascending by
	// Span.Start. Two markers sharing a paragraph carry the identical Span
	// and so sort adjacently in the order they were found scanning that
	// paragraph's text left to right, which [sort.SliceStable] preserves.
	Anchors []Anchor `json:"anchors"`
}
