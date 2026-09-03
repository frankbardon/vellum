package splice_test

import (
	"encoding/base64"
	"testing"

	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/xmlcopy"
)

const (
	nsWordprocessing = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	nsRelationships  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

	ctMainDocument = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	partDocument   = "/word/document.xml"
)

// onePixelPNG is the same fixture doc/doc_test.go uses: a valid, minimal
// one-pixel PNG, so asset splice tests exercise a real (if tiny) image
// rather than an opaque byte slice.
var onePixelPNG = mustDecode(`iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==`)

func mustDecode(s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// wordDoc wraps body in a realistic WordprocessingML w:document/w:body root,
// mirroring the fixture shape xmlcopy's, template/anchor's and
// template/defrag's own tests already use.
func wordDoc(body string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<w:document xmlns:w="` + nsWordprocessing + `" xmlns:r="` + nsRelationships + `">` +
		`<w:body>` + body + `</w:body></w:document>`)
}

// buildPackage wraps documentXML into a minimal opc.Package whose main part
// is /word/document.xml, declared with the same content type
// template.Open's own detection checks. Splice's own tests call it directly
// (they exercise Splice against a *opc.Package, not through template.Open),
// which keeps them independent of template's detection logic.
func buildPackage(t *testing.T, documentXML []byte) *opc.Package {
	t.Helper()
	p := opc.New()
	if err := p.Put(&opc.Part{
		Name:        partDocument,
		ContentType: ctMainDocument,
		Data:        documentXML,
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	return p
}

// elementSpan returns the Span of the nth (0-indexed) element with the given
// local name, in the WordprocessingML namespace, found in src.
func elementSpan(t *testing.T, src []byte, local string, n int) xmlcopy.Span {
	t.Helper()
	var spans []xmlcopy.Span
	if err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if e.Name.Space == nsWordprocessing && e.Name.Local == local {
			spans = append(spans, e.Span)
		}
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if n >= len(spans) {
		t.Fatalf("%s %d not found; only %d found", local, n, len(spans))
	}
	return spans[n]
}

// mustApply applies replacements and fails the test on error, also proving
// the result still parses.
func mustApply(t *testing.T, src []byte, replacements []xmlcopy.Replacement) []byte {
	t.Helper()
	out, err := xmlcopy.Apply(src, replacements)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := xmlcopy.Walk(out, func(xmlcopy.Element) error { return nil }); err != nil {
		t.Fatalf("output does not parse: %v\n%s", err, out)
	}
	return out
}
