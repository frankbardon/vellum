package dettest

import (
	"archive/zip"
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/opc/zipdet"
)

// TestGoldenMediaDecodes asserts that every raster embedded in a golden is one
// a decoder actually accepts.
//
// It exists because a fixture got this wrong in a way nothing else could see.
// The first docx-profile PNG was hand-assembled from a signature, an IHDR and
// an IEND — no IDAT, no chunk CRCs. Vellum's sniffer recognised the signature,
// its probe read the dimensions out of the IHDR, the package was assembled
// correctly and the determinism harness was perfectly happy, because every
// stage was comparing our bytes against our bytes. Word drew "the picture can't
// be displayed".
//
// A golden whose media no reader accepts proves the packaging and proves
// nothing about the embedding, which is most of what a golden containing an
// image is for.
//
// Vectors are excluded deliberately: Go has no SVG decoder, and adding one to
// check a fixture would be adding a renderer to a library whose first principle
// is that it does not render.
func TestGoldenMediaDecodes(t *testing.T) {
	checked := 0

	for _, c := range Cases() {
		body, err := c.Bytes(zipdet.PinnedEpoch)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			// Not every case is an archive; a PDF golden will not be.
			continue
		}

		for _, f := range zr.File {
			if !isRaster(f.Name) {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("%s: opening %s: %v", c.Name, f.Name, err)
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatalf("%s: reading %s: %v", c.Name, f.Name, err)
			}

			cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
			if err != nil {
				t.Errorf("%s: %s does not decode: %v\n\n"+
					"The bytes were embedded, the package is well formed, and no reader will draw it. "+
					"Use a real image; a hand-assembled header sniffs and measures correctly and is "+
					"still not a picture.", c.Name, f.Name, err)
				continue
			}
			if cfg.Width <= 0 || cfg.Height <= 0 {
				t.Errorf("%s: %s decodes as %s with no dimensions", c.Name, f.Name, format)
			}
			checked++
		}
	}

	if checked == 0 {
		t.Fatal("no raster media was checked, so this test asserts nothing; " +
			"a golden exercising an image block should be registered in Cases")
	}
}

func isRaster(name string) bool {
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif"} {
		if strings.HasSuffix(strings.ToLower(name), ext) {
			return true
		}
	}
	return false
}
