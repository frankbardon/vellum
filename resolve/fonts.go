package resolve

import (
	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/asset"
	"github.com/frankbardon/vellum/capability"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/theme"
	"maps"
)

// resolveFonts turns the theme's declared faces into the font manifest.
//
// The policy, in one place because it is the kind of rule that rots when it is
// spread across four writers:
//
//   - Embeddable, and the format can embed: embed. How much is the theme's
//     Embed mode, and the format decides what it can actually deliver.
//   - Embeddable, and the format cannot embed: reference the family by name and
//     warn. Every OOXML target is this case in v1.
//   - Non-embeddable with a declared substitute: use the substitute and warn.
//   - Non-embeddable with no substitute: a validate-time error, raised by the
//     theme's own validation long before here.
//
// What never happens is a fallback to whatever the machine has installed. That
// is precisely how one specification comes to render differently on two
// machines, which defeats byte-identical output and the consumer dedupe resting
// on it. It is also why nothing in this library reaches
// go-text/typesetting/fontscan, and why a CI gate enforces that rather than a
// convention.
func (r *resolver) resolveFonts() error {
	canEmbed := formatCanEmbedFonts(r.opts.Format)

	for i := range r.theme.Fonts {
		f := &r.theme.Fonts[i]
		face := fragment.Face{
			Role:       f.Role,
			Family:     f.Family,
			Requested:  f.Family,
			Embed:      fragment.EmbedNone,
			AssetIndex: -1,
		}
		where := map[string]any{
			"theme_id":  r.theme.ID,
			"font_role": string(f.Role),
			"family":    f.Family,
			"format":    string(r.opts.Format),
		}

		switch {
		case !f.Embeddable:
			// The theme said the licence forbids embedding and named what to
			// use instead. Its own validation has already refused the case
			// where it named nothing, so a substitute is present here.
			//
			// Whether a substitute is any use at all is the format's answer,
			// not this function's: a target that carries no font programs
			// resolves the family by name, and one that requires every font
			// embedded has nothing to work with. The matrix is asked rather
			// than a condition written here, so the two cannot drift.
			if !formatAllowsUnembeddedFonts(r.opts.Format) {
				return verr.NewCodedErrorWithDetails(verr.VELLUM_FONT_EMBED_UNSUPPORTED,
					"the format requires every font embedded and the theme declared this face non-embeddable",
					withValue(where, "substitute", f.Substitute))
			}
			face.Family = f.Substitute
			face.Substituted = true
			r.warn(verr.NewCodedErrorWithDetails(verr.VELLUM_FONT_SUBSTITUTED,
				"a theme font declared non-embeddable was replaced by its declared substitute",
				withValue(where, "substitute", f.Substitute)))

		case !canEmbed:
			// The format carries no font programs at all, so nothing is
			// embedded and the family is referenced by name.
			//
			// This is a degradation and not a refusal even when the theme
			// demanded a particular embed mode. An embed mode is a licence
			// condition on *how* a program may be embedded — subset only, or
			// unmodified — and not embedding it cannot violate a condition
			// about embedding it. The refusal belongs where the format does
			// embed and Vellum cannot honour the mode; that is the CFF case in
			// PDF, and it is raised there.
			//
			// The feature named is the one the theme's mode corresponds to, so
			// a consumer reading the warning sees the row they can look up.
			r.warn(verr.NewCodedErrorWithDetails(verr.VELLUM_CAPABILITY_DEGRADED,
				"the format carries no font programs, so the family is referenced by name",
				withValue(where, "feature", string(embedFeature(f.Embed)))))

		default:
			// Embeddable, and the format can carry a program. The theme's
			// handle is required and its presence has already been validated.
			idx, err := r.ingestAsset(f.Handle, verr.VELLUM_FONT_UNAVAILABLE)
			if err != nil {
				return err
			}
			face.AssetIndex = idx
			face.Embed = embedPlanFor(f.Embed)

			if hasCFFOutlines(r.assets[idx].Bytes) {
				r.degrade(capability.FeatureFontOutlinesCFF,
					withValue(where, "handle", f.Handle))
			}
		}

		r.faces = append(r.faces, face)
	}
	return nil
}

