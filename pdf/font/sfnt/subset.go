package sfnt

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"
)

// Composite glyph component flags, from the glyf table specification.
const (
	argsAreWords    = 0x0001
	haveScale       = 0x0008
	moreComponents  = 0x0020
	haveXAndYScale  = 0x0040
	haveTwoByTwo    = 0x0080
	compositeHeader = 10 // numberOfContours and the bounding box
)

// subsetTables are the tables a subset carries, in the order they are written.
//
// Sorted by tag, which is what the directory must be and which makes the
// physical order deterministic without a second rule.
//
// cmap, name, post and OS/2 are copied through unchanged. A PDF embedding this as a CIDFontType2 with
// an identity CID-to-glyph map never consults it — the character mapping lives
// in the PDF's encoding and ToUnicode — so the first version omitted it. An
// independent parser then refused to load the program at all, which is a useful
// thing to have learnt from a reader rather than from a validator: a font
// program that only the code which wrote it will open is not evidence of
// anything. Copying it is correct because glyph ids are preserved, so every
// mapping still points where it did; entries for discarded glyphs now resolve
// to an empty outline, which is exactly how a font expresses a blank glyph.
// The order is bytewise by tag, which puts OS/2 first because its tag begins
// with an uppercase letter.
var subsetTables = []Tag{
	TagOS2, TagCmap, TagCvt, TagFpgm, TagGlyf, TagHead,
	TagHhea, TagHmtx, TagLoca, TagMaxp, TagName, TagPost, TagPrep,
}

// Subset is a reduced font program together with what a PDF needs to describe
// it.
type Subset struct {
	// Program is the emitted font program.
	Program []byte

	// Tag is the six-letter subset prefix, without the trailing plus.
	//
	// PDF requires a subsetted font's name to be prefixed so that two subsets
	// of one face are not mistaken for each other by a consumer merging files.
	Tag string

	// Glyphs are the retained glyph ids, sorted. It includes glyph 0 and the
	// components of every composite retained, which is why it may be larger
	// than the set that was asked for.
	Glyphs []GlyphID
}

// SubsetGlyphs reduces the font to the given glyphs.
//
// Glyph ids are preserved rather than renumbered. Renumbering would make the
// program smaller by the size of the unused loca entries and would require
// rewriting every composite glyph's component references and remapping the
// caller's ids — three chances to be subtly wrong to save a few kilobytes that
// compress to almost nothing, since the discarded entries are runs of equal
// offsets. Preserving them also lets the PDF use an identity CID-to-glyph map,
// which is the arrangement readers implement most reliably.
//
// The result is a pure function of the source program and the sorted glyph set.
func (f *Font) SubsetGlyphs(want []GlyphID) (*Subset, error) {
	if f.IsCFF {
		return nil, fontErr("this font's outlines are in CFF, which this package does not subset",
			map[string]any{"hint": "a CFF face is embedded whole; see the capability matrix"})
	}

	keep, err := f.closure(want)
	if err != nil {
		return nil, err
	}

	glyf, loca := f.buildGlyfAndLoca(keep)

	tables := make(map[Tag][]byte, len(subsetTables))
	tables[TagGlyf] = glyf
	tables[TagLoca] = loca

	head, err := f.subsetHead()
	if err != nil {
		return nil, err
	}
	tables[TagHead] = head

	// Everything not rebuilt is copied byte for byte. Re-encoding a table is
	// work done only to undo it, and every re-encoding is a chance to write
	// bytes that differ from the ones that came in.
	if post, ok := f.subsetPost(); ok {
		tables[TagPost] = post
	}

	for _, tag := range []Tag{TagOS2, TagCmap, TagHhea, TagHmtx, TagMaxp, TagName, TagCvt, TagFpgm, TagPrep} {
		if b, ok := f.tables[tag]; ok {
			tables[tag] = append([]byte(nil), b...)
		}
	}

	return &Subset{
		Program: assemble(tables),
		Tag:     subsetTag(f.Raw, keep),
		Glyphs:  keep,
	}, nil
}

