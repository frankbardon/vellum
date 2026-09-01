package asset

import (
	"bytes"
	"encoding/binary"
	"regexp"
	"strconv"
	"strings"
)

// SniffMedia identifies an asset from its leading bytes.
//
// Signature-based, never extension-based. The handle is the host's opaque
// identifier and may not be a filename at all, so a media type derived from it
// would be a guess about the host's naming convention rather than about the
// content — and a wrong guess here produces a document that a reader refuses to
// open, several layers away from the mistake.
//
// An unrecognised asset is [verr.VELLUM_ASSET_MEDIA_UNKNOWN] at the call site
// rather than a default. Defaulting would mean writing bytes into a package
// under a content type they do not match.
func SniffMedia(b []byte) (string, bool) {
	switch {
	case len(b) >= 8 && bytes.Equal(b[:8], pngSignature):
		return MediaPNG, true
	case len(b) >= 3 && bytes.Equal(b[:3], jpegSignature):
		return MediaJPEG, true
	case looksLikeSVG(b):
		return MediaSVG, true
	}
	return "", false
}

var (
	pngSignature  = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}
	jpegSignature = []byte{0xFF, 0xD8, 0xFF}
	utf8BOM       = []byte{0xEF, 0xBB, 0xBF}
)

// svgSniffLimit bounds how far into a document the SVG check looks.
//
// XML permits a declaration, a doctype and comments before the root element, so
// the root tag is not necessarily first — but it is not arbitrarily far in
// either, and scanning a whole file to answer "is this SVG" would let a large
// non-SVG asset pay for the question.
const svgSniffLimit = 4096

func looksLikeSVG(b []byte) bool {
	head := b
	if len(head) > svgSniffLimit {
		head = head[:svgSniffLimit]
	}
	head = bytes.TrimPrefix(head, utf8BOM)
	trimmed := bytes.TrimLeft(head, " \t\r\n")
	if len(trimmed) == 0 || trimmed[0] != '<' {
		return false
	}
	// The root element, not merely the string "svg" somewhere in the prologue:
	// a comment mentioning SVG in an unrelated XML document must not match.
	return svgRoot.Match(head)
}

// svgRoot matches an <svg> start tag that is not inside a comment. The check is
// deliberately shallow — this is a sniffer, not a parser.
var svgRoot = regexp.MustCompile(`(?is)<svg[\s>]`)

// Measure returns an asset's intrinsic pixel dimensions.
//
// The aspect ratio is what a box with an intrinsic height needs in order to
// have one, and the asset is the only place it can come from. An asset that
// does not declare its size is not an error: the boolean says so, and a caller
// that needed the ratio raises its own failure with its own context.
func Measure(mediaType string, b []byte) (width, height float64, ok bool) {
	switch NormaliseMedia(mediaType) {
	case MediaPNG:
		return measurePNG(b)
	case MediaJPEG:
		return measureJPEG(b)
	case MediaSVG:
		return measureSVG(b)
	}
	return 0, 0, false
}

// measurePNG reads IHDR, which the specification requires to be the first
// chunk, so no scan is needed.
func measurePNG(b []byte) (float64, float64, bool) {
	// 8 signature + 4 length + 4 type + 8 dimensions.
	const ihdrEnd = 8 + 4 + 4 + 8
	if len(b) < ihdrEnd || !bytes.Equal(b[:8], pngSignature) {
		return 0, 0, false
	}
	if !bytes.Equal(b[12:16], []byte("IHDR")) {
		return 0, 0, false
	}
	w := binary.BigEndian.Uint32(b[16:20])
	h := binary.BigEndian.Uint32(b[20:24])
	if w == 0 || h == 0 {
		return 0, 0, false
	}
	return float64(w), float64(h), true
}

