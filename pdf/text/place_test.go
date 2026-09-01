package text_test

import (
	"strings"
	"testing"

	"github.com/frankbardon/vellum/pdf/content"
	"github.com/frankbardon/vellum/pdf/font"
	"github.com/frankbardon/vellum/pdf/object"
	"github.com/frankbardon/vellum/pdf/text"
	"golang.org/x/image/font/gofont/goregular"
)

func newFace(t *testing.T) *font.Face {
	t.Helper()
	f, err := font.New(font.Options{
		Resource: "F1", BaseName: "GoRegular",
		Program: goregular.TTF, Plan: font.PlanSubset,
	})
	if err != nil {
		t.Fatalf("font.New: %v", err)
	}
	return f
}

func TestOffset_PlacesTheLine(t *testing.T) {
	const width = object.Real(200 * object.RealScale)
	line := wrap(t, "short", width)[0]
	slack := width - line.Width

	cases := []struct {
		align text.Align
		want  object.Real
	}{
		{text.AlignLeft, 0},
		{text.AlignRight, slack},
		{text.AlignCenter, slack / 2},
		{text.AlignJustify, 0},
	}
	for _, c := range cases {
		t.Run(string(c.align), func(t *testing.T) {
			if got := line.Offset(c.align, width); got != c.want {
				t.Errorf("Offset = %s, want %s", got, c.want)
			}
		})
	}
}

// TestOffset_IsZeroWhenTheLineOverflows pins that a line wider than its measure
// is not pulled left of the origin.
func TestOffset_IsZeroWhenTheLineOverflows(t *testing.T) {
	line := wrap(t, "a reasonably long line of text", object.Points(400))[0]

	for _, align := range text.AllAlignments() {
		if got := line.Offset(align, object.Points(10)); got != 0 {
			t.Errorf("%s offset a too-wide line by %s", align, got)
		}
	}
}

// TestJustifiable_ExcludesTheLastLine pins the rule that keeps a paragraph from
// ending in three words stretched across the measure.
func TestJustifiable_ExcludesTheLastLine(t *testing.T) {
	lines := wrap(t, sample, object.Points(150))
	if len(lines) < 2 {
		t.Fatalf("the sample wrapped into %d lines", len(lines))
	}

	if !lines[0].Justifiable(text.AlignJustify) {
		t.Error("the first line is not justifiable")
	}
	if lines[len(lines)-1].Justifiable(text.AlignJustify) {
		t.Error("the last line of the paragraph is justifiable; it would be stretched across the measure")
	}
	for _, align := range []text.Align{text.AlignLeft, text.AlignRight, text.AlignCenter} {
		if lines[0].Justifiable(align) {
			t.Errorf("a %s line reports as justifiable", align)
		}
	}
}

// TestJustifiable_ExcludesALineEndingAtAHardBreak pins the other half.
func TestJustifiable_ExcludesALineEndingAtAHardBreak(t *testing.T) {
	lines := wrap(t, "one short line\nand another", object.Points(400))
	if len(lines) != 2 {
		t.Fatalf("got %d lines", len(lines))
	}
	for i, l := range lines {
		if l.Justifiable(text.AlignJustify) {
			t.Errorf("line %d ended at a hard break and reports as justifiable", i)
		}
	}
}

// showLine draws one line and returns the content stream and the face the line
// is set in.
//
// The face comes from the line rather than from the test, because a line now
// carries its own: that is the arrangement that makes shaping with one face and
// encoding with another unrepresentable.
func showLine(t *testing.T, l text.Line, align text.Align, width object.Real) (string, *font.Face) {
	t.Helper()

	var b content.Builder
	if err := l.Show(&b, align, width); err != nil {
		t.Fatalf("Show: %v", err)
	}
	return string(b.Bytes()), faceOf(t, l)
}

// faceOf returns the face a single-style line is set in.
func faceOf(t *testing.T, l text.Line) *font.Face {
	t.Helper()
	if len(l.Segments) == 0 {
		t.Fatal("the line has no segments")
	}
	return l.Segments[0].Style.Face
}

func TestShow_UnjustifiedUsesTj(t *testing.T) {
	line := wrap(t, "hello world", object.Points(400))[0]
	got, _ := showLine(t, line, text.AlignLeft, object.Points(400))

	if !strings.Contains(got, "Tj") {
		t.Errorf("an unjustified line does not use Tj: %q", got)
	}
	if strings.Contains(got, "TJ") {
		t.Errorf("an unjustified line uses TJ: %q", got)
	}
}

// TestShow_JustifiedUsesTJWithNegativeAdjustments pins the sign, which is the
// part that is easy to get backwards.
//
// A TJ number is *subtracted* from the pen's displacement, so inserting space
// needs a negative one. Getting it wrong produces a line that gets tighter as it
// is justified, and looks like a kerning bug rather than a sign error.
func TestShow_JustifiedUsesTJWithNegativeAdjustments(t *testing.T) {
	const width = object.Real(150 * object.RealScale)
	lines := wrap(t, sample, width)

	var justified text.Line
	for _, l := range lines {
		if l.Justifiable(text.AlignJustify) {
			justified = l
			break
		}
	}
	if justified.Visible() == 0 {
		t.Fatal("no justifiable line in the sample")
	}

	got, _ := showLine(t, justified, text.AlignJustify, width)
	if !strings.Contains(got, "TJ") {
		t.Fatalf("a justified line does not use TJ: %q", got)
	}
	if !strings.Contains(got, "-") {
		t.Errorf("the TJ array carries no negative adjustment, so the line was tightened rather than spread: %q", got)
	}
	if strings.Contains(got, "Tw") {
		t.Error("the line sets word spacing, which does nothing for a two-byte encoding")
	}
}

