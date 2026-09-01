package theme

import (
	"fmt"
	"maps"
	"regexp"
	"sort"

	"github.com/frankbardon/vellum/artifact"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/spec"
)

// hexColor is an uppercase sRGB triplet with no leading hash — the form OOXML
// carries natively, so the theme is not the layer that has to reformat it.
var hexColor = regexp.MustCompile(`^[0-9A-F]{6}$`)

// Validate reports structural problems with the theme document.
//
// It is strict about completeness rather than permissive with defaults. A
// theme missing a colour role could be defaulted, and the hole would then be
// discovered by whichever document first used that role — a long way, in time
// and in blame, from where the theme was authored. Failing here puts the error
// where the mistake is.
func (t *Theme) Validate() error {
	if t == nil {
		return verr.NewCodedError(verr.VELLUM_THEME_INVALID, "the theme is nil")
	}
	if t.FormatVersion != FormatVersion {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
			"unsupported theme format version",
			map[string]any{"format_version": t.FormatVersion, "supported": FormatVersion})
	}
	if t.ID == "" {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
			"the theme has no id", map[string]any{"field": "id"})
	}
	if err := t.validateFonts(); err != nil {
		return err
	}
	if err := t.validateColors(); err != nil {
		return err
	}
	if err := t.validateType(); err != nil {
		return err
	}
	if err := t.validateMarks(); err != nil {
		return err
	}
	return t.validateLayouts()
}

func (t *Theme) validateFonts() error {
	seen := make(map[FontRole]bool, len(t.Fonts))
	for i := range t.Fonts {
		f := &t.Fonts[i]
		where := map[string]any{"theme_id": t.ID, "font_index": i, "font_role": string(f.Role)}

		if !validFontRole(f.Role) {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"unknown font role", withList(where, "known", fontRoleStrings()))
		}
		if seen[f.Role] {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"two fonts claim the same role", where)
		}
		seen[f.Role] = true

		if f.Family == "" {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"the font declares no family", where)
		}
		if !validEmbedMode(f.Embed) {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"unknown embed mode", withList(where, "known", embedModeStrings()))
		}

		// The font policy in one place. Embeddable means embed, so a handle is
		// required — a face declared embeddable with nothing to embed is a
		// promise the theme cannot keep. Non-embeddable means substitute, so a
		// substitute is required; neither is the error that exists to stop a
		// silent system fallback, which is precisely how one specification
		// comes to render differently on two machines.
		switch {
		case f.Embeddable && f.Handle == "":
			return verr.NewCodedErrorWithDetails(verr.VELLUM_FONT_UNAVAILABLE,
				"the font is declared embeddable but names no handle", where)
		case !f.Embeddable && f.Substitute == "":
			return verr.NewCodedErrorWithDetails(verr.VELLUM_FONT_NOT_EMBEDDABLE,
				"the font is declared non-embeddable and names no substitute", where)
		case !f.Embeddable && f.Embed != EmbedAuto:
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"a non-embeddable font declares an embed mode",
				withValue(where, "embed", string(f.Embed)))
		}
	}

	for _, role := range allFontRoles {
		if !seen[role] {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"the theme declares no font for a required role",
				map[string]any{"theme_id": t.ID, "font_role": string(role),
					"required": fontRoleStrings()})
		}
	}
	return nil
}

func (t *Theme) validateColors() error {
	seen := make(map[ColorRole]bool, len(t.Colors))
	for i := range t.Colors {
		c := &t.Colors[i]
		where := map[string]any{"theme_id": t.ID, "color_index": i, "color_role": string(c.Role)}

		if !validColorRole(c.Role) {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"unknown colour role", withList(where, "known", colorRoleStrings()))
		}
		if seen[c.Role] {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"two colours claim the same role", where)
		}
		seen[c.Role] = true

		if !hexColor.MatchString(c.Value) {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"the colour value is not an uppercase six-digit sRGB hex triplet",
				withValue(where, "value", c.Value))
		}
	}
	for _, role := range allColorRoles {
		if !seen[role] {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"the theme declares no colour for a required role",
				map[string]any{"theme_id": t.ID, "color_role": string(role),
					"required": colorRoleStrings()})
		}
	}
	return nil
}