// measureJPEG walks the marker segments to the first start-of-frame.
//
// JPEG has no fixed header carrying the dimensions, so the segments must be
// walked. Every SOF variant is accepted except the four that are not frame
// headers at all — DHT, JPG, DAC and the restart markers — because a
// progressive or arithmetic-coded file is still a file with a size.
func measureJPEG(b []byte) (float64, float64, bool) {
	if len(b) < 4 || !bytes.Equal(b[:3], jpegSignature) {
		return 0, 0, false
	}
	i := 2
	for i+3 < len(b) {
		if b[i] != 0xFF {
			// Fill bytes are legal between segments; anything else means the
			// stream is not one this walk understands, and guessing past it
			// would be inventing an answer.
			i++
			continue
		}
		marker := b[i+1]
		i += 2
		switch {
		case marker == 0xFF:
			// Padding; the next byte is the real marker.
			i--
			continue
		case marker == 0x01 || (marker >= 0xD0 && marker <= 0xD9):
			// Standalone markers carry no length.
			continue
		}
		if i+1 >= len(b) {
			return 0, 0, false
		}
		length := int(binary.BigEndian.Uint16(b[i : i+2]))
		if length < 2 || i+length > len(b) {
			return 0, 0, false
		}
		if isSOF(marker) {
			// length(2) precision(1) height(2) width(2)
			if i+7 > len(b) {
				return 0, 0, false
			}
			h := binary.BigEndian.Uint16(b[i+3 : i+5])
			w := binary.BigEndian.Uint16(b[i+5 : i+7])
			if w == 0 || h == 0 {
				return 0, 0, false
			}
			return float64(w), float64(h), true
		}
		i += length
	}
	return 0, 0, false
}

// isSOF reports whether a marker begins a frame header.
func isSOF(m byte) bool {
	if m < 0xC0 || m > 0xCF {
		return false
	}
	// C4 is DHT, C8 is JPG, CC is DAC — all in the range, none a frame header.
	return m != 0xC4 && m != 0xC8 && m != 0xCC
}

var (
	svgWidthAttr   = regexp.MustCompile(`(?is)<svg[^>]*?\swidth\s*=\s*["']([^"']+)["']`)
	svgHeightAttr  = regexp.MustCompile(`(?is)<svg[^>]*?\sheight\s*=\s*["']([^"']+)["']`)
	svgViewBoxAttr = regexp.MustCompile(`(?is)<svg[^>]*?\sviewBox\s*=\s*["']([^"']+)["']`)
)

// measureSVG prefers explicit width and height, and falls back to the viewBox.
//
// The fallback is the case that matters. A renderer whose default output is
// viewBox-only produces exactly that shape, and it is why the layout query
// exists: without a target box, such an asset has a ratio and no size, so the
// host must be told what size to render at rather than left to scale whatever
// arrives.
func measureSVG(b []byte) (float64, float64, bool) {
	head := b
	if len(head) > svgSniffLimit {
		head = head[:svgSniffLimit]
	}

	w, wOK := svgLength(svgWidthAttr, head)
	h, hOK := svgLength(svgHeightAttr, head)
	if wOK && hOK {
		return w, h, true
	}

	if m := svgViewBoxAttr.FindSubmatch(head); m != nil {
		fields := strings.FieldsFunc(string(m[1]), func(r rune) bool {
			return r == ' ' || r == ',' || r == '\t' || r == '\n' || r == '\r'
		})
		if len(fields) == 4 {
			vw, errW := strconv.ParseFloat(fields[2], 64)
			vh, errH := strconv.ParseFloat(fields[3], 64)
			if errW == nil && errH == nil && vw > 0 && vh > 0 {
				return vw, vh, true
			}
		}
	}
	return 0, 0, false
}

// svgLength parses a CSS length, accepting a unit suffix and ignoring it.
//
// Ignoring the unit is honest rather than lazy: these dimensions are used for
// an aspect ratio, and a ratio is unit-free as long as both dimensions carry
// the same unit — which, in an SVG root element, they do. A percentage is
// refused, because a percentage of an unstated container is not a size.
func svgLength(re *regexp.Regexp, b []byte) (float64, bool) {
	m := re.FindSubmatch(b)
	if m == nil {
		return 0, false
	}
	s := strings.TrimSpace(string(m[1]))
	if strings.HasSuffix(s, "%") {
		return 0, false
	}
	s = strings.TrimRight(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}
