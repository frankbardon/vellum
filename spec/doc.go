// Package spec is Vellum's primary public model: a declarative description of
// an artifact as ordered sections of generic blocks.
//
// The model knows blocks, not semantics. Vocabulary like "cover", "executive
// summary" or "methodology appendix" belongs to whoever is composing the
// document, and a library that shipped one product's section types would make
// the next consumer fight it — so those concepts are expressed by composing
// blocks, never by a kind of their own.
//
// # Status
//
// Under construction. Two of the seven block kinds carry content today,
// enough for the substrate to prove itself end to end; the rest arrive with
// the spec-surface epic, along with strict decoding, the published schema,
// content hashing and per-block marks. A block kind with no arm implemented is
// a loud error, never a silent omission.
package spec