func (t *Theme) validateType() error {
	sizes := []struct {
		field string
		value spec.Length
	}{
		{"type.body", t.Type.Body},
		{"type.caption", t.Type.Caption},
		{"type.notes", t.Type.Notes},
		{"type.table_body", t.Type.TableBody},
	}
	for _, s := range sizes {
		if err := requirePositive(t.ID, s.field, s.value); err != nil {
			return err
		}
	}
	if len(t.Type.Headings) == 0 {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
			"the type scale declares no heading sizes",
			map[string]any{"theme_id": t.ID, "field": "type.headings"})
	}
	for i, h := range t.Type.Headings {
		if err := requirePositive(t.ID, fmt.Sprintf("type.headings[%d]", i), h); err != nil {
			return err
		}
	}
	if t.Spacing.LineHeight <= 0 {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
			"the line height must be positive",
			map[string]any{"theme_id": t.ID, "field": "spacing.line_height",
				"value": t.Spacing.LineHeight})
	}
	return nil
}

func (t *Theme) validateMarks() error {
	seen := make(map[string]bool, len(t.Marks))
	for i := range t.Marks {
		m := &t.Marks[i]
		where := map[string]any{"theme_id": t.ID, "mark_index": i, "mark_name": m.Name}
		if m.Name == "" {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"a mark style has no name", where)
		}
		if seen[m.Name] {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"two mark styles share a name", where)
		}
		seen[m.Name] = true

		// A mark names a colour *role*, never a value, so a mark follows the
		// palette rather than pinning a colour the theme has since moved. A
		// role the theme does not declare would resolve to nothing at render
		// time, so it is caught here.
		// An ordered slice rather than a map literal: with both fields wrong,
		// ranging a map would report whichever one the runtime reached first,
		// so the same broken theme would fail two different ways.
		refs := []struct {
			field string
			role  ColorRole
		}{{"color", m.Color}, {"background", m.Background}}
		for _, ref := range refs {
			field, role := ref.field, ref.role
			if role == "" {
				continue
			}
			if _, ok := t.LookupColor(role); !ok {
				return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
					"a mark style names a colour role the theme does not declare",
					withValue(withValue(where, "field", field), "color_role", string(role)))
			}
		}
	}
	return nil
}

func (t *Theme) validateLayouts() error {
	if len(t.Layouts) == 0 {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
			"the theme declares no layouts", map[string]any{"theme_id": t.ID})
	}

	seenID := make(map[string]bool, len(t.Layouts))
	defaults := make(map[artifact.Format]int, len(artifact.AllFormats()))
	present := make(map[artifact.Format]bool, len(artifact.AllFormats()))

	for i := range t.Layouts {
		l := &t.Layouts[i]
		where := map[string]any{"theme_id": t.ID, "layout_index": i,
			"layout_id": l.ID, "format": string(l.Format)}

		if l.ID == "" {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"a layout has no id", where)
		}
		key := string(l.Format) + "\x00" + l.ID
		if seenID[key] {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"two layouts for the same format share an id", where)
		}
		seenID[key] = true

		if !artifact.ValidFormat(l.Format) {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"a layout names an unknown format",
				withList(where, "known", formatStrings()))
		}
		present[l.Format] = true
		if l.Default {
			defaults[l.Format]++
		}

		if err := validatePage(l.Page, where); err != nil {
			return err
		}
		if err := validateBoxes(l, where); err != nil {
			return err
		}
	}

	// Exactly one default per format the theme covers. Zero would make an
	// unnamed layout ambiguous; two would make it arbitrary. A format the
	// theme covers not at all is legitimate — a theme may be for documents
	// only — and is a lookup failure at resolve time rather than a broken
	// theme.
	for _, f := range artifact.AllFormats() {
		if !present[f] {
			continue
		}
		if defaults[f] != 1 {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"a format must have exactly one default layout",
				map[string]any{"theme_id": t.ID, "format": string(f),
					"default_count": defaults[f]})
		}
	}
	return nil
}

