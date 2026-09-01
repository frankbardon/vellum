// Package sfnt parses and subsets SFNT-housed font programs.
//
// Vellum owns this because it has to. The Go library that solves PDF font
// embedding properly is GPL-3.0 and unavailable to a permissively licensed
// project; the permissive alternative with a real subsetter stamps time.Now()
// into head.modified, which is inside the byte stream Vellum is required to
// pin. go-text/typesetting parses OpenType and shapes text but has no
// subsetter at all.
//
// # Scope
//
// TrueType outlines — the glyf and loca tables. A CFF-housed face is parsed for
// its metrics and embedded whole, which the capability matrix declares as a
// degradation rather than leaving a consumer to discover it: subsetting CFF
// means implementing a second outline format and a charstring interpreter, and
// the size it saves is not worth the ways it can go wrong.
//
// # Determinism
//
// A subset is a pure function of the source program and the sorted set of glyph
// ids retained. Everything that would ordinarily vary is pinned: head.created
// and head.modified take the source's values rather than the clock, tables are
// emitted in tag order, padding is zero, and head.checkSumAdjustment is
// computed over the finished file and written last.
package sfnt

import (
	"encoding/binary"

	verr "github.com/frankbardon/vellum/errors"
)

// Tag is a four-byte SFNT table tag.
type Tag [4]byte

// String renders the tag as its four characters.
func (t Tag) String() string { return string(t[:]) }

// The tables this package reads or writes.
var (
	TagHead = Tag{'h', 'e', 'a', 'd'}
	TagHhea = Tag{'h', 'h', 'e', 'a'}
	TagMaxp = Tag{'m', 'a', 'x', 'p'}
	TagHmtx = Tag{'h', 'm', 't', 'x'}
	TagLoca = Tag{'l', 'o', 'c', 'a'}
	TagGlyf = Tag{'g', 'l', 'y', 'f'}
	TagCmap = Tag{'c', 'm', 'a', 'p'}
	TagName = Tag{'n', 'a', 'm', 'e'}
	TagPost = Tag{'p', 'o', 's', 't'}
	TagOS2  = Tag{'O', 'S', '/', '2'}
	TagCFF  = Tag{'C', 'F', 'F', ' '}
	TagCvt  = Tag{'c', 'v', 't', ' '}
	TagFpgm = Tag{'f', 'p', 'g', 'm'}
	TagPrep = Tag{'p', 'r', 'e', 'p'}
)

// GlyphID is an index into the font's glyph array.
type GlyphID uint16

// Font is a parsed SFNT font program.
//
// It holds the source bytes and the table directory rather than a decoded model
// of every table. Subsetting copies most tables through untouched, so decoding
// them would be work done only to undo it — and every re-encoding is a chance
// to write bytes that differ from the ones that came in.
type Font struct {
	// Raw is the whole source program.
	Raw []byte

	// Tables maps each tag to its slice of Raw, in the order the directory
	// listed them.
	tables map[Tag][]byte
	order  []Tag

	// UnitsPerEm is the design grid, from head. Almost always 1000 or 2048.
	UnitsPerEm int

	// NumGlyphs is the glyph count, from maxp.
	NumGlyphs int

	// IndexToLocFormat is 0 for short loca offsets and 1 for long, from head.
	IndexToLocFormat int

	// NumberOfHMetrics is the count of full metrics in hmtx, from hhea.
	NumberOfHMetrics int

	// IsCFF reports that outlines are in a CFF table rather than glyf.
	IsCFF bool

	// loca holds the decoded glyph offsets, length NumGlyphs+1.
	loca []uint32

	// cmap maps a rune to its glyph, built once on parse.
	cmap map[rune]GlyphID
}

