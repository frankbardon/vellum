package shape

import "bytes"

// newReader adapts a byte slice to the font parser's Resource interface, which
// wants random access.
//
// bytes.Reader satisfies it exactly; the wrapper exists so the type assertion
// happens here rather than at each call site, and so the parser cannot be handed
// a stream it would have to buffer.
func newReader(program []byte) *bytes.Reader { return bytes.NewReader(program) }
