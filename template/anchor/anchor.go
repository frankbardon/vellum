// Package anchor discovers the places a fill-mode binding will splice data
// into: DOCX's native content controls and {{marker}} text, and — as of
// E11-S1 — xlsx's defined names and Excel Table (ListObject) columns.
//
// Discovery is read-only. It locates anchors and reports their location —
// enough for a later story (defrag, splice) to find its way back to the
// exact bytes — but it never edits a part. Every structural read goes
// through xmlcopy.Walk, never encoding/xml directly: see the xmlcopy package
// doc for why re-parsing a source part any other way is the one thing this
// subtree does not do.
//
// # xlsx anchor naming
//
// A defined-name anchor's Name is the defined name's own name attribute,
// unchanged. A table-column anchor's Name is "<table displayName>.<column
// name>" — for example "CustomerTable.Name" for the "Name" column of a table
// whose own displayName is "CustomerTable" — chosen because it is what a
// binding author would write unprompted: the table names the group, the
// column names the field, and a "." reads the same way a dotted path does
// everywhere else in this codebase's own FEEL expressions.
package anchor

import "github.com/frankbardon/vellum/xmlcopy"

// Kind distinguishes the anchor mechanisms a template format offers: DOCX's
// content control and marker text, and — as of E11-S1 — xlsx's defined name
// and Excel Table column. A pptx shape name is E11-S2's job and is
// deliberately not anticipated here.
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

	// KindDefinedName is an xlsx workbook-scoped defined name resolving to a
	// single, absolute cell reference on one sheet — Name is the defined
	// name's own name attribute, and a bind statement fills exactly one
	// scalar into the cell it names. See [Anchor.Span] for what a defined
	// name anchor's span covers.
	KindDefinedName Kind = "defined_name"

	// KindTableColumn is one column of an xlsx Excel Table (ListObject),
	// named "<table displayName>.<column name>" — see [Anchor.Name] on this
	// package's own doc for the exact convention. A KindTableColumn anchor is
	// only ever filled from inside a [bind.Repeat] whose Target is
	// "table_row": it names one cell position within the table's one sample
	// data row, not a standalone splice site the way a native or marker
	// anchor is. See [Anchor.Table] for the extra location information this
	// kind carries beyond Span.
	KindTableColumn Kind = "table_column"
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
	//
	// For a defined-name anchor this is the whole <c>...</c> (or self-closing
	// <c/>) worksheet cell element the defined name's formula resolves to —
	// splicing it replaces that whole span with a freshly rendered cell
	// carrying the same r and s attributes.
	//
	// For a table-column anchor this is the whole <row>...</row> element of
	// the table's own one sample data row — deliberately the same span for
	// every KindTableColumn anchor belonging to that table, mirroring how
	// several DOCX markers sharing one paragraph already share a Span. Span
	// alone does not say which cell within that row is this column's own;
	// [Anchor.Table.Column] does.
	Span xmlcopy.Span `json:"span"`

	// Table carries the extra location information a KindTableColumn anchor
	// needs beyond Span. Nil for every other Kind.
	Table *TableColumn `json:"table,omitempty"`
}

// TableColumn is a KindTableColumn anchor's own location information: which
// cell within its shared row Span is this column's, and how to keep the
// table's own ref attribute consistent after a repeat changes the table's row
// count.
type TableColumn struct {
	// DisplayName is the Excel Table's own displayName — the table half of
	// Anchor.Name's "DisplayName.ColumnName" convention.
	DisplayName string `json:"display_name"`

	// Column is the absolute, 1-based worksheet column this table column
	// occupies (1 = A, 2 = B, ...), letting a splice compute the exact cell
	// reference for any row without re-deriving it from the table's own
	// column order every time.
	Column int `json:"column"`

	// TablePart is the OPC part name of the table definition
	// (e.g. "/xl/tables/table1.xml") that declares this table's own ref — the
	// part a table_row repeat updates alongside the worksheet row it inserts,
	// because the two must stay consistent or Excel refuses to open the file.
	TablePart string `json:"table_part"`

	// HeaderRow is the table's own header row number (the first row of its
	// ref) — the fixed point a new ref is computed from after a repeat
	// changes the data row count.
	HeaderRow int `json:"header_row"`

	// FromColumn and ToColumn are the table's own leftmost and rightmost
	// absolute worksheet columns, carried so the table's ref attribute can be
	// rebuilt without re-parsing it a second time.
	FromColumn int `json:"from_column"`
	ToColumn   int `json:"to_column"`
}

// Inventory is every anchor discovered in a template.
type Inventory struct {
	// Anchors is in document order: for a single part, ascending by
	// Span.Start. Two markers sharing a paragraph carry the identical Span
	// and so sort adjacently in the order they were found scanning that
	// paragraph's text left to right, which [sort.SliceStable] preserves.
	Anchors []Anchor `json:"anchors"`
}
