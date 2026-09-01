// Package font embeds font programs in a PDF.
//
// A face reaches here already decided: the theme said whether its licence
// permits embedding and how much of it to embed, and the resolver turned that
// into a plan. This package carries the plan out. It does not choose a font, it
// does not fall back, and it never reaches for one the machine happens to have
// installed — that is what makes the same specification render identically on
// two machines, and it is enforced by a CI gate rather than left to
// convention.
//
// # What PDF/A demands
//
// Every face used must be embedded. There is no "reference it by name" for a
// conforming file, which is why this cannot be deferred behind a flag: a PDF
// Vellum writes either embeds its fonts or is not the artifact it claims to be.
//
// # Outlines
//
// TrueType outlines are subsetted; see [sfnt]. A CFF-housed face is embedded
// whole, which the capability matrix declares as a degradation rather than
// leaving a consumer to discover it. Subsetting CFF means a second outline
// format and a charstring interpreter, and the size it saves is not worth the
// ways it can be subtly wrong. A theme demanding a subset of a CFF face gets a
// hard error instead of the whole program: subset-only may be a licence
// condition, and quietly embedding everything would substitute Vellum's
// judgement for the vendor's.
package font

import (
	"encoding/binary"
	"sort"
	"strconv"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/font/sfnt"
	"github.com/frankbardon/vellum/pdf/object"
)

// glyphSpace is the grid PDF measures glyphs on: one thousandth of an em.
const glyphSpace = 1000

// Plan is what to do with a face's program.
//
// The values match [fragment.EmbedPlan]'s, without importing it: a font writer
// that depended on the resolved intermediate model would be a format package
// reaching back up the pipeline, and the lowering that connects them is where
// the translation belongs.
type Plan string

const (
	// PlanSubset embeds only the glyphs the document uses.
	PlanSubset Plan = "subset"

	// PlanWhole embeds the entire font program.
	PlanWhole Plan = "whole"
)

// Options describes a face to embed.
type Options struct {
	// Resource is the name a content stream selects this font by.
	Resource object.Name

	// BaseName is the PostScript name written into the file, before the subset
	// prefix is added.
	BaseName string

	// Program is the font program.
	Program []byte

	// Plan is the embedding decision the theme and the resolver reached. The
	// zero value is rejected rather than defaulted: "embed some unspecified
	// amount" is not a decision anybody made.
	Plan Plan

	// Serif and FixedPitch describe the face for the font descriptor.
	//
	// Stated rather than inferred. The OS/2 panose bytes are frequently wrong,
	// and a wrong descriptor flag changes how a reader substitutes when it
	// cannot use the embedded program — which is the situation the flags exist
	// for and the one nobody tests.
	Serif      bool
	FixedPitch bool
}

// Face is a font ready to embed, tracking the glyphs the document uses.
//
// Usage is accumulated here rather than assembled by the caller, because the
// subset and the content stream must agree about exactly which glyphs exist: a
// content stream showing a glyph the subset dropped renders as nothing, and the
// two are written by different code at different times. Making the same object
// answer both questions removes the opportunity to disagree.
type Face struct {
	opts    Options
	program *sfnt.Font

	// used is the glyph set, kept as a membership test plus an ordered list, so
	// the subset is a function of which glyphs were used and not of the order a
	// Go map iterates.
	seen map[sfnt.GlyphID]bool
	used []sfnt.GlyphID
}

