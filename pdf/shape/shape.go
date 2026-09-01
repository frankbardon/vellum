// Package shape turns text into positioned glyphs.
//
// Shaping is what turns a sequence of characters into a sequence of glyphs with
// positions: applying the font's substitutions and kerning, resolving ligatures,
// and attaching marks. It is not the same as looking each character up in a
// character map, and the difference is visible — "office" set without shaping
// has a gap where the ligature should be, and every kerned pair sits fractionally
// wrong.
//
// The implementation is the pure-Go harfbuzz port from go-text/typesetting. That
// is the one part of text layout it would be genuinely unreasonable to write:
// OpenType shaping is a large specification with a large body of
// implementation-defined behaviour, and the reference implementation is the
// specification in practice.
//
// # Units
//
// Positions come back in font design units, as integers, because the harfbuzz
// font is scaled to the face's units per em rather than to a pixel size. Nothing
// here divides, rounds or accumulates in floating point: a shaped run measured
// twice by different routes must give the same answer to the unit, or the same
// paragraph breaks into different lines depending on how it was measured.
//
// Converting to PDF text space happens in [text], once, at the point a size is
// known.
//
// # Scripts
//
// The supported set is declared rather than discovered. A script Vellum has not
// established it can lay out correctly is a named rejection at compose time, not
// a document that renders subtly wrong and is noticed by a reader. See the
// capability matrix rows under text.script.
package shape

import (
	"unicode"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/font/sfnt"
	gofont "github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/harfbuzz"
	"github.com/go-text/typesetting/language"
)

// Script is a writing system Vellum classifies text into.
type Script string

const (
	// ScriptLatin, ScriptGreek and ScriptCyrillic are the systems v1 lays out.
	ScriptLatin    Script = "latin"
	ScriptGreek    Script = "greek"
	ScriptCyrillic Script = "cyrillic"

	// ScriptCommon is punctuation, digits and spaces: characters that belong to
	// no single system and take the surrounding text's.
	ScriptCommon Script = "common"

	// ScriptOther is everything else. It is a rejection, not a fallback.
	ScriptOther Script = "other"
)

// AllScripts returns the classifications, in declaration order.
func AllScripts() []Script {
	return []Script{ScriptLatin, ScriptGreek, ScriptCyrillic, ScriptCommon, ScriptOther}
}

// Supported reports whether v1 lays this script out.
func (s Script) Supported() bool { return s != ScriptOther }

// Glyph is one positioned glyph, in font design units.
type Glyph struct {
	// ID is the glyph in the face.
	ID sfnt.GlyphID

	// Advance is how far the pen moves after drawing it.
	Advance int32

	// XOffset and YOffset displace the glyph without moving the pen. Non-zero
	// for attached marks and for a few kerning arrangements.
	XOffset, YOffset int32

	// Cluster is the index, in runes, of the character this glyph came from.
	// Several glyphs may share one — a decomposed accent — and one glyph may
	// cover several, which is what a ligature is.
	Cluster int
}

// Run is a shaped sequence.
type Run struct {
	// Glyphs are the positioned glyphs, in visual order.
	Glyphs []Glyph

	// Advance is the run's total width, in font design units.
	Advance int32

	// Script is what the text was classified as.
	Script Script
}

// Shaper shapes text with one face.
//
// It holds a second parse of the font program: this package's shaper needs
// go-text's own font model, and [sfnt] needs Vellum's, because the two answer
// different questions — one shapes, the other subsets, and neither library's
// model does both. Parsing twice is the honest cost of that, and it happens once
// per face rather than once per run.
type Shaper struct {
	font *harfbuzz.Font
	upem int
}

// New parses a font program for shaping.
func New(program []byte) (*Shaper, error) {
	face, err := gofont.ParseTTF(newReader(program))
	if err != nil {
		return nil, verr.WrapCodedError(err, verr.VELLUM_PDF_FONT_INVALID,
			"the font program could not be parsed for shaping")
	}

	hb := harfbuzz.NewFont(face)
	upem := int(hb.XScale)
	if upem <= 0 {
		return nil, verr.NewCodedError(verr.VELLUM_PDF_FONT_INVALID,
			"the face declares no units per em, so no advance could be scaled")
	}
	return &Shaper{font: hb, upem: upem}, nil
}

// UnitsPerEm is the face's design grid, which is the unit every advance this
// package reports is in.
func (s *Shaper) UnitsPerEm() int { return s.upem }

