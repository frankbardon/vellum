package image_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/asset"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/internal/imagetest"
	pdfimage "github.com/frankbardon/vellum/pdf/image"
	"github.com/frankbardon/vellum/pdf/object"
)

// build prepares an image and fails the test if it could not be prepared.
func build(t *testing.T, media string, b []byte) *pdfimage.XObject {
	t.Helper()
	x, err := pdfimage.New(pdfimage.Options{
		Resource: "Im1", Handle: "fixture", MediaType: media, Bytes: b,
	})
	if err != nil {
		t.Fatalf("preparing the image failed: %v", err)
	}
	return x
}

// wants asserts the error carries a code, and returns it for a message check.
func wants(t *testing.T, media string, b []byte, code verr.Code) error {
	t.Helper()
	_, err := pdfimage.New(pdfimage.Options{
		Resource: "Im1", Handle: "fixture", MediaType: media, Bytes: b,
	})
	if err == nil {
		t.Fatalf("the image was accepted; %s was expected", code)
	}
	if !verr.HasCode(err, code) {
		t.Fatalf("got %v, want %s", err, code)
	}
	return err
}

// TestPNG_OpaqueFormsPassThrough is the claim the whole package is built
// around: an opaque PNG's own compressed bytes reach the file untouched.
//
// If this ever stops holding, embedding has become a re-encode, and a consumer
// who chose their compression has had that choice quietly overridden.
func TestPNG_OpaqueFormsPassThrough(t *testing.T) {
	for _, tc := range []struct {
		name  string
		bytes []byte
		space string
	}{
		{"truecolour", imagetest.RGB(), "/DeviceRGB"},
		{"greyscale", imagetest.Gray(), "/DeviceGray"},
		{"indexed", imagetest.Paletted(false), "/Indexed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := build(t, asset.MediaPNG, tc.bytes)
			if x.HasAlpha() {
				t.Error("an opaque image was given a soft mask")
			}

			var doc object.Document
			if _, err := x.Write(&doc); err != nil {
				t.Fatalf("writing failed: %v", err)
			}
			out := render(t, &doc)

			idat := extractIDAT(t, tc.bytes)
			if !bytes.Contains(out, idat) {
				t.Fatalf("the PNG's own image data is not in the file verbatim.\n" +
					"Embedding a PNG must not decode and recompress it: the pixels a consumer supplied, " +
					"and the compression they chose, both belong in the document unchanged.")
			}
			for _, want := range []string{"/Filter /FlateDecode", "/Predictor 15", tc.space} {
				if !bytes.Contains(out, []byte(want)) {
					t.Errorf("the image dictionary does not carry %s:\n%s", want, dictOf(out))
				}
			}
		})
	}
}

// TestPNG_AlphaBecomesASoftMask covers the one form that cannot pass through.
func TestPNG_AlphaBecomesASoftMask(t *testing.T) {
	x := build(t, asset.MediaPNG, imagetest.RGBA())
	if !x.HasAlpha() {
		t.Fatal("an image with an alpha channel got no soft mask")
	}

	var doc object.Document
	if _, err := x.Write(&doc); err != nil {
		t.Fatalf("writing failed: %v", err)
	}
	out := render(t, &doc)

	if !bytes.Contains(out, []byte("/SMask ")) {
		t.Error("the image dictionary has no /SMask")
	}
	// The mask must be greyscale and must not itself carry a mask: a soft mask
	// with a soft mask is not something a reader is required to make sense of.
	if n := bytes.Count(out, []byte("/SMask ")); n != 1 {
		t.Errorf("found %d /SMask entries, want exactly one", n)
	}
	if n := bytes.Count(out, []byte("/ColorSpace /DeviceGray")); n != 1 {
		t.Errorf("found %d greyscale images, want exactly the mask", n)
	}
	// Rebuilt rather than passed through, so there is no predictor: the samples
	// were separated, which means they were unfiltered first.
	if bytes.Contains(out, []byte("/Predictor")) {
		t.Error("a de-interleaved image still declares a PNG predictor, which would decode it twice")
	}
}

