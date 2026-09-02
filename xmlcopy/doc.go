// Package xmlcopy is the token-copy XML rewriter fill mode edits every source
// part through.
//
// A template opened by opc.Open (see [github.com/frankbardon/vellum/opc]) is
// held as the bytes it was read as, and fill mode's whole non-destructiveness
// guarantee — untouched parts survive byte-for-byte, touched parts survive
// everywhere except the spans a caller deliberately replaced — rests on never
// re-serialising a part it edits. encoding/xml's Marshal, and the
// Decoder.Token-then-re-encode round trip it enables, cannot deliver that: it
// does not preserve namespace prefixes, attribute order or self-closing form,
// so re-marshalling a part authored by Word reliably produces different bytes
// than Word wrote — the exact failure mode CLAUDE.md's "Do not re-marshal a
// source part with encoding/xml" rule exists to name. Word tolerates a great
// deal; it does not tolerate silently losing the shape of a part it wrote.
//
// So xmlcopy never re-encodes. [Walk] uses an encoding/xml.Decoder read-only,
// purely to learn structure — element names, nesting depth, byte offsets — and
// never to produce output. [Apply] then treats the part as an opaque byte
// stream: every byte outside a caller-identified span is copied through
// unchanged, and each span is replaced with caller-supplied raw bytes. The
// caller is responsible for those bytes being well-formed XML in context —
// text content escaped, attribute values quoted — because xmlcopy has no way
// to check that a spliced-in fragment is correct without parsing the whole
// part again, which is the operation this package exists to avoid.
// [EscapeText] and [EscapeAttr] are offered for building that fragment.
//
// This is the one place in the fill-mode stack allowed to import
// encoding/xml. template, template/defrag and template/splice are built on
// top of xmlcopy and are firewalled from importing encoding/xml directly by
// TestNoEncodingXMLInFill — precisely so that every edit to a source part is
// forced through the byte-span discipline this package implements, rather
// than through a decode-mutate-re-encode path that would quietly reintroduce
// the failure mode above.
package xmlcopy
