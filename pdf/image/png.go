package image

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/object"
)

// PNG structural constants.
const (
	pngHeaderLen = 8
	ihdrLen      = 13

	// maxDimension is PNG's own limit: the specification gives width and height
	// four bytes each and restricts both to 1 through 2^31-1. Checked rather
	// than tolerated, because these two numbers are multiplied together and
	// then by a bit depth, and the product of two unchecked 32-bit values does
	// not fit anywhere.
	maxDimension = 1<<31 - 1

	colorGray      = 0
	colorRGB       = 2
	colorIndexed   = 3
	colorGrayAlpha = 4
	colorRGBA      = 6
)

// newPNG prepares a PNG for embedding.
//
// The opaque forms pass through: PNG compresses its scanlines with zlib after
// filtering them with a per-row predictor, and PDF's FlateDecode with
// /Predictor 15 describes exactly that. The IDAT bytes become the stream, and
// the image in the document is the image on disk.
func newPNG(opts Options) (*XObject, error) {
	c, err := readPNG(opts.Handle, opts.Bytes)
	if err != nil {
		return nil, err
	}

	channels, err := c.channels(opts.Handle)
	if err != nil {
		return nil, err
	}

	x := &XObject{resource: opts.Resource, handle: opts.Handle}

	switch c.colorType {
	case colorGray, colorRGB, colorIndexed:
		space, err := c.colorSpace(opts.Handle)
		if err != nil {
			return nil, err
		}
		x.base = raster{
			width: c.width, height: c.height, bits: c.bitDepth, space: space,
			filter: "FlateDecode",
			parms: object.NewDict(
				"Predictor", object.Int(15),
				"Colors", object.Int(channels),
				"BitsPerComponent", object.Int(c.bitDepth),
				"Columns", object.Int(c.width),
			),
			data: c.idat,
		}

		switch {
		case c.colorType == colorIndexed && len(c.trns) > 0:
			// Per-entry palette alpha. The colour data still passes through —
			// the indices are unchanged — and only the mask is built, by
			// reading the indices out and looking each one up.
			alpha, err := c.paletteAlpha(opts, channels)
			if err != nil {
				return nil, err
			}
			x.alpha = alpha
		case len(c.trns) > 0:
			mask, err := c.colorKeyMask(opts.Handle)
			if err != nil {
				return nil, err
			}
			x.mask = mask
		}

	case colorGrayAlpha, colorRGBA:
		if err := c.split(opts, channels, x); err != nil {
			return nil, err
		}

	default:
		return nil, invalid(opts.Handle, "unknown PNG colour type")
	}

	return x, nil
}

// pngChunks is what the PNG reader keeps: the header fields, the palette, the
// transparency chunk and the concatenated image data.
//
// Every other chunk is discarded. Textual metadata, gamma, chromaticity and
// physical dimensions all have PDF equivalents that are not equivalent enough
// to translate honestly, and carrying them over would be asserting a colour
// management decision Vellum did not make.
type pngChunks struct {
	width, height          int
	bitDepth, colorType    int
	plte, trns, idat       []byte
	interlace, compression int
	filterMethod           int
}

