package sheet

import "strings"

// The XML declaration Excel itself writes, including the CRLF. Matching it
// keeps a Vellum-produced part byte-comparable with an authored one.
const xmlDecl = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"

// Namespace URIs.
const (
	nsSpreadsheet   = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	nsRelationships = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsVML           = "urn:schemas-microsoft-com:vml"
	nsExcelVML      = "urn:schemas-microsoft-com:office:excel"

	nsCoreProps     = "http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
	nsExtendedProps = "http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"
	nsCustomProps   = "http://schemas.openxmlformats.org/officeDocument/2006/custom-properties"
	nsDocPropsVT    = "http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"
	nsDublinCore    = "http://purl.org/dc/elements/1.1/"
	nsDublinTerms   = "http://purl.org/dc/terms/"
	nsXSI           = "http://www.w3.org/2001/XMLSchema-instance"
)

// Relationship type URIs.
const (
	relOfficeDocument = nsRelationships + "/officeDocument"
	relCoreProperties = "http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties"
	relExtendedProps  = nsRelationships + "/extended-properties"
	relCustomProps    = nsRelationships + "/custom-properties"
	relStyles         = nsRelationships + "/styles"
	relSharedStrings  = nsRelationships + "/sharedStrings"
	relWorksheet      = nsRelationships + "/worksheet"
	relComments       = nsRelationships + "/comments"
	relVMLDrawing     = nsRelationships + "/vmlDrawing"
)

// Content types.
const (
	ctWorkbook       = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"
	ctWorksheet      = "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"
	ctStyles         = "application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"
	ctSharedStrings  = "application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"
	ctComments       = "application/vnd.openxmlformats-officedocument.spreadsheetml.comments+xml"
	ctCoreProperties = "application/vnd.openxmlformats-package.core-properties+xml"
	ctExtendedProps  = "application/vnd.openxmlformats-officedocument.extended-properties+xml"
	ctCustomProps    = "application/vnd.openxmlformats-officedocument.custom-properties+xml"
	ctXML            = "application/xml"
	ctVML            = "application/vnd.openxmlformats-officedocument.vmlDrawing"
)

// escapeText escapes character data for an XML text node.
//
// Hand-rolled rather than via xml.EscapeText for the reason [doc] and [deck]'s
// escapers are: that function also escapes tabs, newlines and carriage returns
// as numeric character references, which is correct but noisier than what
// Excel itself writes, and these bytes are part of a determinism contract
// worth controlling directly.
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