// Shape lays out text.
//
// The whole string is shaped as one run. Splitting it at script boundaries would
// matter for a document mixing systems, and v1 does not lay one out: mixed text
// containing an unsupported script is rejected as a whole, so there is no
// arrangement in which the split would change the result.
func (s *Shaper) Shape(text string) (Run, error) {
	script := Classify(text)
	if !script.Supported() {
		return Run{}, verr.NewCodedErrorWithDetails(verr.VELLUM_PDF_SCRIPT_UNSUPPORTED,
			"the text is in a writing system Vellum does not lay out",
			map[string]any{
				"script":    string(script),
				"supported": []any{string(ScriptLatin), string(ScriptGreek), string(ScriptCyrillic)},
				"sample":    firstUnsupported(text),
			})
	}

	runes := []rune(text)
	if len(runes) == 0 {
		return Run{Script: script}, nil
	}

	buf := harfbuzz.NewBuffer()
	buf.AddRunes(runes, 0, len(runes))
	buf.Props = harfbuzz.SegmentProperties{
		Direction: harfbuzz.LeftToRight,
		Script:    language.LookupScript(representative(runes)),
	}
	buf.Shape(s.font, nil)

	out := Run{Glyphs: make([]Glyph, len(buf.Info)), Script: script}
	for i := range buf.Info {
		pos := buf.Pos[i]

		// Glyph zero is .notdef, and a shaper reaching it means the face has no
		// glyph for the character. It has to be caught here: the encoder's
		// character-map lookup would have refused the same text, but shaping
		// does not go through that lookup, so an unmapped character arrives as
		// a silent empty box rather than as an error. That box is the failure
		// mode this library refuses everywhere else — nobody notices it until
		// the document has been sent.
		if buf.Info[i].Glyph == 0 {
			return Run{}, verr.NewCodedErrorWithDetails(verr.VELLUM_PDF_GLYPH_MISSING,
				"the face has no glyph for this character",
				map[string]any{
					"character":  clusterText(runes, buf.Info[i].Cluster),
					"code_point": clusterCodePoint(runes, buf.Info[i].Cluster),
					"script":     string(script),
				})
		}

		out.Glyphs[i] = Glyph{
			ID:      sfnt.GlyphID(buf.Info[i].Glyph),
			Advance: int32(pos.XAdvance),
			XOffset: int32(pos.XOffset),
			YOffset: int32(pos.YOffset),
			Cluster: buf.Info[i].Cluster,
		}
		out.Advance += int32(pos.XAdvance)
	}
	return out, nil
}

// clusterText returns the character a cluster index points at, for an error.
func clusterText(runes []rune, cluster int) string {
	if cluster < 0 || cluster >= len(runes) {
		return ""
	}
	return string(runes[cluster])
}

// clusterCodePoint returns the code point a cluster index points at.
func clusterCodePoint(runes []rune, cluster int) int {
	if cluster < 0 || cluster >= len(runes) {
		return 0
	}
	return int(runes[cluster])
}

// Classify reports the writing system of a string.
//
// The rule is deliberately strict: the result is the single supported system
// present, or ScriptOther if any character belongs to none of them. Characters
// common to every system — spaces, digits, most punctuation — do not decide the
// answer, and a string made only of them is ScriptCommon.
//
// Two different supported systems in one string is also ScriptOther. Mixing them
// is legitimate and Vellum does not yet establish it lays the mixture out
// correctly, so it is a named rejection rather than a guess.
func Classify(text string) Script {
	found := ScriptCommon

	for _, r := range text {
		s := classifyRune(r)
		switch {
		case s == ScriptCommon:
			continue
		case s == ScriptOther:
			return ScriptOther
		case found == ScriptCommon:
			found = s
		case found != s:
			return ScriptOther
		}
	}
	return found
}

// classifyRune places one character.
func classifyRune(r rune) Script {
	switch {
	case r == '\n' || r == '\r' || r == '\t':
		return ScriptCommon
	case unicode.Is(unicode.Latin, r):
		return ScriptLatin
	case unicode.Is(unicode.Greek, r):
		return ScriptGreek
	case unicode.Is(unicode.Cyrillic, r):
		return ScriptCyrillic
	case unicode.Is(unicode.Common, r), unicode.Is(unicode.Inherited, r):
		return ScriptCommon
	default:
		return ScriptOther
	}
}

// representative returns a rune to derive the harfbuzz script property from.
//
// The first character belonging to a specific system, because a leading quote or
// digit is Common and would make harfbuzz choose its default shaping behaviour
// rather than the one the text actually needs.
func representative(runes []rune) rune {
	for _, r := range runes {
		if classifyRune(r) != ScriptCommon {
			return r
		}
	}
	return runes[0]
}

// firstUnsupported returns the first character that caused a rejection, so the
// error names something the author can find in their own text.
func firstUnsupported(text string) string {
	var seen Script = ScriptCommon
	for _, r := range text {
		s := classifyRune(r)
		if s == ScriptOther {
			return string(r)
		}
		if s == ScriptCommon {
			continue
		}
		if seen == ScriptCommon {
			seen = s
			continue
		}
		if seen != s {
			return string(r)
		}
	}
	return ""
}
