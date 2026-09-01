// Package color builds the colour data PDF/A requires.
//
// PDF/A-2b requires an output intent naming the condition the document's
// device-dependent colours are to be interpreted in, and requires the ICC
// profile for that condition to be embedded. Without it a DeviceRGB fill has no
// defined meaning and the file is not conforming.
//
// # Why the profile is built rather than shipped
//
// The obvious answer is to embed the ICC consortium's sRGB2014.icc as a blob.
// Building it instead removes a redistributed binary and the licence notice
// that comes with it, removes the possibility of the blob and the code
// disagreeing about what it contains, and makes the bytes a function of source
// that can be read in review. It is around eight hundred bytes of entirely
// specified structure.
//
// The primaries and white point are sRGB's, chromatically adapted to the D50
// profile connection space, which is what every sRGB profile carries.
package color

import (
	"encoding/binary"
)

// SRGBProfile returns an ICC v2.4 display profile describing sRGB.
//
// The bytes are constant: nothing here reads a clock, a locale or a random
// source, and the creation date in the header is pinned rather than taken from
// the run. A profile that varied would put the variation inside a stream that
// PDF/A requires to be present in every conforming file.
func SRGBProfile() []byte {
	tags := []iccTag{
		{sig: "bTRC", data: curve()},
		{sig: "bXYZ", data: xyz(bluePrimary)},
		{sig: "cprt", data: text("Public Domain")},
		{sig: "desc", data: description("sRGB")},
		{sig: "gTRC", data: curve()},
		{sig: "gXYZ", data: xyz(greenPrimary)},
		{sig: "rTRC", data: curve()},
		{sig: "rXYZ", data: xyz(redPrimary)},
		{sig: "wtpt", data: xyz(d50WhitePoint)},
	}
	return assemble(tags)
}

// NumComponents is the profile's channel count, which the PDF stream must
// declare as /N.
const NumComponents = 3

// s15Fixed16 values for the sRGB primaries and white point, adapted to D50.
//
// These are the numbers every sRGB ICC profile carries. They are written as the
// fixed-point integers that reach the file rather than as decimals converted at
// build time, so no floating-point operation stands between the constant and
// the bytes — the same reason lengths elsewhere in Vellum are integers.
var (
	redPrimary    = [3]int32{0x6FA2, 0x38F5, 0x0390}  // 0.43607 0.22249 0.01392
	greenPrimary  = [3]int32{0x6299, 0xB785, 0x18DA}  // 0.38515 0.71687 0.09708
	bluePrimary   = [3]int32{0x24A0, 0x0F84, 0xB6CF}  // 0.14307 0.06061 0.71410
	d50WhitePoint = [3]int32{0xF6D6, 0x10000, 0xD32D} // 0.96420 1.00000 0.82491
)

// gamma22 is 2.2 in the u8Fixed8 encoding a single-value curve uses.
//
// A single gamma rather than a thousand-entry sampled curve of the true
// piecewise sRGB transfer function. The sampled form would need math.Pow, whose
// results are not guaranteed identical across platforms and Go versions, and
// putting that inside a stream Vellum promises is byte-identical trades a
// difference nobody can see for one everybody's checksum can. The output
// intent declares the viewing condition; it does not colour-manage the content.
const gamma22 = 0x0233

// iccTag is one entry in the tag table.
type iccTag struct {
	sig  string
	data []byte
}

// assemble writes the header, the tag table and the tag data.
func assemble(tags []iccTag) []byte {
	const headerSize = 128
	tableSize := 4 + len(tags)*12
	offset := headerSize + tableSize

	// Tag data is aligned to four bytes, which the specification requires.
	data := make([]byte, 0, 512)
	offsets := make([]uint32, len(tags))
	sizes := make([]uint32, len(tags))
	for i, t := range tags {
		for (offset+len(data))%4 != 0 {
			data = append(data, 0)
		}
		offsets[i] = uint32(offset + len(data))
		sizes[i] = uint32(len(t.data))
		data = append(data, t.data...)
	}

	out := make([]byte, headerSize+tableSize, headerSize+tableSize+len(data))

	binary.BigEndian.PutUint32(out[0:], uint32(len(out)+len(data)))
	copy(out[8:], []byte{0x02, 0x40, 0x00, 0x00}) // version 2.4
	copy(out[12:], "mntr")                        // display device class
	copy(out[16:], "RGB ")
	copy(out[20:], "XYZ ") // profile connection space

	// A pinned creation date. The profile is a constant, so its date is one
	// too; taking it from the clock would make every file Vellum writes differ
	// from every other by twelve bytes buried in a colour profile.
	putDate(out[24:], 2000, 1, 1, 0, 0, 0)

	copy(out[36:], "acsp")
	// Rendering intent 0 is perceptual, which is what an output intent for a
	// display condition declares.
	binary.BigEndian.PutUint32(out[64:], 0)
	putXYZ(out[68:], d50WhitePoint)

	binary.BigEndian.PutUint32(out[headerSize:], uint32(len(tags)))
	for i, t := range tags {
		rec := out[headerSize+4+i*12:]
		copy(rec[:4], t.sig)
		binary.BigEndian.PutUint32(rec[4:], offsets[i])
		binary.BigEndian.PutUint32(rec[8:], sizes[i])
	}
	return append(out, data...)
}

// putDate writes the twelve-byte dateTimeNumber the header carries.
func putDate(dst []byte, year, month, day, hour, minute, second uint16) {
	for i, v := range []uint16{year, month, day, hour, minute, second} {
		binary.BigEndian.PutUint16(dst[i*2:], v)
	}
}

// putXYZ writes three s15Fixed16 values.
func putXYZ(dst []byte, v [3]int32) {
	for i, c := range v {
		binary.BigEndian.PutUint32(dst[i*4:], uint32(c))
	}
}

// xyz builds an XYZType tag element.
func xyz(v [3]int32) []byte {
	out := make([]byte, 20)
	copy(out, "XYZ ")
	putXYZ(out[8:], v)
	return out
}

// curve builds a curveType tag element holding a single gamma value.
func curve() []byte {
	out := make([]byte, 14)
	copy(out, "curv")
	binary.BigEndian.PutUint32(out[8:], 1)
	binary.BigEndian.PutUint16(out[12:], gamma22)
	return out
}

// text builds a textType tag element, which is a NUL-terminated ASCII string.
func text(s string) []byte {
	out := make([]byte, 8, 8+len(s)+1)
	copy(out, "text")
	out = append(out, s...)
	return append(out, 0)
}

// description builds a descType tag element.
//
// The v2 description carries the same text three times — ASCII, UTF-16 and
// Macintosh ScriptCode — because the type predates anybody agreeing on an
// encoding. The latter two are written empty, which is what profiles have done
// for twenty years and what readers expect.
func description(s string) []byte {
	const scriptCodeLen = 67

	ascii := append([]byte(s), 0)
	out := make([]byte, 0, 12+len(ascii)+8+3+scriptCodeLen)
	out = append(out, "desc"...)
	out = append(out, 0, 0, 0, 0) // reserved

	out = binary.BigEndian.AppendUint32(out, uint32(len(ascii)))
	out = append(out, ascii...)

	out = binary.BigEndian.AppendUint32(out, 0) // Unicode language code
	out = binary.BigEndian.AppendUint32(out, 0) // Unicode count

	out = binary.BigEndian.AppendUint16(out, 0) // ScriptCode code
	out = append(out, 0)                        // ScriptCode count
	return append(out, make([]byte, scriptCodeLen)...)
}
