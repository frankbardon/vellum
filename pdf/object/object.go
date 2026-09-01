// Package object writes the PDF object layer: objects, indirect references,
// streams, the cross-reference table and the trailer.
//
// Vellum owns this rather than importing it. The one Go library that solves it
// well is GPL-3.0, which a permissively licensed library cannot take, and the
// permissive alternative writes a wall-clock timestamp into font bytes we are
// required to pin. Owning it also means owning object numbering and xref
// layout, which is what makes a byte-identical PDF possible at all.
//
// # Determinism
//
// Object numbers are assigned in the order objects are added, and nothing here
// sorts, hashes or randomises. A dictionary is an ordered sequence of entries
// rather than a map, so key order is a property of the code that built it
// rather than of the run — the nondeterminism is unrepresentable instead of
// merely tested against.
//
// Numbers are fixed point. See [Real].
//
// # What is deliberately not here
//
// No object streams and no cross-reference streams. Both are permitted by
// PDF 1.5 and later and both would make the file smaller. They also make the
// byte layout depend on how objects were grouped into streams, put a second
// compressed region into the file whose contents vary with the compressor, and
// are the part of the format least uniformly implemented by validators. A
// classic table and trailer is larger, entirely predictable, and readable in a
// hex dump when something is wrong.
package object

import (
	"strconv"
)

// Object is a value that can be written in PDF syntax.
//
// AppendPDF appends rather than writing to an io.Writer because the
// cross-reference table records the byte offset of every indirect object, so
// the body has to be materialised before the file can be finished. Appending
// makes that explicit instead of layering a counting writer over it.
type Object interface {
	// AppendPDF appends this object's serialised form to dst and returns the
	// extended slice.
	AppendPDF(dst []byte) []byte
}

// Null is the PDF null object.
type Null struct{}

// AppendPDF implements [Object].
func (Null) AppendPDF(dst []byte) []byte { return append(dst, "null"...) }

// Bool is a PDF boolean.
type Bool bool

// AppendPDF implements [Object].
func (b Bool) AppendPDF(dst []byte) []byte {
	if b {
		return append(dst, "true"...)
	}
	return append(dst, "false"...)
}

// Name is a PDF name, written without its leading solidus.
//
// Construct it as Name("Type"), not Name("/Type"); the solidus is syntax and is
// added on write. Including it would produce "//Type", which readers accept as
// a name whose first character is a solidus and which is not the name meant.
type Name string

// AppendPDF implements [Object].
//
// Characters outside the regular-character set are escaped as #XX. In practice
// Vellum emits only names drawn from the specification's own vocabulary, so the
// escape never fires — it is here because a name reaching this point from a
// font's PostScript name is possible, and an unescaped delimiter inside a name
// silently reshapes the surrounding object rather than failing.
func (n Name) AppendPDF(dst []byte) []byte {
	dst = append(dst, '/')
	for i := 0; i < len(n); i++ {
		c := n[i]
		if isRegularChar(c) && c != '#' {
			dst = append(dst, c)
			continue
		}
		dst = append(dst, '#', hexDigit(c>>4), hexDigit(c&0x0f))
	}
	return dst
}

// String is a PDF literal string, written in parentheses.
type String []byte

// AppendPDF implements [Object].
func (s String) AppendPDF(dst []byte) []byte {
	dst = append(dst, '(')
	for _, c := range s {
		switch c {
		case '(', ')', '\\':
			dst = append(dst, '\\', c)
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		case '\b':
			dst = append(dst, '\\', 'b')
		case '\f':
			dst = append(dst, '\\', 'f')
		default:
			if c < 32 || c > 126 {
				// Octal rather than raw, so the file stays seven-bit outside
				// stream data and a transport that mangles high bytes cannot
				// corrupt a string without corrupting the length too.
				dst = append(dst, '\\',
					byte('0'+(c>>6)), byte('0'+((c>>3)&7)), byte('0'+(c&7)))
				continue
			}
			dst = append(dst, c)
		}
	}
	return append(dst, ')')
}

// HexString is a PDF string written in angle brackets as hexadecimal.
//
// Used where the bytes are identifiers rather than text — the trailer's /ID,
// most importantly, whose two halves are digests and would be unreadable and
// heavily escaped as literal strings.
type HexString []byte

// AppendPDF implements [Object].
func (h HexString) AppendPDF(dst []byte) []byte {
	dst = append(dst, '<')
	for _, c := range h {
		dst = append(dst, hexDigit(c>>4), hexDigit(c&0x0f))
	}
	return append(dst, '>')
}

// Array is a PDF array.
type Array []Object

// AppendPDF implements [Object].
func (a Array) AppendPDF(dst []byte) []byte {
	dst = append(dst, '[')
	for i, o := range a {
		if i > 0 {
			dst = append(dst, ' ')
		}
		dst = o.AppendPDF(dst)
	}
	return append(dst, ']')
}

// Ref is an indirect reference to a numbered object.
type Ref struct {
	// Number is the object number, assigned by [Document.Add].
	Number int

	// Generation is always zero: Vellum writes files rather than updating them,
	// and a generation above zero only arises from an incremental update.
	Generation int
}

// AppendPDF implements [Object].
func (r Ref) AppendPDF(dst []byte) []byte {
	dst = strconv.AppendInt(dst, int64(r.Number), 10)
	dst = append(dst, ' ')
	dst = strconv.AppendInt(dst, int64(r.Generation), 10)
	return append(dst, " R"...)
}

// IsZero reports whether the reference names no object.
func (r Ref) IsZero() bool { return r.Number == 0 }

// Raw is a pre-serialised fragment, inserted verbatim.
//
// It exists for the one case that needs it — a content stream, which is built
// by an operator emitter rather than out of objects — and is deliberately
// awkward to reach for otherwise. Nothing checks that its bytes are valid PDF.
type Raw []byte

// AppendPDF implements [Object].
func (r Raw) AppendPDF(dst []byte) []byte { return append(dst, r...) }

// isRegularChar reports whether c may appear unescaped in a name.
//
// The specification defines regular characters as everything outside the
// delimiters and whitespace, restricted here to printable ASCII.
func isRegularChar(c byte) bool {
	if c <= 32 || c >= 127 {
		return false
	}
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return false
	}
	return true
}

func hexDigit(n byte) byte {
	if n < 10 {
		return '0' + n
	}
	return 'A' + (n - 10)
}
