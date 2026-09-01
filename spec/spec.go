package spec

import verr "github.com/frankbardon/vellum/errors"

// FormatVersion is the wire version of the specification shape.
const FormatVersion = "1.0"

// BlockKind names a block's type. The vocabulary is closed: seven kinds, all
// format-agnostic, none carrying meaning specific to any one consumer.
type BlockKind string

const (
	// BlockHeading is a titled division of the content.
	BlockHeading BlockKind = "heading"

	// BlockText is a paragraph of prose.
	BlockText BlockKind = "text"

	// BlockAsset embeds an asset the host resolves. Vellum never renders one.
	BlockAsset BlockKind = "asset"

	// BlockTable is an analytical table.
	BlockTable BlockKind = "table"

	// BlockPageBreak starts a new page, slide or sheet, depending on format.
	BlockPageBreak BlockKind = "page_break"

	// BlockNotes is speaker-note or annotation content, native in some formats
	// and degraded in others.
	BlockNotes BlockKind = "notes"

	// BlockSpacer is vertical space.
	BlockSpacer BlockKind = "spacer"
)

// allBlockKinds is the registry. Hand-maintained and ordered; the capability
// matrix and the published schema both read it, so a kind that is declared but
// not listed here is invisible to both.
var allBlockKinds = []BlockKind{
	BlockHeading,
	BlockText,
	BlockAsset,
	BlockTable,
	BlockPageBreak,
	BlockNotes,
	BlockSpacer,
}

// AllBlockKinds returns a copy of the block vocabulary, in declaration order.
func AllBlockKinds() []BlockKind {
	out := make([]BlockKind, len(allBlockKinds))
	copy(out, allBlockKinds)
	return out
}

// ValidBlockKind reports whether k is in the vocabulary.
func ValidBlockKind(k BlockKind) bool {
	for _, v := range allBlockKinds {
		if v == k {
			return true
		}
	}
	return false
}

// Spec describes one artifact.
type Spec struct {
	// FormatVersion is the wire version this specification was authored
	// against.
	FormatVersion string `json:"format_version"`

	// Title is the document title, carried into format metadata.
	Title string `json:"title,omitempty"`

	// Theme names the theme document to render against. Empty selects the
	// built-in theme.
	Theme string `json:"theme,omitempty"`

	// Sections are the ordered divisions of the document. A slice rather than
	// a map, here and everywhere in this model: emitted order is part of the
	// output bytes.
	Sections []Section `json:"sections"`
}

// Section is an ordered run of blocks.
type Section struct {
	// ID identifies the section for a consumer's own bookkeeping. Vellum does
	// not interpret it.
	ID string `json:"id,omitempty"`

	// Layout names the theme master layout to render this section against.
	// Empty selects the theme's default.
	Layout string `json:"layout,omitempty"`

	// Marks are consumer-defined style hooks for the section as a whole.
	Marks []string `json:"marks,omitempty"`

	// Blocks are the section's content, in order.
	Blocks []Block `json:"blocks"`
}

// Block is a tagged union. Exactly one arm is non-nil and Kind names it.
//
// A tagged struct rather than a Go interface, because the model must
// round-trip through strict JSON decoding and reflect into a published schema,
// and an interface does neither without a custom codec that would then be one
// more thing able to drift.
type Block struct {
	// Kind names which arm carries this block's content.
	Kind BlockKind `json:"kind"`

	// Heading is set when Kind is BlockHeading.
	Heading *Heading `json:"heading,omitempty"`

	// Text is set when Kind is BlockText.
	Text *Text `json:"text,omitempty"`

	// Asset is set when Kind is BlockAsset.
	Asset *Asset `json:"asset,omitempty"`

	// Table is set when Kind is BlockTable.
	Table *Table `json:"table,omitempty"`

	// PageBreak is set when Kind is BlockPageBreak.
	PageBreak *PageBreak `json:"page_break,omitempty"`

	// Notes is set when Kind is BlockNotes.
	Notes *Notes `json:"notes,omitempty"`

	// Spacer is set when Kind is BlockSpacer.
	Spacer *Spacer `json:"spacer,omitempty"`

	// Marks are consumer-defined style hooks.
	//
	// Vellum never learns what a mark means. The theme maps a mark name to a
	// style, and nothing in this library branches on a mark's value — if it
	// ever did, the seam would have leaked and the consumer's vocabulary would
	// have become Vellum's business.
	//
	// The motivating case is a document whose underlying data has moved and
	// which must be visibly flagged as stale. Vellum renders the flag without
	// ever learning what "stale" is.
	Marks []string `json:"marks,omitempty"`
}

// Heading is a titled division.
type Heading struct {
	// Level is the outline depth, 1 being the most prominent.
	Level int `json:"level"`

	// Content is the heading text.
	Content string `json:"content"`
}

// Text is a paragraph of prose.
type Text struct {
	// Content is the paragraph text.
	Content string `json:"content"`
}

// Asset embeds an artifact the host resolves.
//
// The block references a handle, never bytes. Keeping bytes out of the model
// is what makes the content hash cheap to compute, and what keeps Vellum
// ignorant of where the host stores anything — which is precisely what makes
// it reusable rather than one product's document writer.
type Asset struct {
	// Handle identifies the asset to the host's resolver. Vellum does not
	// interpret it.
	Handle string `json:"handle"`

	// Role names the theme box this asset fills, so its size comes from the
	// theme rather than from the block. An empty role selects the theme's
	// default asset box.
	Role string `json:"role,omitempty"`

	// AltText is the accessible description.
	AltText string `json:"alt_text,omitempty"`
}