// New parses a font program and prepares it for embedding.
func New(opts Options) (*Face, error) {
	switch opts.Plan {
	case PlanSubset, PlanWhole:
	default:
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_PDF_FONT_INVALID,
			"the face has no embedding plan",
			map[string]any{"base_name": opts.BaseName, "plan": string(opts.Plan)})
	}
	if opts.Resource == "" {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_PDF_FONT_INVALID,
			"the face has no resource name, so no content stream could select it",
			map[string]any{"base_name": opts.BaseName})
	}

	program, err := sfnt.Parse(opts.Program)
	if err != nil {
		return nil, verr.Annotate(err, map[string]any{"base_name": opts.BaseName})
	}

	if program.IsCFF && opts.Plan == PlanSubset {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_FONT_EMBED_UNSUPPORTED,
			"the theme demands a subset of a face whose outlines are CFF, which Vellum does not subset",
			map[string]any{
				"base_name": opts.BaseName,
				"outlines":  "cff",
				"hint": "relax the theme's embed mode to \"auto\", which embeds this face whole. " +
					"Vellum will not relax it itself: subset-only may be a licence condition.",
			})
	}

	return &Face{opts: opts, program: program, seen: map[sfnt.GlyphID]bool{}}, nil
}

// Resource is the name a content stream selects this font by.
func (f *Face) Resource() object.Name { return f.opts.Resource }

// UnitsPerEm is the face's design grid.
func (f *Face) UnitsPerEm() int { return f.program.UnitsPerEm }

// Encode maps text to glyph identifiers and records them as used.
//
// A character the face has no glyph for is an error naming it, not a
// substitution. A document that silently renders a row of empty boxes is one
// nobody notices until it has been sent, and reaching for another installed
// face would make the same specification render differently on two machines.
func (f *Face) Encode(text string) ([]uint16, error) {
	out := make([]uint16, 0, len(text))
	for _, r := range text {
		g, ok := f.program.GlyphFor(r)
		if !ok {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_PDF_GLYPH_MISSING,
				"the face has no glyph for this character",
				map[string]any{
					"base_name":  f.opts.BaseName,
					"character":  string(r),
					"code_point": int(r),
				})
		}
		f.use(g)
		out = append(out, uint16(g))
	}
	return out, nil
}

// EncodeGlyphs records glyph identifiers a caller obtained another way — from
// shaping, which produces glyphs no character maps to directly.
func (f *Face) EncodeGlyphs(gids []sfnt.GlyphID) ([]uint16, error) {
	out := make([]uint16, 0, len(gids))
	for _, g := range gids {
		if int(g) >= f.program.NumGlyphs {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_PDF_FONT_INVALID,
				"the glyph id is outside the face's glyph array",
				map[string]any{"base_name": f.opts.BaseName, "glyph_id": int(g), "num_glyphs": f.program.NumGlyphs})
		}
		f.use(g)
		out = append(out, uint16(g))
	}
	return out, nil
}

// use records a glyph, keeping the list ordered by first use.
func (f *Face) use(g sfnt.GlyphID) {
	if !f.seen[g] {
		f.seen[g] = true
		f.used = append(f.used, g)
	}
}