func readPNG(handle string, b []byte) (*pngChunks, error) {
	if len(b) < pngHeaderLen+12+ihdrLen {
		return nil, invalid(handle, "the file is shorter than a PNG signature and header")
	}

	c := &pngChunks{}
	seenIHDR := false
	// CRCs are not verified. The sniffer has already matched the signature, the
	// zlib stream carries its own Adler-32, and a reader that rejects the file
	// would reject it for the same reason we would — so checking here would buy
	// a second opinion on a question already answered, at the cost of a pass
	// over every byte of every asset.
	for i := pngHeaderLen; i+8 <= len(b); {
		length := int(binary.BigEndian.Uint32(b[i : i+4]))
		if length < 0 || i+12+length > len(b) {
			return nil, invalid(handle, "a chunk length runs past the end of the file")
		}
		typ := string(b[i+4 : i+8])
		data := b[i+8 : i+8+length]
		i += 12 + length

		switch typ {
		case "IHDR":
			if length != ihdrLen {
				return nil, invalid(handle, "IHDR is not thirteen bytes")
			}
			c.width = int(binary.BigEndian.Uint32(data[0:4]))
			c.height = int(binary.BigEndian.Uint32(data[4:8]))
			c.bitDepth = int(data[8])
			c.colorType = int(data[9])
			c.compression = int(data[10])
			c.filterMethod = int(data[11])
			c.interlace = int(data[12])
			seenIHDR = true
		case "PLTE":
			c.plte = data
		case "tRNS":
			c.trns = data
		case "IDAT":
			c.idat = append(c.idat, data...)
		case "IEND":
			i = len(b)
		}
	}

	switch {
	case !seenIHDR:
		return nil, invalid(handle, "the file carries no IHDR chunk")
	case c.width <= 0 || c.height <= 0 || c.width > maxDimension || c.height > maxDimension:
		return nil, invalid(handle, "the image's dimensions are outside the range PNG defines")
	case len(c.idat) == 0:
		return nil, invalid(handle, "the file carries no IDAT chunk")
	case c.compression != 0:
		return nil, invalid(handle, "the compression method is not the one PNG defines")
	case c.filterMethod != 0:
		return nil, invalid(handle, "the filter method is not the one PNG defines")
	case !validBitDepth(c.colorType, c.bitDepth):
		return nil, invalid(handle, "the bit depth is not one PNG permits for this colour type")
	case c.interlace != 0:
		return nil, unsupported(handle, "interlaced PNG",
			"Adam7 reorders the pixels into seven passes, which no PDF filter describes. "+
				"De-interlacing means fully decoding and recompressing the image, which changes "+
				"the bytes you supplied to work around how you saved the file. Save it non-interlaced.")
	}
	return c, nil
}

// validBitDepth reports whether a bit depth is legal for a colour type.
//
// Checked rather than tolerated, because the depth is arithmetic: it decides
// how many bytes a scanline is and how a sample is unpacked. A depth PNG does
// not define is a number nothing downstream has a correct answer for, and
// letting it through means each of those places inventing one.
func validBitDepth(colorType, bits int) bool {
	switch colorType {
	case colorGray:
		return bits == 1 || bits == 2 || bits == 4 || bits == 8 || bits == 16
	case colorIndexed:
		return bits == 1 || bits == 2 || bits == 4 || bits == 8
	case colorRGB, colorGrayAlpha, colorRGBA:
		return bits == 8 || bits == 16
	}
	return false
}

// channels returns the number of samples per pixel.
func (c *pngChunks) channels(handle string) (int, error) {
	switch c.colorType {
	case colorGray, colorIndexed:
		return 1, nil
	case colorRGB:
		return 3, nil
	case colorGrayAlpha:
		return 2, nil
	case colorRGBA:
		return 4, nil
	}
	return 0, invalid(handle, "unknown PNG colour type")
}

// colorSpace maps a PNG colour type to a PDF colour space.
func (c *pngChunks) colorSpace(handle string) (object.Object, error) {
	switch c.colorType {
	case colorGray, colorGrayAlpha:
		return object.Name("DeviceGray"), nil
	case colorRGB, colorRGBA:
		return object.Name("DeviceRGB"), nil
	case colorIndexed:
		if len(c.plte) == 0 || len(c.plte)%3 != 0 {
			return nil, invalid(handle, "an indexed image has no usable palette")
		}
		// The lookup goes in as a hexadecimal string. A literal string would be
		// legal and would need escaping for every byte colliding with PDF
		// syntax — and palette entries collide constantly, because they are
		// arbitrary bytes.
		return object.Array{
			object.Name("Indexed"),
			object.Name("DeviceRGB"),
			object.Int(len(c.plte)/3 - 1),
			object.HexString(c.plte),
		}, nil
	}
	return nil, invalid(handle, "unknown PNG colour type")
}

