package sfnt_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/font/sfnt"
	"golang.org/x/image/font/gofont/goregular"
	xsfnt "golang.org/x/image/font/sfnt"
)

func parseGoRegular(t *testing.T) *sfnt.Font {
	t.Helper()
	f, err := sfnt.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("parsing the test face: %v", err)
	}
	return f
}

func TestParse_ReadsTheFaceHeader(t *testing.T) {
	f := parseGoRegular(t)

	if f.UnitsPerEm != 2048 {
		t.Errorf("UnitsPerEm = %d, want 2048", f.UnitsPerEm)
	}
	if f.NumGlyphs < 200 {
		t.Errorf("NumGlyphs = %d, which is too few for this face", f.NumGlyphs)
	}
	if f.IsCFF {
		t.Error("the face was read as CFF; it has glyf outlines")
	}
	if f.NumberOfHMetrics == 0 {
		t.Error("NumberOfHMetrics is zero")
	}
}

func TestParse_RejectsMalformedInput(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{"empty", nil},
		{"too short for a header", []byte{0x00, 0x01, 0x00}},
		{"unknown sfnt version", []byte{'n', 'o', 'p', 'e', 0, 0, 0, 0, 0, 0, 0, 0}},
		{"a collection", []byte{'t', 't', 'c', 'f', 0, 0, 0, 0, 0, 0, 0, 0}},
		{"directory past the end", append([]byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x40}, make([]byte, 6)...)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := sfnt.Parse(c.in)
			if !verr.HasCode(err, verr.VELLUM_PDF_FONT_INVALID) {
				t.Fatalf("got %v, want VELLUM_PDF_FONT_INVALID", err)
			}
		})
	}
}

// TestParse_TruncationIsCaughtEverywhere feeds progressively truncated copies
// of a real face and requires every one to be refused rather than to panic.
//
// The parser reads offsets and lengths out of the file and then indexes with
// them, which is the shape of code that reads untrusted input and panics on it.
// A theme's font arrives through the asset resolver, so this input is exactly
// as trusted as whatever the host wired up.
func TestParse_TruncationIsCaughtEverywhere(t *testing.T) {
	for n := 1; n < len(goregular.TTF); n = n*2 + 1 {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("parsing %d bytes panicked: %v", n, r)
				}
			}()
			if _, err := sfnt.Parse(goregular.TTF[:n]); err == nil {
				t.Fatalf("parsing %d bytes of a %d byte face succeeded", n, len(goregular.TTF))
			}
		}()
	}
}

func TestGlyphFor_MapsLatin(t *testing.T) {
	f := parseGoRegular(t)
	for _, r := range "Hello, world" {
		if _, ok := f.GlyphFor(r); !ok {
			t.Errorf("the face does not map %q", r)
		}
	}
	if _, ok := f.GlyphFor('\uFFFF'); ok {
		t.Error("the face claims to map a noncharacter")
	}
}

// TestRuneFor_IsTheLowestMatch pins the tie-break.
//
// A glyph reachable from several characters must always report the same one,
// because that character is what text extraction shows. Ranging the map would
// return whichever the runtime happened to visit first.
func TestRuneFor_IsTheLowestMatch(t *testing.T) {
	f := parseGoRegular(t)
	g, ok := f.GlyphFor('A')
	if !ok {
		t.Fatal("the face does not map 'A'")
	}
	first, ok := f.RuneFor(g)
	if !ok {
		t.Fatal("no rune maps to the glyph for 'A'")
	}
	for range 20 {
		again, _ := f.RuneFor(g)
		if again != first {
			t.Fatalf("RuneFor returned %q then %q for one glyph", first, again)
		}
	}
}

func TestAdvanceWidth_IsPositiveForLetters(t *testing.T) {
	f := parseGoRegular(t)
	g, _ := f.GlyphFor('M')
	w, err := f.AdvanceWidth(g)
	if err != nil {
		t.Fatalf("AdvanceWidth: %v", err)
	}
	if w <= 0 || w > f.UnitsPerEm*2 {
		t.Errorf("the advance for 'M' is %d units on a %d grid, which is not plausible", w, f.UnitsPerEm)
	}
}

