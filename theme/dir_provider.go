package theme

import (
	"context"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fs"
	"github.com/spf13/afero"
)

// DirProvider serves theme documents read from files under a directory.
//
// It composes with the built-in-theme special case rather than replacing it:
// an empty id, or [BuiltinID], still serves [Builtin] exactly as
// [BuiltinProvider] does — a DirProvider is what a host wires when it wants
// its *own* themes to be reachable too, not a second way of naming the
// built-in one.
//
// File naming convention: an id "foo" is read from "<root>/foo.json" and
// decoded with [Decode] — the same strict decode every theme document goes
// through, whether embedded, registered with [NewStaticProvider], or served
// from here, so a theme on disk validates by the same rule a theme anywhere
// else in this library does.
//
// An id the directory carries no file for is VELLUM_THEME_NOT_FOUND, exactly
// matching [BuiltinProvider]'s own contract: "an id the provider does not
// carry must be VELLUM_THEME_NOT_FOUND rather than a substituted default."
type DirProvider struct {
	fs   afero.Fs
	root string
}

// NewDirProvider constructs a DirProvider serving theme documents from root
// on fsys.
func NewDirProvider(fsys afero.Fs, root string) *DirProvider {
	return &DirProvider{fs: fsys, root: root}
}

// Theme implements [Provider].
func (p *DirProvider) Theme(_ context.Context, id string) (*Theme, error) {
	if id == "" || id == BuiltinID {
		return Builtin()
	}
	path, ok := fs.SafeJoin(p.root, id+".json")
	if !ok {
		// A handle attempting to escape the configured directory gets the
		// same honest answer a genuinely absent id gets: this provider does
		// not carry it. A distinct code here would tell a caller probing ids
		// which failure mode it hit, which is not an answer Vellum owes it.
		return nil, themeNotFound(id)
	}
	data, err := afero.ReadFile(p.fs, path)
	if err != nil {
		return nil, themeNotFound(id)
	}
	return Decode(data)
}

func themeNotFound(id string) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_NOT_FOUND,
		"no theme is registered under this id",
		map[string]any{"theme_id": id})
}