// TestPNG_AlphaSurvivesTheSplit checks the mask carries the source's alpha
// rather than a constant.
//
// A de-interleaving bug does not produce an error. It produces a mask that is
// uniformly opaque, or one built from the blue channel — both of which render,
// and neither of which is what the consumer supplied. The fixture's alpha
// varies across the image precisely so this can be told apart.
func TestPNG_AlphaSurvivesTheSplit(t *testing.T) {
	x := build(t, asset.MediaPNG, imagetest.RGBA())

	var doc object.Document
	doc.Uncompressed = true
	if _, err := x.Write(&doc); err != nil {
		t.Fatalf("writing failed: %v", err)
	}
	out := render(t, &doc)

	mask := streamFor(t, out, "/ColorSpace /DeviceGray")
	if len(mask) != imagetest.Size*imagetest.Size {
		t.Fatalf("the mask is %d bytes, want one per pixel (%d)", len(mask), imagetest.Size*imagetest.Size)
	}

	// The fixture's alpha is (x+y) scaled across the image, so the corners are
	// fully transparent and fully opaque and nothing in between is constant.
	if mask[0] != 0x00 {
		t.Errorf("the top-left pixel's alpha is %#02x, want 0x00", mask[0])
	}
	if last := mask[len(mask)-1]; last != 0xFF {
		t.Errorf("the bottom-right pixel's alpha is %#02x, want 0xff", last)
	}
	same := true
	for _, b := range mask {
		if b != mask[0] {
			same = false
			break
		}
	}
	if same {
		t.Error("every alpha sample is identical; the mask was not built from the image's own channel")
	}
}

// TestPNG_PaletteTransparencyBecomesAMask checks per-entry palette alpha while
// the colour data still passes through.
func TestPNG_PaletteTransparencyBecomesAMask(t *testing.T) {
	src := imagetest.Paletted(true)
	x := build(t, asset.MediaPNG, src)
	if !x.HasAlpha() {
		t.Fatal("a palette with a transparent entry produced no soft mask")
	}

	var doc object.Document
	if _, err := x.Write(&doc); err != nil {
		t.Fatalf("writing failed: %v", err)
	}
	out := render(t, &doc)

	if !bytes.Contains(out, extractIDAT(t, src)) {
		t.Error("the indexed image's own data was not passed through; only the mask needs building")
	}
	if !bytes.Contains(out, []byte("/Indexed")) {
		t.Error("the colour space is not indexed")
	}
}

// TestPNG_ColourKeyTransparencyIsAMaskArray covers tRNS on a truecolour image,
// where transparency is a colour rather than a channel.
func TestPNG_ColourKeyTransparencyIsAMaskArray(t *testing.T) {
	src := withTRNS(t, imagetest.RGB(), []byte{0x00, 0x00, 0x00, 0x20, 0x00, 0x00})
	x := build(t, asset.MediaPNG, src)
	if x.HasAlpha() {
		t.Error("colour-key transparency was turned into a soft mask; it is six numbers in the dictionary")
	}

	var doc object.Document
	if _, err := x.Write(&doc); err != nil {
		t.Fatalf("writing failed: %v", err)
	}
	out := render(t, &doc)
	if !bytes.Contains(out, []byte("/Mask [0 0 32 32 0 0]")) {
		t.Errorf("the colour key is not in the dictionary:\n%s", dictOf(out))
	}
}

// TestJPEG_PassesThroughWhole is the JPEG counterpart to the PNG claim.
func TestJPEG_PassesThroughWhole(t *testing.T) {
	src := imagetest.JPEGColor()
	x := build(t, asset.MediaJPEG, src)

	var doc object.Document
	if _, err := x.Write(&doc); err != nil {
		t.Fatalf("writing failed: %v", err)
	}
	out := render(t, &doc)

	if !bytes.Contains(out, src) {
		t.Fatal("the JPEG's bytes are not in the file verbatim; DCTDecode is JPEG, so nothing needs to happen to them")
	}
	for _, want := range []string{"/Filter /DCTDecode", "/ColorSpace /DeviceRGB", "/BitsPerComponent 8"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("the image dictionary does not carry %s:\n%s", want, dictOf(out))
		}
	}
}

