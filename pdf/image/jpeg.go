package image

import (
	"encoding/binary"

	"github.com/frankbardon/vellum/pdf/object"
)

// newJPEG prepares a JPEG for embedding.
//
// The whole file becomes the stream. PDF's DCTDecode filter is JPEG, so nothing
// is decoded, nothing is recompressed, and the image in the document is the
// image on disk down to the byte. What this function does is establish that the
// file is a variant DCTDecode actually describes, because the alternative is a
// document that opens in the reader it was tested against and shows a blank
// frame in the next one.
func newJPEG(opts Options) (*XObject, error) {
	f, err := scanJPEG(opts.Handle, opts.Bytes)
	if err != nil {
		return nil, err
	}

	switch f.marker {
	case 0xC0, 0xC1:
		// Baseline and extended sequential: what DCTDecode is defined over.
	case 0xC2:
		return nil, unsupported(opts.Handle, "progressive JPEG",
			"DCTDecode is defined over baseline and extended sequential JPEG. Progressive is legal "+
				"JPEG that a conforming PDF reader is not obliged to decode, so embedding it produces a "+
				"file whose behaviour depends on the reader. Save it as baseline.")
	default:
		return nil, unsupported(opts.Handle, "arithmetic, lossless or hierarchical JPEG",
			"DCTDecode is defined over Huffman-coded baseline and extended sequential JPEG only. Save it as baseline.")
	}

	if f.precision != 8 {
		return nil, unsupported(opts.Handle, "high-precision JPEG",
			"PDF readers are obliged to decode eight-bit samples. Twelve-bit support is optional and "+
				"unevenly implemented, so a file relying on it is one whose appearance depends on the reader.")
	}

	var space object.Name
	switch f.components {
	case 1:
		space = "DeviceGray"
	case 3:
		space = "DeviceRGB"
	case 4:
		return nil, unsupported(opts.Handle, "CMYK JPEG",
			"a DeviceCMYK image needs a CMYK output intent for the file to stay PDF/A conforming, and "+
				"Vellum ships one intent: sRGB. Converting CMYK to RGB is a colour management decision with "+
				"no single right answer, and making it silently would change the colours you chose. Supply RGB or greyscale.")
	default:
		return nil, invalid(opts.Handle, "the frame header declares a component count JPEG does not define")
	}

	return &XObject{
		resource: opts.Resource,
		handle:   opts.Handle,
		base: raster{
			width: f.width, height: f.height, bits: f.precision,
			space: space, filter: "DCTDecode", data: opts.Bytes,
		},
	}, nil
}

// frame is what a JPEG's start-of-frame segment says about the image.
type frame struct {
	marker        byte
	precision     int
	width, height int
	components    int
}

// scanJPEG walks the marker segments to the first frame header.
//
// JPEG has no fixed header carrying any of this, so the segments must be
// walked. The scan stops at the first start-of-frame: a hierarchical file has
// several, and this refuses that marker anyway.
func scanJPEG(handle string, b []byte) (frame, error) {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return frame{}, invalid(handle, "the file does not begin with a start-of-image marker")
	}

	for i := 2; i+1 < len(b); {
		if b[i] != 0xFF {
			// Fill bytes are legal between segments; anything else means the
			// stream is not one this walk understands, and scanning past it
			// would be guessing.
			i++
			continue
		}
		marker := b[i+1]
		i += 2

		switch {
		case marker == 0xFF:
			// Padding before the real marker.
			i--
			continue
		case marker == 0x01 || (marker >= 0xD0 && marker <= 0xD9):
			// Standalone markers carry no length.
			continue
		}

		if i+2 > len(b) {
			return frame{}, invalid(handle, "a segment header runs past the end of the file")
		}
		length := int(binary.BigEndian.Uint16(b[i : i+2]))
		if length < 2 || i+length > len(b) {
			return frame{}, invalid(handle, "a segment length runs past the end of the file")
		}

		if isFrameMarker(marker) {
			if length < 8 {
				return frame{}, invalid(handle, "the frame header is shorter than JPEG defines")
			}
			f := frame{
				marker:     marker,
				precision:  int(b[i+2]),
				height:     int(binary.BigEndian.Uint16(b[i+3 : i+5])),
				width:      int(binary.BigEndian.Uint16(b[i+5 : i+7])),
				components: int(b[i+7]),
			}
			if f.width == 0 || f.height == 0 {
				return frame{}, invalid(handle, "the frame header declares a zero dimension")
			}
			return f, nil
		}
		i += length
	}
	return frame{}, invalid(handle, "the file carries no frame header")
}

// isFrameMarker reports whether a marker begins a frame.
//
// The SOF range is 0xC0 to 0xCF with three holes: DHT, JPG and DAC sit inside
// it and are not frame headers. Every remaining marker in the range is
// accepted here and sorted out by newJPEG, so an unembeddable variant gets a
// message naming what it is rather than "no frame header found".
func isFrameMarker(m byte) bool {
	if m < 0xC0 || m > 0xCF {
		return false
	}
	return m != 0xC4 && m != 0xC8 && m != 0xCC
}
