package defrag

import (
	"unicode"
	"unicode/utf8"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/xmlcopy"
)

// Piece is text to preserve at one edge of a match, with its original run's
// formatting deep-cloned rather than reconstructed.
type Piece struct {
	// Text is the decoded plain text surviving from the run: the portion
	// that falls outside [matchStart, matchEnd).
	Text string

	// RPr is the exact source bytes of the run's own <w:rPr>...</w:rPr>,
	// copied verbatim, or nil if the run carried none. Never reconstructed:
	// a run's rPr can hold properties Vellum's own style model does not
	// represent at all — w:rsid* revision-save IDs, a w:lang mark,
	// east-Asian-specific properties — and rebuilding only the properties
	// Vellum understands would silently drop the rest.
	RPr []byte

	// Preserve reports whether the rebuilt w:t needs xml:space="preserve":
	// true when Text itself has leading or trailing whitespace that would
	// otherwise be a reader's to collapse, or when the run it came from
	// already carried the attribute explicitly — checked on the source run's
	// own w:t, not inferred from whitespace alone, so an author's explicit
	// declaration survives even when the surviving substring's own edges
	// happen not to need it.
	Preserve bool
}

// Site describes the single-replacement span needed to substitute the
// content in [matchStart, matchEnd) of a Flat's text with anything else,
// while preserving every run that substitution partially touches.
type Site struct {
	// Affected is the span, in the original source bytes, from the start of
	// the first run the match touches through the end of the last run it
	// touches. A caller replaces exactly this span — nothing more, nothing
	// less — with Prefix's rendering (if any) + new content + Suffix's
	// rendering (if any).
	Affected xmlcopy.Span

	// Prefix is the text of the first affected run that comes before the
	// match starts, if the match does not begin exactly on a run boundary.
	// Nil when it does — nothing to preserve.
	Prefix *Piece

	// Suffix is the text of the last affected run that comes after the
	// match ends, mirroring Prefix. Nil when the match ends exactly on a
	// run boundary.
	Suffix *Piece
}

// Locate computes the [Site] needed to replace [matchStart, matchEnd) — rune
// indices into f.Text, the same coordinate space [Flat.FindAll] returns —
// with new content, while preserving every run the match only partially
// covers.
//
// matchStart == matchEnd is a legitimate zero-width match: a pure insertion
// point with nothing being replaced. Locate resolves it to the smallest
// Affected span that still lets the insertion land at exactly that point —
// splitting the one run it falls inside if it falls strictly inside a run's
// own text, or a zero-width span at the nearest run boundary (or the
// container's own Content span, if the container carries no runs at all)
// when it does not.
//
// Anything outside 0 <= matchStart <= matchEnd <= the rune length of f.Text
// is a caller bug — [verr.VELLUM_DEFRAG_RANGE_INVALID] — never something an
// untrusted template's own bytes can trigger, since a caller derives both
// bounds from f.Text itself.
func (f *Flat) Locate(matchStart, matchEnd int) (Site, error) {
	if matchStart < 0 || matchEnd < matchStart || matchEnd > f.runeLen {
		return Site{}, verr.NewCodedErrorWithDetails(verr.VELLUM_DEFRAG_RANGE_INVALID,
			"match range is outside the flattened text's bounds, or starts after it ends",
			map[string]any{
				"match_start":           matchStart,
				"match_end":             matchEnd,
				"flattened_rune_length": f.runeLen,
			})
	}

	if matchStart == matchEnd {
		return f.locateInsertion(matchStart), nil
	}

	startIdx, ok := f.runeIndexOf(matchStart)
	if !ok {
		return Site{}, internalRuneNotFoundErr(matchStart, f.runeLen)
	}
	endIdx, ok := f.runeIndexOf(matchEnd - 1)
	if !ok {
		return Site{}, internalRuneNotFoundErr(matchEnd-1, f.runeLen)
	}

	startRun := &f.runs[startIdx]
	endRun := &f.runs[endIdx]

	site := Site{
		Affected: xmlcopy.Span{Start: startRun.span.Start, End: endRun.span.End},
	}
	if matchStart > startRun.textStart {
		site.Prefix = buildPiece(startRun, 0, matchStart-startRun.textStart)
	}
	if matchEnd < endRun.textStart+endRun.textLen {
		site.Suffix = buildPiece(endRun, matchEnd-endRun.textStart, endRun.textLen)
	}
	return site, nil
}