// TestRejectedVariants pins every declared rejection.
//
// Each of these is a capability matrix row, and each error must name what the
// consumer should do instead — "unsupported" on its own tells them nothing they
// can act on.
func TestRejectedVariants(t *testing.T) {
	for _, tc := range []struct {
		name    string
		media   string
		bytes   []byte
		wantSay string
	}{
		{"interlaced PNG", asset.MediaPNG, imagetest.Interlaced(), "non-interlaced"},
		{"progressive JPEG", asset.MediaJPEG, imagetest.JPEGFrame(0xC2, 8, 3), "baseline"},
		{"CMYK JPEG", asset.MediaJPEG, imagetest.JPEGFrame(0xC0, 8, 4), "sRGB"},
		{"twelve-bit JPEG", asset.MediaJPEG, imagetest.JPEGFrame(0xC0, 12, 3), "eight-bit"},
		{"arithmetic JPEG", asset.MediaJPEG, imagetest.JPEGFrame(0xC9, 8, 3), "baseline"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := wants(t, tc.media, tc.bytes, verr.VELLUM_PDF_IMAGE_UNSUPPORTED)

			// The message is one line for every variant, so what distinguishes
			// them — and what a consumer can act on — is the detail. A
			// rejection that does not say what to do instead is a dead end.
			var coded *verr.CodedError
			if !errors.As(err, &coded) {
				t.Fatalf("the error is not a CodedError: %v", err)
			}
			for _, key := range []string{"variant", "reason"} {
				if _, ok := coded.Detail(key); !ok {
					t.Errorf("the error carries no %q detail", key)
				}
			}
			reason, _ := coded.Detail("reason")
			if s, _ := reason.(string); !strings.Contains(s, tc.wantSay) {
				t.Errorf("the reason does not say what to do instead (wanted it to mention %q):\n%v", tc.wantSay, reason)
			}
		})
	}
}

// TestMalformedInput pins the difference between "this is not the file it says
// it is" and "this is that file in a form PDF cannot carry".
func TestMalformedInput(t *testing.T) {
	t.Run("truncated PNG", func(t *testing.T) {
		src := imagetest.RGB()
		wants(t, asset.MediaPNG, src[:len(src)-40], verr.VELLUM_PDF_IMAGE_INVALID)
	})
	t.Run("PNG with no image data", func(t *testing.T) {
		src := imagetest.RGB()
		// Rename IDAT so the chunk walk skips it and the file has none.
		copy(src[bytes.Index(src, []byte("IDAT")):], []byte("iDAT"))
		wants(t, asset.MediaPNG, src, verr.VELLUM_PDF_IMAGE_INVALID)
	})
	t.Run("JPEG with no frame header", func(t *testing.T) {
		wants(t, asset.MediaJPEG, []byte{0xFF, 0xD8, 0xFF, 0xD9}, verr.VELLUM_PDF_IMAGE_INVALID)
	})
	t.Run("a media type PDF does not embed", func(t *testing.T) {
		wants(t, asset.MediaSVG, []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
			verr.VELLUM_ASSET_MEDIA_UNSUPPORTED)
	})
}

// TestDimensionsComeFromTheImage checks the intrinsic size is read rather than
// assumed, for both formats.
func TestDimensionsComeFromTheImage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		media string
		bytes []byte
	}{
		{"png", asset.MediaPNG, imagetest.RGBA()},
		{"jpeg", asset.MediaJPEG, imagetest.JPEGColor()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := build(t, tc.media, tc.bytes)
			if x.WidthPx() != imagetest.Size || x.HeightPx() != imagetest.Size {
				t.Errorf("got %dx%d, want %dx%d", x.WidthPx(), x.HeightPx(), imagetest.Size, imagetest.Size)
			}
		})
	}
}

// TestFingerprintDistinguishesContent guards the file identifier.
//
// Two documents differing only in which picture they draw must not claim one
// identity, and the content stream cannot tell them apart because it names the
// image by resource name.
func TestFingerprintDistinguishesContent(t *testing.T) {
	a := build(t, asset.MediaPNG, imagetest.RGB())
	b := build(t, asset.MediaPNG, imagetest.Gray())
	if bytes.Equal(a.Fingerprint(), b.Fingerprint()) {
		t.Error("two different images have the same fingerprint")
	}

	again := build(t, asset.MediaPNG, imagetest.RGB())
	if !bytes.Equal(a.Fingerprint(), again.Fingerprint()) {
		t.Error("the same image fingerprinted differently twice")
	}
}
