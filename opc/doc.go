// Package opc implements Open Packaging Conventions containers — the ZIP-of-
// XML-parts structure that DOCX, XLSX and PPTX all share.
//
// This layer is shared verbatim across the three OOXML formats and by fill
// mode, which makes it the highest-leverage code in the project: a defect here
// is inherited by every writer above it.
//
// # Two guarantees
//
// **Deterministic write.** Parts are emitted in a canonical order derived from
// their names, relationship identifiers come from a sorted walk rather than
// from insertion order, and [Content_Types].xml is always the first entry.
// Nothing in the write path iterates a map.
//
// **Byte-preserving round trip.** [Open] followed by [Package.WriteTo], with no
// mutation in between, reproduces the input bytes exactly. Fill mode's entire
// non-destructiveness claim rests on this, so it is established and tested
// here rather than discovered later: parts are held as opaque bytes and are
// never re-serialised, and the relationship and content-type layers re-emit
// only when something has actually changed them.
//
// # What this layer does not do
//
// It moves bytes and knows content types. It does not parse WordprocessingML,
// SpreadsheetML or PresentationML — anything that understands a markup
// vocabulary belongs in the package for that format. It also does not model
// the OPC digital signature or thumbnail parts; they are carried through like
// any other part.
package opc
