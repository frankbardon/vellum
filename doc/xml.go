package doc

import "strings"

// The XML declaration Word itself writes, including the CRLF. Matching it
// keeps a Vellum-produced part byte-comparable with an authored one.
const xmlDecl = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"

// Namespace URIs. Declared as constants rather than inlined so a typo is a
// compile-time concern in one place rather than a document that opens as
// gibberish.
const (
	nsWordprocessing = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	nsCoreProps      = "http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
	nsExtendedProps  = "http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"
	nsDublinCore     = "http://purl.org/dc/elements/1.1/"
	nsDublinTerms    = "http://purl.org/dc/terms/"
	nsXSI            = "http://www.w3.org/2001/XMLSchema-instance"
)

// Relationship type URIs.
const (
	relOfficeDocument   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	relCoreProperties   = "http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties"
	relExtendedProps    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties"
	ctMainDocument      = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	ctCoreProperties    = "application/vnd.openxmlformats-package.core-properties+xml"
	ctExtendedProps     = "application/vnd.openxmlformats-officedocument.extended-properties+xml"
	ctXML               = "application/xml"
	defaultExtensionXML = "xml"
)

// escapeText escapes character data for an XML text node.
//
// Hand-rolled rather than via xml.EscapeText because that function also
// escapes tabs, newlines and carriage returns as numeric character
// references. That is correct but noisier than Word's own output, and these
// bytes are part of a determinism contract worth controlling directly.
func escapeText(s string) string {
	if !strings.ContainsAny(s, "&<>") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// escapeAttr escapes a string for an XML attribute value.
func escapeAttr(s string) string {
	if !strings.ContainsAny(s, `&<>"'`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
