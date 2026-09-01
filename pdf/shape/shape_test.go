package shape_test

import (
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/font/sfnt"
	"github.com/frankbardon/vellum/pdf/shape"
	"golang.org/x/image/font/gofont/goregular"
)

func newShaper(t *testing.T) *shape.Shaper {
	t.Helper()
	s, err := shape.New(goregular.TTF)
	if err != nil {
		t.Fatalf("shape.New: %v", err)
	}
	return s
}

func TestNew_RejectsAnUnparseableProgram(t *testing.T) {
	_, err := shape.New([]byte("not a font"))
	if !verr.HasCode(err, verr.VELLUM_PDF_FONT_INVALID) {
		t.Fatalf("got %v, want VELLUM_PDF_FONT_INVALID", err)
	}
}

func TestShaper_ReportsFontUnits(t *testing.T) {
	s := newShaper(t)
	if got := s.UnitsPerEm(); got != 2048 {
		t.Errorf("UnitsPerEm = %d, want the face's 2048", got)
	}
}

func TestShape_ProducesOneGlyphPerLatinCharacter(t *testing.T) {
	run, err := newShaper(t).Shape("Hello")
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}
	if len(run.Glyphs) != 5 {
		t.Fatalf("got %d glyphs for five characters", len(run.Glyphs))
	}
	if run.Advance <= 0 {
		t.Errorf("the run has advance %d", run.Advance)
	}
	if run.Script != shape.ScriptLatin {
		t.Errorf("classified as %q, want latin", run.Script)
	}
}

// TestShape_AdvanceIsTheSumOfItsGlyphs pins the invariant the line breaker
// depends on.
func TestShape_AdvanceIsTheSumOfItsGlyphs(t *testing.T) {
	run, err := newShaper(t).Shape("The quick brown fox")
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}
	var sum int32
	for _, g := range run.Glyphs {
		sum += g.Advance
	}
	if sum != run.Advance {
		t.Errorf("the run reports advance %d and its glyphs sum to %d", run.Advance, sum)
	}
}

// TestShape_ComposesADecomposedAccent is what distinguishes shaping from a
// character-map lookup.
//
// "é" written as e followed by U+0301 COMBINING ACUTE ACCENT is two characters.
// A lookup produces two glyphs: the e, and .notdef for the combining mark, which
// this face has no glyph for — a visible empty box beside the letter. The shaper
// normalises the pair and produces the single precomposed glyph, the same one
// the single-character form gives.
//
// This is the strongest available demonstration that shaping is happening, and
// it is weaker than it should be: see TestShape_TheTestFaceCarriesNoLayoutTables.
func TestShape_ComposesADecomposedAccent(t *testing.T) {
	s := newShaper(t)

	decomposed, err := s.Shape("e\u0301")
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}
	precomposed, err := s.Shape("\u00e9")
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}

	if len(decomposed.Glyphs) != 1 {
		t.Fatalf("the decomposed form produced %d glyphs; it was not composed", len(decomposed.Glyphs))
	}
	if decomposed.Glyphs[0].ID != precomposed.Glyphs[0].ID {
		t.Errorf("the decomposed form gave glyph %d and the precomposed one %d",
			decomposed.Glyphs[0].ID, precomposed.Glyphs[0].ID)
	}
	if decomposed.Advance != precomposed.Advance {
		t.Errorf("the two forms advance by %d and %d", decomposed.Advance, precomposed.Advance)
	}
}

// TestShape_TheTestFaceCarriesNoLayoutTables records the limit of what this
// suite establishes.
//
// Go Regular has no GPOS, no GSUB, no GDEF and no kern. It carries no OpenType
// layout data at all, so harfbuzz has nothing to apply: with this face the
// shaper is provably equivalent to a character-map lookup plus Unicode
// normalisation. Kerning, ligatures and mark attachment are implemented — by the
// shaping library — and are not exercised by anything here.
//
// This is stated as a test rather than a comment so that the day a face with
// layout tables enters the tree, it fails and says why. That is the point at
// which the kerning and ligature assertions become writable, and until then
// pretending otherwise would be the more comfortable and less honest option.
func TestShape_TheTestFaceCarriesNoLayoutTables(t *testing.T) {
	f, err := sfnt.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("parsing the test face: %v", err)
	}

	for _, tag := range []sfnt.Tag{
		{'G', 'P', 'O', 'S'}, {'G', 'S', 'U', 'B'}, {'G', 'D', 'E', 'F'}, {'k', 'e', 'r', 'n'},
	} {
		if _, ok := f.Table(tag); ok {
			t.Errorf("the test face now carries a %s table.\n\n"+
				"Shaping can finally be tested for what it is for. Add assertions for kerning "+
				"(a pair like AV shaping narrower than the sum of its glyphs) and for ligature "+
				"substitution (fi shaping to one glyph), and delete this test.", tag)
		}
	}
}

