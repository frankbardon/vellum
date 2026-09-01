package image_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"testing"

	"github.com/frankbardon/vellum/pdf/object"
)

// render writes a document and returns its bytes.
//
// A catalog is supplied because the writer requires one; nothing here reads it.
func render(t *testing.T, doc *object.Document) []byte {
	t.Helper()
	if doc.Root.IsZero() {
		doc.Root = doc.Add(object.NewDict("Type", object.Name("Catalog")))
	}
	var buf bytes.Buffer
	if err := doc.Write(&buf); err != nil {
		t.Fatalf("writing the document failed: %v", err)
	}
	return buf.Bytes()
}

// dictOf returns the dictionaries in a rendered file, for failure messages.
func dictOf(out []byte) string {
	var b bytes.Buffer
	for _, line := range bytes.Split(out, []byte("\n")) {
		if bytes.HasPrefix(bytes.TrimSpace(line), []byte("<<")) {
			b.Write(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// streamFor returns the data of the first stream whose dictionary contains a
// marker.
func streamFor(t *testing.T, out []byte, marker string) []byte {
	t.Helper()

	at := bytes.Index(out, []byte(marker))
	if at < 0 {
		t.Fatalf("no object in the file carries %q", marker)
	}
	rest := out[at:]
	start := bytes.Index(rest, []byte("stream\n"))
	end := bytes.Index(rest, []byte("\nendstream"))
	if start < 0 || end < 0 || end < start {
		t.Fatalf("the object carrying %q is not a stream", marker)
	}
	return rest[start+len("stream\n") : end]
}

// extractIDAT concatenates a PNG's image data chunks.
//
// The comparison this feeds is what "passes through" means: the same bytes, in
// the same order, in the PDF.
func extractIDAT(t *testing.T, png []byte) []byte {
	t.Helper()

	var out []byte
	for i := 8; i+8 <= len(png); {
		length := int(binary.BigEndian.Uint32(png[i : i+4]))
		if i+12+length > len(png) {
			t.Fatalf("the fixture PNG is malformed at offset %d", i)
		}
		if string(png[i+4:i+8]) == "IDAT" {
			out = append(out, png[i+8:i+8+length]...)
		}
		i += 12 + length
	}
	if len(out) == 0 {
		t.Fatal("the fixture PNG carries no image data")
	}
	return out
}

// withTRNS inserts a transparency chunk before the first IDAT.
//
// The standard library encodes no tRNS for a truecolour image, so colour-key
// transparency has to be added by hand. The CRC is computed properly here
// rather than left stale, because unlike the interlaced fixture this file is
// read past its header.
func withTRNS(t *testing.T, png, data []byte) []byte {
	t.Helper()

	at := bytes.Index(png, []byte("IDAT"))
	if at < 4 {
		t.Fatal("the fixture PNG carries no image data")
	}
	at -= 4 // back up over the length field

	chunk := make([]byte, 0, len(data)+12)
	chunk = binary.BigEndian.AppendUint32(chunk, uint32(len(data)))
	chunk = append(chunk, "tRNS"...)
	chunk = append(chunk, data...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))

	out := make([]byte, 0, len(png)+len(chunk))
	out = append(out, png[:at]...)
	out = append(out, chunk...)
	return append(out, png[at:]...)
}