func validatePage(p Page, where map[string]any) error {
	dims := []struct {
		field string
		value spec.Length
	}{{"page.width", p.Width}, {"page.height", p.Height}}
	for _, d := range dims {
		if d.value.IsZero() {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"the layout declares no page size", withValue(where, "field", d.field))
		}
		emu, err := d.value.EMU()
		if err != nil {
			return verr.WrapCodedErrorWithDetails(err, verr.VELLUM_THEME_INVALID,
				"the page dimension is not a valid length", withValue(where, "field", d.field))
		}
		if emu <= 0 {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"the page dimension must be positive", withValue(where, "field", d.field))
		}
	}

	// Margins that meet or cross leave no content column, which would make
	// every box on the page zero or negative wide. Checked here so the layout
	// query cannot return a nonsense answer.
	cw, err := p.ContentWidth()
	if err != nil {
		return verr.WrapCodedErrorWithDetails(err, verr.VELLUM_THEME_INVALID,
			"the page margins are not valid lengths", where)
	}
	ch, err := p.ContentHeight()
	if err != nil {
		return verr.WrapCodedErrorWithDetails(err, verr.VELLUM_THEME_INVALID,
			"the page margins are not valid lengths", where)
	}
	if cw <= 0 || ch <= 0 {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
			"the page margins leave no content area",
			withValue(withValue(where, "content_width_emu", cw), "content_height_emu", ch))
	}
	return nil
}

func validateBoxes(l *Layout, where map[string]any) error {
	seen := make(map[BoxRole]bool, len(l.Boxes))
	for _, b := range l.Boxes {
		bw := withValue(where, "box_role", string(b.Role))
		if !validBoxRole(b.Role) {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"unknown box role", withList(bw, "known", boxRoleStrings()))
		}
		if seen[b.Role] {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"two boxes in one layout claim the same role", bw)
		}
		seen[b.Role] = true

		if b.Width.IsZero() {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"a box declares no width; only its height may be intrinsic", bw)
		}
		w, err := b.Width.EMU()
		if err != nil {
			return verr.WrapCodedErrorWithDetails(err, verr.VELLUM_THEME_INVALID,
				"the box width is not a valid length", bw)
		}
		if w <= 0 {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"the box width must be positive", bw)
		}
		if !b.Height.IsZero() {
			h, err := b.Height.EMU()
			if err != nil {
				return verr.WrapCodedErrorWithDetails(err, verr.VELLUM_THEME_INVALID,
					"the box height is not a valid length", bw)
			}
			if h <= 0 {
				return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
					"a declared box height must be positive; zero means intrinsic", bw)
			}
		}
	}
	return nil
}

func requirePositive(themeID, field string, l spec.Length) error {
	where := map[string]any{"theme_id": themeID, "field": field}
	if l.IsZero() {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
			"the type scale declares no size for this slot", where)
	}
	emu, err := l.EMU()
	if err != nil {
		return verr.WrapCodedErrorWithDetails(err, verr.VELLUM_THEME_INVALID,
			"the size is not a valid length", where)
	}
	if emu <= 0 {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
			"the size must be positive", where)
	}
	return nil
}

// withValue and withList copy a details map before adding to it, so a caller's
// map is never mutated by an error that merely borrowed it.
func withValue(where map[string]any, key string, value any) map[string]any {
	out := make(map[string]any, len(where)+1)
	maps.Copy(out, where)
	out[key] = value
	return out
}

func withList(where map[string]any, key string, values []string) map[string]any {
	return withValue(where, key, values)
}

func validFontRole(r FontRole) bool {
	for _, k := range allFontRoles {
		if k == r {
			return true
		}
	}
	return false
}

func validColorRole(r ColorRole) bool {
	for _, k := range allColorRoles {
		if k == r {
			return true
		}
	}
	return false
}

func validBoxRole(r BoxRole) bool {
	for _, k := range allBoxRoles {
		if k == r {
			return true
		}
	}
	return false
}

func validEmbedMode(m EmbedMode) bool {
	for _, k := range AllEmbedModes() {
		if k == m {
			return true
		}
	}
	return false
}

func fontRoleStrings() []string {
	out := make([]string, 0, len(allFontRoles))
	for _, r := range allFontRoles {
		out = append(out, string(r))
	}
	sort.Strings(out)
	return out
}

func colorRoleStrings() []string {
	out := make([]string, 0, len(allColorRoles))
	for _, r := range allColorRoles {
		out = append(out, string(r))
	}
	sort.Strings(out)
	return out
}

func boxRoleStrings() []string { return roleStrings(allBoxRoles) }

func embedModeStrings() []string {
	out := make([]string, 0, len(AllEmbedModes()))
	for _, m := range AllEmbedModes() {
		out = append(out, string(m))
	}
	sort.Strings(out)
	return out
}

func formatStrings() []string {
	all := artifact.AllFormats()
	out := make([]string, 0, len(all))
	for _, f := range all {
		out = append(out, string(f))
	}
	sort.Strings(out)
	return out
}
