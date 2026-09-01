package text_test

import (
	stderrors "errors"
	"strings"
	"sync"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/font/sfnt"
	"github.com/frankbardon/vellum/pdf/object"
	"github.com/frankbardon/vellum/pdf/shape"
	"github.com/frankbardon/vellum/pdf/text"
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

const sample = "The quick brown fox jumps over the lazy dog and keeps going for a while longer."

func wrap(t *testing.T, s string, width object.Real) []text.Line {
	t.Helper()
	lines, err := text.Wrap(newShaper(t), s, text.WrapOptions{
		Size: object.Points(12), Width: width,
	})
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	return lines
}

func TestWrap_RejectsDegenerateGeometry(t *testing.T) {
	s := newShaper(t)
	for _, opts := range []text.WrapOptions{
		{Size: 0, Width: object.Points(200)},
		{Size: object.Points(12), Width: 0},
		{Size: object.Points(-1), Width: object.Points(200)},
	} {
		if _, err := text.Wrap(s, "hello", opts); err == nil {
			t.Errorf("Wrap accepted size=%s width=%s", opts.Size, opts.Width)
		}
	}
}

func TestWrap_EmptyTextProducesNoLines(t *testing.T) {
	if lines := wrap(t, "", object.Points(200)); len(lines) != 0 {
		t.Errorf("got %d lines for empty text", len(lines))
	}
}

// TestWrap_EveryLineFitsTheMeasure is the property the whole package exists for.
func TestWrap_EveryLineFitsTheMeasure(t *testing.T) {
	for _, width := range []object.Real{
		object.Points(80), object.Points(120), object.Points(200), object.Points(400),
	} {
		t.Run(width.String()+"pt", func(t *testing.T) {
			lines := wrap(t, sample, width)
			if len(lines) == 0 {
				t.Fatal("no lines")
			}
			for i, l := range lines {
				if l.Width > width {
					t.Errorf("line %d is %s wide in a %s measure: %q", i, l.Width, width, lineText(l))
				}
			}
		})
	}
}

// TestWrap_ANarrowerMeasureNeedsMoreLines pins that the fill responds to the
// width at all.
func TestWrap_ANarrowerMeasureNeedsMoreLines(t *testing.T) {
	wide := wrap(t, sample, object.Points(400))
	narrow := wrap(t, sample, object.Points(120))

	if len(narrow) <= len(wide) {
		t.Errorf("a 120pt measure took %d lines and a 400pt one took %d", len(narrow), len(wide))
	}
}

// TestWrap_LosesNoText pins that breaking is a partition.
//
// A greedy fill that advances its start index wrongly drops or repeats a word,
// and the result still looks like a paragraph. Reassembling the glyph clusters
// and comparing against the input is what catches it.
func TestWrap_LosesNoText(t *testing.T) {
	const input = "one two three four five six seven eight nine ten eleven twelve"

	for _, width := range []object.Real{object.Points(60), object.Points(100), object.Points(300)} {
		lines := wrap(t, input, width)

		var got []string
		for _, l := range lines {
			got = append(got, strings.Fields(lineText(l))...)
		}
		want := strings.Fields(input)

		if len(got) != len(want) {
			t.Fatalf("at %s: %d words came back from %d, as %q", width, len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("at %s: word %d is %q, want %q", width, i, got[i], want[i])
			}
		}
	}
}

// TestWrap_IsGreedy pins the fill strategy.
//
// Greedy means each line takes the last opportunity that fits, so no line can be
// extended by moving the first word of the next line up onto it. An optimal-fit
// shaper would legitimately fail this, which is the point: the two produce
// different breaks, and a document's pagination must not change because a later
// version got cleverer about ragged edges.
func TestWrap_IsGreedy(t *testing.T) {
	const width = object.Real(150 * object.RealScale)
	lines := wrap(t, sample, width)

	for i := range len(lines) - 1 {
		this, next := lines[i], lines[i+1]
		if this.Mandatory {
			continue
		}
		firstWord := strings.Fields(lineText(next))
		if len(firstWord) == 0 {
			continue
		}

		combined := strings.TrimRight(lineText(this), " ") + " " + firstWord[0]
		measured := wrap(t, combined, object.Points(10000))
		if len(measured) != 1 {
			t.Fatalf("the probe wrapped: %q", combined)
		}
		if measured[0].Width <= width {
			t.Errorf("line %d could have taken %q and did not; the fill is not greedy",
				i, firstWord[0])
		}
	}
}

// TestWrap_BreaksAtMandatoryBreaks pins that a newline ends a line.
func TestWrap_BreaksAtMandatoryBreaks(t *testing.T) {
	lines := wrap(t, "first line\nsecond line", object.Points(400))

	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if !strings.HasPrefix(lineText(lines[0]), "first") {
		t.Errorf("line 0 is %q", lineText(lines[0]))
	}
	if !strings.HasPrefix(lineText(lines[1]), "second") {
		t.Errorf("line 1 is %q", lineText(lines[1]))
	}
}

// TestWrap_KeepsBlankLines pins that a gap the author asked for survives.
func TestWrap_KeepsBlankLines(t *testing.T) {
	lines := wrap(t, "above\n\nbelow", object.Points(400))
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3 with the blank one kept", len(lines))
	}
	if lines[1].Visible != 0 {
		t.Errorf("the middle line is not blank: %q", lineText(lines[1]))
	}
}

