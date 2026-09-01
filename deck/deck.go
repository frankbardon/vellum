// Package deck emits PresentationML — the .pptx format.
//
// # The model is public
//
// A consumer composing from the block model gets a deck assembled by the
// lowering; a consumer needing reach the block vocabulary does not express
// builds a [Deck] directly and writes it. Both converge on one writer, so there
// is no second serialiser able to drift.
//
// # Masters are authored, not shipped
//
// A .pptx does not carry loose formatting the way a .docx does. Every slide
// inherits from a layout, every layout from a master, and the master from a
// theme part that declares the colour scheme and the font scheme by name. A
// deck with no master is not a deck with default styling — it is a file
// PowerPoint refuses.
//
// The two available answers were to ship a fixed .pptx and copy its parts
// through, or to author the master, layouts and theme from the theme document.
// Vellum authors them, so the built-in theme produces a working deck with
// nothing wired and a consumer's theme produces a deck in the consumer's
// colours without them supplying a template. See [Author].
//
// # Overrides only
//
// The inheritance chain is the reason this package writes as little as it can.
// A slide that restates the family, size and colour its layout already gives it
// produces a deck that looks correct and cannot be restyled: changing the
// master changes nothing, because every slide overrides it. So a run carries a
// size only when it differs from the level style it inherits, and a shape
// carries a frame only when it differs from its placeholder's.
//
// That is a property of what the model is asked to hold rather than a filter
// applied on the way out. A zero [RunStyle] field means inherit, and there is
// no way to say "inherit" other than by leaving it zero.
//
// # What this model does not hold yet
//
// Tables. A DrawingML table in a slide is restylable only through a table
// style, and a table style is built from colours this package is not yet
// handed — so a Table arm here would be a shape the writer refuses, which is
// worse than not having one. It arrives with the overflow policy, where a
// table's appearance and its continuation across slides are decided together.
package deck
