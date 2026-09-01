package pdf

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

// Font is a face to embed, together with the glyphs the document uses.
//
// The glyph set is an input rather than something discovered while writing,
// because the subset has to exist before the font objects can be written and
// the content stream has to name glyph ids that the subset retained. Collecting
// them is the caller's job, which is also what lets one face serve several
// pages without being embedded twice.
type Font struct {
	// Resource is the name the content stream selects this font by.
	Resource object.Name

	// BaseName is the PostScript name written into the file, before the subset
	// prefix is added.
	BaseName string

	// Program is the parsed source face.
	Program *sfnt.Font

	// Glyphs are the glyph ids the document uses.
	Glyphs []sfnt.GlyphID

	// Serif and FixedPitch describe the face for the font descriptor. Vellum
	// does not infer them: the OS/2 panose bytes are frequently wrong, and a
	// wrong descriptor flag changes how a reader substitutes when it cannot use
	// the embedded program.
	Serif      bool
	FixedPitch bool
}

// write emits the font objects and returns the reference the page resource
// dictionary should carry.
func (f *Font) write(doc *object.Document) (object.Ref, error) {
	if f.Program == nil {
		return object.Ref{}, verr.NewCodedError(verr.VELLUM_PDF_FONT_INVALID,
			"the font has no parsed program")
	}

	subset, err := f.Program.SubsetGlyphs(f.Glyphs)
	if err != nil {
		return object.Ref{}, err
	}
	name := object.Name(subset.Tag + "+" + f.BaseName)

	fileRef, err := f.writeProgram(doc, subset)
	if err != nil {
		return object.Ref{}, err
	}
	descriptor, err := f.writeDescriptor(doc, name, fileRef)
	if err != nil {
		return object.Ref{}, err
	}
	widths, err := f.widths(subset.Glyphs)
	if err != nil {
		return object.Ref{}, err
	}

	descendant := doc.Add(object.NewDict(
		"Type", object.Name("Font"),
		"Subtype", object.Name("CIDFontType2"),
		"BaseFont", name,
		"CIDSystemInfo", object.NewDict(
			"Registry", object.String("Adobe"),
			"Ordering", object.String("Identity"),
			"Supplement", object.Int(0),
		),
		"FontDescriptor", descriptor,
		"DW", object.Int(glyphSpace),
		"W", widths,
		// Identity, because the subset preserved glyph ids. The alternative is
		// a stream mapping CID to glyph, which exists for subsetters that
		// renumber — and which is one more table that can disagree with the
		// font program.
		"CIDToGIDMap", object.Name("Identity"),
	))

	toUnicode, err := f.writeToUnicode(doc, subset.Glyphs)
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

// writeProgram embeds the subsetted font program.
func (f *Font) writeProgram(doc *object.Document, subset *sfnt.Subset) (object.Ref, error) {
	// Length1 is the uncompressed size, which a reader needs because the
	// program is a length-prefixed structure and the filter hides its size.
	dict := object.NewDict("Length1", object.Int(len(subset.Program)))
	return doc.AddStream(dict, subset.Program)
}

// writeDescriptor emits the font descriptor.
//
// PDF/A requires every key here to be present, and a validator checks them
// against the embedded program. They are read out of the face rather than
// guessed, with the single documented exception of StemV.
func (f *Font) writeDescriptor(doc *object.Document, name object.Name, file object.Ref) (object.Ref, error) {
	head, ok := f.Program.Table(sfnt.TagHead)
	if !ok || len(head) < 44 {
		return object.Ref{}, verr.NewCodedError(verr.VELLUM_PDF_FONT_INVALID,
			"the face has no usable head table, so its bounding box is unknown")
	}
	hhea, ok := f.Program.Table(sfnt.TagHhea)
	if !ok || len(hhea) < 8 {
		return object.Ref{}, verr.NewCodedError(verr.VELLUM_PDF_FONT_INVALID,
			"the face has no usable hhea table, so its vertical metrics are unknown")
	}

	upem := f.Program.UnitsPerEm
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
		"FontFile2", file,
	)
	return doc.Add(d), nil
}

// flags builds the descriptor flag word.
//
// Exactly one of Symbolic and Nonsymbolic must be set, and getting it wrong
// changes how a reader interprets the encoding. A face with a Unicode character
// map used through Identity-H is nonsymbolic.
func (f *Font) flags() int {
	const (
		fixedPitch  = 1 << 0
		serif       = 1 << 1
		nonsymbolic = 1 << 5
		italic      = 1 << 6
	)
	flags := nonsymbolic
	if f.FixedPitch {
		flags |= fixedPitch
	}
	if f.Serif {
		flags |= serif
	}
	if f.italicAngle() != 0 {
		flags |= italic
	}
	return flags
}

// italicAngle reads the angle from post, in degrees, truncated to a whole
// number.
func (f *Font) italicAngle() int {
	post, ok := f.Program.Table(sfnt.TagPost)
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
func (f *Font) capHeight(ascent int) int {
	os2, ok := f.Program.Table(sfnt.TagOS2)
	if !ok || len(os2) < 90 || binary.BigEndian.Uint16(os2) < 2 {
		return ascent
	}
	if v := int(int16(binary.BigEndian.Uint16(os2[88:]))); v > 0 {
		return scale(v, f.Program.UnitsPerEm)
	}
	return ascent
}

// widths builds the /W array: runs of consecutive glyph ids and their advances.
//
// The run form rather than one entry per glyph, because a subset's retained
// glyphs are usually clustered — a Latin alphabet is contiguous in most faces —
// and the array is otherwise several times the size of the outlines it
// describes.
func (f *Font) widths(glyphs []sfnt.GlyphID) (object.Array, error) {
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
			w, err := f.Program.AdvanceWidth(g)
			if err != nil {
				return nil, err
			}
			run = append(run, object.Int(scale(w, f.Program.UnitsPerEm)))
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
func (f *Font) writeToUnicode(doc *object.Document, glyphs []sfnt.GlyphID) (object.Ref, error) {
	type mapping struct {
		gid sfnt.GlyphID
		r   rune
	}
	var pairs []mapping
	for _, g := range glyphs {
		if r, ok := f.Program.RuneFor(g); ok {
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