// glyphsFor maps a string to its glyph ids.
func glyphsFor(t *testing.T, f *sfnt.Font, s string) []sfnt.GlyphID {
	t.Helper()
	var out []sfnt.GlyphID
	for _, r := range s {
		g, ok := f.GlyphFor(r)
		if !ok {
			t.Fatalf("the face does not map %q", r)
		}
		out = append(out, g)
	}
	return out
}

func TestSubset_RetainsTheGlyphsAskedFor(t *testing.T) {
	f := parseGoRegular(t)
	want := glyphsFor(t, f, "Hello, world")

	sub, err := f.SubsetGlyphs(want)
	if err != nil {
		t.Fatalf("SubsetGlyphs: %v", err)
	}

	got, err := sfnt.Parse(sub.Program)
	if err != nil {
		t.Fatalf("the subset does not parse: %v", err)
	}
	if got.NumGlyphs != f.NumGlyphs {
		t.Errorf("the subset declares %d glyphs, want %d preserved", got.NumGlyphs, f.NumGlyphs)
	}

	for _, g := range want {
		src, err := f.GlyphData(g)
		if err != nil {
			t.Fatalf("source glyph %d: %v", g, err)
		}
		out, err := got.GlyphData(g)
		if err != nil {
			t.Fatalf("subset glyph %d: %v", g, err)
		}
		// The subset pads each glyph to a four-byte boundary, so the retained
		// outline is a prefix of what loca now spans.
		if !bytes.HasPrefix(out, src) {
			t.Errorf("glyph %d was rewritten rather than copied", g)
		}
	}
}

// TestSubset_DiscardsWhatWasNotAskedFor is the check that the subset is
// actually a subset.
func TestSubset_DiscardsWhatWasNotAskedFor(t *testing.T) {
	f := parseGoRegular(t)
	sub, err := f.SubsetGlyphs(glyphsFor(t, f, "Hi"))
	if err != nil {
		t.Fatalf("SubsetGlyphs: %v", err)
	}
	got, err := sfnt.Parse(sub.Program)
	if err != nil {
		t.Fatalf("the subset does not parse: %v", err)
	}

	kept := map[sfnt.GlyphID]bool{}
	for _, g := range sub.Glyphs {
		kept[g] = true
	}

	dropped, emptied := 0, 0
	for g := range f.NumGlyphs {
		if kept[sfnt.GlyphID(g)] {
			continue
		}
		dropped++
		out, err := got.GlyphData(sfnt.GlyphID(g))
		if err != nil {
			t.Fatalf("subset glyph %d: %v", g, err)
		}
		if len(out) == 0 {
			emptied++
		}
	}
	if dropped == 0 {
		t.Fatal("nothing was dropped; the subset retained the whole face")
	}
	if emptied != dropped {
		t.Errorf("%d of %d discarded glyphs still carry outlines", dropped-emptied, dropped)
	}

	if len(sub.Program) >= len(goregular.TTF)/2 {
		t.Errorf("the subset is %d bytes against a %d byte source, which is not a useful reduction",
			len(sub.Program), len(goregular.TTF))
	}
}