// colorKeyMask turns a greyscale or truecolour tRNS chunk into a /Mask array.
//
// This form of transparency says "this exact sample value is transparent"
// rather than carrying a channel, so it costs nothing: the colour data still
// passes through and the mask is six numbers in the image dictionary.
func (c *pngChunks) colorKeyMask(handle string) (object.Array, error) {
	switch c.colorType {
	case colorGray:
		if len(c.trns) < 2 {
			return nil, invalid(handle, "a greyscale tRNS chunk is shorter than one sample")
		}
		v := object.Int(binary.BigEndian.Uint16(c.trns[0:2]))
		return object.Array{v, v}, nil
	case colorRGB:
		if len(c.trns) < 6 {
			return nil, invalid(handle, "a truecolour tRNS chunk is shorter than three samples")
		}
		r := object.Int(binary.BigEndian.Uint16(c.trns[0:2]))
		g := object.Int(binary.BigEndian.Uint16(c.trns[2:4]))
		bl := object.Int(binary.BigEndian.Uint16(c.trns[4:6]))
		return object.Array{r, r, g, g, bl, bl}, nil
	}
	return nil, nil
}

// paletteAlpha builds a soft mask from an indexed image's per-entry alpha.
//
// The indices have to be read out to do it, which means inflating and
// unfiltering — but only to build the mask. The colour stream is still the
// original IDAT, so the palette and the indices in the document are the ones on
// disk.
func (c *pngChunks) paletteAlpha(opts Options, channels int) (*raster, error) {
	handle := opts.Handle
	rows, err := c.scanlines(opts, channels)
	if err != nil {
		return nil, err
	}

	// tRNS may be shorter than the palette; entries past its end are opaque.
	alphaFor := func(index int) byte {
		if index < len(c.trns) {
			return c.trns[index]
		}
		return 0xFF
	}

	out := make([]byte, 0, c.width*c.height)
	for _, row := range rows {
		for x := 0; x < c.width; x++ {
			idx, err := sampleAt(row, x, c.bitDepth)
			if err != nil {
				return nil, invalid(handle, err.Error())
			}
			out = append(out, alphaFor(idx))
		}
	}

	return &raster{
		width: c.width, height: c.height, bits: 8,
		space: object.Name("DeviceGray"), data: out,
	}, nil
}

// split separates an image whose alpha is interleaved with its colour.
//
// This is the one PNG form that cannot pass through, and the reason is
// structural rather than a limitation: PNG stores alpha in the same scanline as
// the colour it belongs to, and PDF stores it in a separate image. Every sample
// survives; only the arrangement changes.
func (c *pngChunks) split(opts Options, channels int, x *XObject) error {
	handle := opts.Handle
	rows, err := c.scanlines(opts, channels)
	if err != nil {
		return err
	}
	if c.bitDepth != 8 && c.bitDepth != 16 {
		// PNG itself only permits 8 and 16 for these colour types, so this is
		// a malformed file rather than an unsupported one.
		return invalid(handle, "an image with an alpha channel has a bit depth PNG does not permit for it")
	}

	sampleBytes := c.bitDepth / 8
	colorChannels := channels - 1
	pixels := c.width * c.height

	color := make([]byte, 0, pixels*colorChannels*sampleBytes)
	alpha := make([]byte, 0, pixels*sampleBytes)

	stride := channels * sampleBytes
	for _, row := range rows {
		if len(row) < c.width*stride {
			return invalid(handle, "a scanline is shorter than the image is wide")
		}
		for px := 0; px < c.width; px++ {
			at := px * stride
			color = append(color, row[at:at+colorChannels*sampleBytes]...)
			alpha = append(alpha, row[at+colorChannels*sampleBytes:at+stride]...)
		}
	}

	space, err := c.colorSpace(handle)
	if err != nil {
		return err
	}
	x.base = raster{
		width: c.width, height: c.height, bits: c.bitDepth,
		space: space, data: color,
	}
	x.alpha = &raster{
		width: c.width, height: c.height, bits: c.bitDepth,
		space: object.Name("DeviceGray"), data: alpha,
	}
	return nil
}