// Parse reads a font program.
//
// TrueType Collections are rejected rather than reading the first face: a theme
// naming a collection means a face was chosen by whoever wrote it, and picking
// one here would silently render in a face nobody asked for.
func Parse(raw []byte) (*Font, error) {
	if len(raw) < 12 {
		return nil, fontErr("the font program is shorter than an SFNT header", nil)
	}

	switch version := binary.BigEndian.Uint32(raw); version {
	case 0x00010000, 0x4F54544F: // 1.0 (TrueType), 'OTTO' (CFF)
	case 0x74746366: // 'ttcf'
		return nil, fontErr("the font program is a TrueType Collection, which names several faces",
			map[string]any{"hint": "supply a single face rather than a collection"})
	default:
		return nil, fontErr("the font program has an unrecognised SFNT version",
			map[string]any{"sfnt_version": version})
	}

	numTables := int(binary.BigEndian.Uint16(raw[4:]))
	if 12+numTables*16 > len(raw) {
		return nil, fontErr("the table directory runs past the end of the font program",
			map[string]any{"num_tables": numTables, "size_bytes": len(raw)})
	}

	f := &Font{Raw: raw, tables: make(map[Tag][]byte, numTables)}
	for i := range numTables {
		rec := raw[12+i*16:]
		var tag Tag
		copy(tag[:], rec[:4])
		off := binary.BigEndian.Uint32(rec[8:])
		length := binary.BigEndian.Uint32(rec[12:])

		end := uint64(off) + uint64(length)
		if end > uint64(len(raw)) {
			return nil, fontErr("a table runs past the end of the font program",
				map[string]any{"table": tag.String(), "offset": off, "length": length, "size_bytes": len(raw)})
		}
		if _, dup := f.tables[tag]; dup {
			return nil, fontErr("the table directory lists a table twice",
				map[string]any{"table": tag.String()})
		}
		f.tables[tag] = raw[off:end]
		f.order = append(f.order, tag)
	}

	if err := f.readHead(); err != nil {
		return nil, err
	}
	if err := f.readMaxp(); err != nil {
		return nil, err
	}
	if err := f.readHhea(); err != nil {
		return nil, err
	}

	_, f.IsCFF = f.tables[TagCFF]
	if !f.IsCFF {
		if err := f.readLoca(); err != nil {
			return nil, err
		}
	}
	if err := f.readCmap(); err != nil {
		return nil, err
	}
	return f, nil
}

// Table returns a table's bytes. The second result reports presence.
func (f *Font) Table(tag Tag) ([]byte, bool) {
	b, ok := f.tables[tag]
	return b, ok
}

func (f *Font) readHead() error {
	head, ok := f.tables[TagHead]
	if !ok || len(head) < 54 {
		return fontErr("the font program has no usable head table", nil)
	}
	f.UnitsPerEm = int(binary.BigEndian.Uint16(head[18:]))
	if f.UnitsPerEm == 0 {
		return fontErr("head declares unitsPerEm of zero, so no metric can be scaled", nil)
	}
	f.IndexToLocFormat = int(int16(binary.BigEndian.Uint16(head[50:])))
	return nil
}

func (f *Font) readMaxp() error {
	maxp, ok := f.tables[TagMaxp]
	if !ok || len(maxp) < 6 {
		return fontErr("the font program has no usable maxp table", nil)
	}
	f.NumGlyphs = int(binary.BigEndian.Uint16(maxp[4:]))
	if f.NumGlyphs == 0 {
		return fontErr("maxp declares no glyphs", nil)
	}
	return nil
}

func (f *Font) readHhea() error {
	hhea, ok := f.tables[TagHhea]
	if !ok || len(hhea) < 36 {
		return fontErr("the font program has no usable hhea table", nil)
	}
	f.NumberOfHMetrics = int(binary.BigEndian.Uint16(hhea[34:]))
	if f.NumberOfHMetrics == 0 {
		return fontErr("hhea declares no horizontal metrics", nil)
	}
	return nil
}

