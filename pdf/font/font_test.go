package font_test

import (
	"bytes"
	"encoding/binary"
	"sort"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/font"
	"github.com/frankbardon/vellum/pdf/font/sfnt"
	"github.com/frankbardon/vellum/pdf/object"
	"golang.org/x/image/font/gofont/goregular"
)

// newFace builds a face over the test program, failing the test if it cannot.
func newFace(t *testing.T, opts font.Options) *font.Face {
	t.Helper()
	if opts.Resource == "" {
		opts.Resource = "F1"
	}
	if opts.BaseName == "" {
		opts.BaseName = "GoRegular"
	}
	if opts.Program == nil {
		opts.Program = goregular.TTF
	}
	if opts.Plan == "" {
		opts.Plan = font.PlanSubset
	}

	f, err := font.New(opts)
	if err != nil {
		t.Fatalf("font.New: %v", err)
	}
	return f
}

func TestNew_RequiresAnExplicitPlan(t *testing.T) {
	_, err := font.New(font.Options{Resource: "F1", BaseName: "X", Program: goregular.TTF})
	if !verr.HasCode(err, verr.VELLUM_PDF_FONT_INVALID) {
		t.Fatalf("got %v, want VELLUM_PDF_FONT_INVALID", err)
	}
}

func TestNew_RequiresAResourceName(t *testing.T) {
	_, err := font.New(font.Options{BaseName: "X", Program: goregular.TTF, Plan: font.PlanSubset})
	if !verr.HasCode(err, verr.VELLUM_PDF_FONT_INVALID) {
		t.Fatalf("got %v, want VELLUM_PDF_FONT_INVALID", err)
	}
}

func TestNew_RejectsAnUnparseableProgram(t *testing.T) {
	_, err := font.New(font.Options{Resource: "F1", BaseName: "X",
		Program: []byte("not a font"), Plan: font.PlanSubset})
	if !verr.HasCode(err, verr.VELLUM_PDF_FONT_INVALID) {
		t.Fatalf("got %v, want VELLUM_PDF_FONT_INVALID", err)
	}
}

func TestEncode_MapsTextAndRecordsUsage(t *testing.T) {
	f := newFace(t, font.Options{})

	gids, err := f.Encode("Hello")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(gids) != 5 {
		t.Fatalf("Encode returned %d glyphs for five characters", len(gids))
	}
	// "Hello" has four distinct glyphs; the repeated l is recorded once.
	if got := len(f.Used()); got != 4 {
		t.Errorf("Used() has %d glyphs, want 4 distinct", got)
	}
}

// TestUsed_IsSortedRegardlessOfTextOrder pins the determinism property.
//
// The subset is a property of which glyphs the document contains, not of the
// order the text happened to be written. Two documents with the same glyphs
// must produce the same subset and the same subset tag, or a consumer merging
// them sees two faces where there is one.
func TestUsed_IsSortedRegardlessOfTextOrder(t *testing.T) {
	a := newFace(t, font.Options{})
	b := newFace(t, font.Options{})

	if _, err := a.Encode("abcdef"); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if _, err := b.Encode("fedcba"); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	ua, ub := a.Used(), b.Used()
	if len(ua) != len(ub) {
		t.Fatalf("the two faces recorded %d and %d glyphs", len(ua), len(ub))
	}
	if !sort.SliceIsSorted(ua, func(i, j int) bool { return ua[i] < ua[j] }) {
		t.Error("Used() is not sorted")
	}
	for i := range ua {
		if ua[i] != ub[i] {
			t.Fatalf("the same glyphs in a different text order produced different sets: %v vs %v", ua, ub)
		}
	}
}

// TestEncode_RefusesAnUncoveredCharacter pins that a missing glyph is an error.
//
// A substitution here would render a row of empty boxes, which nobody notices
// until the document has been sent; reaching for another installed face would
// make the same specification render differently on two machines.
func TestEncode_RefusesAnUncoveredCharacter(t *testing.T) {
	f := newFace(t, font.Options{})

	_, err := f.Encode("字")
	if !verr.HasCode(err, verr.VELLUM_PDF_GLYPH_MISSING) {
		t.Fatalf("got %v, want VELLUM_PDF_GLYPH_MISSING", err)
	}
	var ce *verr.CodedError
	if errorsAs(err, &ce) {
		if v, ok := ce.Detail("character"); !ok || v != "字" {
			t.Errorf("the error does not name the character: %v", ce.Details)
		}
	}
}

