package splice_test

// TestSpliceInto_* proves the srcPkg/assetPkg split SpliceInto exists for:
// an Asset block's new media part and relationship land in assetPkg, never
// in srcPkg, even when srcPkg is a throwaway single-part view built from an
// extracted, relocated slice of the real document — exactly the shape
// template/bind's own repeat execution builds one per iteration of a
// repeated row or content control. This is the mechanism that makes "a
// repeat whose iterations embed different images" correct: each
// iteration's own splice reads its bytes from a throwaway srcPkg discarded
// the moment that iteration's own output is captured, but writes any new
// asset to the one assetPkg that survives to become the file a caller
// actually writes.

import (
	"bytes"
	"testing"

	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/template/splice"
)

func TestSpliceInto_AssetLandsInAssetPkgNotSrcPkg(t *testing.T) {
	// The "real" output package: it already carries the document part (as
	// Fill's own clone of the opened template would), and nothing else.
	assetPkg := opc.New()
	realDoc := nativeDoc(`<w:p><w:r><w:t>placeholder</w:t></w:r></w:p>`)
	if err := assetPkg.Put(&opc.Part{Name: partDocument, ContentType: ctMainDocument, Data: realDoc}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	assetPartsBefore := len(assetPkg.Names())

	images := []struct {
		hash  string
		bytes []byte
	}{
		{"iter0hash", onePixelPNG},
		{"iter1hash", onePixelPNG},
	}

	for i, img := range images {
		// One throwaway srcPkg per iteration, exactly as execRepeat builds
		// one per item: it carries only this iteration's own extracted
		// slice, under the same part name the real document uses, and is
		// discarded at the end of this loop body.
		extracted := nativeDoc(`<w:p><w:r><w:t>placeholder</w:t></w:r></w:p>`)
		srcPkg := opc.New()
		if err := srcPkg.Put(&opc.Part{Name: partDocument, ContentType: ctMainDocument, Data: extracted}); err != nil {
			t.Fatalf("iteration %d: Put: %v", i, err)
		}
		a := nativeAnchor(t, extracted, "logo")

		seq := fragment.Sequence{
			Assets: []fragment.Asset{{MediaType: "image/png", Hash: img.hash, Bytes: img.bytes}},
			Blocks: []fragment.Block{{Kind: spec.BlockAsset, Asset: &fragment.AssetRef{
				AssetIndex: 0, WidthEMU: 914400, HeightEMU: 914400,
			}}},
		}

		if _, err := splice.SpliceInto(srcPkg, assetPkg, a, seq); err != nil {
			t.Fatalf("iteration %d: SpliceInto: %v", i, err)
		}

		// The throwaway package gained no media part of its own: every
		// asset write from this call landed in assetPkg instead.
		if got := len(srcPkg.Names()); got != 1 {
			t.Errorf("iteration %d: throwaway srcPkg has %d parts, want 1 (the document part only): %v", i, got, srcPkg.Names())
		}
	}

	// assetPkg gained exactly one new media part per iteration, each
	// distinct (content-hash-named), on top of the document part it
	// started with plus the relationships part registering them.
	afterNames := assetPkg.Names()
	if got, want := len(afterNames), assetPartsBefore+len(images)+1; got != want {
		t.Fatalf("assetPkg has %d parts, want %d (the original document part, one media part per iteration, and the relationships part): %v", got, want, afterNames)
	}
	for _, img := range images {
		mediaPart := "/word/media/img" + img.hash + ".png"
		part, ok := assetPkg.Get(mediaPart)
		if !ok {
			t.Errorf("assetPkg is missing media part %s; has %v", mediaPart, afterNames)
			continue
		}
		b, err := part.Bytes()
		if err != nil {
			t.Fatalf("Bytes(%s): %v", mediaPart, err)
		}
		if !bytes.Equal(b, img.bytes) {
			t.Errorf("media part %s bytes do not match the source asset", mediaPart)
		}
	}

	// Both iterations' relationships are registered against the real
	// document part in assetPkg, one id per distinct image.
	rels, ok := assetPkg.RelationshipsFor(partDocument)
	if !ok {
		t.Fatal("assetPkg carries no relationships for the document part")
	}
	const relImage = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"
	seen := map[string]bool{}
	for _, img := range images {
		id, ok := rels.IDFor(relImage, "media/img"+img.hash+".png")
		if !ok {
			t.Errorf("no image relationship registered for %s", img.hash)
			continue
		}
		if seen[id] {
			t.Errorf("relationship id %s reused across two distinct images", id)
		}
		seen[id] = true
	}
}

func TestSplice_IsSpliceIntoWithOnePackage(t *testing.T) {
	src := nativeDoc(`<w:p><w:r><w:t>placeholder</w:t></w:r></w:p>`)
	pkg := buildPackage(t, src)
	a := nativeAnchor(t, src, "body")
	seq := fragment.Sequence{Blocks: []fragment.Block{textBlock(run("hello", fragment.TextStyle{}))}}

	viaSplice, err := splice.Splice(pkg, a, seq)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}

	pkg2 := buildPackage(t, src)
	viaSpliceInto, err := splice.SpliceInto(pkg2, pkg2, a, seq)
	if err != nil {
		t.Fatalf("SpliceInto: %v", err)
	}

	if viaSplice.Start != viaSpliceInto.Start || viaSplice.End != viaSpliceInto.End || !bytes.Equal(viaSplice.Data, viaSpliceInto.Data) {
		t.Errorf("Splice and SpliceInto(pkg, pkg, ...) disagree:\nSplice:     %+v\nSpliceInto: %+v", viaSplice, viaSpliceInto)
	}
}
