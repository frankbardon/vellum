// Package doc emits WordprocessingML — the .docx format.
//
// # Status
//
// A walking skeleton. It renders headings and paragraphs into a package that
// opens cleanly in Word and LibreOffice, which is enough to prove the
// substrate carries a real artifact end to end. Styles, numbering, tables,
// images, headers and footers, footnotes and the table-of-contents field
// arrive with the DOCX epic.
//
// Breadth is deliberately not the goal here. What matters is that the package
// is structurally correct, that its bytes are identical on every run, and that
// a block kind this writer cannot yet render is a loud error naming the kind
// rather than a silently missing paragraph.
package doc