func TestWrite_RefusesAFaceWithNoGlyphsUsed(t *testing.T) {
	f := newFace(t, font.Options{})

	var doc object.Document
	_, err := f.Write(&doc)
	if !verr.HasCode(err, verr.VELLUM_PDF_FONT_INVALID) {
		t.Fatalf("got %v, want VELLUM_PDF_FONT_INVALID", err)
	}
}

// writeFace embeds a face into a document and returns the written bytes.
func writeFace(t *testing.T, f *font.Face, text string) []byte {
	t.Helper()

	if _, err := f.Encode(text); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	var doc object.Document
	ref, err := f.Write(&doc)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	doc.Root = doc.Add(object.NewDict("Type", object.Name("Catalog"), "Font", ref))

	var buf bytes.Buffer
	if err := doc.Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return buf.Bytes()
}

func TestWrite_TrueTypeIsACIDFontType2(t *testing.T) {
	raw := writeFace(t, newFace(t, font.Options{}), "Hello, world")

	for _, want := range []string{
		"/Subtype /Type0",
		"/Encoding /Identity-H",
		"/Subtype /CIDFontType2",
		"/CIDToGIDMap /Identity",
		"/FontFile2 ",
		"/ToUnicode ",
		"/Length1 ",
	} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Errorf("the embedded font does not carry %q", want)
		}
	}
	if bytes.Contains(raw, []byte("/FontFile3")) {
		t.Error("a TrueType face was filed under FontFile3")
	}
}

// TestWrite_SubsetIsSmallerThanWhole pins that the plan is acted on.
func TestWrite_SubsetIsSmallerThanWhole(t *testing.T) {
	subset := writeFace(t, newFace(t, font.Options{Plan: font.PlanSubset}), "Hello, world")
	whole := writeFace(t, newFace(t, font.Options{Plan: font.PlanWhole}), "Hello, world")

	if len(subset) >= len(whole) {
		t.Errorf("the subset embedding is %d bytes and the whole one %d; the plan was not acted on",
			len(subset), len(whole))
	}
}

// TestWrite_TagsDifferByGlyphSet pins that two subsets of one face are
// distinguishable to a consumer merging documents.
func TestWrite_TagsDifferByGlyphSet(t *testing.T) {
	a := writeFace(t, newFace(t, font.Options{}), "Hello")
	b := writeFace(t, newFace(t, font.Options{}), "Goodbye")

	if bytes.Equal(baseFontName(t, a), baseFontName(t, b)) {
		t.Errorf("two different subsets share the base font name %q", baseFontName(t, a))
	}
}

// TestWrite_WholeEmbeddingStillCarriesATag pins that the prefix is written even
// when the program was not reduced.
//
// Two documents embedding the same face with different glyph sets are different
// subsets from a consumer's point of view — their /W arrays differ — so PDF
// requires the base font name to distinguish them regardless of whether the
// programs are byte-identical.
func TestWrite_WholeEmbeddingStillCarriesATag(t *testing.T) {
	raw := writeFace(t, newFace(t, font.Options{Plan: font.PlanWhole}), "Hello")

	name := baseFontName(t, raw)
	if len(name) < 7 || name[6] != '+' {
		t.Errorf("the base font name %q carries no six-letter subset prefix", name)
	}
}

var baseFontRe = []byte("/BaseFont /")

// baseFontName extracts the first BaseFont name from a written document.
func baseFontName(t *testing.T, raw []byte) []byte {
	t.Helper()

	i := bytes.Index(raw, baseFontRe)
	if i < 0 {
		t.Fatal("the document carries no BaseFont")
	}
	rest := raw[i+len(baseFontRe):]
	end := bytes.IndexAny(rest, "/ \n\r\t<>[]")
	if end < 0 {
		t.Fatal("the BaseFont name does not terminate")
	}
	return rest[:end]
}

func TestWrite_IsDeterministic(t *testing.T) {
	first := writeFace(t, newFace(t, font.Options{}), "Determinism")
	for range 25 {
		if got := writeFace(t, newFace(t, font.Options{}), "Determinism"); !bytes.Equal(first, got) {
			t.Fatal("two identical embeddings produced different bytes")
		}
	}
}

// --- CFF -------------------------------------------------------------------