// scanlines inflates the image data and reverses the per-row filters.
//
// The size is checked against the bound before anything is allocated, and the
// arithmetic is in int64 throughout. The header declaring it is supplied by
// whoever supplied the file: a PNG can claim to be 65535 by 65535 in thirteen
// bytes, and computing a buffer length from that in int is both an overflow and
// a seventeen-gigabyte allocation, reached before a single byte is inflated.
func (c *pngChunks) scanlines(opts Options, channels int) ([][]byte, error) {
	handle := opts.Handle

	maxDecoded := opts.MaxDecodedBytes
	if maxDecoded <= 0 {
		maxDecoded = DefaultMaxDecodedBytes
	}
	// Divided rather than multiplied. A 2^31-1 square image at sixteen bits
	// across four channels describes about 2^70 bytes, so computing the product
	// first and comparing it against the bound overflows int64 to a negative
	// number, the comparison passes, and make panics with a length out of
	// range — a check that refuses everything except the inputs it exists to
	// refuse. The fuzzer found this in seven seconds.
	rowBytes64 := (int64(c.width)*int64(c.bitDepth)*int64(channels) + 7) / 8
	if rowBytes64+1 > maxDecoded/int64(c.height) {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_ASSET_TOO_LARGE,
			"the image's samples would exceed the decoding bound",
			map[string]any{
				"handle":      handle,
				"width":       c.width,
				"height":      c.height,
				"row_bytes":   rowBytes64,
				"limit_bytes": maxDecoded,
			})
	}
	total := (rowBytes64 + 1) * int64(c.height)

	zr, err := zlib.NewReader(bytes.NewReader(c.idat))
	if err != nil {
		return nil, invalid(handle, "the image data is not a zlib stream")
	}
	defer zr.Close()

	rowBytes := int(rowBytes64)
	// The filter operates on whole bytes, over the pixel one to its left. For
	// sub-byte depths that neighbour is the byte itself, so the offset floors
	// to one rather than to zero.
	bpp := (c.bitDepth*channels + 7) / 8
	if bpp < 1 {
		bpp = 1
	}

	raw := make([]byte, total)
	if _, err := io.ReadFull(zr, raw); err != nil {
		return nil, invalid(handle, "the image data is shorter than the header describes")
	}

	rows := make([][]byte, c.height)
	prev := make([]byte, rowBytes)
	for y := 0; y < c.height; y++ {
		at := y * (rowBytes + 1)
		ft := raw[at]
		cur := raw[at+1 : at+1+rowBytes]

		switch ft {
		case 0: // None
		case 1: // Sub
			for i := bpp; i < rowBytes; i++ {
				cur[i] += cur[i-bpp]
			}
		case 2: // Up
			for i := 0; i < rowBytes; i++ {
				cur[i] += prev[i]
			}
		case 3: // Average
			for i := 0; i < rowBytes; i++ {
				var left byte
				if i >= bpp {
					left = cur[i-bpp]
				}
				cur[i] += byte((int(left) + int(prev[i])) / 2)
			}
		case 4: // Paeth
			for i := 0; i < rowBytes; i++ {
				var left, upLeft byte
				if i >= bpp {
					left = cur[i-bpp]
					upLeft = prev[i-bpp]
				}
				cur[i] += paeth(left, prev[i], upLeft)
			}
		default:
			return nil, invalid(handle, "a scanline carries a filter type PNG does not define")
		}

		rows[y] = cur
		prev = cur
	}
	return rows, nil
}

// paeth is the predictor PNG defines, byte for byte.
func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa, pb, pc := absDiff(p, int(a)), absDiff(p, int(b)), absDiff(p, int(c))
	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	}
	return c
}

func absDiff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

// sampleAt reads one packed sample out of a scanline.
//
// Indexed images may be stored at one, two, four or eight bits per pixel, so
// the index is not always a byte.
func sampleAt(row []byte, x, bits int) (int, error) {
	switch bits {
	case 8:
		if x >= len(row) {
			return 0, errShortRow
		}
		return int(row[x]), nil
	case 1, 2, 4:
		per := 8 / bits
		at := x / per
		if at >= len(row) {
			return 0, errShortRow
		}
		shift := 8 - bits*(x%per+1)
		mask := byte(1<<bits - 1)
		return int((row[at] >> shift) & mask), nil
	}
	return 0, errIndexedDepth
}

// The two failures sampleAt can report, as values so the message is written
// once rather than at each call site.
var (
	errShortRow     = pngError("a scanline is shorter than the image is wide")
	errIndexedDepth = pngError("an indexed image has a bit depth PNG does not permit for it")
)

// pngError carries a reason into [invalid], which supplies the code.
type pngError string

func (e pngError) Error() string { return string(e) }