// closure returns the glyphs to retain: the requested set, glyph 0, and every
// component reachable from a composite in it.
//
// A composite glyph is a reference to other glyphs — an accented letter is
// usually the base and the accent, positioned. Dropping a component leaves a
// glyph that renders as half of itself, or as nothing, which is the classic way
// a hand-rolled subsetter produces a document whose accented characters are
// invisible.
func (f *Font) closure(want []GlyphID) ([]GlyphID, error) {
	in := make(map[GlyphID]bool, len(want)+1)

	// Glyph 0 is .notdef and is required to be present.
	queue := []GlyphID{0}
	in[0] = true
	for _, g := range want {
		if int(g) >= f.NumGlyphs {
			return nil, fontErr("a requested glyph is outside the font's glyph array",
				map[string]any{"glyph_id": int(g), "num_glyphs": f.NumGlyphs})
		}
		if !in[g] {
			in[g] = true
			queue = append(queue, g)
		}
	}

	for len(queue) > 0 {
		g := queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		components, err := f.components(g)
		if err != nil {
			return nil, err
		}
		for _, c := range components {
			if int(c) >= f.NumGlyphs {
				return nil, fontErr("a composite glyph names a component outside the glyph array",
					map[string]any{"glyph_id": int(g), "component_id": int(c), "num_glyphs": f.NumGlyphs})
			}
			if !in[c] {
				in[c] = true
				queue = append(queue, c)
			}
		}
	}

	// Collected by walking the glyph array rather than by ranging the map, so
	// the result is sorted by construction rather than by a sort applied after.
	out := make([]GlyphID, 0, len(in))
	for g := range f.NumGlyphs {
		if in[GlyphID(g)] {
			out = append(out, GlyphID(g))
		}
	}
	return out, nil
}

// components returns the glyphs a composite glyph refers to, or nothing for a
// simple glyph.
func (f *Font) components(gid GlyphID) ([]GlyphID, error) {
	data, err := f.GlyphData(gid)
	if err != nil {
		return nil, err
	}
	if len(data) < compositeHeader {
		// An empty glyph — a space, most often. Not an error: loca legitimately
		// gives equal offsets for a glyph with no outline.
		return nil, nil
	}
	if int16(binary.BigEndian.Uint16(data)) >= 0 {
		return nil, nil
	}

	var out []GlyphID
	at := compositeHeader
	for {
		if at+4 > len(data) {
			return nil, fontErr("a composite glyph's component list runs past the end of the glyph",
				map[string]any{"glyph_id": int(gid)})
		}
		flags := binary.BigEndian.Uint16(data[at:])
		out = append(out, GlyphID(binary.BigEndian.Uint16(data[at+2:])))
		at += 4

		if flags&argsAreWords != 0 {
			at += 4
		} else {
			at += 2
		}
		switch {
		case flags&haveTwoByTwo != 0:
			at += 8
		case flags&haveXAndYScale != 0:
			at += 4
		case flags&haveScale != 0:
			at += 2
		}
		if flags&moreComponents == 0 {
			return out, nil
		}
		if at > len(data) {
			return nil, fontErr("a composite glyph's component list runs past the end of the glyph",
				map[string]any{"glyph_id": int(gid)})
		}
	}
}

// buildGlyfAndLoca emits the outline table and its index.
//
// Every glyph gets a loca entry — retained glyphs get their outlines, discarded
// ones get a zero-length entry, which is exactly how a font expresses a glyph
// with no contours and is what makes glyph ids preservable.
//
// The long loca format is used unconditionally. The short format halves each
// offset, so it can only describe a glyf table under 128 KB and only at even
// offsets; choosing between them by size would make the format a function of
// which glyphs were retained, and therefore make head differ between two
// subsets of one face for a reason unrelated to either.
func (f *Font) buildGlyfAndLoca(keep []GlyphID) (glyf, loca []byte) {
	inSet := make([]bool, f.NumGlyphs)
	for _, g := range keep {
		inSet[g] = true
	}

	glyf = make([]byte, 0, len(f.tables[TagGlyf]))
	loca = make([]byte, 0, (f.NumGlyphs+1)*4)

	for g := range f.NumGlyphs {
		loca = binary.BigEndian.AppendUint32(loca, uint32(len(glyf)))
		if !inSet[g] {
			continue
		}
		start, end := f.loca[g], f.loca[g+1]
		glyf = append(glyf, f.tables[TagGlyf][start:end]...)

		// Glyph data is aligned to four bytes. The specification recommends it,
		// several rasterisers read multi-byte fields directly out of the table,
		// and the padding is deterministic zero rather than whatever followed
		// in the source.
		for len(glyf)%4 != 0 {
			glyf = append(glyf, 0)
		}
	}
	loca = binary.BigEndian.AppendUint32(loca, uint32(len(glyf)))
	return glyf, loca
}

// subsetHead copies head with the loca format corrected and the checksum
// placeholder cleared.
//
// created and modified are carried over from the source rather than set from
// the clock. That is the single most important line in this package for
// determinism: it is precisely what the one permissively licensed alternative
// subsetter gets wrong, and it is why Vellum has its own.
func (f *Font) subsetHead() ([]byte, error) {
	src, ok := f.tables[TagHead]
	if !ok || len(src) < 54 {
		return nil, fontErr("the font program has no usable head table", nil)
	}
	head := append([]byte(nil), src...)

	// checkSumAdjustment is computed over the finished file, so it must be zero
	// while the file is being summed.
	binary.BigEndian.PutUint32(head[8:], 0)
	binary.BigEndian.PutUint16(head[50:], 1) // indexToLocFormat: long
	return head, nil
}