// TestShow_JustificationFillsTheMeasureExactly is the property justifying
// promises.
//
// The drawn line — its natural width plus every displacement inserted between
// words — must reach the measure. Dividing the slack by the gap count and
// discarding the remainder leaves every line short by up to one unit per gap,
// which down a column reads as a wobbling right edge rather than a straight one,
// and is the entire thing justification exists to prevent.
//
// The check inverts the TJ conversion rather than trusting it, so a sign error
// or a factor of a thousand shows up as a line that does not reach the measure.
func TestShow_JustificationFillsTheMeasureExactly(t *testing.T) {
	const width = object.Real(150 * object.RealScale)
	const size = object.Real(12 * object.RealScale)

	checked := 0
	for _, l := range wrap(t, sample, width) {
		if !l.Justifiable(text.AlignJustify) {
			continue
		}
		checked++

		stream, _ := showLine(t, l, text.AlignJustify, width)
		numbers := tjNumbers(t, stream)
		if len(numbers) != l.Gaps {
			t.Errorf("the line has %d gaps and %d adjustments", l.Gaps, len(numbers))
			continue
		}

		// displacement = -number/1000 * size, with both number and size held as
		// thousandths.
		var inserted object.Real
		for _, n := range numbers {
			inserted += object.Real(-int64(n) * int64(size) / (1000 * object.RealScale))
		}

		drawn := l.Width + inserted
		// One unit of tolerance per gap: the TJ number is rounded to
		// thousandths, so the inverse cannot be exact.
		if diff := drawn - width; diff > object.Real(l.Gaps) || diff < -object.Real(l.Gaps) {
			t.Errorf("the justified line draws to %s in a %s measure (natural %s, inserted %s)",
				drawn, width, l.Width, inserted)
		}
	}
	if checked == 0 {
		t.Fatal("no justifiable line in the sample; this test would pass vacuously")
	}
}

func TestShow_IsDeterministic(t *testing.T) {
	const width = object.Real(150 * object.RealScale)
	line := wrap(t, sample, width)[0]

	first, _ := showLine(t, line, text.AlignJustify, width)
	for range 25 {
		if got, _ := showLine(t, line, text.AlignJustify, width); got != first {
			t.Fatal("two identical lines drew differently")
		}
	}
}

// TestShow_RecordsGlyphsOnTheFace pins that drawing and subsetting agree.
//
// The subset is built from what the face was told was used. A line drawn without
// telling it would show glyphs the subset dropped, which renders as nothing.
func TestShow_RecordsGlyphsOnTheFace(t *testing.T) {
	line := wrap(t, "hello world", object.Points(400))[0]

	f := faceOf(t, line)
	if len(f.Used()) != 0 {
		t.Fatal("a new face already has glyphs recorded")
	}

	var b content.Builder
	if err := line.Show(&b, text.AlignLeft, object.Points(400)); err != nil {
		t.Fatalf("Show: %v", err)
	}
	// "hello world" has eight distinct glyphs: h e l o space w r d.
	if got := len(f.Used()); got != 8 {
		t.Errorf("the face recorded %d glyphs, want 8 distinct", got)
	}
}

// TestShow_DoesNotDrawTrailingWhitespace pins that a break space stays out of
// the drawn line.
func TestShow_DoesNotDrawTrailingWhitespace(t *testing.T) {
	withSpace := wrap(t, "word ", object.Points(400))[0]
	without := wrap(t, "word", object.Points(400))[0]

	a, _ := showLine(t, withSpace, text.AlignLeft, object.Points(400))
	b, _ := showLine(t, without, text.AlignLeft, object.Points(400))

	if a != b {
		t.Errorf("a trailing space reached the content stream:\n with %q\n without %q", a, b)
	}
}

// tjNumbers extracts the numeric entries from a TJ array, as thousandths.
//
// Parsed to full precision rather than truncated to whole numbers, because the
// test above inverts them and a discarded fraction would look like the rounding
// error it is meant to detect.
func tjNumbers(t *testing.T, stream string) []object.Real {
	t.Helper()

	open := strings.Index(stream, "[")
	shut := strings.LastIndex(stream, "]")
	if open < 0 || shut < open {
		return nil
	}

	var out []object.Real
	for _, field := range strings.Fields(splitOutHexStrings(stream[open+1 : shut])) {
		v, ok := parseReal(field)
		if !ok {
			t.Fatalf("the TJ array holds %q, which is not a number", field)
		}
		out = append(out, v)
	}
	return out
}

// parseReal reads a PDF real into thousandths.
func parseReal(s string) (object.Real, bool) {
	if s == "" {
		return 0, false
	}
	neg := s[0] == '-'
	if neg || s[0] == '+' {
		s = s[1:]
	}

	whole, frac, hasFrac := strings.Cut(s, ".")
	var n int64
	for _, c := range whole {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
	}
	n *= object.RealScale

	if hasFrac {
		scale := int64(object.RealScale / 10)
		for _, c := range frac {
			if c < '0' || c > '9' {
				return 0, false
			}
			if scale > 0 {
				n += int64(c-'0') * scale
				scale /= 10
			}
		}
	}
	if neg {
		n = -n
	}
	return object.Real(n), true
}

// splitOutHexStrings replaces <...> runs with spaces so the numbers can be read.
func splitOutHexStrings(s string) string {
	var b strings.Builder
	depth := 0
	for _, c := range s {
		switch c {
		case '<':
			depth++
			b.WriteByte(' ')
		case '>':
			depth--
			b.WriteByte(' ')
		default:
			if depth == 0 {
				b.WriteRune(c)
			}
		}
	}
	return b.String()
}
