package asset

import (
	"context"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fs"
	"github.com/spf13/afero"
)

// DirResolver resolves a handle to bytes read from a file under a directory.
//
// It is the one [Resolver] in this package that does treat the handle as a
// path. [Request.Handle]'s own doc comment says a resolver "does not
// interpret it, does not treat it as a path" in general — true of [Inline]
// and [Map], both of which key on the handle as an opaque string — but a
// directory-backed resolver is exactly the implementation that must
// interpret it as one, in order to find the file at all. [fs.SafeJoin] is
// what keeps a handle like "../../etc/passwd", read off a specification this
// library did not author, from escaping the directory this resolver was
// configured to serve.
//
// MediaType is left unset on the returned [Asset], the same choice [Inline]
// makes for a data URI carrying no declared type: [Ingest] already sniffs
// the bytes with [SniffMedia] when a resolver leaves MediaType empty, so
// DirResolver reuses that path rather than sniffing a second time.
//
// A handle SafeJoin rejects, or that names a file the directory does not
// have, is VELLUM_ASSET_NOT_FOUND — the same code and the same honest answer
// either way, rather than a distinct code that would tell a caller probing
// handles which failure mode it hit.
type DirResolver struct {
	fs   afero.Fs
	root string
}

// NewDirResolver constructs a DirResolver serving assets from root on fsys.
func NewDirResolver(fsys afero.Fs, root string) *DirResolver {
	return &DirResolver{fs: fsys, root: root}
}

// Resolve implements [Resolver].
func (r *DirResolver) Resolve(_ context.Context, req Request) (*Asset, error) {
	path, ok := fs.SafeJoin(r.root, req.Handle)
	if !ok {
		return nil, assetNotFound(req)
	}
	data, err := afero.ReadFile(r.fs, path)
	if err != nil {
		return nil, assetNotFound(req)
	}
	return &Asset{Handle: req.Handle, Bytes: data}, nil
}

func assetNotFound(req Request) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_ASSET_NOT_FOUND,
		"no asset file exists under this handle in the resolver's configured directory",
		map[string]any{"handle": truncateHandle(req.Handle), "format": string(req.Format)})
}
