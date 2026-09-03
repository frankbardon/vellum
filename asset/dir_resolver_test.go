package asset_test

import (
	"context"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/asset"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/spf13/afero"
)

func TestDirResolver_ResolvesAFileUnderRoot(t *testing.T) {
	fsys := afero.NewMemMapFs()
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n', 0, 0, 0, 0, 'I', 'H', 'D', 'R'}
	if err := afero.WriteFile(fsys, "/assets/pic.png", png, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	r := asset.NewDirResolver(fsys, "/assets")
	got, err := r.Resolve(context.Background(), asset.Request{Handle: "pic.png", Format: artifact.FormatDOCX})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if string(got.Bytes) != string(png) {
		t.Errorf("Bytes = %v, want %v", got.Bytes, png)
	}
	if got.Handle != "pic.png" {
		t.Errorf("Handle = %q, want %q", got.Handle, "pic.png")
	}
	if got.MediaType != "" {
		t.Errorf("MediaType = %q, want empty so Ingest sniffs it", got.MediaType)
	}
}

func TestDirResolver_ResolvesANestedFile(t *testing.T) {
	fsys := afero.NewMemMapFs()
	if err := afero.WriteFile(fsys, "/assets/sub/dir/pic.png", []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	r := asset.NewDirResolver(fsys, "/assets")
	got, err := r.Resolve(context.Background(), asset.Request{Handle: "sub/dir/pic.png"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if string(got.Bytes) != "data" {
		t.Errorf("Bytes = %q, want %q", got.Bytes, "data")
	}
}

func TestDirResolver_MissingFileIsNotFound(t *testing.T) {
	r := asset.NewDirResolver(afero.NewMemMapFs(), "/assets")
	_, err := r.Resolve(context.Background(), asset.Request{Handle: "nope.png"})
	if !verr.HasCode(err, verr.VELLUM_ASSET_NOT_FOUND) {
		t.Fatalf("err = %v, want VELLUM_ASSET_NOT_FOUND", err)
	}
}

func TestDirResolver_PathTraversalIsNotFound(t *testing.T) {
	fsys := afero.NewMemMapFs()
	// A file genuinely present just outside the configured root, so a
	// successful escape would actually find something.
	if err := afero.WriteFile(fsys, "/secret.png", []byte("shh"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	r := asset.NewDirResolver(fsys, "/assets")
	_, err := r.Resolve(context.Background(), asset.Request{Handle: "../secret.png"})
	if !verr.HasCode(err, verr.VELLUM_ASSET_NOT_FOUND) {
		t.Fatalf("err = %v, want VELLUM_ASSET_NOT_FOUND", err)
	}
}

func TestDirResolver_ThroughIngestSniffsAndHashes(t *testing.T) {
	fsys := afero.NewMemMapFs()
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n',
		0, 0, 0, 13, 'I', 'H', 'D', 'R',
		0, 0, 0, 2, 0, 0, 0, 2, 8, 6, 0, 0, 0}
	if err := afero.WriteFile(fsys, "/assets/pic.png", png, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	r := asset.NewDirResolver(fsys, "/assets")
	got, err := asset.Ingest(context.Background(), r, asset.Request{Handle: "pic.png", Format: artifact.FormatDOCX}, asset.Options{})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if got.MediaType != asset.MediaPNG {
		t.Errorf("MediaType = %q, want %q", got.MediaType, asset.MediaPNG)
	}
	if got.Hash == "" {
		t.Error("Hash is empty, want a content hash")
	}
}
