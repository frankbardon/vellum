package theme

import (
	"sort"

	"github.com/frankbardon/vellum/artifact"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/spec"
)

// Layout is a master layout for one target format.
//
// A theme carries one or more per format, because a deck's title slide and its
// content slides are different masters, and a document's landscape section is a
// different page setup from its portrait one.
type Layout struct {
	// ID identifies the layout. A section names it.
	ID string `json:"id"`

	// Format is the target this layout applies to. A layout is per-format
	// because the same theme's page is A4 in a document and 16:9 in a deck,
	// and pretending otherwise produces a model that fits neither.
	Format artifact.Format `json:"format"`

	// Default marks the layout a section selects by naming nothing. Exactly
	// one layout per format must be the default.
	Default bool `json:"default,omitempty"`

	// Page is the page, slide or sheet geometry.
	Page Page `json:"page"`

	// Boxes are the asset slots this layout offers, which is what the layout
	// query reports. See [Theme.Boxes].
	Boxes []Box `json:"boxes,omitempty"`
}

// Page is the geometry of one page, slide or sheet.
type Page struct {
	// Width and Height are the trim size.
	Width  spec.Length `json:"width"`
	Height spec.Length `json:"height"`

	// MarginTop, MarginRight, MarginBottom and MarginLeft are the content
	// insets.
	MarginTop    spec.Length `json:"margin_top"`
	MarginRight  spec.Length `json:"margin_right"`
	MarginBottom spec.Length `json:"margin_bottom"`
	MarginLeft   spec.Length `json:"margin_left"`
}

// ContentWidth is the page width less its horizontal margins — the column an
// asset in a flowing format is fitted to.
func (p Page) ContentWidth() (int64, error) {
	w, err := p.Width.EMU()
	if err != nil {
		return 0, err
	}
	l, err := p.MarginLeft.EMU()
	if err != nil {
		return 0, err
	}
	r, err := p.MarginRight.EMU()
	if err != nil {
		return 0, err
	}
	return w - l - r, nil
}

// ContentHeight is the page height less its vertical margins.
func (p Page) ContentHeight() (int64, error) {
	h, err := p.Height.EMU()
	if err != nil {
		return 0, err
	}
	t, err := p.MarginTop.EMU()
	if err != nil {
		return 0, err
	}
	b, err := p.MarginBottom.EMU()
	if err != nil {
		return 0, err
	}
	return h - t - b, nil
}

// BoxRole names an asset slot.
//
// The set is small and enumerable on purpose. The host renders one artifact per
// distinct box, so a per-instance query would make the artifact set grow with a
// document's *layouts* rather than with its *content* — and near-identical
// boxes would produce near-identical artifacts no cache could unify. A
// theme-level set is bounded by construction, and every full-width chart in a
// document is then rendered at exactly the same size, which is the property the
// query exists to deliver.
type BoxRole string

const (
	// BoxAssetFull is an asset spanning the content width.
	BoxAssetFull BoxRole = "asset.full"

	// BoxAssetHalf is an asset at half the content width, for a two-up row.
	BoxAssetHalf BoxRole = "asset.half"

	// BoxAssetQuarter is an asset at a quarter of the content width.
	BoxAssetQuarter BoxRole = "asset.quarter"

	// BoxLogo is the brand slot a theme places in a header, footer or master.
	BoxLogo BoxRole = "logo"
)

var allBoxRoles = []BoxRole{BoxAssetFull, BoxAssetHalf, BoxAssetQuarter, BoxLogo}

// AllBoxRoles returns the box roles, in declaration order.
func AllBoxRoles() []BoxRole { return append([]BoxRole(nil), allBoxRoles...) }

// DefaultBoxRole is the box an asset block selects by naming none.
const DefaultBoxRole = BoxAssetFull

// Box is one asset slot: the size the host should render at.
type Box struct {
	// Role names the slot.
	Role BoxRole `json:"role"`

	// Width is the slot's width. Always concrete.
	Width spec.Length `json:"width"`

	// Height is the slot's height. Zero means the height follows the asset's
	// own aspect ratio.
	//
	// That is not vagueness. In a fixed-geometry format — a slide — the theme
	// declares both dimensions and the answer is a rectangle. In a flowing
	// format the width is the theme's column width and the height is whatever
	// the asset's aspect ratio makes it, so a declared height would be a
	// constraint the theme has no basis to impose. Zero says which of the two
	// this is, rather than inventing a number.
	Height spec.Length `json:"height,omitempty"`
}

// IntrinsicHeight reports whether this box takes its height from the asset.
func (b Box) IntrinsicHeight() bool { return b.Height.IsZero() }