// TestSubset_PullsInCompositeComponents is the correctness property that a
// naive subsetter gets wrong.
//
// An accented letter is a composite: a reference to the base glyph and to the
// accent. Retaining the composite without its components produces a glyph that
// renders as nothing, and the failure is invisible until somebody reads a
// document containing an accent.
func TestSubset_PullsInCompositeComponents(t *testing.T) {
	f := parseGoRegular(t)

	// Find an accented character the face maps as a composite.
	var composite sfnt.GlyphID
	found := false
	for _, r := range "áéíóúàèäöüçñÁÉÍÓÚ" {
		g, ok := f.GlyphFor(r)
		if !ok {
			continue
		}
		data, err := f.GlyphData(g)
		if err != nil || len(data) < 10 {
			continue
		}
		if int16(binary.BigEndian.Uint16(data)) < 0 {
			composite, found = g, true
			break
		}
	}
	if !found {
		t.Skip("this face has no composite glyph among the accented Latin letters")
	}

	sub, err := f.SubsetGlyphs([]sfnt.GlyphID{composite})
	if err != nil {
		t.Fatalf("SubsetGlyphs: %v", err)
	}
	if len(sub.Glyphs) < 3 {
		t.Fatalf("subsetting one composite retained %d glyphs; its components were not pulled in", len(sub.Glyphs))
	}

	got, err := sfnt.Parse(sub.Program)
	if err != nil {
		t.Fatalf("the subset does not parse: %v", err)
	}
	for _, g := range sub.Glyphs {
		src, _ := f.GlyphData(g)
		out, err := got.GlyphData(g)
		if err != nil {
			t.Fatalf("subset glyph %d: %v", g, err)
		}
		if len(src) > 0 && len(out) == 0 {
			t.Errorf("glyph %d was retained in the set but carries no outline", g)
		}
	}
}

// TestSubset_ChecksumAdjustmentIsCorrect verifies the field that is computed
// over the finished file.
//
// It is the last thing written and the easiest to get wrong, because it depends
// on every byte including its own zeroed placeholder. A wrong value does not
// stop most readers, which is why it needs a test rather than a rendering
// check: validators do report it.
func TestSubset_ChecksumAdjustmentIsCorrect(t *testing.T) {
	f := parseGoRegular(t)
	sub, err := f.SubsetGlyphs(glyphsFor(t, f, "Hello"))
	if err != nil {
		t.Fatalf("SubsetGlyphs: %v", err)
	}
	got, err := sfnt.Parse(sub.Program)
	if err != nil {
		t.Fatalf("the subset does not parse: %v", err)
	}
	head, ok := got.Table(sfnt.TagHead)
	if !ok {
		t.Fatal("the subset has no head table")
	}
	stored := binary.BigEndian.Uint32(head[8:])

	// Recompute the way a validator does: zero the field, sum the file, and
	// subtract from the magic constant.
	program := append([]byte(nil), sub.Program...)
	headAt := bytes.Index(sub.Program, head)
	binary.BigEndian.PutUint32(program[headAt+8:], 0)

	var sum uint32
	for i := 0; i < len(program); i += 4 {
		var word [4]byte
		copy(word[:], program[i:])
		sum += binary.BigEndian.Uint32(word[:])
	}
	if want := 0xB1B0AFBA - sum; stored != want {
		t.Errorf("head.checkSumAdjustment is %#08x, want %#08x", stored, want)
	}
}

// TestSubset_PinsTheFontDates is the determinism property this package exists
// for.
//
// The one permissively licensed Go subsetter writes time.Now() into
// head.modified. That is inside the byte stream Vellum is required to pin, and
// it is the reason none of the available libraries could be used.
func TestSubset_PinsTheFontDates(t *testing.T) {
	f := parseGoRegular(t)
	sub, err := f.SubsetGlyphs(glyphsFor(t, f, "Hello"))
	if err != nil {
		t.Fatalf("SubsetGlyphs: %v", err)
	}
	got, err := sfnt.Parse(sub.Program)
	if err != nil {
		t.Fatalf("the subset does not parse: %v", err)
	}

	srcHead, _ := f.Table(sfnt.TagHead)
	outHead, _ := got.Table(sfnt.TagHead)

	// created at 20..28, modified at 28..36, both LONGDATETIME.
	if !bytes.Equal(srcHead[20:36], outHead[20:36]) {
		t.Errorf("the subset's dates differ from the source's:\n source % x\n subset % x",
			srcHead[20:36], outHead[20:36])
	}
}

