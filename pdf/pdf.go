// Package pdf writes PDF/A-2b documents.
//
// Vellum owns every layer of this: the object writer, the font subsetter, the
// colour profile and the conformance metadata. That is not a preference. The
// one Go library that solves PDF generation well is GPL-3.0 and unavailable to
// a permissively licensed project, and the permissive alternative with a real
// subsetter writes a wall-clock timestamp into the font bytes — inside the
// stream Vellum is required to pin.
//
// # Determinism
//
// Object numbers follow the order objects are added, which follows the order
// pages and fonts are declared. Dates come from the caller rather than the
// clock, and the trailer's file identifier is derived from the document's
// content rather than generated, so nothing in the output varies between two
// runs over the same input.
//
// # Conformance
//
// The output claims PDF/A-2b, and that claim is checked: TestPDFAConformance
// runs veraPDF over every PDF golden. A file asserting conformance and failing
// validation is worse than one making no claim at all, which is why the gate
// exists rather than a comment saying the output should conform.
package pdf

import (
	"crypto/sha256"
	"io"
	"time"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/object"
	"github.com/frankbardon/vellum/pdf/pdfa"
	"github.com/frankbardon/vellum/pdf/xmp"
)

// Page is one page of a document.
type Page struct {
	// Width and Height are the media box, in points.
	Width, Height object.Real

	// Content is the page's content stream, as built by [content.Builder].
	Content []byte

	// Fonts are the fonts this page's content stream selects, by resource name.
	// A font shared between pages is embedded once.
	Fonts []*Font
}

// Document is a PDF/A-2b document ready to be written.
type Document struct {
	// Metadata is written twice — to the information dictionary and to the XMP
	// packet — from one value, because PDF/A requires the two to agree and a
	// validator compares them. See [xmp.Metadata].
	Metadata xmp.Metadata

	// Pages are the document's pages, in order.
	Pages []Page
}

// WriteOptions configures a write. The zero value is the deterministic default.
type WriteOptions struct {
	// SourceDateEpoch is the timestamp every date in the document takes: the
	// information dictionary's, the XMP packet's, and any the metadata did not
	// set for itself. The zero value selects the pinned epoch.
	//
	// ISO 19005 requires those dates to agree with each other, so they are all
	// derived from this one value rather than gathered separately and
	// reconciled. veraPDF's 2b profile was observed not to check the agreement,
	// which is a reason to keep the invariant structural rather than to lean on
	// the validator for it.
	SourceDateEpoch time.Time

	// Uncompressed stores every stream verbatim. See
	// [object.Document.Uncompressed].
	Uncompressed bool

	// PageTree configures the page tree's shape. The zero value balances at
	// [object.DefaultBranching] and lifts the attributes every page shares onto
	// the root.
	PageTree object.PageTreeOptions
}

// WriteTo emits the document.
func (d *Document) WriteTo(w io.Writer, opts WriteOptions) error {
	if len(d.Pages) == 0 {
		return verr.NewCodedError(verr.VELLUM_PDF_OBJECT_UNRESOLVED,
			"the document has no pages")
	}

	meta := d.Metadata
	if meta.Date.IsZero() {
		meta.Date = opts.SourceDateEpoch
	}
	if meta.Date.IsZero() {
		meta.Date = PinnedEpoch
	}

	var doc object.Document
	doc.Uncompressed = opts.Uncompressed

	// Fonts first, so a face shared by several pages is embedded once and
	// carries a lower object number than the pages referring to it. Neither is
	// required; both make the file readable in a hex dump.
	fontRefs, err := d.writeFonts(&doc)
	if err != nil {
		return err
	}

	dicts := make([]object.Dict, len(d.Pages))
	for i := range d.Pages {
		dicts[i], err = d.pageDict(&doc, &d.Pages[i], fontRefs)
		if err != nil {
			return err
		}
	}

	pagesRef, err := object.BuildPageTree(&doc, dicts, opts.PageTree)
	if err != nil {
		return err
	}

	metadata := doc.AddRawStream(object.NewDict(
		"Type", object.Name("Metadata"),
		"Subtype", object.Name("XML"),
	), meta.Packet())

	intent := pdfa.AddSRGBOutputIntent(&doc)

	doc.Root = doc.Add(object.NewDict(
		"Type", object.Name("Catalog"),
		"Pages", pagesRef,
		"Metadata", metadata,
		"OutputIntents", object.Array{intent},
	))
	doc.Info = doc.Add(meta.InfoEntries())
	doc.ID = d.fileID(meta)

	return doc.Write(w)
}

// writeFonts embeds each distinct font once and returns its reference.
//
// Identity is the Font pointer, so a caller sharing one value across pages gets
// one embedding and a caller building two equivalent values gets two. That is
// the honest reading of what was asked for: two Font values with the same face
// and different glyph sets are two different subsets.
func (d *Document) writeFonts(doc *object.Document) (map[*Font]object.Ref, error) {
	refs := make(map[*Font]object.Ref)
	for i := range d.Pages {
		for _, f := range d.Pages[i].Fonts {
			if _, seen := refs[f]; seen {
				continue
			}
			ref, err := f.write(doc)
			if err != nil {
				return nil, err
			}
			refs[f] = ref
		}
	}
	return refs, nil
}

// pageDict builds one page's dictionary and its content stream.
//
// It sets neither /Type nor /Parent: both belong to the page tree, which knows
// the parent and would otherwise be trusting this function to have agreed with
// it. The media box is set here and may be lifted onto the tree's root if every
// page shares it.
func (d *Document) pageDict(doc *object.Document, p *Page, fontRefs map[*Font]object.Ref) (object.Dict, error) {
	contents, err := doc.AddStream(object.Dict{}, p.Content)
	if err != nil {
		return object.Dict{}, err
	}

	// Built by walking the page's own font slice rather than by ranging the
	// reference map, so the resource dictionary's key order is the order the
	// caller declared and not the order a Go map happened to iterate.
	var fonts object.Dict
	for _, f := range p.Fonts {
		ref, ok := fontRefs[f]
		if !ok {
			return object.Dict{}, verr.NewCodedErrorWithDetails(verr.VELLUM_PDF_OBJECT_UNRESOLVED,
				"a page names a font that was not embedded",
				map[string]any{"resource": string(f.Resource)})
		}
		fonts.Set(f.Resource, ref)
	}

	resources := object.NewDict("ProcSet", object.Array{object.Name("PDF"), object.Name("Text")})
	resources.SetIf(fonts.Len() > 0, "Font", fonts)

	return object.NewDict(
		"MediaBox", object.Array{object.Int(0), object.Int(0), p.Width, p.Height},
		"Resources", resources,
		"Contents", contents,
	), nil
}

// fileID derives the trailer's file identifier from the document's content.
//
// PDF/A requires the identifier. The specification suggests deriving it from
// the file's contents and the time of writing; the time is exactly the part
// that must not be here, so it is derived from the content alone.
//
// Both halves are equal, which is what the specification prescribes for a file
// that is not an update of an earlier one. Vellum never writes an update.
func (d *Document) fileID(meta xmp.Metadata) [2][]byte {
	h := sha256.New()
	h.Write([]byte(meta.Title))
	h.Write([]byte{0})
	h.Write([]byte(meta.Producer))
	h.Write([]byte{0})
	h.Write([]byte(meta.Date.UTC().Format("20060102150405")))
	for i := range d.Pages {
		h.Write([]byte{0})
		h.Write(d.Pages[i].Content)
	}

	sum := h.Sum(nil)[:16]
	return [2][]byte{sum, sum}
}
