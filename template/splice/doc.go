// Package splice turns one discovered [anchor.Anchor] and one resolved
// [fragment.Sequence] into the edits needed to bind that content into a
// fill-mode template, using xmlcopy to do it non-destructively.
//
// It is the last of the three fill-mode building blocks bind (E10) composes:
// anchor finds where to splice, defrag resolves exactly which bytes a marker
// occupies, and this package renders the content and produces the single
// [xmlcopy.Replacement] a caller applies to the anchor's own part. Splice
// itself never calls [xmlcopy.Apply] and never touches the package's parts
// map for the anchor's own part — only [Splice]'s caller does that, once, for
// every anchor a binding fills. The one exception is an asset block: adding a
// media part and its relationship *is* the whole representation of "add a
// part," so those are direct, immediate [opc.Package] mutations rather than
// spans, exactly mirroring how doc's own writer works.
//
// # Fill has no theme
//
// A [fragment.Run]'s Style carries fields resolved against a theme —
// FaceIndex, a literal Color, a Background — that mean nothing here: fill
// mode never resolves against a theme, and there is no Doc.Fonts or
// Doc.Palette in this call to resolve them against. What a spliced run's
// character appearance is drawn from is the *template's own* existing run
// formatting at the splice site: the placeholder content's rPr for a native
// anchor, the marker's own original rPr for a marker anchor. Style.Bold,
// Style.Italic and Style.Underline are the one part of TextStyle honoured
// directly, because they are self-contained booleans rather than
// theme-relative values, and they are layered as *additions* on top of the
// template's own formatting: a run whose Style does not ask for bold does not
// strip bold the template already had, because [fragment.TextStyle] has no
// way to represent "explicitly not bold" as distinct from "the binding did
// not say" — see [layerRPr] for exactly how, and where, an addition is
// positioned inside an existing rPr's element sequence.
//
// # Two splice targets, two different shapes
//
// A native anchor's Span is a whole w:sdt; its content is block-level, so it
// can legitimately hold any mix of paragraphs, a table and an image. Splicing
// there replaces the whole of its w:sdtContent's own Content span. See
// [spliceNative].
//
// A marker anchor's Span is the whole enclosing w:p; the marker itself sits
// mid-run, at a specific rune range inside that paragraph's flattened text.
// Nothing block-level can go there without producing a w:tbl or a second
// w:p nested inside a w:r, which is not well-formed WordprocessingML — so a
// marker splice accepts exactly one Paragraph block and refuses everything
// else with [verr.VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED]. This is a
// permanent scope boundary, not a "not implemented yet" gap: block-level
// content belongs behind a native content control. See [spliceMarker].
//
// # No encoding/xml
//
// Like every package in this subtree, splice never imports encoding/xml
// (TestNoEncodingXMLInFill enforces it transitively over template/...). Every
// read of source structure goes through [xmlcopy.Walk]; every byte-level
// inspection of a cloned rPr — deciding whether it already carries a <w:b>,
// finding where in its child sequence a new element belongs — is a raw
// substring search over bytes this package already owns, never a second
// parse.
package splice