// TestSubset_IsDeterministic emits the same subset repeatedly.
func TestSubset_IsDeterministic(t *testing.T) {
	f := parseGoRegular(t)
	want := glyphsFor(t, f, "Determinism")

	first, err := f.SubsetGlyphs(want)
	if err != nil {
		t.Fatalf("SubsetGlyphs: %v", err)
	}
	for range 25 {
		g, err := f.SubsetGlyphs(want)
		if err != nil {
			t.Fatalf("SubsetGlyphs: %v", err)
		}
		if !bytes.Equal(first.Program, g.Program) {
			t.Fatal("two identical subsets produced different bytes")
		}
		if g.Tag != first.Tag {
			t.Fatalf("the subset tag varied: %q then %q", first.Tag, g.Tag)
		}
	}
}

// TestSubsetTag_DependsOnTheGlyphSet pins that two different subsets of one
// face are distinguishable.
func TestSubsetTag_DependsOnTheGlyphSet(t *testing.T) {
	f := parseGoRegular(t)

	a, err := f.SubsetGlyphs(glyphsFor(t, f, "Hello"))
	if err != nil {
		t.Fatalf("SubsetGlyphs: %v", err)
	}
	b, err := f.SubsetGlyphs(glyphsFor(t, f, "Goodbye"))
	if err != nil {
		t.Fatalf("SubsetGlyphs: %v", err)
	}
	if a.Tag == b.Tag {
		t.Errorf("two different subsets share the tag %q; a consumer merging files would conflate them", a.Tag)
	}
	if len(a.Tag) != 6 {
		t.Errorf("the tag %q is not six letters", a.Tag)
	}
	for _, c := range a.Tag {
		if c < 'A' || c > 'Z' {
			t.Errorf("the tag %q is not all uppercase letters", a.Tag)
			break
		}
	}
}

func TestSubset_RejectsAGlyphOutsideTheFace(t *testing.T) {
	f := parseGoRegular(t)
	_, err := f.SubsetGlyphs([]sfnt.GlyphID{sfnt.GlyphID(f.NumGlyphs)})
	if !verr.HasCode(err, verr.VELLUM_PDF_FONT_INVALID) {
		t.Fatalf("got %v, want VELLUM_PDF_FONT_INVALID", err)
	}
}

// TestSubset_ParsesInAnIndependentReader is the check our own parser cannot
// make.
//
// Reading the subset back with the parser that wrote it proves the two agree
// and nothing more — the same closed loop that let three OOXML defects through.
// x/image/font/sfnt is an unrelated implementation, so its acceptance is
// evidence from outside.
func TestSubset_ParsesInAnIndependentReader(t *testing.T) {
	f := parseGoRegular(t)
	text := "Hello, world"
	want := glyphsFor(t, f, text)

	sub, err := f.SubsetGlyphs(want)
	if err != nil {
		t.Fatalf("SubsetGlyphs: %v", err)
	}

	other, err := xsfnt.Parse(sub.Program)
	if err != nil {
		t.Fatalf("an independent parser rejects the subset: %v", err)
	}
	if got := other.NumGlyphs(); got != f.NumGlyphs {
		t.Errorf("the independent parser sees %d glyphs, want %d", got, f.NumGlyphs)
	}

	// Advances must survive, because they are what the PDF's /W array will
	// declare and a disagreement shows as text that overlaps or spreads.
	var b xsfnt.Buffer
	for _, g := range want {
		srcAdvance, err := f.AdvanceWidth(g)
		if err != nil {
			t.Fatalf("AdvanceWidth: %v", err)
		}
		got, err := other.GlyphAdvance(&b, xsfnt.GlyphIndex(g),
			fixedInt(f.UnitsPerEm), 0)
		if err != nil {
			t.Fatalf("the independent parser cannot measure glyph %d: %v", g, err)
		}
		if int(got>>6) != srcAdvance {
			t.Errorf("glyph %d advance is %d in the subset, %d in the source", g, int(got>>6), srcAdvance)
		}
	}
}
