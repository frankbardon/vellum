package xmlcopy

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
)

// DecodeText decodes XML character data — entity references (&amp;, &lt;,
// &gt;, &quot;, &apos;) and numeric character references (&#38;, &#x26;) —
// back to the plain text they represent.
//
// raw is expected to be the still-escaped bytes of a single leaf element's
// text content, typically an [Element.Content] slice a caller has already
// isolated with [Walk] — a `<w:t>`'s content span, for instance. It must not
// itself contain child elements: nothing in this package ever hands a caller
// a Content span that does, so this is a decode of an already-located leaf,
// not a second parse of a source part.
//
// It is implemented by wrapping raw in a synthetic single-element document
// and reading it back with an encoding/xml.Decoder purely for its CharData
// tokens. That is the correct way to resolve every legal entity and numeric
// character reference without hand-rolling an XML entity table, and it stays
// inside the one package in the fill-mode stack allowed to import
// encoding/xml — everything above xmlcopy calls this rather than decoding
// entities itself, which is what keeps [Walk]'s own package doc's
// non-destructiveness argument intact for every caller built on top of it.
func DecodeText(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}

	wrapped := make([]byte, 0, len(raw)+7)
	wrapped = append(wrapped, "<x>"...)
	wrapped = append(wrapped, raw...)
	wrapped = append(wrapped, "</x>"...)

	dec := xml.NewDecoder(bytes.NewReader(wrapped))
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", verr.WrapCodedErrorWithDetails(
				err,
				verr.VELLUM_TEMPLATE_XML_MALFORMED,
				"text content does not decode as well-formed XML character data",
				map[string]any{"raw_length": len(raw)},
			)
		}
		if cd, ok := tok.(xml.CharData); ok {
			b.Write(cd)
		}
	}
	return b.String(), nil
}