// TestWrap_TrailingSpaceHangsIntoTheMargin pins the measurement rule.
//
// The space at a break belongs to the text and is kept on the line, but it does
// not occupy the measure — which is what makes a ragged right edge look straight
// rather than jittering by a space width per line.
func TestWrap_TrailingSpaceHangsIntoTheMargin(t *testing.T) {
	withSpace := wrap(t, "word ", object.Points(400))
	without := wrap(t, "word", object.Points(400))

	if len(withSpace) != 1 || len(without) != 1 {
		t.Fatalf("got %d and %d lines", len(withSpace), len(without))
	}
	if withSpace[0].Width != without[0].Width {
		t.Errorf("a trailing space changed the measured width: %s against %s",
			withSpace[0].Width, without[0].Width)
	}
	if withSpace[0].Visible != 4 {
		t.Errorf("the trailing space was counted as visible: %d glyphs", withSpace[0].Visible)
	}
	if len(withSpace[0].Glyphs) != 5 {
		t.Errorf("the trailing space was dropped from the line: %d glyphs", len(withSpace[0].Glyphs))
	}
}

// TestWrap_RefusesToBreakMidWord pins that an over-wide unbreakable piece is a
// diagnosable failure rather than a break Unicode does not permit.
func TestWrap_RefusesToBreakMidWord(t *testing.T) {
	_, err := text.Wrap(newShaper(t), "Pneumonoultramicroscopicsilicovolcanoconiosis",
		text.WrapOptions{Size: object.Points(12), Width: object.Points(20)})

	if !verr.HasCode(err, verr.VELLUM_PDF_TEXT_OVERFLOW) {
		t.Fatalf("got %v, want VELLUM_PDF_TEXT_OVERFLOW", err)
	}
	var ce *verr.CodedError
	if stderrors.As(err, &ce) {
		if v, ok := ce.Detail("text"); !ok || v == "" {
			t.Error("the error does not name the text that would not fit")
		}
	}
}

// TestWrap_BreaksAtAHyphen pins that UAX#14 opportunities are honoured, not just
// spaces.
func TestWrap_BreaksAtAHyphen(t *testing.T) {
	lines, err := text.Wrap(newShaper(t), "well-being",
		text.WrapOptions{Size: object.Points(12), Width: object.Points(40)})
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want a break after the hyphen", len(lines))
	}
	if got := lineText(lines[0]); !strings.HasSuffix(got, "-") {
		t.Errorf("line 0 is %q, want it to end at the hyphen", got)
	}
}

func TestWrap_IsDeterministic(t *testing.T) {
	first := wrap(t, sample, object.Points(150))
	for range 25 {
		got := wrap(t, sample, object.Points(150))
		if len(got) != len(first) {
			t.Fatal("two identical wraps produced different line counts")
		}
		for i := range got {
			if got[i].Width != first[i].Width || got[i].Visible != first[i].Visible {
				t.Fatalf("line %d differs between two identical wraps", i)
			}
		}
	}
}

// TestWrap_MeasuresWithoutFloatAccumulation pins that a line's width does not
// depend on how it was reached.
//
// Converting each glyph to text space and summing would round per glyph; the
// width would then differ from the same glyphs measured as one run, and a
// paragraph would break differently depending on which code path measured it.
func TestWrap_MeasuresWithoutFloatAccumulation(t *testing.T) {
	s := newShaper(t)
	const line = "The quick brown fox"

	whole, err := text.Wrap(s, line, text.WrapOptions{Size: object.Points(12), Width: object.Points(10000)})
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	if len(whole) != 1 {
		t.Fatalf("the probe wrapped into %d lines", len(whole))
	}

	run, err := s.Shape(line)
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}
	var advance int64
	for _, g := range run.Glyphs {
		advance += int64(g.Advance)
	}
	// The same conversion, done directly: advance * size / upem, rounded once.
	upem := int64(s.UnitsPerEm())
	want := object.Real((advance*int64(object.Points(12)) + upem/2) / upem)

	if whole[0].Width != want {
		t.Errorf("the wrapped width is %s and the direct conversion gives %s", whole[0].Width, want)
	}
}

// lineText reassembles a line's text from its glyphs.
//
// Usable only because the test face maps one glyph per character and carries no
// ligatures — TestShape_TheTestFaceCarriesNoLayoutTables records that. A face
// with substitutions would make this lossy, and the assertions above would have
// to be about glyph identifiers rather than about words.
func lineText(l text.Line) string {
	table := reverseCmap()

	var b strings.Builder
	for _, g := range l.Glyphs {
		if r, ok := table[g.ID]; ok {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var (
	reverseOnce  sync.Once
	reverseTable map[sfnt.GlyphID]rune
)

// reverseCmap builds the glyph-to-character table once.
//
// The lowest character mapping to each glyph, so a glyph reachable from several
// characters resolves the same way on every run — the same rule [sfnt.Font.RuneFor]
// follows, for the same reason.
func reverseCmap() map[sfnt.GlyphID]rune {
	reverseOnce.Do(func() {
		f, err := sfnt.Parse(goregular.TTF)
		if err != nil {
			panic("text_test: the test face does not parse: " + err.Error())
		}
		reverseTable = make(map[sfnt.GlyphID]rune)
		for _, r := range f.Runes() {
			g, ok := f.GlyphFor(r)
			if !ok {
				continue
			}
			if existing, seen := reverseTable[g]; !seen || r < existing {
				reverseTable[g] = r
			}
		}
	})
	return reverseTable
}
