package xmlcopy

import (
	verr "github.com/frankbardon/vellum/errors"
)

// Span is a half-open byte range [Start, End) into a part's raw source bytes.
// Offsets are int64 to match encoding/xml.Decoder.InputOffset, which is what
// every Span in this package is ultimately derived from.
type Span struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// Empty reports whether the span covers no bytes. A zero-width span is not
// invalid — it is a valid insertion point, positioned at Start — and Apply
// treats it as such.
func (s Span) Empty() bool { return s.Start == s.End }

// Replacement pairs a Span in the source with the raw bytes that take its
// place in the output.
//
// Data is spliced in verbatim. xmlcopy performs no escaping and does not
// check that Data is well-formed XML in context — the caller building it
// (typically template/splice) already knows what it is writing and is
// responsible for [EscapeText] / [EscapeAttr] on any text it embeds.
type Replacement struct {
	Start int64
	End   int64
	Data  []byte
}

// Span returns the source span this replacement covers.
func (r Replacement) Span() Span { return Span{Start: r.Start, End: r.End} }

// Replace returns a Replacement covering s, carrying data.
func (s Span) Replace(data []byte) Replacement {
	return Replacement{Start: s.Start, End: s.End, Data: data}
}

// Apply produces the final bytes for src: every byte outside a replacement's
// span is copied through unchanged, and each replacement's span is replaced
// with its Data.
//
// replacements must be supplied in ascending, non-overlapping order — the
// order a caller naturally produces by walking the source once with [Walk]
// and collecting spans as it goes. Apply does not sort them: silently
// reordering a caller's replacements would hide the exact class of bug this
// check exists to catch, a splice site computed against the wrong pass over
// the document. A span that starts before the previous one ended — whether
// because the two overlap or because they were supplied out of order — is
// rejected with [verr.VELLUM_TEMPLATE_XML_SPAN_INVALID], and so is a span
// that runs outside [0, len(src)] or has Start > End.
//
// This is an internal-invariant class of error: nothing an untrusted
// template's own bytes can do produces it, only a bug in the caller
// assembling replacements, which is why the code carries no fixup a template
// author could act on.
func Apply(src []byte, replacements []Replacement) ([]byte, error) {
	n := int64(len(src))
	out := make([]byte, 0, len(src))
	var cursor int64

	for i, r := range replacements {
		if r.Start < 0 || r.End < r.Start || r.End > n {
			return nil, verr.NewCodedErrorWithDetails(
				verr.VELLUM_TEMPLATE_XML_SPAN_INVALID,
				"replacement span is out of bounds",
				map[string]any{
					"index":      i,
					"start":      r.Start,
					"end":        r.End,
					"source_len": n,
				},
			)
		}
		if r.Start < cursor {
			return nil, verr.NewCodedErrorWithDetails(
				verr.VELLUM_TEMPLATE_XML_SPAN_INVALID,
				"replacement spans overlap or are out of order",
				map[string]any{
					"index":          i,
					"start":          r.Start,
					"end":            r.End,
					"previous_index": i - 1,
					"previous_end":   cursor,
				},
			)
		}
		out = append(out, src[cursor:r.Start]...)
		out = append(out, r.Data...)
		cursor = r.End
	}
	out = append(out, src[cursor:]...)
	return out, nil
}
