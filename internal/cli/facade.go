package cli

import (
	"os"
	"strconv"

	"github.com/frankbardon/vellum"
	"github.com/frankbardon/vellum/asset"
	verr "github.com/frankbardon/vellum/errors"
	vfs "github.com/frankbardon/vellum/fs"
	"github.com/frankbardon/vellum/theme"
)

// newFacade constructs the library facade every verb calls, wiring
// VELLUM_THEME_DIR, VELLUM_ASSET_DIR and VELLUM_MAX_ASSET_BYTES from the
// environment when set. This is the one place a Vellum is constructed, so no
// verb file reads these variables itself.
//
// Purely additive when neither directory variable is set: [vellum.Options]
// is left at its zero value in that case, which is the exact behaviour every
// earlier version of newFacade produced — built-in theme only, inline assets
// only. A caller that wants a --theme-dir or --asset-dir *flag* rather than
// an environment variable still builds it here, without touching any verb
// file.
func newFacade() (*vellum.Vellum, error) {
	opts := vellum.Options{}

	if dir := os.Getenv("VELLUM_THEME_DIR"); dir != "" {
		opts.Themes = theme.NewDirProvider(vfs.Default(), dir)
	}
	if dir := os.Getenv("VELLUM_ASSET_DIR"); dir != "" {
		opts.Assets = asset.NewDirResolver(vfs.Default(), dir)
	}

	if raw := os.Getenv("VELLUM_MAX_ASSET_BYTES"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n <= 0 {
			// A malformed VELLUM_MAX_ASSET_BYTES is a configuration mistake
			// the caller should hear about immediately, at the command that
			// happened to construct a facade first — not silently ignored in
			// favour of asset.DefaultMaxBytes, which would let an operator
			// believe a bound is enforced when none is.
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_CLI_USAGE,
				"VELLUM_MAX_ASSET_BYTES does not parse as a positive integer",
				map[string]any{"value": raw})
		}
		opts.AssetOptions.MaxBytes = n
	}

	return vellum.New(opts)
}
