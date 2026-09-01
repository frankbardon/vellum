package sfnt

import (
	"encoding/binary"
	"sort"
)

// readCmap builds the rune-to-glyph map from the best available subtable.
//
// Preference order is Unicode full repertoire first, then the basic multilingual
// plane. A symbol-encoded subtable (3,0) is deliberately not read: those map
// characters into the private use area by a convention that differs per foundry,
// and guessing at it produces text that renders but extracts as nonsense.
func (f *Font) readCmap() error {
	f.cmap = map[rune]GlyphID{}

	cmap, ok := f.tables[TagCmap]
	if !ok || len(cmap) < 4 {
		// A face with no Unicode cmap can still be used when the caller supplies
		// glyph ids directly, so this is not fatal here. Asking it to map a rune
		// is what fails, and that error names the character.
		return nil
	}

	numTables := int(binary.BigEndian.Uint16(cmap[2:]))
	if 4+numTables*8 > len(cmap) {
		return fontErr("the cmap subtable directory runs past the end of the table",
			map[string]any{"num_subtables": numTables, "cmap_bytes": len(cmap)})
	}

	type candidate struct {
		rank   int
		offset uint32
	}
	var best candidate
	best.rank = -1

	for i := range numTables {
		rec := cmap[4+i*8:]
		platform := binary.BigEndian.Uint16(rec)
		encoding := binary.BigEndian.Uint16(rec[2:])
		offset := binary.BigEndian.Uint32(rec[4:])
		if uint64(offset)+4 > uint64(len(cmap)) {
			continue
		}

		rank := -1
		switch {
		case platform == 3 && encoding == 10: // Windows, full repertoire
			rank = 3
		case platform == 0 && encoding >= 4: // Unicode, full repertoire
			rank = 2
		case platform == 3 && encoding == 1: // Windows, BMP
			rank = 1
		case platform == 0: // Unicode, BMP
			rank = 0
		}
		if rank > best.rank {
			best = candidate{rank: rank, offset: offset}
		}
	}
	if best.rank < 0 {
		return nil
	}

	sub := cmap[best.offset:]
	switch format := binary.BigEndian.Uint16(sub); format {
	case 4:
		return f.readCmapFormat4(sub)
	case 12:
		return f.readCmapFormat12(sub)
	case 6:
		return f.readCmapFormat6(sub)
	default:
		return fontErr("the font's best Unicode cmap is in a format this package does not read",
			map[string]any{"cmap_format": format})
	}
}

// readCmapFormat4 reads the segmented BMP mapping, which is what almost every
// Latin font actually ships.
func (f *Font) readCmapFormat4(sub []byte) error {
	if len(sub) < 14 {
		return fontErr("the format 4 cmap is too short for its header", nil)
	}
	segCountX2 := int(binary.BigEndian.Uint16(sub[6:]))
	segCount := segCountX2 / 2
	if segCount == 0 {
		return nil
	}

	endAt := 14
	startAt := endAt + segCountX2 + 2 // +2 for reservedPad
	deltaAt := startAt + segCountX2
	rangeAt := deltaAt + segCountX2
	if rangeAt+segCountX2 > len(sub) {
		return fontErr("the format 4 cmap is too short for its segment count",
			map[string]any{"seg_count": segCount, "subtable_bytes": len(sub)})
	}

	for i := range segCount {
		end := binary.BigEndian.Uint16(sub[endAt+i*2:])
		start := binary.BigEndian.Uint16(sub[startAt+i*2:])
		delta := binary.BigEndian.Uint16(sub[deltaAt+i*2:])
		rangeOffset := binary.BigEndian.Uint16(sub[rangeAt+i*2:])
		if start > end {
			continue
		}

		for c := uint32(start); c <= uint32(end); c++ {
			if c == 0xFFFF {
				// The final segment is required to end at 0xFFFF mapping to
				// nothing; including it would map the noncharacter to glyph 0.
				continue
			}
			var g uint16
			if rangeOffset == 0 {
				g = uint16(c) + delta
			} else {
				// The offset is relative to its own position in the array,
				// which is the format's one genuinely awkward feature.
				at := rangeAt + i*2 + int(rangeOffset) + int(c-uint32(start))*2
				if at+2 > len(sub) {
					continue
				}
				g = binary.BigEndian.Uint16(sub[at:])
				if g != 0 {
					g += delta
				}
			}
			if g != 0 && int(g) < f.NumGlyphs {
				f.cmap[rune(c)] = GlyphID(g)
			}
		}
	}
	return nil
}

// readCmapFormat12 reads the segmented coverage mapping, which is what a font
// carrying anything outside the basic multilingual plane uses.
func (f *Font) readCmapFormat12(sub []byte) error {
	if len(sub) < 16 {
		return fontErr("the format 12 cmap is too short for its header", nil)
	}
	numGroups := int(binary.BigEndian.Uint32(sub[12:]))
	if 16+numGroups*12 > len(sub) {
		return fontErr("the format 12 cmap is too short for its group count",
			map[string]any{"num_groups": numGroups, "subtable_bytes": len(sub)})
	}

	for i := range numGroups {
		rec := sub[16+i*12:]
		start := binary.BigEndian.Uint32(rec)
		end := binary.BigEndian.Uint32(rec[4:])
		startGlyph := binary.BigEndian.Uint32(rec[8:])
		if start > end || end-start > 0x110000 {
			continue
		}
		for c := start; c <= end; c++ {
			g := startGlyph + (c - start)
			if g != 0 && int(g) < f.NumGlyphs {
				f.cmap[rune(c)] = GlyphID(g)
			}
		}
	}
	return nil
}

// readCmapFormat6 reads the trimmed table mapping, which a few older faces use
// for a small contiguous range.
func (f *Font) readCmapFormat6(sub []byte) error {
	if len(sub) < 10 {
		return fontErr("the format 6 cmap is too short for its header", nil)
	}
	first := int(binary.BigEndian.Uint16(sub[6:]))
	count := int(binary.BigEndian.Uint16(sub[8:]))
	if 10+count*2 > len(sub) {
		return fontErr("the format 6 cmap is too short for its entry count",
			map[string]any{"count": count, "subtable_bytes": len(sub)})
	}
	for i := range count {
		g := binary.BigEndian.Uint16(sub[10+i*2:])
		if g != 0 && int(g) < f.NumGlyphs {
			f.cmap[rune(first+i)] = GlyphID(g)
		}
	}
	return nil
}

// Runes returns every character the font maps, sorted.
//
// Sorted because callers build a ToUnicode mapping from it, and that mapping is
// written into the file. Returning map order would put the nondeterminism of a
// Go map range directly into the output bytes.
func (f *Font) Runes() []rune {
	out := make([]rune, 0, len(f.cmap))
	for r := range f.cmap {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// RuneFor returns a character that maps to the glyph, and whether one was found.
//
// The lowest such character, so that a glyph reachable from several characters
// — a space and a non-breaking space sharing an outline, say — yields the same
// answer on every run. Text extraction shows this character, so the choice is
// observable and must not vary.
func (f *Font) RuneFor(gid GlyphID) (rune, bool) {
	found := rune(-1)
	for r, g := range f.cmap {
		if g == gid && (found < 0 || r < found) {
			found = r
		}
	}
	return found, found >= 0
}
