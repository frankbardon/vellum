// Package content builds PDF content streams.
//
// A content stream is a sequence of operators in postfix form — operands first,
// then the operator — and it is the one part of a PDF that is not built out of
// objects. This package emits it as bytes rather than modelling it, because
// there is nothing to gain from an intermediate tree that is written once and
// never queried.
//
// Numbers are [object.Real], so nothing on this path passes through a float.
package content

import (
	"github.com/frankbardon/vellum/pdf/object"
)

// Builder accumulates content stream operators.
//
// The zero value is ready to use. Operator methods return the builder so a page
// reads as a sequence rather than as a column of statements.
type Builder struct {
	buf   []byte
	depth int
}

// Bytes returns the stream.
func (b *Builder) Bytes() []byte { return b.buf }

// InText reports whether a text object is currently open. Used by callers that
// assemble a page in pieces and must not close one twice.
func (b *Builder) InText() bool { return b.depth > 0 }

// BeginText emits BT, opening a text object.
func (b *Builder) BeginText() *Builder {
	b.depth++
	return b.op("BT")
}

// EndText emits ET, closing a text object.
func (b *Builder) EndText() *Builder {
	if b.depth > 0 {
		b.depth--
	}
	return b.op("ET")
}

// SetFont emits Tf, selecting a font resource at a size in points.
//
// The name is a resource name from the page's /Font dictionary, not a font
// family: PDF addresses fonts through the page's resources, so a content stream
// that names a family directly names nothing.
func (b *Builder) SetFont(resource object.Name, size object.Real) *Builder {
	b.arg(resource)
	b.arg(size)
	return b.op("Tf")
}

// SetLeading emits TL, the distance between baselines used by T*.
func (b *Builder) SetLeading(leading object.Real) *Builder {
	b.arg(leading)
	return b.op("TL")
}

// MoveText emits Td, moving to the start of the next line offset from the
// current line's origin.
func (b *Builder) MoveText(dx, dy object.Real) *Builder {
	b.arg(dx)
	b.arg(dy)
	return b.op("Td")
}

// NextLine emits T*, moving down by the leading.
func (b *Builder) NextLine() *Builder { return b.op("T*") }

// ShowGlyphs emits Tj for a run of glyph identifiers.
//
// The identifiers are written as a hexadecimal string of big-endian sixteen-bit
// values, which is what an Identity-H encoded composite font expects. Writing
// them as a literal string would work and would need escaping for every byte
// that collides with PDF syntax — and glyph identifiers collide constantly,
// because they are small numbers and the low bytes land on parentheses and
// backslashes.
func (b *Builder) ShowGlyphs(gids []uint16) *Builder {
	raw := make([]byte, 0, len(gids)*2)
	for _, g := range gids {
		raw = append(raw, byte(g>>8), byte(g))
	}
	b.arg(object.HexString(raw))
	return b.op("Tj")
}

// SetFillRGB emits rg, setting the non-stroking colour.
//
// Components are in the range zero to one, so [object.Ratio] is the usual way
// to build them from an integer channel value.
func (b *Builder) SetFillRGB(r, g, bl object.Real) *Builder {
	b.arg(r)
	b.arg(g)
	b.arg(bl)
	return b.op("rg")
}

// Rect emits re, appending a rectangle to the current path.
func (b *Builder) Rect(x, y, w, h object.Real) *Builder {
	b.arg(x)
	b.arg(y)
	b.arg(w)
	b.arg(h)
	return b.op("re")
}

// Fill emits f, filling the current path.
func (b *Builder) Fill() *Builder { return b.op("f") }

// Save emits q, pushing the graphics state.
func (b *Builder) Save() *Builder { return b.op("q") }

// Restore emits Q, popping the graphics state.
func (b *Builder) Restore() *Builder { return b.op("Q") }

// arg appends an operand.
func (b *Builder) arg(o object.Object) {
	b.buf = o.AppendPDF(b.buf)
	b.buf = append(b.buf, ' ')
}

// op appends an operator and ends the line.
//
// One operator per line. PDF does not care, and a person reading a content
// stream out of a broken file very much does.
func (b *Builder) op(name string) *Builder {
	b.buf = append(b.buf, name...)
	b.buf = append(b.buf, '\n')
	return b
}
