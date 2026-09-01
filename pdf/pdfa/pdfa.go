// Package pdfa builds the structures ISO 19005-2 requires beyond a plain PDF.
//
// Level B conformance — "basic" — guarantees that the visual appearance of the
// document is reproducible in the long term. It requires every font to be
// embedded, colour to have a defined interpretation, and the file's metadata to
// be stated in XMP and to agree with the information dictionary. It does not
// require the document to be tagged or its text to be reliably extractable;
// those are levels A and U.
package pdfa

import (
	"github.com/frankbardon/vellum/pdf/color"
	"github.com/frankbardon/vellum/pdf/object"
)

// OutputIntentSubtype is the intent subtype PDF/A defines.
const OutputIntentSubtype = "GTS_PDFA1"

// AddSRGBOutputIntent embeds the sRGB profile and returns the output intent
// dictionary naming it.
//
// PDF/A requires exactly one output intent with this subtype, and requires its
// destination profile to be embedded rather than referenced by name. The
// requirement is the whole point of the standard: a DeviceRGB fill means
// nothing without a stated output condition, so a file without one cannot
// promise its appearance is reproducible.
//
// The profile stream is stored uncompressed. A validator has to read it, and a
// few will not look inside a filter to do so.
func AddSRGBOutputIntent(doc *object.Document) object.Dict {
	profile := color.SRGBProfile()

	ref := doc.AddRawStream(object.NewDict(
		"N", object.Int(color.NumComponents),
	), profile)

	return object.NewDict(
		"Type", object.Name("OutputIntent"),
		"S", object.Name(OutputIntentSubtype),
		"OutputConditionIdentifier", object.String("sRGB"),
		"Info", object.String("sRGB IEC61966-2.1"),
		"RegistryName", object.String("http://www.color.org"),
		"DestOutputProfile", ref,
	)
}