// readLoca decodes the glyph offset array.
func (f *Font) readLoca() error {
	loca, ok := f.tables[TagLoca]
	if !ok {
		return fontErr("the font program has glyf outlines but no loca table", nil)
	}
	n := f.NumGlyphs + 1
	f.loca = make([]uint32, n)

	switch f.IndexToLocFormat {
	case 0:
		if len(loca) < n*2 {
			return fontErr("the short loca table is too small for the declared glyph count",
				map[string]any{"num_glyphs": f.NumGlyphs, "loca_bytes": len(loca)})
		}
		for i := range n {
			// Short offsets are stored halved, which is the format's way of
			// reaching 128 KB of outlines in sixteen bits.
			f.loca[i] = uint32(binary.BigEndian.Uint16(loca[i*2:])) * 2
		}
	case 1:
		if len(loca) < n*4 {
			return fontErr("the long loca table is too small for the declared glyph count",
				map[string]any{"num_glyphs": f.NumGlyphs, "loca_bytes": len(loca)})
		}
		for i := range n {
			f.loca[i] = binary.BigEndian.Uint32(loca[i*4:])
		}
	default:
		return fontErr("head declares an unknown indexToLocFormat",
			map[string]any{"index_to_loc_format": f.IndexToLocFormat})
	}

	glyf, ok := f.tables[TagGlyf]
	if !ok {
		return fontErr("the font program has a loca table but no glyf table", nil)
	}
	if int(f.loca[n-1]) > len(glyf) {
		return fontErr("loca points past the end of the glyf table",
			map[string]any{"last_offset": f.loca[n-1], "glyf_bytes": len(glyf)})
	}
	return nil
}

// GlyphData returns one glyph's outline bytes, which are empty for a glyph with
// no contours such as a space.
func (f *Font) GlyphData(gid GlyphID) ([]byte, error) {
	if f.IsCFF {
		return nil, fontErr("this font's outlines are in CFF, which this package does not read", nil)
	}
	if int(gid) >= f.NumGlyphs {
		return nil, fontErr("the glyph id is outside the font's glyph array",
			map[string]any{"glyph_id": int(gid), "num_glyphs": f.NumGlyphs})
	}
	start, end := f.loca[gid], f.loca[gid+1]
	if end < start {
		return nil, fontErr("loca offsets for this glyph run backwards",
			map[string]any{"glyph_id": int(gid), "start": start, "end": end})
	}
	glyf := f.tables[TagGlyf]
	if int(end) > len(glyf) {
		return nil, fontErr("this glyph runs past the end of the glyf table",
			map[string]any{"glyph_id": int(gid), "end": end, "glyf_bytes": len(glyf)})
	}
	return glyf[start:end], nil
}

// AdvanceWidth returns the glyph's advance in font units.
//
// Glyphs at or beyond numberOfHMetrics share the last full metric's advance,
// which is how a font with a long tail of equal-width glyphs — most CJK faces,
// and the trailing accents of many Latin ones — avoids storing it repeatedly.
func (f *Font) AdvanceWidth(gid GlyphID) (int, error) {
	hmtx, ok := f.tables[TagHmtx]
	if !ok {
		return 0, fontErr("the font program has no hmtx table", nil)
	}
	i := int(gid)
	if i >= f.NumberOfHMetrics {
		i = f.NumberOfHMetrics - 1
	}
	if (i+1)*4 > len(hmtx) {
		return 0, fontErr("hmtx is too small for the metric this glyph needs",
			map[string]any{"glyph_id": int(gid), "hmtx_bytes": len(hmtx)})
	}
	return int(binary.BigEndian.Uint16(hmtx[i*4:])), nil
}

// GlyphFor returns the glyph the character maps to. The second result reports
// whether the font covers it.
func (f *Font) GlyphFor(r rune) (GlyphID, bool) {
	g, ok := f.cmap[r]
	return g, ok
}

// fontErr builds the package's coded error.
func fontErr(message string, details map[string]any) error {
	if details == nil {
		return verr.NewCodedError(verr.VELLUM_PDF_FONT_INVALID, message)
	}
	return verr.NewCodedErrorWithDetails(verr.VELLUM_PDF_FONT_INVALID, message, details)
}
