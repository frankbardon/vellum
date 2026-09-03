package defrag

import (
	"strings"
	"unicode/utf8"
)

// Match is one occurrence of a literal string in a [Flat]'s flattened text,
// as rune indices — the same coordinate space [Flat.Locate] expects.
type Match struct {
	Start, End int
}

// FindAll returns every non-overlapping occurrence of literal in f.Text,
// left to right, as rune-index ranges ready to hand to [Flat.Locate].
//
// Matching happens against f.Text — the safe, already-decoded, already-
// defragmented flattened text — never against raw XML: a regex or substring
// search over the source bytes directly would match across escaped entities
// and run boundaries in ways that have nothing to do with what a reader
// sees, which is exactly the failure mode flattening exists to avoid.
//
// Plain substring search is sufficient for a literal marker like
// "{{customer_name}}"; this is deliberately not a pattern or regex matcher —
// that is more machinery than a literal marker search needs, and building it
// ahead of a story that requires it is scope this one does not take on.
func (f *Flat) FindAll(literal string) []Match {
	if literal == "" {
		return nil
	}

	var out []Match
	searched := 0 // byte offset into f.Text already scanned
	for {
		idx := strings.Index(f.Text[searched:], literal)
		if idx < 0 {
			break
		}
		byteStart := searched + idx
		byteEnd := byteStart + len(literal)
		out = append(out, Match{
			Start: utf8.RuneCountInString(f.Text[:byteStart]),
			End:   utf8.RuneCountInString(f.Text[:byteEnd]),
		})
		searched = byteEnd
	}
	return out
}
