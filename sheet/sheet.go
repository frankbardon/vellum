// Package sheet is the SpreadsheetML model and its writer.
//
// The model is public, for the same reason [doc] and [deck]'s are: a consumer
// composing from the block model gets a workbook assembled by the lowering,
// and a consumer needing format-specific reach the block vocabulary does not
// express — a second table on a sheet driven straight by column and row
// numbers, a custom fill — builds a [Workbook] directly and writes it. Both
// paths converge on one writer, so there is no second serialiser able to
// drift.
//
// # Presentation tables, not a spreadsheet
//
// A workbook Vellum writes carries no formula, no pivot table, no macro and no
// external reference. It is the numbers behind a table, laid out so the reader
// who wants the live figures rather than the printed report can get them —
// which is the whole reason a workbook is in scope at all, and it is also the
// whole reason nothing further is. A consumer wanting a live model, with
// formulas and charts, builds it in a spreadsheet application from data Vellum
// gave it; Vellum is not that application. TestBoundary_NoLiveModel pins the
// boundary by asserting no golden this package writes carries any of the four.
//
// # What is different from doc and deck
//
// Neither this package nor a real spreadsheet application paginates.
// [FeatureOverflowContinue] degrades to "one continuous sheet" rather than to
// a split, because a sheet has no page: a table longer than any container
// simply continues down the rows it already occupies. What xlsx does have that
// nothing else in this library needs is a live cell — a value with a type and
// a number-format code applied to it, sortable and computable, rather than
// text a reader can only look at. [numfmt.Serial] exists for this package
// alone: every other writer renders a date through [numfmt.Format.Apply] and
// writes the resulting string, because a document or a deck has no live-cell
// concept for the underlying number to stay behind.
package sheet
