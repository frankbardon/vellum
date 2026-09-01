package spec

import "github.com/frankbardon/vellum/canon"

// hashTag namespaces a specification hash. See [canon.CanonicalHash].
const hashTag = "spec"

// Hash returns the specification's canonical content hash.
//
// The guarantees, all of which the pinned vectors enforce:
//
//   - the same logical specification produces the same hash across processes
//     and across Vellum versions;
//   - field order does not affect it, so JSON and YAML authoring agree;
//   - defaults are normalised first, so setting a field to its default and
//     omitting it hash alike;
//   - adding a new omitempty field does not move the hash of specifications
//     that omit it.
//
// The last is the one that is easy to break and expensive to discover. A
// consumer keyed on this hash treats a change as "this is a different
// document"; a hash that moved because Vellum gained a field would tell every
// consumer that every document had changed.
func (s *Spec) Hash() string {
	if s == nil {
		return canon.CanonicalHash(hashTag, (*Spec)(nil))
	}
	return canon.CanonicalHash(hashTag, s.normalizedForHash())
}

// normalizedForHash returns a copy with defaults applied and redundant
// spellings collapsed.
//
// It works on a clone. Normalising in place would mean asking for a hash
// quietly rewrote the caller's specification, which is the kind of side effect
// that is invisible until it is catastrophic.
func (s *Spec) normalizedForHash() *Spec {
	out := &Spec{
		FormatVersion: s.FormatVersion,
		Title:         s.Title,
		Theme:         s.Theme,
	}
	if out.FormatVersion == "" {
		out.FormatVersion = FormatVersion
	}
	if out.Theme == "" {
		// An empty theme reference and an explicit "default" select the same
		// theme, so they must hash alike.
		out.Theme = DefaultThemeID
	}

	out.Sections = make([]Section, len(s.Sections))
	for i := range s.Sections {
		out.Sections[i] = normalizeSection(&s.Sections[i])
	}
	return out
}

// DefaultThemeID names the built-in theme. An empty theme reference resolves
// to it.
const DefaultThemeID = "default"

func normalizeSection(in *Section) Section {
	out := Section{
		ID:     in.ID,
		Layout: in.Layout,
		Marks:  normalizeMarks(in.Marks),
	}
	out.Blocks = make([]Block, len(in.Blocks))
	for i := range in.Blocks {
		out.Blocks[i] = normalizeBlock(&in.Blocks[i])
	}
	return out
}

func normalizeBlock(in *Block) Block {
	out := Block{Kind: in.Kind, Marks: normalizeMarks(in.Marks)}
	switch in.Kind {
	case BlockHeading:
		h := *in.Heading
		out.Heading = &h
	case BlockText:
		t := *in.Text
		out.Text = &t
	case BlockAsset:
		a := *in.Asset
		out.Asset = &a
	case BlockTable:
		out.Table = normalizeTable(in.Table)
	case BlockPageBreak:
		out.PageBreak = &PageBreak{}
	case BlockNotes:
		n := *in.Notes
		out.Notes = &n
	case BlockSpacer:
		sp := *in.Spacer
		out.Spacer = &sp
	}
	return out
}

func normalizeTable(in *Table) *Table {
	out := &Table{
		Caption: in.Caption,
		Marks:   normalizeMarks(in.Marks),
	}
	out.ColumnHeaders = normalizeHeaders(in.ColumnHeaders)
	out.RowHeaders = normalizeHeaders(in.RowHeaders)

	out.Body = make([][]Cell, len(in.Body))
	for r := range in.Body {
		out.Body[r] = make([]Cell, len(in.Body[r]))
		for c := range in.Body[r] {
			out.Body[r][c] = normalizeCell(&in.Body[r][c])
		}
	}
	return out
}

// normalizeHeaders collapses a span that merely restates what the tree shape
// already says, so an author who spells it out and one who does not produce
// the same hash.
func normalizeHeaders(in HeaderTree) HeaderTree {
	if len(in) == 0 {
		return nil
	}
	out := make(HeaderTree, len(in))
	for i := range in {
		out[i] = normalizeHeaderNode(&in[i])
	}
	return out
}

func normalizeHeaderNode(in *HeaderNode) HeaderNode {
	out := HeaderNode{
		Label:    in.Label,
		Span:     in.Span,
		Marks:    normalizeMarks(in.Marks),
		Children: normalizeHeaders(in.Children),
	}
	if derived, err := in.width(nil); err == nil && in.Span == derived {
		out.Span = 0
	}
	return out
}

// normalizeCell collapses spans of one, which is what an omitted span means,
// and drops a value arm that carries nothing.
func normalizeCell(in *Cell) Cell {
	out := Cell{
		Text:    in.Text,
		Format:  in.Format,
		Class:   in.Class,
		RowSpan: in.RowSpan,
		ColSpan: in.ColSpan,
		Marks:   normalizeMarks(in.Marks),
	}
	if out.RowSpan == 1 {
		out.RowSpan = 0
	}
	if out.ColSpan == 1 {
		out.ColSpan = 0
	}
	if in.Value != nil && in.Value.Kind != ValueEmpty {
		v := *in.Value
		out.Value = &v
	}
	if len(in.Annotations) > 0 {
		out.Annotations = make([]Annotation, len(in.Annotations))
		for i := range in.Annotations {
			a := in.Annotations[i]
			if a.Position == AnnotationSuperscript {
				// Superscript is the default, so stating it and omitting it
				// must hash alike.
				a.Position = ""
			}
			a.Marks = normalizeMarks(a.Marks)
			out.Annotations[i] = a
		}
	}
	return out
}

// normalizeMarks drops an empty mark slice so that nil and []string{} agree.
// Mark order is preserved: a consumer may layer marks, and the order they are
// applied in is theirs to decide.
func normalizeMarks(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
