package defrag_test

import (
	"testing"

	"github.com/frankbardon/vellum/template/defrag"
	"github.com/frankbardon/vellum/xmlcopy"
)

// nsWordprocessing mirrors the constant xmlcopy's and template/anchor's own
// fixtures use.
const nsWordprocessing = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

// wordDoc wraps body in a realistic WordprocessingML w:document/w:body root —
// the same fixture shape xmlcopy's and template/anchor's own tests use, so
// namespace-prefix resolution and the XML declaration are exercised rather
// than assumed away by a hand-rolled fragment.
func wordDoc(body string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<w:document xmlns:w="` + nsWordprocessing + `">` +
		`<w:body>` + body + `</w:body>` +
		`</w:document>`)
}

// paragraphSpan returns the Span of the nth (0-indexed) w:p element in src.
func paragraphSpan(t *testing.T, src []byte, n int) xmlcopy.Span {
	t.Helper()
	var spans []xmlcopy.Span
	if err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if e.Name.Space == nsWordprocessing && e.Name.Local == "p" {
			spans = append(spans, e.Span)
		}
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if n >= len(spans) {
		t.Fatalf("paragraph %d not found; only %d paragraphs in the fixture", n, len(spans))
	}
	return spans[n]
}

// flatten is a small convenience wrapping defrag.Flatten with a t.Fatal on
// error, for tests that are not themselves exercising Flatten's own error
// path.
func flatten(t *testing.T, src []byte, span xmlcopy.Span) *defrag.Flat {
	t.Helper()
	f, err := defrag.Flatten(src, span)
	if err != nil {
		t.Fatalf("Flatten: %v", err)
	}
	return f
}

// runSpans returns the Span of every w:r element nested inside container, in
// document order — used by tests to check a Site.Affected or a Piece's
// origin against the run the fixture actually wrote.
func runSpans(t *testing.T, src []byte, container xmlcopy.Span) []xmlcopy.Span {
	t.Helper()
	var out []xmlcopy.Span
	if err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if e.Name.Space == nsWordprocessing && e.Name.Local == "r" &&
			e.Span.Start >= container.Start && e.Span.End <= container.End {
			out = append(out, e.Span)
		}
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	return out
}

// oneMatch runs FindAll and fails the test unless it finds exactly one
// occurrence, returning it.
func oneMatch(t *testing.T, f *defrag.Flat, literal string) defrag.Match {
	t.Helper()
	matches := f.FindAll(literal)
	if len(matches) != 1 {
		t.Fatalf("FindAll(%q) = %d matches, want 1: %+v", literal, len(matches), matches)
	}
	return matches[0]
}

// runeSlice slices s by rune indices [start, end) — the coordinate space
// Match and Locate use, which is not the same as a byte-index slice once
// non-ASCII text is involved.
func runeSlice(s string, start, end int) string {
	return string([]rune(s)[start:end])
}

// runeLen returns the rune count of s.
func runeLen(s string) int {
	return len([]rune(s))
}