// formatAllowsUnembeddedFonts asks the matrix whether a family may be
// referenced by name.
//
// PDF/A-2b is the format that says no: an archival document depending on a font
// installed where it is opened is not archival. Every OOXML target says yes,
// which is what every face gets in v1.
func formatAllowsUnembeddedFonts(format artifact.Format) bool {
	e, ok := capability.Lookup(capability.FeatureFontEmbedNone, format)
	return ok && e.Outcome != capability.Rejects
}

// formatCanEmbedFonts asks the matrix rather than deciding.
//
// The matrix is the single declaration of what each format does, so a change
// there changes behaviour here without a second edit — which is the property
// that keeps the declaration from becoming a description written afterwards.
func formatCanEmbedFonts(format artifact.Format) bool {
	for _, f := range []capability.Feature{
		capability.FeatureFontEmbedSubset,
		capability.FeatureFontEmbedWhole,
	} {
		if e, ok := capability.Lookup(f, format); ok && e.Outcome == capability.Renders {
			return true
		}
	}
	return false
}

// embedPlanFor maps the theme's mode to the plan.
//
// EmbedAuto becomes a subset, because a subset is smaller and is what a format
// that can embed at all prefers. A face whose outlines cannot in fact be
// subsetted is caught by the writer that tries, which is the only layer that
// knows what a given font program contains.
func embedPlanFor(m theme.EmbedMode) fragment.EmbedPlan {
	switch m {
	case theme.EmbedWhole:
		return fragment.EmbedWhole
	default:
		return fragment.EmbedSubset
	}
}

// ingestAsset resolves a handle and returns its index in the manifest,
// deduplicating by content hash.
//
// notFound names the code to use when the handle cannot be produced, because
// a missing font and a missing picture are the same failure with different
// consequences and a consumer routing on the code needs them apart.
func (r *resolver) ingestAsset(handle string, notFound verr.Code) (int, error) {
	a, err := asset.Ingest(r.ctx, r.opts.Assets, asset.Request{
		Handle: handle,
		Format: r.opts.Format,
		Accept: capability.AcceptedMedia(r.opts.Format),
	}, r.opts.AssetOptions)
	if err != nil {
		if notFound != verr.VELLUM_ASSET_NOT_FOUND && verr.HasCode(err, verr.VELLUM_ASSET_NOT_FOUND) {
			return -1, verr.WrapCodedErrorWithDetails(err, notFound,
				"the asset resolver could not produce the font program the theme names",
				map[string]any{"handle": handle, "format": string(r.opts.Format)})
		}
		return -1, err
	}

	if idx, ok := r.byHash[a.Hash]; ok {
		return idx, nil
	}
	r.assets = append(r.assets, fragment.Asset{
		Handle:    a.Handle,
		MediaType: a.MediaType,
		Hash:      a.Hash,
		Bytes:     a.Bytes,
		WidthPx:   a.WidthPx,
		HeightPx:  a.HeightPx,
	})
	idx := len(r.assets) - 1
	r.byHash[a.Hash] = idx
	return idx, nil
}

func withValue(where map[string]any, key string, value any) map[string]any {
	out := make(map[string]any, len(where)+1)
	maps.Copy(out, where)
	out[key] = value
	return out
}

// embedFeature is the matrix row an embed mode corresponds to.
//
// EmbedAuto maps to subset, because subsetting is what auto would have done in
// a format that could. A consumer reading the warning can then look up the row
// that produced it.
func embedFeature(m theme.EmbedMode) capability.Feature {
	if m == theme.EmbedWhole {
		return capability.FeatureFontEmbedWhole
	}
	return capability.FeatureFontEmbedSubset
}

// hasCFFOutlines reports whether a font program carries CFF outlines.
//
// Read from the four-byte SFNT version tag rather than by parsing the file: a
// program whose outlines are CFF declares itself as "OTTO", and everything else
// this library accepts is TrueType. That is the whole question here, and
// answering it with a parser would put a font parser in the resolve pass to
// learn one bit.
//
// The bit is worth learning because it is a declared degradation with nothing
// else to report it. Vellum subsets glyf and not CFF, so a CFF face is embedded
// whole — a larger file carrying glyphs the document never draws, which is a
// difference a consumer can act on by supplying a TrueType cut of the same
// family.
func hasCFFOutlines(program []byte) bool {
	return len(program) >= 4 && string(program[:4]) == "OTTO"
}
