package content_test

import (
	"strings"
	"testing"

	"github.com/frankbardon/vellum/pdf/content"
	"github.com/frankbardon/vellum/pdf/object"
)

func TestBuilder_EmitsPostfixOperators(t *testing.T) {
	var b content.Builder
	b.BeginText().
		SetFont("F1", object.Points(12)).
		SetLeading(object.Thousandths(14400)).
		MoveText(object.Points(72), object.Points(720)).
		ShowGlyphs([]uint16{0x0024, 0x0025}).
		NextLine().
		EndText()

	want := strings.Join([]string{
		"BT",
		"/F1 12 Tf",
		"14.4 TL",
		"72 720 Td",
		"<00240025> Tj",
		"T*",
		"ET",
		"",
	}, "\n")

	if got := string(b.Bytes()); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestShowGlyphs_UsesHexRatherThanALiteralString pins the encoding choice.
//
// Glyph identifiers are small numbers, so their low bytes land on parentheses
// and backslashes constantly. A literal string would need escaping for every
// one of them, and an escape missed produces a content stream that reparses as
// something else entirely rather than failing.
func TestShowGlyphs_UsesHexRatherThanALiteralString(t *testing.T) {
	var b content.Builder
	// 0x0028 is '(' and 0x005C is '\\'.
	b.ShowGlyphs([]uint16{0x0028, 0x005C, 0x0029})

	got := string(b.Bytes())
	if !strings.HasPrefix(got, "<0028005C0029>") {
		t.Errorf("got %q, want a hex string", got)
	}
	if strings.ContainsAny(got, "()") {
		t.Errorf("got %q, which carries unescaped string delimiters", got)
	}
}

func TestBuilder_TracksTextObjectDepth(t *testing.T) {
	var b content.Builder
	if b.InText() {
		t.Error("a new builder reports being inside a text object")
	}
	b.BeginText()
	if !b.InText() {
		t.Error("BeginText did not open a text object")
	}
	b.EndText()
	if b.InText() {
		t.Error("EndText did not close the text object")
	}
	// An unbalanced ET must not drive the depth negative, or a later BeginText
	// would report the wrong state.
	b.EndText()
	if b.InText() {
		t.Error("an unbalanced EndText left the builder inside a text object")
	}
}

func TestBuilder_GraphicsOperators(t *testing.T) {
	var b content.Builder
	b.Save().
		SetFillRGB(object.Ratio(1, 2), object.Thousandths(0), object.Points(1)).
		Rect(object.Points(10), object.Points(20), object.Points(30), object.Points(40)).
		Fill().
		Restore()

	want := "q\n0.5 0 1 rg\n10 20 30 40 re\nf\nQ\n"
	if got := string(b.Bytes()); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestBuilder_IsDeterministic(t *testing.T) {
	build := func() string {
		var b content.Builder
		b.BeginText().
			SetFont("F1", object.Points(11)).
			MoveText(object.Points(72), object.Points(700)).
			ShowGlyphs([]uint16{1, 2, 3}).
			EndText()
		return string(b.Bytes())
	}

	first := build()
	for range 25 {
		if build() != first {
			t.Fatal("two identical content streams differ")
		}
	}
}