// subsetPost rewrites post in format 3.0, which carries no glyph names.
//
// A format 2.0 post table stores a PostScript name for every glyph in the face,
// which for a text font is most of what is left after the outlines are dropped
// — eight kilobytes here against a twenty-five kilobyte subset. The names have
// no reader: this program is embedded as a CIDFontType2 addressed by glyph id,
// and nothing consults a glyph name to render or to extract text.
//
// The header is copied rather than synthesised, so the italic angle and
// underline metrics stay the face's own.
func (f *Font) subsetPost() ([]byte, bool) {
	src, ok := f.tables[TagPost]
	if !ok || len(src) < 32 {
		return nil, false
	}
	post := append([]byte(nil), src[:32]...)
	binary.BigEndian.PutUint32(post, 0x00030000)
	return post, true
}

// assemble writes the table directory and the tables.
func assemble(tables map[Tag][]byte) []byte {
	present := make([]Tag, 0, len(tables))
	for _, tag := range subsetTables {
		if _, ok := tables[tag]; ok {
			present = append(present, tag)
		}
	}

	numTables := len(present)
	dirSize := 12 + numTables*16
	out := make([]byte, dirSize)

	binary.BigEndian.PutUint32(out, 0x00010000)
	binary.BigEndian.PutUint16(out[4:], uint16(numTables))

	// The binary-search hint fields. Readers are not required to use them and
	// most do not, but a validator will notice they disagree with the table
	// count.
	entrySelector := 0
	for 1<<(entrySelector+1) <= numTables {
		entrySelector++
	}
	searchRange := 16 << entrySelector
	binary.BigEndian.PutUint16(out[6:], uint16(searchRange))
	binary.BigEndian.PutUint16(out[8:], uint16(entrySelector))
	binary.BigEndian.PutUint16(out[10:], uint16(numTables*16-searchRange))

	for i, tag := range present {
		data := tables[tag]
		offset := len(out)
		out = append(out, data...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}

		rec := out[12+i*16:]
		copy(rec[:4], tag[:])
		binary.BigEndian.PutUint32(rec[4:], tableChecksum(data))
		binary.BigEndian.PutUint32(rec[8:], uint32(offset))
		binary.BigEndian.PutUint32(rec[12:], uint32(len(data)))
	}

	// head.checkSumAdjustment is written last, over the finished file, which is
	// the order the specification requires and the reason head was emitted with
	// a zero there.
	if headOffset, ok := offsetOf(out, present, TagHead); ok {
		adjustment := 0xB1B0AFBA - fileChecksum(out)
		binary.BigEndian.PutUint32(out[headOffset+8:], adjustment)
	}
	return out
}

// offsetOf finds a table's offset in the assembled program.
func offsetOf(out []byte, present []Tag, want Tag) (int, bool) {
	for i, tag := range present {
		if tag == want {
			return int(binary.BigEndian.Uint32(out[12+i*16+8:])), true
		}
	}
	return 0, false
}

// tableChecksum sums a table as big-endian uint32s, zero-padding the tail.
func tableChecksum(b []byte) uint32 {
	var sum uint32
	for i := 0; i < len(b); i += 4 {
		var word [4]byte
		copy(word[:], b[i:])
		sum += binary.BigEndian.Uint32(word[:])
	}
	return sum
}

// fileChecksum sums the whole program the same way.
func fileChecksum(b []byte) uint32 { return tableChecksum(b) }

// subsetTag derives the six-letter prefix from the source program and the
// retained glyphs.
//
// Derived rather than allocated from a counter, because a counter's value
// depends on how many fonts happened to be embedded before this one — so the
// same face in the same document would carry a different tag depending on the
// order sections were composed in, and the bytes would differ for no reason a
// reader could see.
func subsetTag(program []byte, glyphs []GlyphID) string {
	h := sha256.New()
	sum := sha256.Sum256(program)
	h.Write(sum[:])

	ids := make([]GlyphID, len(glyphs))
	copy(ids, glyphs)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var buf [2]byte
	for _, g := range ids {
		binary.BigEndian.PutUint16(buf[:], uint16(g))
		h.Write(buf[:])
	}

	digest := h.Sum(nil)
	var n uint64
	for _, b := range digest[:8] {
		n = n<<8 | uint64(b)
	}

	tag := make([]byte, 6)
	for i := range tag {
		tag[i] = byte('A' + n%26)
		n /= 26
	}
	return string(tag)
}
