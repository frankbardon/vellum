// Package fs is the filesystem seam every directory-backed provider or
// resolver in Vellum is built over.
//
// Vellum's default construction touches no filesystem at all: the built-in
// theme is embedded and the default asset resolver ([asset.Inline]) serves
// only inline data URIs. A host that opts into a directory of its own —
// through [theme.DirProvider], [asset.DirResolver], or the CLI's
// VELLUM_THEME_DIR / VELLUM_ASSET_DIR — needs a filesystem to read from, and
// this package is the one place that names which one. Every directory-backed
// type in this codebase is constructed against an afero.Fs rather than
// against the OS filesystem directly, so its tests can run hermetically
// against afero.NewMemMapFs() and never touch a real disk — see CLAUDE.md's
// "Determinism" section.
package fs

import (
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

// Default returns the filesystem backing ordinary, non-test operation: the
// real OS filesystem, wrapped so every caller in this codebase depends on
// afero.Fs rather than on the os package directly.
func Default() afero.Fs {
	return afero.NewOsFs()
}

// SafeJoin joins root and rel, treating rel as a path relative to root, and
// reports whether the result stays within root.
//
// This is the guard every directory-backed [theme.Provider] and
// [asset.Resolver] needs and neither may skip: rel is a handle read out of a
// specification, which this library did not necessarily author, and
// filepath.Join alone is not a safe way to resolve it — Join cleans ".."
// segments away silently, so "../../etc/passwd" joins to a path outside root
// without complaint. SafeJoin reports false instead of returning a path that
// merely looks plausible, and the caller is expected to treat false exactly
// as it would a file that does not exist: this package has no opinion about
// which coded error that becomes.
func SafeJoin(root, rel string) (string, bool) {
	cleanRoot := filepath.Clean(root)
	joined := filepath.Clean(filepath.Join(cleanRoot, rel))
	if joined == cleanRoot {
		// rel resolved to the root itself, not a file under it.
		return "", false
	}
	if !strings.HasPrefix(joined, cleanRoot+string(filepath.Separator)) {
		return "", false
	}
	return joined, true
}
