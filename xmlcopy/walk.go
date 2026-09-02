package xmlcopy

import (
	"bytes"
	"encoding/xml"
	"io"

	verr "github.com/frankbardon/vellum/errors"
)

// Element describes one XML element discovered while walking a source
// document with [Walk]: its expanded name, its attributes as
// encoding/xml.Decoder resolved them, its nesting depth, and the exact byte
// spans of the whole element and of its content.
//
// Everything here is read-only structural information. Name and Attr exist
// so a caller can match on them; they are never used to re-emit the element
// — the bytes for that come from Span, sliced out of the original source.
type Element struct {
	// Name is the element's expanded name: Local is the tag's local part and
	// Space, when the element or an ancestor declared a default or prefixed
	// namespace, is the resolved namespace URI rather than the prefix that
	// appeared in the source. Matching on the URI is what makes a caller's
	// match independent of which prefix a particular authoring tool chose for
	// it.
	Name xml.Name

	// Attr is the element's attributes, in document order, as
	// encoding/xml.Decoder parsed them — including namespace declarations
	// (xmlns, xmlns:*), which the decoder reports as ordinary attributes
	// carrying the synthetic Space "xmlns" rather than folding away. A
	// caller matching on a specific attribute matches by expanded Name, the
	// same as for elements.
	Attr []xml.Attr

	// Depth counts nesting from 0 at the outermost elements Walk visits — the
	// document's own root, or every top-level element if the source is a
	// fragment with siblings at the top. A caller distinguishing an outer
	// <w:sdt> from one nested inside it matches on Depth as well as Name:
	// depth is what makes that a correct depth-count rather than a
	// first-close-tag-wins guess.
	Depth int

	// Span is the whole element as it appears in the source, open tag through
	// close tag inclusive (or the whole of a self-closing tag):
	// src[Span.Start:Span.End].
	Span Span

	// Content is the span strictly between the end of the open tag and the
	// start of the close tag. For a self-closing element, Content is the
	// empty span positioned where content would begin, and equals Span.End —
	// SelfClosing reports exactly that condition.
	Content Span
}

// SelfClosing reports whether the element was written as a self-closing tag
// (<w:p/>) rather than an explicit empty pair (<w:p></w:p>). Both parse to an
// element with empty Content; they are told apart by whether the close tag
// consumed any bytes of its own, which is exactly what Span.End == Content.End
// means.
func (e Element) SelfClosing() bool { return e.Span.End == e.Content.End }

// Walk parses src as XML and calls visit once for every element, in document
// order, immediately after the element's closing tag — or its own
// self-closing tag — has been consumed, so Span and Content are always
// complete by the time visit sees them.
//
// Parsing is read-only: Walk uses an encoding/xml.Decoder purely to locate
// byte offsets and never constructs output from what it decodes. See the
// package doc for why that split is the whole of xmlcopy's non-destructiveness
// guarantee.
//
// visit returning a non-nil error stops the walk immediately and Walk returns
// that error unchanged, which lets a caller use a sentinel to mean "stop
// early, found what I need" and distinguish it from a genuine failure by
// comparing errors.Is against its own sentinel.
//
// A source that does not parse as well-formed XML — a mismatched close tag,
// a truncated document, an unterminated attribute — is reported as
// [verr.VELLUM_TEMPLATE_XML_MALFORMED]. Unlike a bad replacement span, this
// one can genuinely originate in the input: an untrusted template can be
// corrupt, so the code carries a fixup pointed at re-saving it from the
// authoring application rather than being marked not-applicable.
func Walk(src []byte, visit func(Element) error) error {
	dec := xml.NewDecoder(bytes.NewReader(src))

	type frame struct {
		name         xml.Name
		attr         []xml.Attr
		depth        int
		spanStart    int64
		contentStart int64
	}
	var stack []frame

	for {
		before := dec.InputOffset()
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return verr.WrapCodedErrorWithDetails(
				err,
				verr.VELLUM_TEMPLATE_XML_MALFORMED,
				"the source does not parse as well-formed XML",
				map[string]any{"at_offset": before},
			)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			after := dec.InputOffset()
			attr := make([]xml.Attr, len(t.Attr))
			copy(attr, t.Attr)
			stack = append(stack, frame{
				name:         t.Name,
				attr:         attr,
				depth:        len(stack),
				spanStart:    before,
				contentStart: after,
			})

		case xml.EndElement:
			if len(stack) == 0 {
				// encoding/xml itself refuses an unmatched close tag before
				// this is reachable, so this is defensive rather than a path
				// a malformed template can take.
				return verr.NewCodedErrorWithDetails(
					verr.VELLUM_TEMPLATE_XML_MALFORMED,
					"encountered a close tag with no matching open tag",
					map[string]any{"at_offset": before, "name": t.Name.Local},
				)
			}
			after := dec.InputOffset()
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			el := Element{
				Name:    top.name,
				Attr:    top.attr,
				Depth:   top.depth,
				Span:    Span{Start: top.spanStart, End: after},
				Content: Span{Start: top.contentStart, End: before},
			}
			if err := visit(el); err != nil {
				return err
			}
		}
	}

	return nil
}
