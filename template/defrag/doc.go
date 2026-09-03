// Package defrag resolves Word's own text fragmentation down to exact run
// boundaries, so a fill can splice data in without disturbing formatting it
// does not understand.
//
// Word fragments text across runs for reasons that have nothing to do with
// what the author intended: the spell-checker splitting a word mid-marker to
// underline half of it, a language-mark boundary, a save-revision boundary, a
// paste boundary. A {{marker}} an author sees as one contiguous run of text
// can be several w:r elements in the actual XML, split at an arbitrary byte
// offset, each carrying its own w:rPr for reasons unrelated to the marker at
// all.
//
// template/anchor (E9-S2) already finds that a marker exists somewhere in a
// paragraph, by flattening the paragraph's w:t text for detection and
// recording the whole paragraph's span — deliberately not a run's, because
// resolving that needs exactly the algorithm this package implements. See
// [anchor.Anchor.Span]'s doc comment for the handoff this package picks up.
//
// The shape is two steps:
//
//   - [Flatten] walks a container element (a w:p paragraph, in the anchor
//     handoff, but the algorithm has no reason to know that) and builds a
//     [Flat]: the decoded, concatenated text of every w:r run nested anywhere
//     inside it, plus a position map from a rune index in that text back to
//     the run it came from.
//
//   - [Flat.Locate] takes a matched rune range — from [Flat.FindAll] for a
//     literal marker, or from wherever a later matcher gets one — and
//     computes a [Site]: the single, contiguous byte span a caller needs to
//     replace with a [xmlcopy.Replacement], plus the formatted text
//     ([Piece]) at either edge of the match that has to survive the edit
//     because it belongs to a run the match only partially consumes.
//
// The key design point is that resplitting the boundary runs and substituting
// new content collapse into one replacement, not two edit passes over the
// document: [Site.Affected] already spans from the start of the first run the
// match touches through the end of the last one, so a caller (template/splice,
// E9-S4) builds one [xmlcopy.Replacement] whose Data is Prefix's rendering (if
// any) + the new content + Suffix's rendering (if any), and hands it to
// [xmlcopy.Apply] alongside every other part's edits in one pass.
//
// Every read of structure goes through [xmlcopy.Walk]; this package never
// imports encoding/xml directly (TestNoEncodingXMLInFill enforces it) —
// [xmlcopy.DecodeText] is what performs entity decoding, on xmlcopy's side of
// that firewall.
//
// # Scope boundaries, stated rather than silently assumed
//
// A run that contributes no w:t — one holding only a w:tab, a w:br, or a
// bookmark — contributes zero runes to the flattened text. It can never be
// *inside* a match (there is nothing of it in the flattened text to match
// against), only positioned adjacent to one in the source. Whether such a run
// ends up inside [Site.Affected] or outside it falls out of where it sits
// relative to the first and last run the match *does* touch, not from any
// rule this package states about it specifically: a zero-text run wholly
// between the first and last affected run is silently discarded along with
// everything else in that span (it carried no formatting a caller could have
// meaningfully preserved anyway); one outside that range is untouched.
//
// A run entirely consumed by the match — not just partially overlapping a
// boundary — carries no [Piece] forward. Only the two boundary runs, if the
// match does not begin or end exactly on a run boundary, get a Piece. This is
// deliberate, not a gap: the substituted content can only sensibly carry one
// formatting choice, so preserving several different discarded runs'
// formatting for text that no longer exists in that form would not be
// meaningful.
package defrag
