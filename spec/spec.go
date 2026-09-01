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
			return verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_INVALID,
				"block is a heading but carries no heading content", where)
		}
		if b.Heading.Level < 1 {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_INVALID,
				"heading level must be at least 1", where)
		}
	case BlockText:
		if b.Text == nil {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_INVALID,
				"block is text but carries no text content", where)
		}
	}
	return nil
}
