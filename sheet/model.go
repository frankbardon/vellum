package sheet

import (
	"github.com/frankbardon/vellum/provenance"
)

// Workbook is a SpreadsheetML workbook.
type Workbook struct {
	// Title is carried into the package's core properties.
	Title string

	// Sheets are the workbook's tabs, in tab order.
	Sheets []Sheet

	// Styles is the styles part. A cell whose StyleID names an entry this does
	// not carry is a document Excel repairs, so this is assembled by the
	// lowering rather than left to the caller. See [StyleSheet] for the fixed
	// preamble it has to keep intact.
	Styles StyleSheet

	// Provenance, when set, is embedded in the package's custom document
	// properties. It is part of the bytes and therefore part of the
	// determinism guarantee, which is why it carries no machine identity.
	Provenance *provenance.Record
}

// Sheet is one worksheet tab.
type Sheet struct {
	// Name is the tab name. Excel bounds it at 31 characters and forbids
	// `: \ / ? * [ ]`; [sanitizeSheetName] enforces both when the lowering
	// derives one, and a hand-built [Workbook] naming one outside those rules
	// is [VELLUM_SHEET_INVALID].
	Name string

	// Rows are the sheet's populated rows, in ascending [Row.Index] order. A
	// sheet is sparse: a row with no cells is simply absent rather than
	// present and empty, which is what keeps a wide gap from writing megabytes
	// of nothing.
	Rows []Row

	// Merges are the merged-cell ranges this sheet declares.
	Merges []Merge

	// Comments are the cell comments this sheet carries. Writing even one
	// produces two further parts — a comments part and a legacy VML drawing —
	// which is why they are collected here rather than emitted inline with the
	// cell that carries one: the writer needs the whole set before either part
	// can be built.
	Comments []Comment

	// Freeze is the sheet's frozen pane, or nil for none. A sheet may declare
	// at most one: SpreadsheetML has one view per sheet, so a sheet carrying a
	// second table freezes at the first and leaves the rest to the reader's
	// own scrolling. A caller wanting independent freezing for two tables puts
	// a page_break between them, which is what starts a new sheet.
	Freeze *FreezePane

	// AutoFilter is the sheet's filter range, or nil for none. Also
	// one-per-sheet, for the same reason as Freeze.
	AutoFilter *AutoFilter

	// Columns are column-width overrides, keyed by 1-based column number. A
	// column absent here takes Excel's own default width.
	Columns []ColumnWidth
}

// Row is one row's populated cells.
type Row struct {
	// Index is the 1-based row number.
	Index int

	// Cells are the row's populated cells, in ascending [Cell.Column] order.
	Cells []Cell
}

// Cell is one populated grid position.
type Cell struct {
	// Column is the 1-based column number.
	Column int

	// Value is the cell's typed content.
	Value CellValue

	// StyleID indexes [StyleSheet.Formats]. Zero is the default format index
	// 0 always carries — see [StyleSheet] — so a cell that names nothing gets
	// the workbook's ordinary appearance.
	StyleID int
}

// CellKind names which arm of a [CellValue] carries content.
type CellKind uint8

const (
	// CellEmpty carries no value. A cell exists at this position — it may
	// still be merged into, or carry a comment — without carrying content.
	CellEmpty CellKind = iota

	// CellText is a string, written through the shared string table.
	CellText

	// CellNumber is a live numeric value.
	CellNumber

	// CellDate is a live numeric value — the 1900-system serial from
	// [numfmt.Serial] — that a date-formatted [Cell.StyleID] renders as a
	// date. Distinguished from CellNumber only so a writer building the shared
	// string table and a consumer inspecting the model can tell a date from an
	// ordinary number without also inspecting the style.
	CellDate

	// CellBool is a live boolean value.
	CellBool
)

// AllCellKinds returns the cell value kinds, in declaration order.
func AllCellKinds() []CellKind {
	return []CellKind{CellEmpty, CellText, CellNumber, CellDate, CellBool}
}

// CellValue is one cell's typed content.
//
// Exactly the arm [Kind] names carries a meaningful value; the others are
// zero. A closed struct rather than an `any`, for the reason [numfmt.Value]
// is one: an interface would arrive from resolution as whatever shape the
// caller happened to build, and Excel's own type system for a cell is exactly
// these four kinds and no others.
type CellValue struct {
	Kind   CellKind
	Text   string
	Number float64
	Bool   bool
}

// Text returns a text value.
func Text(s string) CellValue { return CellValue{Kind: CellText, Text: s} }

// Number returns a numeric value.
func Number(n float64) CellValue { return CellValue{Kind: CellNumber, Number: n} }

// Date returns a date value: a 1900-system serial rendered through a
// date-formatted style.
func Date(serial float64) CellValue { return CellValue{Kind: CellDate, Number: serial} }

// Bool returns a boolean value.
func Bool(b bool) CellValue { return CellValue{Kind: CellBool, Bool: b} }

// Merge is one merged-cell range, inclusive of both corners.
type Merge struct {
	FromRow, FromCol int
	ToRow, ToCol     int
}

// Comment is one cell comment.
//
// SpreadsheetML calls this element a "comment" in the modern schema and a
// "note" in Excel's own UI since the 2018 introduction of threaded comments;
// this is the legacy, non-threaded form, which is the one every reader back to
// Excel 2007 draws correctly. Threaded comments need an author-identity part
// this library has no source for, and degrading a note to something half of
// the installed base cannot see is not a degradation worth making.
type Comment struct {
	Row, Col int

	// Author is attributed text Excel shows above the comment body. Empty
	// omits the byline, which is what an author-less note looks like.
	Author string

	Text string
}

// FreezePane is a sheet's frozen rows and columns.
type FreezePane struct {
	// Rows is how many leading rows stay fixed while the sheet scrolls, and
	// Cols how many leading columns.
	Rows, Cols int
}

// AutoFilter is a sheet's filter range: the header row the dropdown arrows sit
// on, through the last row of data beneath it.
type AutoFilter struct {
	FromRow, FromCol int
	ToRow, ToCol     int
}

// ColumnWidth overrides one column's width, in characters — the unit
// SpreadsheetML itself uses, roughly the count of "0" glyphs in the workbook's
// default font that fit across the column.
type ColumnWidth struct {
	Column int
	Width  float64
}