// TestShape_RefusesAnUnmappedCharacter pins that a shaped .notdef is an error.
//
// The encoder's character-map lookup refuses a character the face does not
// cover. Shaping does not go through that lookup, so without this check an
// unmapped character arrives as glyph zero and draws an empty box — the failure
// mode this library refuses everywhere else, and one nobody notices until the
// document has been sent.
func TestShape_RefusesAnUnmappedCharacter(t *testing.T) {
	// A lone combining acute: the face has no glyph for it, and with nothing to
	// compose onto, shaping resolves it to .notdef.
	_, err := newShaper(t).Shape("a\u0301\u0301\u0301")
	if !verr.HasCode(err, verr.VELLUM_PDF_GLYPH_MISSING) {
		t.Fatalf("got %v, want VELLUM_PDF_GLYPH_MISSING", err)
	}
}

// TestShape_ClustersMapBackToRunes pins what the line breaker uses to find a
// break position.
func TestShape_ClustersMapBackToRunes(t *testing.T) {
	const text = "one two three"
	run, err := newShaper(t).Shape(text)
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}

	previous := -1
	for i, g := range run.Glyphs {
		if g.Cluster < 0 || g.Cluster >= len([]rune(text)) {
			t.Fatalf("glyph %d has cluster %d, outside the text", i, g.Cluster)
		}
		if g.Cluster < previous {
			t.Fatalf("glyph %d has cluster %d after %d; clusters must ascend for left-to-right text",
				i, g.Cluster, previous)
		}
		previous = g.Cluster
	}
}

func TestShape_IsDeterministic(t *testing.T) {
	s := newShaper(t)
	first, err := s.Shape("Determinism, repeatedly.")
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}
	for range 25 {
		got, err := s.Shape("Determinism, repeatedly.")
		if err != nil {
			t.Fatalf("Shape: %v", err)
		}
		if len(got.Glyphs) != len(first.Glyphs) || got.Advance != first.Advance {
			t.Fatal("two identical shapes differ")
		}
		for i := range got.Glyphs {
			if got.Glyphs[i] != first.Glyphs[i] {
				t.Fatalf("glyph %d differs between two identical shapes", i)
			}
		}
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want shape.Script
	}{
		{"latin", "Hello, world", shape.ScriptLatin},
		{"greek", "Ελληνικά", shape.ScriptGreek},
		{"cyrillic", "Привет", shape.ScriptCyrillic},
		{"digits and punctuation alone", "12.5% (400) — 1/3", shape.ScriptCommon},
		{"latin with digits", "Base 400", shape.ScriptLatin},
		{"empty", "", shape.ScriptCommon},
		{"arabic", "مرحبا", shape.ScriptOther},
		{"han", "汉字", shape.ScriptOther},
		{"mixed supported systems", "Hello Привет", shape.ScriptOther},
		{"newlines are common", "a\nb", shape.ScriptLatin},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shape.Classify(c.in); got != c.want {
				t.Errorf("Classify(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestShape_RefusesAnUndeclaredScript pins that the supported set is declared
// rather than discovered by a reader.
//
// A script laid out with the wrong algorithm produces a document that renders
// and is wrong — Arabic set left to right and unjoined, say. Nobody's test
// catches that; a reader does, after it has been sent.
func TestShape_RefusesAnUndeclaredScript(t *testing.T) {
	for _, text := range []string{"مرحبا", "汉字", "Hello Привет"} {
		_, err := newShaper(t).Shape(text)
		if !verr.HasCode(err, verr.VELLUM_PDF_SCRIPT_UNSUPPORTED) {
			t.Errorf("Shape(%q) = %v, want VELLUM_PDF_SCRIPT_UNSUPPORTED", text, err)
			continue
		}
		var ce *verr.CodedError
		if errorsAs(err, &ce) {
			if _, ok := ce.Detail("sample"); !ok {
				t.Errorf("Shape(%q) does not name the offending character", text)
			}
			if _, ok := ce.Detail("supported"); !ok {
				t.Errorf("Shape(%q) does not name what is supported", text)
			}
		}
	}
}

func TestShape_AcceptsEveryDeclaredScript(t *testing.T) {
	s := newShaper(t)
	for _, text := range []string{"Latin text", "Ελληνικά", "Привет", "12.5%"} {
		if _, err := s.Shape(text); err != nil {
			t.Errorf("Shape(%q): %v", text, err)
		}
	}
}

func TestAllScripts_IsComplete(t *testing.T) {
	got := shape.AllScripts()
	if len(got) != 5 {
		t.Fatalf("AllScripts returned %d entries", len(got))
	}
	supported := 0
	for _, s := range got {
		if s.Supported() {
			supported++
		}
	}
	if supported != 4 {
		t.Errorf("%d scripts report as supported, want 4 (three systems plus common)", supported)
	}
}
