package errors

// FixupAction names the kind of change that resolves an error. It is a small
// closed vocabulary so a tool — or an LLM composing a spec — can act on a
// fixup mechanically rather than parsing the hint prose.
type FixupAction string

const (
	// FixupSetField sets or corrects a field at Path.
	FixupSetField FixupAction = "SET_FIELD"

	// FixupRemoveField removes the field at Path.
	FixupRemoveField FixupAction = "REMOVE_FIELD"

	// FixupReplaceValue substitutes one of Examples for the value at Path.
	FixupReplaceValue FixupAction = "REPLACE_VALUE"

	// FixupChangeFormat retargets the render to a different output format,
	// because the requested feature is not available in the one asked for.
	FixupChangeFormat FixupAction = "CHANGE_FORMAT"

	// FixupSupplyAsset provides an asset the resolver could not produce, or
	// provides it in an accepted media type.
	FixupSupplyAsset FixupAction = "SUPPLY_ASSET"

	// FixupSupplyTheme corrects the theme document — a missing font
	// declaration, an undeclared mark, an absent layout.
	FixupSupplyTheme FixupAction = "SUPPLY_THEME"

	// FixupRepairInput indicates the supplied bytes are damaged and must be
	// replaced. It is the honest answer for a truncated archive: there is no
	// field to edit.
	FixupRepairInput FixupAction = "REPAIR_INPUT"

	// FixupRaiseLimit increases a configured bound that the input exceeded.
	FixupRaiseLimit FixupAction = "RAISE_LIMIT"
)

// Fixup is one actionable suggestion for resolving an error.
type Fixup struct {
	// Action names the mechanical change.
	Action FixupAction `json:"action"`

	// Path locates the field to change, as an abstract pointer with "*"
	// standing for any index: ["Sections", "*", "Blocks", "*", "Kind"].
	// Concrete index resolution belongs to whatever surfaces the error, which
	// has the actual document in hand; the metadata table does not.
	Path []string `json:"path,omitempty"`

	// Hint is one imperative sentence addressed to the person who must act.
	Hint string `json:"hint"`

	// Examples are concrete acceptable values, where a small closed set
	// exists.
	Examples []any `json:"examples,omitempty"`
}

// Metadata is the registry row for a code: its canonical message and the
// guidance attached to it.
type Metadata struct {
	// Message is the canonical human description, independent of any
	// particular occurrence.
	Message string `json:"message"`

	// Fixups are the suggested resolutions. Empty if and only if
	// FixupNotApplicable is set.
	Fixups []Fixup `json:"fixups,omitempty"`

	// FixupNotApplicable marks a code no caller can act on — an internal
	// invariant, or a condition whose only resolution is "report a bug".
	// It is the honest signal, and it must be chosen deliberately: it is not
	// a way past the gate for a code whose guidance has not been written yet.
	FixupNotApplicable bool `json:"fixup_not_applicable,omitempty"`
}

// MetadataFor returns the registry row for code. The second result reports
// whether the code has one; a registered code without a row fails
// TestCodesHaveFixups, so a false here in production means the code itself is
// unregistered.
func MetadataFor(code Code) (Metadata, bool) {
	m, ok := codeMetadata[code]
	return m, ok
}

// Fixups returns the suggested resolutions for code, or nil when the code is
// unknown or marked not-applicable.
func (c Code) Fixups() []Fixup {
	m, ok := codeMetadata[c]
	if !ok || m.FixupNotApplicable {
		return nil
	}
	out := make([]Fixup, len(m.Fixups))
	copy(out, m.Fixups)
	return out
}

// LookupResult is the full public description of a code, as served by the
// CLI's lookup verb and the corresponding MCP tool.
type LookupResult struct {
	Code     Code   `json:"code"`
	Domain   string `json:"domain"`
	Metadata `json:",inline"`
}

// Lookup returns the full description of code.
func Lookup(code Code) (LookupResult, bool) {
	m, ok := codeMetadata[code]
	if !ok {
		return LookupResult{}, false
	}
	return LookupResult{Code: code, Domain: code.Domain(), Metadata: m}, true
}