// PageBreak starts a new page, slide or sheet, depending on the target format.
//
// It carries no fields today and is a struct rather than a bare kind so that
// it can gain one — a break type, say — without changing the shape of every
// other block.
type PageBreak struct{}

// Notes is annotation content: speaker notes in a deck, a footnote in a
// document, a cell comment in a workbook. What it becomes in each format is
// declared by the capability matrix rather than discovered at render time.
type Notes struct {
	// Content is the note text.
	Content string `json:"content"`
}

// Spacer is vertical space.
type Spacer struct {
	// Height is the space to insert.
	Height Length `json:"height"`
}

// Validate reports structural problems with the specification.
//
// It checks shape only — that sections and blocks exist, that a block's kind
// is in the vocabulary, and that the arm matching its kind is present. Whether
// a given kind can be rendered in a given format is a separate question, asked
// of the capability matrix, because the answer depends on the target and this
// one does not.
func (s *Spec) Validate() error {
	if s == nil {
		return verr.NewCodedError(verr.VELLUM_SPEC_INVALID, "specification is nil")
	}
	if len(s.Sections) == 0 {
		return verr.NewCodedError(verr.VELLUM_SPEC_INVALID, "specification has no sections")
	}

	for si := range s.Sections {
		sec := &s.Sections[si]
		if len(sec.Blocks) == 0 {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_INVALID,
				"section has no blocks",
				map[string]any{"section_index": si, "section_id": sec.ID})
		}
		for bi := range sec.Blocks {
			if err := sec.Blocks[bi].validate(si, bi, sec.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *Block) validate(sectionIndex, blockIndex int, sectionID string) error {
	where := map[string]any{
		"section_index": sectionIndex,
		"block_index":   blockIndex,
		"kind":          string(b.Kind),
	}
	if sectionID != "" {
		where["section_id"] = sectionID
	}

	if !ValidBlockKind(b.Kind) {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_BLOCK_KIND_UNKNOWN,
			"block declares a kind that is not in the vocabulary", where)
	}

	switch b.Kind {
	case BlockHeading:
		if b.Heading == nil {
			return missingArm(where, "heading")
		}
		if b.Heading.Level < 1 {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_INVALID,
				"heading level must be at least 1", where)
		}
	case BlockText:
		if b.Text == nil {
			return missingArm(where, "text")
		}
	case BlockAsset:
		if b.Asset == nil {
			return missingArm(where, "asset")
		}
		if b.Asset.Handle == "" {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_INVALID,
				"asset block has no handle", where)
		}
	case BlockTable:
		if b.Table == nil {
			return missingArm(where, "table")
		}
		if err := b.Table.Validate(); err != nil {
			return withLocation(err, where)
		}
	case BlockPageBreak:
		if b.PageBreak == nil {
			return missingArm(where, "page_break")
		}
	case BlockNotes:
		if b.Notes == nil {
			return missingArm(where, "notes")
		}
	case BlockSpacer:
		if b.Spacer == nil {
			return missingArm(where, "spacer")
		}
		if _, err := b.Spacer.Height.EMU(); err != nil {
			return withLocation(err, where)
		}
	}

	// A block must not carry an arm for a kind it is not. A spec that sets
	// both a heading and a table has been assembled wrongly, and silently
	// honouring whichever one the discriminator names would hide the mistake.
	if err := b.checkNoStrayArms(where); err != nil {
		return err
	}
	return nil
}

// checkNoStrayArms rejects a block carrying content for a kind other than its
// own.
func (b *Block) checkNoStrayArms(where map[string]any) error {
	present := make([]string, 0, 2)
	add := func(name string, set bool, isOwn bool) {
		if set && !isOwn {
			present = append(present, name)
		}
	}
	add("heading", b.Heading != nil, b.Kind == BlockHeading)
	add("text", b.Text != nil, b.Kind == BlockText)
	add("asset", b.Asset != nil, b.Kind == BlockAsset)
	add("table", b.Table != nil, b.Kind == BlockTable)
	add("page_break", b.PageBreak != nil, b.Kind == BlockPageBreak)
	add("notes", b.Notes != nil, b.Kind == BlockNotes)
	add("spacer", b.Spacer != nil, b.Kind == BlockSpacer)

	if len(present) == 0 {
		return nil
	}
	detail := make(map[string]any, len(where)+1)
	for k, v := range where {
		detail[k] = v
	}
	detail["stray_arms"] = present
	return verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_INVALID,
		"block carries content for a kind other than its own", detail)
}

func missingArm(where map[string]any, arm string) error {
	detail := make(map[string]any, len(where)+1)
	for k, v := range where {
		detail[k] = v
	}
	detail["missing_arm"] = arm
	return verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_INVALID,
		"block declares a kind but carries no content for it", detail)
}

// withLocation re-raises a nested error with the block's coordinates attached,
// so a table fault reports where in the document it is rather than only what
// it is.
func withLocation(err error, where map[string]any) error {
	var ce *verr.CodedError
	if !asCodedError(err, &ce) {
		return err
	}
	detail := make(map[string]any, len(ce.Details)+len(where))
	for k, v := range ce.Details {
		detail[k] = v
	}
	for k, v := range where {
		detail[k] = v
	}
	return verr.NewCodedErrorWithDetails(ce.Code, ce.Message, detail)
}

func asCodedError(err error, target **verr.CodedError) bool {
	for err != nil {
		if ce, ok := err.(*verr.CodedError); ok {
			*target = ce
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
