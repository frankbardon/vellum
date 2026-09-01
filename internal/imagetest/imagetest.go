// Package imagetest builds the raster assets the image tests and the
// determinism fixtures embed.
//
// The images are generated rather than committed, for the reason every other
// fixture in this tree is: a binary blob in the repository is a thing nobody
// can review, and a golden built on one is evidence of nothing in particular.
// Generated here, the exact contents of every fixture are readable, and a test
// asserting "the alpha channel became a soft mask" can say which pixels.
//
// It is test tooling and stays test tooling. It draws gradients and squares,
// which is one step from rasterising, and Vellum does not rasterise — a shipped
// package reaching this would have an image source the host did not supply.
// TestNoImagetestOnTheLibraryPath is the firewall.
package imagetest

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
)

// Size is the edge length of every generated fixture.
//
// Small on purpose. These exist to prove a code path, and a fixture large
// enough to be interesting is one whose golden nobody reads.
const Size = 8

// RGBA returns a non-interlaced truecolour PNG with an alpha channel.
//
// The alpha varies across the image rather than being uniform, so a soft mask
// built from it is distinguishable from one filled with a constant — which is
// what a de-interleaving bug produces.
func RGBA() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, Size, Size))
	for y := 0; y < Size; y++ {
		for x := 0; x < Size; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x * 255 / (Size - 1)),
				G: uint8(y * 255 / (Size - 1)),
				B: 0x40,
				A: uint8((x + y) * 255 / (2*Size - 2)),
			})
		}
	}
	return encodePNG(img)
}

// RGB returns an opaque non-interlaced truecolour PNG.
func RGB() []byte {
	img := image.NewRGBA(image.Rect(0, 0, Size, Size))
	for y := 0; y < Size; y++ {
		for x := 0; x < Size; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(x * 255 / (Size - 1)),
				G: 0x20,
				B: uint8(y * 255 / (Size - 1)),
				A: 0xFF,
			})
		}
	}
	return encodePNG(img)
}

// Gray returns an opaque non-interlaced greyscale PNG.
func Gray() []byte {
	img := image.NewGray(image.Rect(0, 0, Size, Size))
	for y := 0; y < Size; y++ {
		for x := 0; x < Size; x++ {
			img.SetGray(x, y, color.Gray{Y: uint8((x + y) * 255 / (2*Size - 2))})
		}
	}
	return encodePNG(img)
}

// Paletted returns an indexed PNG. When transparent is set, the first palette
// entry is fully transparent, which is what puts a tRNS chunk in the file and
// exercises the per-entry alpha path.
func Paletted(transparent bool) []byte {
	first := color.NRGBA{R: 0xFF, G: 0x00, B: 0x00, A: 0xFF}
	if transparent {
		first = color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x00}
	}
	pal := color.Palette{
		first,
		color.NRGBA{R: 0x00, G: 0xFF, B: 0x00, A: 0xFF},
		color.NRGBA{R: 0x00, G: 0x00, B: 0xFF, A: 0xFF},
		color.NRGBA{R: 0xFF, G: 0xFF, B: 0x00, A: 0xFF},
	}
	img := image.NewPaletted(image.Rect(0, 0, Size, Size), pal)
	for y := 0; y < Size; y++ {
		for x := 0; x < Size; x++ {
			img.SetColorIndex(x, y, uint8((x+y)%len(pal)))
		}
	}
	return encodePNG(img)
}

// Interlaced returns a PNG whose header declares Adam7.
//
// Built by flipping the interlace byte in IHDR of a real PNG rather than by
// encoding one, because the standard library has no interlaced encoder and the
// bytes past the header are never reached: the rejection happens on the header.
// The CRC is left stale, which nothing here checks and which is stated so the
// fixture is not mistaken for a valid interlaced file.
func Interlaced() []byte {
	b := append([]byte(nil), RGB()...)
	// 8 signature + 4 length + 4 type + 12 IHDR fields = the interlace byte.
	b[8+4+4+12] = 1
	return b
}

// JPEGColor returns a baseline YCbCr JPEG.
func JPEGColor() []byte {
	img := image.NewRGBA(image.Rect(0, 0, Size, Size))
	for y := 0; y < Size; y++ {
		for x := 0; x < Size; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 31), G: uint8(y * 31), B: 0x80, A: 0xFF})
		}
	}
	var buf bytes.Buffer
	// The quality is pinned rather than left to the default, because it is an
	// input to the bytes and a default that changes would move every golden.
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// JPEGFrame builds a minimal JPEG carrying one frame header and nothing else.
//
// The variants a PDF cannot embed — progressive, CMYK, twelve-bit — are all
// rejected on the frame header, so a fixture only needs to get that far. An
// encoder for them would be a large amount of code to exercise a check that
// reads four bytes.
func JPEGFrame(marker byte, precision, components int) []byte {
	length := 8 + 3*components
	out := []byte{0xFF, 0xD8, 0xFF, marker}
	out = binary.BigEndian.AppendUint16(out, uint16(length))
	out = append(out, byte(precision))
	out = binary.BigEndian.AppendUint16(out, Size)
	out = binary.BigEndian.AppendUint16(out, Size)
	out = append(out, byte(components))
	for i := 0; i < components; i++ {
		out = append(out, byte(i+1), 0x11, 0x00)
	}
	return append(out, 0xFF, 0xD9)
}

func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