// TestNew_RejectsASubsetOfACFFFace is the licence-sensitive refusal.
//
// A theme setting embed: "subset" may be recording a condition of the font's
// licence rather than a preference. Vellum does not subset CFF, so the only
// alternatives are to fail or to quietly embed the whole program — and the
// second substitutes Vellum's judgement for the vendor's on a question Vellum
// cannot see the answer to.
func TestNew_RejectsASubsetOfACFFFace(t *testing.T) {
	_, err := font.New(font.Options{
		Resource: "F1", BaseName: "SomeCFF",
		Program: syntheticCFF(t), Plan: font.PlanSubset,
	})
	if !verr.HasCode(err, verr.VELLUM_FONT_EMBED_UNSUPPORTED) {
		t.Fatalf("got %v, want VELLUM_FONT_EMBED_UNSUPPORTED", err)
	}
	var ce *verr.CodedError
	if errorsAs(err, &ce) {
		if _, ok := ce.Detail("hint"); !ok {
			t.Error("the error does not say what to do instead")
		}
	}
}

// TestWrite_CFFIsACIDFontType0 pins the other half of the CFF path.
//
// The descendant subtype and the descriptor key are not interchangeable with
// the TrueType ones: the subtype tells the reader how to interpret the embedded
// program, and a mismatch produces a font that loads and draws nothing.
func TestWrite_CFFIsACIDFontType0(t *testing.T) {
	f, err := font.New(font.Options{
		Resource: "F1", BaseName: "SomeCFF",
		Program: syntheticCFF(t), Plan: font.PlanWhole,
	})
	if err != nil {
		t.Fatalf("font.New: %v", err)
	}
	raw := writeFace(t, f, "Hello")

	for _, want := range []string{
		"/Subtype /CIDFontType0",
		"/FontFile3 ",
		"/Subtype /OpenType",
	} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Errorf("the embedded CFF face does not carry %q", want)
		}
	}
	for _, unwanted := range []string{"/CIDFontType2", "/FontFile2", "/CIDToGIDMap"} {
		if bytes.Contains(raw, []byte(unwanted)) {
			t.Errorf("the embedded CFF face carries %q, which describes a table it does not have", unwanted)
		}
	}
}

// syntheticCFF builds an OTTO-flavoured program out of the test face's real
// tables plus a stand-in CFF table.
//
// A structural stand-in, not a font: the CFF payload is arbitrary bytes and
// nothing would render from it. That is enough and no more than enough for what
// these tests assert, which is that a CFF face takes the CIDFontType0 path and
// is filed under FontFile3 — decisions this package makes from the table
// directory alone, without reading an outline.
//
// The honest alternative is committing a real OTF, which means a binary in
// testdata and a licence to carry for a code path that copies bytes through
// unchanged. If CFF ever gains behaviour that depends on the outlines, this
// stops being adequate and that trade changes.
func syntheticCFF(t *testing.T) []byte {
	t.Helper()

	src, err := sfnt.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("parsing the test face: %v", err)
	}

	// Bytewise tag order, which is what a table directory requires.
	tags := []sfnt.Tag{sfnt.TagCFF, sfnt.TagCmap, sfnt.TagHead, sfnt.TagHhea,
		sfnt.TagHmtx, sfnt.TagMaxp, sfnt.TagOS2, sfnt.TagPost}
	sort.Slice(tags, func(i, j int) bool {
		return bytes.Compare(tags[i][:], tags[j][:]) < 0
	})

	type entry struct {
		tag  sfnt.Tag
		data []byte
	}
	var tables []entry
	for _, tag := range tags {
		if tag == sfnt.TagCFF {
			tables = append(tables, entry{tag: tag, data: []byte("stand-in CFF payload")})
			continue
		}
		data, ok := src.Table(tag)
		if !ok {
			t.Fatalf("the test face has no %s table", tag)
		}
		tables = append(tables, entry{tag: tag, data: data})
	}

	dirSize := 12 + len(tables)*16
	out := make([]byte, dirSize)
	binary.BigEndian.PutUint32(out, 0x4F54544F) // 'OTTO'
	binary.BigEndian.PutUint16(out[4:], uint16(len(tables)))

	for i, tb := range tables {
		offset := len(out)
		out = append(out, tb.data...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
		rec := out[12+i*16:]
		copy(rec[:4], tb.tag[:])
		binary.BigEndian.PutUint32(rec[8:], uint32(offset))
		binary.BigEndian.PutUint32(rec[12:], uint32(len(tb.data)))
	}
	return out
}
