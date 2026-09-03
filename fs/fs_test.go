package fs_test

import (
	"testing"

	"github.com/frankbardon/vellum/fs"
	"github.com/spf13/afero"
)

func TestDefault_IsAnOSFilesystem(t *testing.T) {
	if _, ok := fs.Default().(*afero.OsFs); !ok {
		t.Errorf("Default() = %T, want *afero.OsFs", fs.Default())
	}
}

func TestSafeJoin_OrdinaryRelativePathStaysWithinRoot(t *testing.T) {
	got, ok := fs.SafeJoin("/themes", "foo.json")
	if !ok {
		t.Fatal("SafeJoin ok = false, want true")
	}
	if want := "/themes/foo.json"; got != want {
		t.Errorf("SafeJoin = %q, want %q", got, want)
	}
}

func TestSafeJoin_NestedRelativePathStaysWithinRoot(t *testing.T) {
	got, ok := fs.SafeJoin("/assets", "sub/dir/pic.png")
	if !ok {
		t.Fatal("SafeJoin ok = false, want true")
	}
	if want := "/assets/sub/dir/pic.png"; got != want {
		t.Errorf("SafeJoin = %q, want %q", got, want)
	}
}

func TestSafeJoin_RejectsTraversalOutOfRoot(t *testing.T) {
	cases := []string{
		"../secret.json",
		"../../etc/passwd",
		"a/../../b",
		"..",
	}
	for _, rel := range cases {
		if _, ok := fs.SafeJoin("/themes", rel); ok {
			t.Errorf("SafeJoin(%q) ok = true, want false", rel)
		}
	}
}

func TestSafeJoin_AnAbsoluteLookingHandleIsTreatedAsRelative(t *testing.T) {
	// filepath.Join never treats a second argument's leading slash as an
	// instruction to discard root — it is joined and cleaned like any other
	// path segment — so a handle of "/etc/passwd" against root "/themes"
	// lands at "/themes/etc/passwd", inside root, not outside it.
	got, ok := fs.SafeJoin("/themes", "/etc/passwd")
	if !ok {
		t.Fatal("SafeJoin ok = false, want true")
	}
	if want := "/themes/etc/passwd"; got != want {
		t.Errorf("SafeJoin = %q, want %q", got, want)
	}
}

func TestSafeJoin_RejectsTheRootItself(t *testing.T) {
	if _, ok := fs.SafeJoin("/themes", "."); ok {
		t.Error("SafeJoin(\".\") ok = true, want false — root is not a file")
	}
	if _, ok := fs.SafeJoin("/themes", ""); ok {
		t.Error("SafeJoin(\"\") ok = true, want false — root is not a file")
	}
}
