package fragment

import (
	"github.com/frankbardon/vellum/numfmt"
	"github.com/frankbardon/vellum/spec"
)

// Table is a resolved analytical table.
//
// The structure is the specification's, because the structure was already
// right: hierarchical headers on both axes, cell-level annotations attached to
// values rather than replacing them, and distinguishable margin rows. What
// changed is that every cell now carries both its typed value and the text that
// value renders to, so a writer chooses by what its format can carry rather
// than by re-deriving the string.
type Table struct {
	// ColumnHeaders and RowHeaders are the banners, each a forest of spanning
	// nodes.
	ColumnHeaders HeaderTree
	RowHeaders    HeaderTree

	// Body is the grid, row-major.
	Body [][]Cell

	// Caption is the table's caption, or nil.
	Caption *Paragraph

	// Width is the number of grid columns the column headers tile.
	Width int
}

// HeaderTree is a forest of header nodes.
type HeaderTree []HeaderNode

// HeaderNode is one banner cell.
type HeaderNode struct {
	// Label is the header text.
	Label string

	// Span is how many leaf positions this node covers.
	Span int

	// Children are the nodes beneath it.
	Children HeaderTree

	// Style is the resolved appearance.
	Style TextStyle
}

// Cell is one resolved grid position.
type Cell struct {
	// Text is what the cell reads as: the value put through its format code, or
	// the consumer's own text when they supplied one.
	//
	// A flowing target writes this. A spreadsheet target writes Value and
	// FormatCode instead, so the workbook stays live — which is why both are
	// carried rather than one being derived at the point of use.
	Text string

	// Value is the typed value, or nil for a cell with none.
	Value *Value

	// FormatCode is the number-format code, retained verbatim. Empty means the
	// general format.
	FormatCode string

	// Annotations attach to the value rather than replacing it.
	Annotations []Annotation

	// RowSpan and ColSpan are counts; one is the natural value.
	RowSpan, ColSpan int

	// Class distinguishes the cell's role from its content — a margin row is a
	// margin row whatever numbers it holds.
	Class spec.CellClass

	// Style is the resolved appearance.
	Style TextStyle
}

// Value is a resolved typed value.
//
// It mirrors [numfmt.Value] rather than [spec.Value] because by this point the
// date has been parsed from its RFC 3339 form into a time, and a writer that
// had to parse it again would be a second place for that parse to disagree.
type Value = numfmt.Value

// Annotation is a resolved marker attached to a cell's value.
type Annotation struct {
	// Text is the annotation's content — a significance letter, a footnote
	// marker.
	Text string

	// Position says where it sits relative to the value.
	Position spec.AnnotationPosition

	// Style is the resolved appearance.
	Style TextStyle
}
