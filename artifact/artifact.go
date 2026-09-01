// Package artifact names the output formats Vellum emits and the contract a
// writer satisfies.
//
// It sits at the bottom of the import graph so that anything needing to speak
// about a format — the capability matrix, the manifest, the CLI — can do so
// without pulling in a writer.
package artifact

import "strings"

// Format is an output format.
type Format string

const (
	// FormatDOCX is WordprocessingML: a flowing document.
	FormatDOCX Format = "docx"

	// FormatXLSX is SpreadsheetML: a workbook of presentation tables. Not a
	// spreadsheet — Vellum emits no formulas and no pivot tables.
	FormatXLSX Format = "xlsx"

	// FormatPPTX is PresentationML: a slide deck.
	FormatPPTX Format = "pptx"

	// FormatPDF is PDF/A-2b, emitted directly rather than converted. A
	// conversion's output varies with the renderer version and the fonts
	// installed on the converting machine, which would defeat byte-identical
	// output and the consumer dedupe that rests on it.
	FormatPDF Format = "pdf"
)

// allFormats is the registry, in declaration order.
var allFormats = []Format{FormatDOCX, FormatXLSX, FormatPPTX, FormatPDF}

// AllFormats returns a copy of the format registry.
func AllFormats() []Format {
	out := make([]Format, len(allFormats))
	copy(out, allFormats)
	return out
}

// ValidFormat reports whether f is a known format.
func ValidFormat(f Format) bool {
	for _, v := range allFormats {
		if v == f {
			return true
		}
	}
	return false
}

// ParseFormat resolves a string to a Format, accepting a leading dot and any
// letter case, because both turn up in a filename and in a flag.
func ParseFormat(s string) (Format, bool) {
	normalised := Format(strings.ToLower(strings.TrimPrefix(strings.TrimSpace(s), ".")))
	if ValidFormat(normalised) {
		return normalised, true
	}
	return "", false
}

// Extension returns the file extension for the format, without the dot.
func (f Format) Extension() string { return string(f) }

// IsOOXML reports whether the format is an OPC package rather than a
// standalone file. The three OOXML formats share the entire packaging
// substrate; PDF shares none of it.
func (f Format) IsOOXML() bool {
	switch f {
	case FormatDOCX, FormatXLSX, FormatPPTX:
		return true
	default:
		return false
	}
}