// Used returns the glyphs recorded so far, sorted.
//
// Sorted rather than in first-use order: the subset is a property of which
// glyphs the document contains, not of the order the text happened to be
// written, so two documents with the same glyphs produce the same subset and
// the same subset tag.
func (f *Face) Used() []sfnt.GlyphID {
	out := append([]sfnt.GlyphID(nil), f.used...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// AdvanceWidth returns a glyph's advance in PDF glyph space.
func (f *Face) AdvanceWidth(g sfnt.GlyphID) (int, error) {
	w, err := f.program.AdvanceWidth(g)
	if err != nil {
		return 0, err
	}
	return scale(w, f.program.UnitsPerEm), nil
}

// Write emits the font objects and returns the reference a page's resource
// dictionary should carry.
func (f *Face) Write(doc *object.Document) (object.Ref, error) {
	if len(f.used) == 0 {
		return object.Ref{}, verr.NewCodedErrorWithDetails(verr.VELLUM_PDF_FONT_INVALID,
			"the face was embedded without a single glyph being used",
			map[string]any{"base_name": f.opts.BaseName})
	}

	program, tag, glyphs, err := f.embedProgram()
	if err != nil {
		return object.Ref{}, err
	}
	name := object.Name(tag + "+" + f.opts.BaseName)

	fileRef, err := f.writeProgram(doc, program)
	if err != nil {
		return object.Ref{}, err
	}
	descriptor, err := f.writeDescriptor(doc, name, fileRef)
	if err != nil {
		return object.Ref{}, err
	}
	widths, err := f.widths(glyphs)
	if err != nil {
		return object.Ref{}, err
	}

	// CIDFontType0 for CFF outlines, CIDFontType2 for TrueType. The pair is not
	// interchangeable: the subtype tells the reader how to interpret the
	// embedded program, and a mismatch produces a font that loads and draws
	// nothing.
	subtype := object.Name("CIDFontType2")
	if f.program.IsCFF {
		subtype = "CIDFontType0"
	}

	descendantDict := object.NewDict(
		"Type", object.Name("Font"),
		"Subtype", subtype,
		"BaseFont", name,
		"CIDSystemInfo", object.NewDict(
			"Registry", object.String("Adobe"),
			"Ordering", object.String("Identity"),
			"Supplement", object.Int(0),
		),
		"FontDescriptor", descriptor,
		"DW", object.Int(glyphSpace),
		"W", widths,
	)

	// Identity, because the subset preserved glyph ids. The alternative is a
	// stream mapping CID to glyph, which exists for subsetters that renumber
	// and is one more table that can disagree with the font program.
	//
	// Set only for CIDFontType2: a CIDFontType0's mapping lives in the CFF
	// charset, and writing the key here would state something about a table
	// this font does not have.
	descendantDict.SetIf(!f.program.IsCFF, "CIDToGIDMap", object.Name("Identity"))

	descendant := doc.Add(descendantDict)

	toUnicode, err := f.writeToUnicode(doc, glyphs)
	if err != nil {
		return object.Ref{}, err
	}

	return doc.Add(object.NewDict(
		"Type", object.Name("Font"),
		"Subtype", object.Name("Type0"),
		"BaseFont", name,
		// Identity-H addresses glyphs directly by two-byte identifier. It is
		// the encoding that works for any face regardless of its own character
		// mapping, which is why the content stream shows glyph ids rather than
		// text.
		"Encoding", object.Name("Identity-H"),
		"DescendantFonts", object.Array{descendant},
		"ToUnicode", toUnicode,
	)), nil
}

// embedProgram produces the bytes to embed, the subset tag, and the glyphs the
// font declares metrics for.
func (f *Face) embedProgram() (program []byte, tag string, glyphs []sfnt.GlyphID, err error) {
	if f.opts.Plan == PlanWhole || f.program.IsCFF {
		// The whole program, and metrics for every glyph the document uses.
		// A tag is still written: two documents embedding the same face with
		// different glyph sets are different subsets from a consumer's point of
		// view even when the programs are identical, and PDF requires the name
		// to distinguish them.
		return f.opts.Program, sfnt.SubsetTag(f.opts.Program, f.Used()), f.Used(), nil
	}

	subset, err := f.program.SubsetGlyphs(f.Used())
	if err != nil {
		return nil, "", nil, verr.Annotate(err, map[string]any{"base_name": f.opts.BaseName})
	}
	return subset.Program, subset.Tag, subset.Glyphs, nil
}

// writeProgram embeds the font program and returns its stream.
//
// The key it will be referenced under differs by outline format, and the
// descriptor has to agree: FontFile2 for TrueType, FontFile3 for CFF. A program
// filed under the wrong key is one the reader will not load.
func (f *Face) writeProgram(doc *object.Document, program []byte) (object.Ref, error) {
	if f.program.IsCFF {
		// OpenType rather than Type1C, because what is embedded is the whole
		// SFNT container and not a bare CFF table. Declaring Type1C would tell
		// the reader to expect a charstring program and hand it a font file.
		return doc.AddStream(object.NewDict("Subtype", object.Name("OpenType")), program)
	}
	// Length1 is the uncompressed size, which a reader needs because the
	// program is a length-prefixed structure and the filter hides its size.
	return doc.AddStream(object.NewDict("Length1", object.Int(len(program))), program)
}

// fontFileKey is the descriptor key the embedded program is filed under.
func (f *Face) fontFileKey() object.Name {
	if f.program.IsCFF {
		return "FontFile3"
	}
	return "FontFile2"
}

// writeDescriptor emits the font descriptor.
//
// PDF/A requires every key here to be present, and a validator checks them
// against the embedded program. They are read out of the face rather than
// guessed, with the single documented exception of StemV.
func (f *Face) writeDescriptor(doc *object.Document, name object.Name, file object.Ref) (object.Ref, error) {
	head, ok := f.program.Table(sfnt.TagHead)
	if !ok || len(head) < 44 {
		return object.Ref{}, verr.NewCodedError(verr.VELLUM_PDF_FONT_INVALID,
			"the face has no usable head table, so its bounding box is unknown")
	}
	hhea, ok := f.program.Table(sfnt.TagHhea)
	if !ok || len(hhea) < 8 {
		return object.Ref{}, verr.NewCodedError(verr.VELLUM_PDF_FONT_INVALID,
			"the face has no usable hhea table, so its vertical metrics are unknown")
	}

	upem := f.program.UnitsPerEm
	bbox := object.Array{
		object.Int(scale(int(int16(binary.BigEndian.Uint16(head[36:]))), upem)),
		object.Int(scale(int(int16(binary.BigEndian.Uint16(head[38:]))), upem)),
		object.Int(scale(int(int16(binary.BigEndian.Uint16(head[40:]))), upem)),
		object.Int(scale(int(int16(binary.BigEndian.Uint16(head[42:]))), upem)),
	}

	ascent := scale(int(int16(binary.BigEndian.Uint16(hhea[4:]))), upem)
	descent := scale(int(int16(binary.BigEndian.Uint16(hhea[6:]))), upem)

	d := object.NewDict(
		"Type", object.Name("FontDescriptor"),
		"FontName", name,
		"Flags", object.Int(f.flags()),
		"FontBBox", bbox,
		"ItalicAngle", object.Int(f.italicAngle()),
		"Ascent", object.Int(ascent),
		"Descent", object.Int(descent),
		"CapHeight", object.Int(f.capHeight(ascent)),
		// StemV has no source in an SFNT face. It describes the vertical stem
		// width and exists for readers synthesising a substitute, which cannot
		// happen here because the program is embedded. PDF requires it present,
		// so a plausible constant is written and declared rather than a
		// computed number that would look derived and would not be.
		"StemV", object.Int(80),
		f.fontFileKey(), file,
	)
	return doc.Add(d), nil
}

// flags builds the descriptor flag word.
//
// Exactly one of Symbolic and Nonsymbolic must be set, and getting it wrong
// changes how a reader interprets the encoding. A face with a Unicode character
// map used through Identity-H is nonsymbolic.
func (f *Face) flags() int {
	const (
		fixedPitch  = 1 << 0
		serif       = 1 << 1
		nonsymbolic = 1 << 5
		italic      = 1 << 6
	)
	flags := nonsymbolic
	if f.opts.FixedPitch {
		flags |= fixedPitch
	}
	if f.opts.Serif {
		flags |= serif
	}
	if f.italicAngle() != 0 {
		flags |= italic
	}
	return flags
}

// italicAngle reads the angle from post, in degrees, truncated to a whole
// number.
func (f *Face) italicAngle() int {
	post, ok := f.program.Table(sfnt.TagPost)
	if !ok || len(post) < 8 {
		return 0
	}
	// A 16.16 fixed-point value; the fractional part is discarded because the
	// descriptor takes a number and no reader acts on a hundredth of a degree.
	return int(int32(binary.BigEndian.Uint32(post[4:])) >> 16)
}

// capHeight reads the capital height from OS/2, falling back to the ascent.
//
// The field only exists in version 2 and later of the table. Falling back to
// the ascent overstates it slightly, which is the harmless direction: the
// descriptor's capital height is used to size a substitute face, and there is
// no substitute here because the program is embedded.
func (f *Face) capHeight(ascent int) int {
	os2, ok := f.program.Table(sfnt.TagOS2)
	if !ok || len(os2) < 90 || binary.BigEndian.Uint16(os2) < 2 {
		return ascent
	}
	if v := int(int16(binary.BigEndian.Uint16(os2[88:]))); v > 0 {
		return scale(v, f.program.UnitsPerEm)
	}
	return ascent
}

// widths builds the /W array: runs of consecutive glyph ids and their advances.
//
// The run form rather than one entry per glyph, because a subset's retained
// glyphs are usually clustered — a Latin alphabet is contiguous in most faces —
// and the array is otherwise several times the size of the outlines it
// describes.
func (f *Face) widths(glyphs []sfnt.GlyphID) (object.Array, error) {
	if len(glyphs) == 0 {
		return object.Array{}, nil
	}
	ids := append([]sfnt.GlyphID(nil), glyphs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	var out object.Array
	for i := 0; i < len(ids); {
		j := i + 1
		for j < len(ids) && ids[j] == ids[j-1]+1 {
			j++
		}

		run := make(object.Array, 0, j-i)
		for _, g := range ids[i:j] {
			w, err := f.program.AdvanceWidth(g)
			if err != nil {
				return nil, err
			}
			run = append(run, object.Int(scale(w, f.program.UnitsPerEm)))
		}
		out = append(out, object.Int(ids[i]), run)
		i = j
	}
	return out, nil
}

// writeToUnicode emits the CMap mapping glyph ids back to characters.
//
// PDF/A-2b does not require it — that is level U — but without it the document
// has no extractable text at all, and a document whose text cannot be selected,
// searched or read aloud is one that fails the purpose of archiving it. It also
// gives the test suite a way to ask a reader what it sees, which is the only
// check that catches content reaching the file but not reaching a reader.
func (f *Face) writeToUnicode(doc *object.Document, glyphs []sfnt.GlyphID) (object.Ref, error) {
	type mapping struct {
		gid sfnt.GlyphID
		r   rune
	}
	var pairs []mapping
	for _, g := range glyphs {
		if r, ok := f.program.RuneFor(g); ok {
			pairs = append(pairs, mapping{gid: g, r: r})
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].gid < pairs[j].gid })

	var b strings.Builder
	b.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n")
	b.WriteString("/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n")
	b.WriteString("/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n")
	b.WriteString("1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")

	// A bfchar section takes at most a hundred entries, which is a hard limit
	// in the CMap specification rather than a convention.
	const perSection = 100
	for start := 0; start < len(pairs); start += perSection {
		end := min(start+perSection, len(pairs))

		b.WriteString(strconv.Itoa(end-start) + " beginbfchar\n")
		for _, p := range pairs[start:end] {
			b.WriteString("<" + hex16(uint16(p.gid)) + "> <" + utf16Hex(p.r) + ">\n")
		}
		b.WriteString("endbfchar\n")
	}

	b.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	return doc.AddStream(object.Dict{}, []byte(b.String()))
}

// scale converts a value in font units to PDF glyph space, rounding half away
// from zero.
func scale(v, unitsPerEm int) int {
	if unitsPerEm == 0 {
		return 0
	}
	n := v * glyphSpace
	if (n < 0) != (unitsPerEm < 0) {
		return (n - unitsPerEm/2) / unitsPerEm
	}
	return (n + unitsPerEm/2) / unitsPerEm
}

// hex16 renders a sixteen-bit value as four uppercase hex digits.
func hex16(v uint16) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{
		digits[v>>12], digits[(v>>8)&0xf], digits[(v>>4)&0xf], digits[v&0xf],
	})
}

// utf16Hex renders a rune as its UTF-16 big-endian code units in hex, which is
// the encoding a ToUnicode destination uses.
func utf16Hex(r rune) string {
	if r > 0xFFFF {
		r -= 0x10000
		return hex16(uint16(0xD800+(r>>10))) + hex16(uint16(0xDC00+(r&0x3FF)))
	}
	return hex16(uint16(r))
}
