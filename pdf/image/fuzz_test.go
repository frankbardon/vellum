package image_test

import (
	"bytes"
	"testing"

	"github.com/frankbardon/vellum/asset"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/internal/imagetest"
	pdfimage "github.com/frankbardon/vellum/pdf/image"
	"github.com/frankbardon/vellum/pdf/object"
)

// fuzzSeeds are the inputs both image targets start from.
//
// The valid fixtures and the malformed ones together, because a fuzzer given
// only garbage spends its budget rediscovering that garbage is rejected. The
// interesting inputs are one byte away from valid, and the only way to get
// there cheaply is to start from valid.
func fuzzSeeds() [][]byte {
	return [][]byte{
		imagetest.RGB(),
		imagetest.RGBA(),
		imagetest.Gray(),
		imagetest.Paletted(false),
		imagetest.Paletted(true),
		imagetest.Interlaced(),
		imagetest.JPEGColor(),
		imagetest.JPEGFrame(0xC0, 8, 3),
		imagetest.JPEGFrame(0xC2, 8, 3),
		imagetest.JPEGFrame(0xC0, 8, 4),
		{},
		[]byte("\x89PNG\r\n\x1a\n"),
		[]byte{0xFF, 0xD8, 0xFF},
	}
}

// FuzzImage fuzzes the readers that parse an asset a consumer supplied.
//
// This is the only place in the library that reads bytes Vellum did not write.
// A host resolving an asset from user-supplied storage hands them straight
// here, so the property under test is not correctness of the parse but the
// absence of catastrophe: no panic, no allocation the header alone can demand,
// and no image accepted whose dictionary contradicts its own stream.
//
// The media type is chosen from the data rather than fuzzed separately, so the
// budget goes on the image bytes instead of on a boolean.
func FuzzImage(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		media, ok := asset.SniffMedia(data)
		if !ok {
			// Sniffing is what decides the media type in the real path too, so
			// an input that matches no signature never reaches this package.
			return
		}

		x, err := pdfimage.New(pdfimage.Options{
			Resource: "Im1", Handle: "fuzz", MediaType: media, Bytes: data,
			// A small bound, so a decode bomb is refused rather than expanded:
			// without it the fuzzer would find one and the process would die of
			// memory exhaustion rather than report a finding.
			MaxDecodedBytes: 1 << 20,
		})
		if err != nil {
			// Every rejection must be a coded error. An uncoded one is a
			// failure a consumer cannot handle programmatically, which is the
			// same as a panic to anything downstream of the facade.
			if _, coded := verr.CodeOf(err); !coded {
				t.Fatalf("a rejection carries no code: %v", err)
			}
			return
		}
		if x == nil {
			t.Fatal("New returned no image and no error")
		}

		// An accepted image must have a size a layout can use. Zero would
		// divide by zero in an aspect ratio, several layers away from here.
		if x.WidthPx() <= 0 || x.HeightPx() <= 0 {
			t.Fatalf("an accepted image has dimensions %dx%d", x.WidthPx(), x.HeightPx())
		}

		// And it must write. A parse that succeeds and an embedding that panics
		// is the worst of the two outcomes, because the failure lands in the
		// writer with none of the asset's context.
		var doc object.Document
		doc.Root = doc.Add(object.NewDict("Type", object.Name("Catalog")))
		if _, err := x.Write(&doc); err != nil {
			t.Fatalf("an accepted image could not be written: %v", err)
		}

		var buf bytes.Buffer
		if err := doc.Write(&buf); err != nil {
			t.Fatalf("writing the document failed: %v", err)
		}
		assertStreamLengthsAgree(t, buf.Bytes())
	})
}

// FuzzImageFingerprint checks the property the file identifier rests on.
//
// Two images with different content must not fingerprint alike, and one image
// must fingerprint the same way twice. The second half is the one a fuzzer can
// actually falsify: a fingerprint over a map, or over a slice built by ranging
// one, would be stable in every fixture and unstable for some input nobody
// wrote a fixture for.
func FuzzImageFingerprint(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		media, ok := asset.SniffMedia(data)
		if !ok {
			return
		}
		opts := pdfimage.Options{
			Resource: "Im1", Handle: "fuzz", MediaType: media, Bytes: data,
			MaxDecodedBytes: 1 << 20,
		}

		a, err := pdfimage.New(opts)
		if err != nil {
			return
		}
		b, err := pdfimage.New(opts)
		if err != nil {
			t.Fatalf("the same input was accepted once and refused once: %v", err)
		}
		if !bytes.Equal(a.Fingerprint(), b.Fingerprint()) {
			t.Fatal("the same image fingerprinted differently twice; the file identifier is not a function of content")
		}
	})
}

// assertStreamLengthsAgree checks every stream's /Length against its data.
//
// A stream whose declared length disagrees with its content is the single most
// common way a hand-built PDF becomes unreadable, and no reader reports it
// usefully — the file simply fails to open, or opens with everything after the
// stream misparsed. The writer sets /Length from the data rather than trusting
// the dictionary, and this is the assertion that the arrangement holds for
// input no fixture covers.
func assertStreamLengthsAgree(t *testing.T, out []byte) {
	t.Helper()

	for i := 0; ; {
		at := bytes.Index(out[i:], []byte("\nstream\n"))
		if at < 0 {
			return
		}
		start := i + at + len("\nstream\n")
		end := bytes.Index(out[start:], []byte("\nendstream"))
		if end < 0 {
			t.Fatal("a stream was opened and never closed")
		}

		declared := declaredLength(t, out[:start])
		if declared != end {
			t.Fatalf("a stream declares /Length %d and carries %d bytes", declared, end)
		}
		i = start + end
	}
}

// declaredLength reads the /Length of the stream dictionary ending at the tail
// of prefix.
func declaredLength(t *testing.T, prefix []byte) int {
	t.Helper()

	at := bytes.LastIndex(prefix, []byte("/Length "))
	if at < 0 {
		t.Fatal("a stream carries no /Length")
	}
	n := 0
	digits := 0
	for _, c := range prefix[at+len("/Length "):] {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
		digits++
	}
	if digits == 0 {
		t.Fatal("a stream's /Length is not a number")
	}
	return n
}
