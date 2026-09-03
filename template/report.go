package template

import "github.com/frankbardon/vellum/template/anchor"

// InspectReport is the full result of inspecting a fill-mode template: FR-F7's
// "anchor inventory and font requirements", together. It is what [Inspect]
// returns.
//
// The struct is JSON-tagged end to end so an agent-facing caller can marshal
// it directly — E12's --json path wraps it in a [descriptor.Envelope] the
// same way every other --json path in this codebase does; this package does
// not build its own envelope. [InspectReport.AnchorsTable] and
// [InspectReport.FontsTable] cover the human-facing half: rows of strings a
// column-padded terminal renderer can print without needing to know this
// struct's field layout. Padding and aligning those rows into terminal
// output is epic E12's job, not this package's.
type InspectReport struct {
	// Anchors is every fillable location the template's own XML declares, in
	// the document order [anchor.Inventory] establishes. Flattened directly
	// onto this struct — rather than nesting an anchor.Inventory value — so
	// the wire shape is {"anchors": [...], "fonts": [...]}, not a doubled
	// "anchors" key one level down.
	Anchors []anchor.Anchor `json:"anchors"`

	// Fonts is every distinct font family the template's own XML references,
	// sorted by family name. Vellum reports the names a template already
	// states via w:rFonts; it neither discovers nor validates an embedded
	// font program (fill mode's compose-mode counterpart never embeds one
	// into a .docx either — see font.embed.none in capability/matrix.go). A
	// caller checking availability resolves each name against its own font
	// inventory; that check is not this package's job.
	Fonts []FontRequirement `json:"fonts"`
}

// AnchorsTable renders the anchor inventory as rows of strings: a header row
// naming the columns, then one row per anchor in [InspectReport.Anchors]'s
// own order. Columns are Name, Kind, Alias, Part.
//
// This is data shaping only. Column padding, truncation and terminal
// alignment are epic E12's concern; a nil receiver still returns the header
// row so a caller need not nil-check before ranging the result.
func (r *InspectReport) AnchorsTable() [][]string {
	rows := [][]string{{"Name", "Kind", "Alias", "Part"}}
	if r == nil {
		return rows
	}
	for _, a := range r.Anchors {
		rows = append(rows, []string{a.Name, string(a.Kind), a.Alias, a.Part})
	}
	return rows
}

// FontsTable renders the font requirements as rows of strings: a header row,
// then one row per distinct family in [InspectReport.Fonts]'s own (sorted)
// order. Columns are Family and Categories — the family's w:rFonts attribute
// categories, comma-joined in the fixed order [FontRequirement.Categories]
// itself is reported in.
func (r *InspectReport) FontsTable() [][]string {
	rows := [][]string{{"Family", "Categories"}}
	if r == nil {
		return rows
	}
	for _, f := range r.Fonts {
		rows = append(rows, []string{f.Family, f.categoriesJoined()})
	}
	return rows
}
