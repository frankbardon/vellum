// Package theme is the theme document: the resolved answer to every question
// about how a specification should look, held apart from what it says.
//
// A theme is a document rather than a build-time enum. Vellum ships one — the
// built-in theme — plus the schema, and never a list of brands. Which brands
// exist is the consumer's business, and a library that shipped a list of them
// would make the next consumer fight it.
//
// # Why the model holds no maps
//
// Every collection here is an ordered slice, including the ones that are
// logically keyed — colour roles, mark styles, boxes. Ranging a Go map yields
// its entries in a different order on every run, and a theme is read on the
// output path, so a map here would be a nondeterminism source sitting directly
// upstream of the bytes. Taking a slice makes the disorder unrepresentable
// rather than merely tested against.
//
// Lookup is therefore a linear scan. The collections are small by
// construction: a theme that needed a hash map to find a colour role would
// have too many colour roles.
package theme

import (
	"sort"

	"github.com/frankbardon/vellum/spec"
)

// FormatVersion is the wire version of the theme document this package reads.
const FormatVersion = "1.0"

// BuiltinID is the id of the theme Vellum ships. An empty [spec.Spec.Theme]
// selects it.
const BuiltinID = "default"

// Theme is a complete set of appearance decisions.
type Theme struct {
	// FormatVersion is the wire version this document was authored against.
	FormatVersion string `json:"format_version"`

	// ID identifies the theme to a [Provider]. A specification names it.
	ID string `json:"id"`

	// Name is the human-readable label. Vellum does not interpret it.
	Name string `json:"name,omitempty"`

	// Fonts declares every face this theme uses, with its embedding rights.
	// See [Font] — the rights are declared here because they are a property of
	// the licence, which only whoever authored the theme knows.
	Fonts []Font `json:"fonts"`

	// Colors assigns a value to each colour role.
	Colors []Color `json:"colors"`

	// Type is the type scale: what size body text is, and each heading level.
	Type TypeScale `json:"type"`

	// Spacing is the vertical rhythm.
	Spacing Spacing `json:"spacing"`

	// Marks maps a consumer's mark name to a style.
	//
	// This is the only place a mark name is interpreted, and it is interpreted
	// as data. Nothing in Vellum branches on a mark's value — if anything did,
	// the seam would have leaked and the consumer's vocabulary would have
	// become Vellum's business.
	Marks []MarkStyle `json:"marks,omitempty"`

	// Layouts are the master layouts, one or more per target format.
	Layouts []Layout `json:"layouts"`
}

// Font is one declared face.
type Font struct {
	// Role is the slot this face fills — body text, headings, monospace.
	Role FontRole `json:"role"`

	// Family is the family name written into the document.
	Family string `json:"family"`

	// Handle identifies the font program to the asset resolver. Required when
	// Embeddable is true, because a face that cannot be obtained cannot be
	// embedded, and unused otherwise.
	Handle string `json:"handle,omitempty"`

	// Embeddable declares whether this face's licence permits embedding.
	//
	// Vellum cannot know this and does not try. Embedding rights vary per
	// licence, and the theme is where licence knowledge lives. True means
	// embed; false means use Substitute and warn; false with no Substitute is
	// a validate-time error, never a system fallback.
	Embeddable bool `json:"embeddable"`

	// Embed selects how much of the program to embed. The zero value is
	// EmbedAuto, which is correct for almost every theme.
	Embed EmbedMode `json:"embed,omitempty"`

	// Substitute names the family to use when Embeddable is false. It is a
	// family name rather than a handle: a substitute is by definition a face
	// Vellum does not carry.
	Substitute string `json:"substitute,omitempty"`
}

// EmbedMode is how much of a font program to embed.
type EmbedMode string

const (
	// EmbedAuto subsets where Vellum can and embeds whole where it cannot.
	// The zero value, and the right answer unless a licence says otherwise.
	EmbedAuto EmbedMode = ""

	// EmbedSubset demands a subset. A face whose outlines Vellum cannot subset
	// is a hard error rather than a degradation, because subset-only is a
	// licence condition a library must not quietly override.
	EmbedSubset EmbedMode = "subset"

	// EmbedWhole demands the whole program, for a licence that forbids
	// modifying the font.
	EmbedWhole EmbedMode = "whole"
)

// AllEmbedModes returns the modes, in declaration order.
func AllEmbedModes() []EmbedMode { return []EmbedMode{EmbedAuto, EmbedSubset, EmbedWhole} }

// FontRole names a typographic slot.
type FontRole string

const (
	// FontBody is running prose.
	FontBody FontRole = "body"

	// FontHeading is section titles.
	FontHeading FontRole = "heading"

	// FontMono is tabular figures and code.
	FontMono FontRole = "mono"
)

var allFontRoles = []FontRole{FontBody, FontHeading, FontMono}

// AllFontRoles returns the font roles, in declaration order.
func AllFontRoles() []FontRole { return append([]FontRole(nil), allFontRoles...) }

// Color assigns a value to a colour role.
type Color struct {
	// Role is the semantic slot.
	Role ColorRole `json:"role"`

	// Value is an sRGB hex triplet, uppercase, with no leading hash —
	// the form OOXML carries natively, so the theme is not the layer that
	// has to reformat it.
	Value string `json:"value"`
}

// ColorRole names a semantic colour slot.
//
// Roles rather than named colours, so a theme change is a theme change rather
// than a search for every place "the blue one" was meant.
type ColorRole string

