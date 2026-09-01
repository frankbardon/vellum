package opc

import (
	"bytes"
	"io"

	verr "github.com/frankbardon/vellum/errors"
)

// Part is a single member of an OPC package.
//
// Exactly one of Data and Open must be set. Open exists so a package need not
// hold every part in memory at once: parts are materialised one at a time
// during a write, so peak memory is proportional to the largest part rather
// than to the whole document — which matters, because decks get large and the
// consumer contract asks for streaming.
type Part struct {
	// Name is the absolute, forward-slashed part name: "/word/document.xml".
	Name string

	// ContentType is the part's MIME type. It may be empty when the part's
	// extension is covered by a package-level default declaration.
	ContentType string

	// Data is the part's content.
	Data []byte

	// Open lazily supplies the part's content. Called at most once per write.
	Open func() (io.ReadCloser, error)

	// method is the ZIP compression method this part was read with. It is
	// preserved across a round trip because re-deflating a stored part, or
	// storing a deflated one, would change bytes Vellum promised not to touch.
	// Unset on a part built rather than read, where the method follows from
	// the content type by rule.
	method    uint16
	hasMethod bool
}

// srcMethod reports the compression method this part was read with, if any.
func (p *Part) srcMethod() (uint16, bool) {
	if p == nil {
		return 0, false
	}
	return p.method, p.hasMethod
}

// Bytes materialises the part's content.
//
// For a part carrying Data this is the stored slice itself, not a copy: the
// caller must not mutate it, and the alternative — copying every part on every
// access — would defeat the memory bound the lazy path exists to provide.
func (p *Part) Bytes() ([]byte, error) {
	if p == nil {
		return nil, verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT, "nil part")
	}
	if (p.Data == nil) == (p.Open == nil) {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"part must set exactly one of Data and Open",
			map[string]any{"part_name": p.Name})
	}
	if p.Data != nil {
		return p.Data, nil
	}

	rc, err := p.Open()
	if err != nil {
		return nil, verr.WrapCodedErrorWithDetails(err, verr.VELLUM_OPC_INVALID,
			"opening the part source failed",
			map[string]any{"part_name": p.Name})
	}
	defer rc.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rc); err != nil {
		return nil, verr.WrapCodedErrorWithDetails(err, verr.VELLUM_OPC_INVALID,
			"reading the part source failed",
			map[string]any{"part_name": p.Name})
	}
	return buf.Bytes(), nil
}

// clone returns a shallow copy. The content is shared rather than duplicated;
// callers treat part content as immutable.
func (p *Part) clone() *Part {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}