// BoxSet is a layout's boxes, in a stable order.
//
// A slice with a lookup rather than a map, for the reason the package doc
// gives: this is read on the output path, and a map ranged there is a
// nondeterminism source sitting directly upstream of the bytes.
type BoxSet []Box

// Lookup returns the box for a role and whether the set declares it.
func (s BoxSet) Lookup(role BoxRole) (Box, bool) {
	for _, b := range s {
		if b.Role == role {
			return b, true
		}
	}
	return Box{}, false
}

// Roles returns the declared roles, in the set's order.
func (s BoxSet) Roles() []BoxRole {
	out := make([]BoxRole, 0, len(s))
	for _, b := range s {
		out = append(out, b.Role)
	}
	return out
}

// Boxes reports the asset slots this theme offers for a format.
//
// This is the layout query. It is answered per (theme, format, role) from the
// theme's master layouts, never per block instance, and it is therefore
// answerable *before a specification exists* — a host can pre-render, cache and
// warm against a theme rather than against a document.
//
// The failure it prevents is not blurring; a vector asset scales cleanly. It is
// that text scales with the graphic. A chart scaled into a half-width box gets
// half-size axis labels while a full-width chart gets full-size ones, so type
// size varies arbitrarily across a document and matches the theme's scale
// nowhere. A renderer whose default output is viewBox-only makes that the
// *default* outcome unless it is told the target box.
//
// The result unions every layout the theme declares for the format, so the
// answer is every box the format can present, not merely the default layout's.
// It is sorted by role so two calls agree.
func (t *Theme) Boxes(format artifact.Format) BoxSet {
	var out BoxSet
	seen := make(map[BoxRole]bool)
	for i := range t.Layouts {
		l := &t.Layouts[i]
		if l.Format != format {
			continue
		}
		for _, b := range l.Boxes {
			if seen[b.Role] {
				continue
			}
			seen[b.Role] = true
			out = append(out, b)
		}
	}
	// Sorted rather than left in declaration order: the union of several
	// layouts has no declaration order of its own, and a caller enumerating
	// render presets needs the same list every time.
	sort.Slice(out, func(i, j int) bool { return out[i].Role < out[j].Role })
	return out
}

// LayoutFor resolves the layout a section should render against.
//
// An empty id selects the format's default layout. A named id that the theme
// does not declare for this format is an error rather than a fallback: a
// section rendered against a layout it did not ask for is wrong in a way that
// looks right.
func (t *Theme) LayoutFor(format artifact.Format, id string) (*Layout, error) {
	var fallback *Layout
	for i := range t.Layouts {
		l := &t.Layouts[i]
		if l.Format != format {
			continue
		}
		if id != "" && l.ID == id {
			return l, nil
		}
		if id == "" && l.Default {
			return l, nil
		}
		if fallback == nil {
			fallback = l
		}
	}
	if id == "" && fallback != nil {
		// Reached only when the theme declares layouts for this format but
		// marks none of them default. Validate rejects that, so this arm
		// exists for a Theme assembled in Go without passing through it.
		return fallback, nil
	}
	return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_LAYOUT_NOT_FOUND,
		"the theme declares no layout under this id for the target format",
		map[string]any{
			"theme_id":  t.ID,
			"layout_id": id,
			"format":    string(format),
			"available": t.layoutIDs(format),
		})
}

// BoxFor resolves the box an asset block should be rendered into.
//
// An empty role selects [DefaultBoxRole].
func (l *Layout) BoxFor(role BoxRole) (Box, error) {
	if role == "" {
		role = DefaultBoxRole
	}
	if b, ok := BoxSet(l.Boxes).Lookup(role); ok {
		return b, nil
	}
	return Box{}, verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_BOX_NOT_FOUND,
		"the resolved layout declares no box under this role",
		map[string]any{
			"layout_id": l.ID,
			"format":    string(l.Format),
			"box_role":  string(role),
			"available": roleStrings(BoxSet(l.Boxes).Roles()),
		})
}

// layoutIDs lists the layouts declared for a format, sorted, for an error's
// details. Sorted so the same failure reads the same way twice.
func (t *Theme) layoutIDs(format artifact.Format) []string {
	var out []string
	for i := range t.Layouts {
		if t.Layouts[i].Format == format {
			out = append(out, t.Layouts[i].ID)
		}
	}
	sort.Strings(out)
	return out
}

func roleStrings(roles []BoxRole) []string {
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		out = append(out, string(r))
	}
	sort.Strings(out)
	return out
}