const (
	// ColorBackground is the page or slide ground.
	ColorBackground ColorRole = "background"

	// ColorText is running prose.
	ColorText ColorRole = "text"

	// ColorTextMuted is secondary prose: captions, footnotes, source lines.
	ColorTextMuted ColorRole = "text_muted"

	// ColorHeading is section titles.
	ColorHeading ColorRole = "heading"

	// ColorAccent is the emphasis colour.
	ColorAccent ColorRole = "accent"

	// ColorAccentText is text placed on ColorAccent.
	ColorAccentText ColorRole = "accent_text"

	// ColorRule is hairlines: table borders, dividers.
	ColorRule ColorRole = "rule"

	// ColorTableHeaderBackground is the fill behind a table's header band.
	ColorTableHeaderBackground ColorRole = "table_header_background"

	// ColorTableHeaderText is text in that band.
	ColorTableHeaderText ColorRole = "table_header_text"

	// ColorTableStripe is the alternating body-row fill.
	ColorTableStripe ColorRole = "table_stripe"
)

var allColorRoles = []ColorRole{
	ColorBackground,
	ColorText,
	ColorTextMuted,
	ColorHeading,
	ColorAccent,
	ColorAccentText,
	ColorRule,
	ColorTableHeaderBackground,
	ColorTableHeaderText,
	ColorTableStripe,
}

// AllColorRoles returns the colour roles, in declaration order.
//
// Every role is required of every theme. A theme with a hole in it would be
// discovered by whichever document first used the missing role, which is a
// long way from where the theme was authored.
func AllColorRoles() []ColorRole { return append([]ColorRole(nil), allColorRoles...) }

// TypeScale is the document's type sizes.
type TypeScale struct {
	// Body is running prose.
	Body spec.Length `json:"body"`

	// Headings are the heading sizes, index 0 being level 1. A heading deeper
	// than the scale declares uses the last entry — a slice rather than a
	// fixed set of fields so a theme can declare as many levels as it uses.
	Headings []spec.Length `json:"headings"`

	// Caption is asset captions and table captions.
	Caption spec.Length `json:"caption"`

	// Notes is speaker notes, footnotes and cell comments.
	Notes spec.Length `json:"notes"`

	// TableBody is text inside a table, usually smaller than Body.
	TableBody spec.Length `json:"table_body"`
}

// HeadingSize returns the size for a heading level, 1-based.
//
// A level past the end of the scale takes the last declared size rather than
// failing: an outline deeper than the theme anticipated is a document that
// should still render, and clamping is the conventional answer.
func (t TypeScale) HeadingSize(level int) spec.Length {
	if len(t.Headings) == 0 {
		return t.Body
	}
	if level < 1 {
		level = 1
	}
	if level > len(t.Headings) {
		level = len(t.Headings)
	}
	return t.Headings[level-1]
}

// Spacing is the vertical rhythm.
type Spacing struct {
	// ParagraphBefore and ParagraphAfter bracket a paragraph.
	ParagraphBefore spec.Length `json:"paragraph_before"`
	ParagraphAfter  spec.Length `json:"paragraph_after"`

	// HeadingBefore and HeadingAfter bracket a heading.
	HeadingBefore spec.Length `json:"heading_before"`
	HeadingAfter  spec.Length `json:"heading_after"`

	// LineHeight is the multiple of the type size a line occupies.
	LineHeight float64 `json:"line_height"`
}

// MarkStyle is what a consumer's mark name looks like.
type MarkStyle struct {
	// Name is the consumer's mark. Vellum never learns what it means.
	Name string `json:"name"`

	// Bold, Italic and Underline are the character-level switches.
	Bold      bool `json:"bold,omitempty"`
	Italic    bool `json:"italic,omitempty"`
	Underline bool `json:"underline,omitempty"`

	// Color names a colour role, not a value, so a mark follows the palette
	// rather than pinning a colour the theme has since moved.
	Color ColorRole `json:"color,omitempty"`

	// Background names a colour role for a fill behind the marked content.
	Background ColorRole `json:"background,omitempty"`
}

// LookupColor returns the value for a role, and whether the theme declares it.
func (t *Theme) LookupColor(role ColorRole) (string, bool) {
	for i := range t.Colors {
		if t.Colors[i].Role == role {
			return t.Colors[i].Value, true
		}
	}
	return "", false
}

// LookupFont returns the face for a role, and whether the theme declares it.
func (t *Theme) LookupFont(role FontRole) (*Font, bool) {
	for i := range t.Fonts {
		if t.Fonts[i].Role == role {
			return &t.Fonts[i], true
		}
	}
	return nil, false
}

// LookupMark returns the style for a mark name, and whether the theme declares
// one. A mark with no style is a warning at resolve time, never an error: see
// [errors.VELLUM_MARK_UNKNOWN].
func (t *Theme) LookupMark(name string) (*MarkStyle, bool) {
	for i := range t.Marks {
		if t.Marks[i].Name == name {
			return &t.Marks[i], true
		}
	}
	return nil, false
}

// MarkNames returns every mark the theme styles, sorted bytewise.
//
// Sorted rather than in declaration order because this is an enumeration for a
// consumer to read, and a stable answer is more useful than a faithful one.
func (t *Theme) MarkNames() []string {
	out := make([]string, 0, len(t.Marks))
	for i := range t.Marks {
		out = append(out, t.Marks[i].Name)
	}
	sort.Strings(out)
	return out
}
