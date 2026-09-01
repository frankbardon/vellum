package deck

import "strings"

// The XML declaration PowerPoint itself writes, including the CRLF. Matching it
// keeps a Vellum-produced part byte-comparable with an authored one.
const xmlDecl = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"

// Namespace URIs, declared as constants rather than inlined so a typo is a
// compile-time concern in one place rather than a deck that opens as gibberish.
const (
	nsPresentation  = "http://schemas.openxmlformats.org/presentationml/2006/main"
	nsDrawingMain   = "http://schemas.openxmlformats.org/drawingml/2006/main"
	nsRelationships = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsSVG           = "http://schemas.microsoft.com/office/drawing/2016/SVG/main"
	nsMarkupCompat  = "http://schemas.openxmlformats.org/markup-compatibility/2006"

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
	relSlideMaster    = nsRelationships + "/slideMaster"
	relSlideLayout    = nsRelationships + "/slideLayout"
	relSlide          = nsRelationships + "/slide"
	relNotesSlide     = nsRelationships + "/notesSlide"
	relNotesMaster    = nsRelationships + "/notesMaster"
	relTheme          = nsRelationships + "/theme"
	relPresProps      = nsRelationships + "/presProps"
	relViewProps      = nsRelationships + "/viewProps"
	relTableStyles    = nsRelationships + "/tableStyles"
	relImage          = nsRelationships + "/image"
)

// Content types.
const (
	ctPresentation   = "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
	ctSlide          = "application/vnd.openxmlformats-officedocument.presentationml.slide+xml"
	ctSlideMaster    = "application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"
	ctSlideLayout    = "application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"
	ctNotesSlide     = "application/vnd.openxmlformats-officedocument.presentationml.notesSlide+xml"
	ctNotesMaster    = "application/vnd.openxmlformats-officedocument.presentationml.notesMaster+xml"
	ctPresProps      = "application/vnd.openxmlformats-officedocument.presentationml.presProps+xml"
	ctViewProps      = "application/vnd.openxmlformats-officedocument.presentationml.viewProps+xml"
	ctTableStyles    = "application/vnd.openxmlformats-officedocument.presentationml.tableStyles+xml"
	ctTheme          = "application/vnd.openxmlformats-officedocument.theme+xml"
	ctCoreProperties = "application/vnd.openxmlformats-package.core-properties+xml"
	ctExtendedProps  = "application/vnd.openxmlformats-officedocument.extended-properties+xml"
	ctCustomProps    = "application/vnd.openxmlformats-officedocument.custom-properties+xml"
	ctXML            = "application/xml"

	ctPNG  = "image/png"
	ctJPEG = "image/jpeg"
	ctSVG  = "image/svg+xml"
)

// escapeText escapes character data for an XML text node.
//
// Hand-rolled rather than via xml.EscapeText because that function also escapes
// tabs, newlines and carriage returns as numeric character references. That is
// correct but noisier than PowerPoint's own output, and these bytes are part of
// a determinism contract worth controlling directly.
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

// mediaExtension returns the file extension for an embedded image.
func mediaExtension(mediaType string) string {
	switch mediaType {
	case ctPNG:
		return "png"
	case ctJPEG:
		return "jpeg"
	case ctSVG:
		return "svg"
	}
	return "bin"
}
