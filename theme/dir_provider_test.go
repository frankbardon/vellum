package theme_test

import (
	"context"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/theme"
	"github.com/spf13/afero"
)

func TestDirProvider_EmptyIDServesTheBuiltinTheme(t *testing.T) {
	p := theme.NewDirProvider(afero.NewMemMapFs(), "/themes")
	got, err := p.Theme(context.Background(), "")
	if err != nil {
		t.Fatalf("Theme(\"\"): %v", err)
	}
	want, _ := theme.Builtin()
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
}

func TestDirProvider_BuiltinIDServesTheBuiltinThemeEvenWithAFileOnDisk(t *testing.T) {
	fsys := afero.NewMemMapFs()
	if err := afero.WriteFile(fsys, "/themes/"+theme.BuiltinID+".json", theme.BuiltinJSON(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	p := theme.NewDirProvider(fsys, "/themes")
	got, err := p.Theme(context.Background(), theme.BuiltinID)
	if err != nil {
		t.Fatalf("Theme(%q): %v", theme.BuiltinID, err)
	}
	want, _ := theme.Builtin()
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
}

func TestDirProvider_ServesARegisteredFile(t *testing.T) {
	fsys := afero.NewMemMapFs()
	if err := afero.WriteFile(fsys, "/themes/custom.json", theme.BuiltinJSON(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	p := theme.NewDirProvider(fsys, "/themes")
	got, err := p.Theme(context.Background(), "custom")
	if err != nil {
		t.Fatalf("Theme(\"custom\"): %v", err)
	}
	if got == nil {
		t.Fatal("Theme returned nil")
	}
}

func TestDirProvider_UnknownIDIsNotFound(t *testing.T) {
	p := theme.NewDirProvider(afero.NewMemMapFs(), "/themes")
	_, err := p.Theme(context.Background(), "nope")
	if !verr.HasCode(err, verr.VELLUM_THEME_NOT_FOUND) {
		t.Fatalf("err = %v, want VELLUM_THEME_NOT_FOUND", err)
	}
}

func TestDirProvider_MalformedFileIsThemeInvalid(t *testing.T) {
	fsys := afero.NewMemMapFs()
	if err := afero.WriteFile(fsys, "/themes/broken.json", []byte(`{"id":"broken"`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	p := theme.NewDirProvider(fsys, "/themes")
	_, err := p.Theme(context.Background(), "broken")
	if !verr.HasCode(err, verr.VELLUM_THEME_INVALID) {
		t.Fatalf("err = %v, want VELLUM_THEME_INVALID", err)
	}
}

func TestDirProvider_PathTraversalIsNotFound(t *testing.T) {
	fsys := afero.NewMemMapFs()
	// A file that genuinely exists just outside the configured root, so a
	// successful traversal would actually find something.
	if err := afero.WriteFile(fsys, "/secret.json", theme.BuiltinJSON(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	p := theme.NewDirProvider(fsys, "/themes")
	_, err := p.Theme(context.Background(), "../secret")
	if !verr.HasCode(err, verr.VELLUM_THEME_NOT_FOUND) {
		t.Fatalf("err = %v, want VELLUM_THEME_NOT_FOUND", err)
	}
}

func TestDirProvider_ResolveComposesWithTheBuiltinFallback(t *testing.T) {
	// Resolve is the dispatch point every caller uses; a DirProvider must
	// work through it exactly as BuiltinProvider and StaticProvider do.
	p := theme.NewDirProvider(afero.NewMemMapFs(), "/themes")
	got, err := theme.Resolve(context.Background(), p, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ID != theme.BuiltinID {
		t.Errorf("ID = %q, want %q", got.ID, theme.BuiltinID)
	}
}