// runeIndexOf returns the index into f.runs of the run whose own text
// contains rune index pos, i.e. run.textStart <= pos < run.textStart +
// run.textLen. Every index in [0, f.runeLen) falls inside exactly one run's
// interval: contributing runs partition the flattened text contiguously and
// a zero-text run's interval is empty, so it never claims an index.
func (f *Flat) runeIndexOf(pos int) (int, bool) {
	for i := range f.runs {
		r := &f.runs[i]
		if pos >= r.textStart && pos < r.textStart+r.textLen {
			return i, true
		}
	}
	return 0, false
}

// locateInsertion resolves a zero-width match at rune position pos.
func (f *Flat) locateInsertion(pos int) Site {
	// pos falls strictly inside a run's own text: split that run into a
	// Prefix and a Suffix around the insertion point, and the whole run
	// becomes Affected so both survive in the rebuilt replacement.
	for i := range f.runs {
		r := &f.runs[i]
		if r.textLen == 0 {
			continue
		}
		if pos > r.textStart && pos < r.textStart+r.textLen {
			return Site{
				Affected: r.span,
				Prefix:   buildPiece(r, 0, pos-r.textStart),
				Suffix:   buildPiece(r, pos-r.textStart, r.textLen),
			}
		}
	}

	// pos aligns exactly with a run boundary: attach the insertion
	// immediately before the earliest run (by document order) that starts
	// there — which may itself be a zero-text run, a perfectly good anchor
	// point since nothing about it is being consumed. Neither run on either
	// side of the boundary needs to be touched.
	for i := range f.runs {
		r := &f.runs[i]
		if r.textStart == pos {
			return Site{Affected: xmlcopy.Span{Start: r.span.Start, End: r.span.Start}}
		}
	}

	// pos is past every run's own text (inserting at the very end), or the
	// container carries no runs at all: anchor at the end of the last run,
	// or at the container's own Content span when there is no run to anchor
	// to.
	if n := len(f.runs); n > 0 {
		end := f.runs[n-1].span.End
		return Site{Affected: xmlcopy.Span{Start: end, End: end}}
	}
	return Site{Affected: xmlcopy.Span{Start: f.content.End, End: f.content.End}}
}

// buildPiece slices run's own text by rune offsets [from, to) and builds the
// Piece a caller preserves at the corresponding edge of a match.
func buildPiece(r *runInfo, from, to int) *Piece {
	runes := []rune(r.text)
	text := string(runes[from:to])
	return &Piece{
		Text:     text,
		RPr:      cloneBytes(r.rpr),
		Preserve: r.hadPreserve || needsPreserve(text),
	}
}

// cloneBytes returns an independent copy of b, or nil for a nil input — the
// deep clone [Piece.RPr] documents: it must survive independent of whatever
// else later touches src's backing array.
func cloneBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	return append([]byte(nil), b...)
}

// needsPreserve reports whether s has leading or trailing whitespace
// significant enough that a reader is otherwise entitled to trim it,
// mirroring the sheet package's own rule for the same question over a
// shared-string cell.
func needsPreserve(s string) bool {
	if s == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(s)
	last, _ := utf8.DecodeLastRuneInString(s)
	return unicode.IsSpace(first) || unicode.IsSpace(last)
}

// internalRuneNotFoundErr reports the internal-invariant condition where a
// rune index inside [0, runeLen) claimed no owning run — unreachable given
// Flatten's own construction, since contributing runs partition the
// flattened text with no gaps, but checked rather than assumed so a future
// change to that invariant fails loudly here instead of panicking on a slice
// index.
func internalRuneNotFoundErr(pos, runeLen int) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
		"a rune index inside the flattened text's own bounds claimed no owning run",
		map[string]any{"rune_index": pos, "flattened_rune_length": runeLen})
}
