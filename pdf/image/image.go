// Package image embeds raster assets into a PDF as image XObjects.
//
// # Nothing is decoded that does not have to be
//
// PNG and JPEG are already compressed with schemes PDF's own filters describe,
// so their encoded bytes are handed to the file untouched: a JPEG becomes a
// DCTDecode stream, and an opaque PNG becomes a FlateDecode stream carrying the
// PNG predictor its scanlines are already filtered with. The asset a consumer
// supplied reaches the document pixel for pixel and byte for byte, and Vellum
// makes no decision about how their picture should be compressed.
//
// The price is that a variant no PDF filter describes cannot be carried. An
// interlaced PNG, a progressive JPEG and a CMYK JPEG are each a named rejection
// rather than a silent re-encode, because re-encoding would change the bytes a
// consumer chose without telling them, and every one of those answers is a
// capability matrix row before it is code here.
//
// The one exception is a PNG with an alpha channel. PNG interleaves alpha with
// colour on the same scanline; PDF keeps them in separate streams. So those two
// are separated — unfiltered, split, and recompressed — which rearranges the
// samples and changes not one of them.
package image

import (
	"crypto/sha256"

	"github.com/frankbardon/vellum/asset"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/object"
)

// Options describes one image to embed.
type Options struct {
	// Resource is the name the page's /XObject dictionary gives this image,
	// and the name a content stream draws it by.
	Resource object.Name

	// Handle is the asset handle, carried only so a failure names the asset the
	// consumer knows rather than the byte offset Vellum knows.
	Handle string

	// MediaType is the asset's media type, already established by
	// [asset.Ingest]. Parameters and case are normalised here.
	MediaType string

	// Bytes is the encoded image, verbatim.
	Bytes []byte
}

// XObject is a raster asset prepared for embedding.
//
// Built once and written once, however many pages draw it: the resource name is
// what a page refers to, and the bytes live in a single object.
type XObject struct {
	resource object.Name
	handle   string

	// base is the colour data. alpha is the soft mask, present only for an
	// image that carries a per-pixel alpha channel.
	base  raster
	alpha *raster

	// mask is a colour-key /Mask array: the sample ranges that are transparent.
	// It is how PNG's tRNS is expressed for greyscale and truecolour images,
	// where transparency is "this exact colour" rather than a channel, and it
	// costs nothing because the colour data still passes through untouched.
	mask object.Array
}

// raster is one plane of samples with everything needed to write it as a
// stream: colour data, or an alpha channel.
type raster struct {
	width, height int
	bits          int
	space         object.Object

	// filter names the encoding data is already in. Empty means the data is
	// raw samples and the document compresses it on the way out, which is the
	// only case where [object.Document.Uncompressed] has any effect on an
	// image.
	filter object.Name
	parms  object.Dict
	data   []byte
}

// New prepares an asset for embedding.
func New(opts Options) (*XObject, error) {
	if opts.Resource == "" {
		return nil, verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT,
			"an image XObject was built without a resource name")
	}

	switch asset.NormaliseMedia(opts.MediaType) {
	case asset.MediaPNG:
		return newPNG(opts)
	case asset.MediaJPEG:
		return newJPEG(opts)
	}

	// Reached only if a caller skipped the media policy: the matrix already
	// declares what this format accepts, and the lowering enforces it against
	// the accept list before an asset arrives here.
	return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_ASSET_MEDIA_UNSUPPORTED,
		"a PDF image can be built from PNG or JPEG only",
		map[string]any{
			"handle":     opts.Handle,
			"media_type": asset.NormaliseMedia(opts.MediaType),
			"accepted":   []string{asset.MediaJPEG, asset.MediaPNG},
		})
}

// Resource returns the name a content stream draws this image by.
func (x *XObject) Resource() object.Name { return x.resource }

// Handle returns the asset handle this image came from.
func (x *XObject) Handle() string { return x.handle }

// WidthPx and HeightPx are the image's intrinsic pixel dimensions. They are the
// aspect ratio a layout needs; they are not a size in points, because an image
// has no physical size until something places it.
func (x *XObject) WidthPx() int  { return x.base.width }
func (x *XObject) HeightPx() int { return x.base.height }

// HasAlpha reports whether the image carries a per-pixel soft mask.
func (x *XObject) HasAlpha() bool { return x.alpha != nil }

// Fingerprint is a content hash of everything this image contributes to a file.
//
// It exists so the document's file identifier can depend on its pictures. The
// identifier is derived from content rather than generated, and without this a
// document differing from another only in which image it draws would claim the
// same identity — sixteen bytes buried in the trailer, and everything else
// matching.
func (x *XObject) Fingerprint() []byte {
	h := sha256.New()
	h.Write([]byte(x.resource))
	h.Write([]byte{0})
	h.Write(x.base.data)
	if x.alpha != nil {
		h.Write([]byte{0})
		h.Write(x.alpha.data)
	}
	for _, m := range x.mask {
		h.Write([]byte{0})
		h.Write(m.AppendPDF(nil))
	}
	return h.Sum(nil)
}

// Write adds the image, and its soft mask when it has one, to a document.
func (x *XObject) Write(doc *object.Document) (object.Ref, error) {
	extra := object.Dict{}

	// The mask is written first so it carries the lower object number, which
	// keeps a soft mask next to the image it belongs to in a hex dump. Nothing
	// requires it; a person debugging a broken file appreciates it.
	if x.alpha != nil {
		ref, err := x.alpha.write(doc, object.Dict{})
		if err != nil {
			return object.Ref{}, err
		}
		extra.Set("SMask", ref)
	}
	if len(x.mask) > 0 {
		extra.Set("Mask", x.mask)
	}

	return x.base.write(doc, extra)
}

// write emits one plane as an image XObject.
func (r raster) write(doc *object.Document, extra object.Dict) (object.Ref, error) {
	d := object.NewDict(
		"Type", object.Name("XObject"),
		"Subtype", object.Name("Image"),
		"Width", object.Int(r.width),
		"Height", object.Int(r.height),
		"ColorSpace", r.space,
		"BitsPerComponent", object.Int(r.bits),
	)
	for _, k := range extra.Keys() {
		v, _ := extra.Get(k)
		d.Set(k, v)
	}

	if r.filter == "" {
		return doc.AddStream(d, r.data)
	}
	d.Set("Filter", r.filter)
	if r.parms.Len() > 0 {
		d.Set("DecodeParms", r.parms)
	}
	// Already in the encoding the filter names, so it goes in verbatim. Passing
	// it through AddStream would compress a JPEG a second time, which costs
	// time and makes the file larger.
	return doc.AddRawStream(d, r.data), nil
}

// unsupported reports an image variant PDF cannot carry unmodified.
func unsupported(handle, variant, why string) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_PDF_IMAGE_UNSUPPORTED,
		"the image is in an encoding variant a PDF cannot carry unmodified",
		map[string]any{"handle": handle, "variant": variant, "reason": why})
}

// invalid reports bytes that do not parse as the format they were typed as.
func invalid(handle, why string) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_PDF_IMAGE_INVALID,
		"the image bytes do not parse",
		map[string]any{"handle": handle, "reason": why})
}
