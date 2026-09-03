// Package fragtest generates realistic multi-run WordprocessingML paragraph
// fixtures whose {{marker}} text is fragmented across several w:r runs the
// same way Word's own editing behaviors fragment it, rather than the way a
// test author would naively type it as a single run.
//
// Five behaviors are modelled, one per [Strategy]: a spell-check re-run
// splitting a word mid-marker, a language-mark boundary, a save-revision-ID
// boundary, a paste boundary, and the debris an accepted tracked change can
// leave sitting next to a marker without wrapping it.
//
// Every fixture is pure: [Fragment] is a function of its two arguments only
// -- no time, no counters, no map iteration -- so the same [Strategy] and the
// same marker string produce the same bytes on every call, in every process.
// That is deliberate: this package exists to feed golden-style and fuzz-style
// tests in later stories (E10, E11) as well as this one, and a generator
// whose own output drifts between runs would be useless for that.
//
// template/anchor, template/defrag and template/splice already have their
// own hand-written unit tests for the individual mechanisms this package
// exercises; Fragment's job is systematic coverage across the five
// fragmentation shapes, not a replacement for those.
package fragtest

import (
	"strings"

	"github.com/frankbardon/vellum/xmlcopy"
)

// Strategy names one real Word editing behavior that fragments a
// {{marker}}'s text across multiple w:r runs.
type Strategy int

const (
	// MidWordSpellCheck splits the marker at an arbitrary internal rune
	// boundary -- not aligned to "{{" or "}}" -- into two runs carrying
	// identical (empty) formatting, the shape a spell-check re-run
	// produces: it does not usually change formatting, only where the run
	// boundary falls. This is the case that defeats a naive
	// regex-over-raw-XML matcher, since the literal marker text never
	// appears contiguous in the source bytes.
	MidWordSpellCheck Strategy = iota

	// LanguageMarkBoundary splits the marker where the two runs' w:rPr
	// disagree on w:lang -- the shape Word's own autocorrect can leave when
	// it decided the language changed mid-word. Both runs are entirely
	// consumed by the match, so the only formatting source is the first
	// touched run's own rPr.
	LanguageMarkBoundary

	// RevisionSaveIDSplit splits the marker across two runs carrying
	// different w:rsidR / w:rsidRPr attributes directly on the w:r
	// elements themselves (not on w:rPr) -- the shape a save touching only
	// part of a paragraph leaves. The first run also carries a bold w:rPr,
	// so a test can confirm the formatting basis still comes through
	// correctly regardless of the rsid attributes neither run's own
	// [xmlcopy.Element] carries forward past template/defrag.
	RevisionSaveIDSplit

	// PasteBoundary splits the marker across four runs simulating
	// pasted-then-typed content: a boundary run mixing non-marker text with
	// the match's own start, two fully-consumed middle runs each carrying
	// different formatting (bold, then italic), and a closing boundary run
	// mixing the match's own end with trailing non-marker text.
	PasteBoundary

	// AcceptedTrackedChangeResidue places the marker's own two runs --
	// themselves carrying residual w:rsidR / w:rsidRPr attributes --
	// immediately after an empty w:ins shell: the debris Word can leave
	// behind after a tracked insertion is accepted, sitting next to the
	// marker rather than wrapping it.
	AcceptedTrackedChangeResidue
)

// String names the strategy, for subtest names and failure messages.
func (s Strategy) String() string {
	switch s {
	case MidWordSpellCheck:
		return "mid_word_spell_check"
	case LanguageMarkBoundary:
		return "language_mark_boundary"
	case RevisionSaveIDSplit:
		return "revision_save_id_split"
	case PasteBoundary:
		return "paste_boundary"
	case AcceptedTrackedChangeResidue:
		return "accepted_tracked_change_residue"
	default:
		return "unknown"
	}
}

// All returns every strategy, in declaration order. Copy-returning, per this
// codebase's registry convention: nothing here is a package-level slice a
// caller could otherwise mutate, but a fixed, deliberate return shape costs
// nothing and keeps this package consistent with the rest of the tree.
func All() []Strategy {
	return []Strategy{
		MidWordSpellCheck,
		LanguageMarkBoundary,
		RevisionSaveIDSplit,
		PasteBoundary,
		AcceptedTrackedChangeResidue,
	}
}

// Fragment returns a <w:p>...</w:p> WordprocessingML paragraph: marker (for
// example "{{customer_name}}") wrapped in fixed surrounding text ("See " ...
// " today."), with marker's own runs split according to strategy.
//
// marker must be at least six runes long (enough for "{{" + a two-rune name
// + "}}"); every caller in this tree passes a realistic marker name, and a
// shorter one is a caller bug rather than a shape this package tries to
// accommodate gracefully.
func Fragment(strategy Strategy, marker string) []byte {
	const prefix = "See "
	const suffix = " today."

	var runs string
	switch strategy {
	case MidWordSpellCheck:
		runs = midWordSpellCheck(marker)
	case LanguageMarkBoundary:
		runs = languageMarkBoundary(marker)
	case RevisionSaveIDSplit:
		runs = revisionSaveIDSplit(marker)
	case PasteBoundary:
		runs = pasteBoundary(marker)
	case AcceptedTrackedChangeResidue:
		runs = acceptedTrackedChangeResidue(marker)
	default:
		panic("fragtest: unknown strategy")
	}

	var b strings.Builder
	b.WriteString("<w:p>")
	b.WriteString(plainRun(prefix))
	b.WriteString(runs)
	b.WriteString(plainRun(suffix))
	b.WriteString("</w:p>")
	return []byte(b.String())
}

// plainRun renders an unformatted <w:r><w:t>...</w:t></w:r>.
func plainRun(text string) string {
	return "<w:r><w:t>" + xmlcopy.EscapeText(text) + "</w:t></w:r>"
}

// splitPoint returns a rune index strictly inside marker's own name -- away
// from the "{{" and "}}" delimiters at each end -- so a two-way split lands
// genuinely mid-word rather than coincidentally on a brace boundary.
func splitPoint(runes []rune) int {
	n := len(runes)
	lo, hi := 2, n-2
	if hi <= lo {
		// A marker too short to have an interior falls back to the
		// midpoint. Nothing in this package promises every possible
		// marker string is long enough to have one; Fragment's own doc
		// comment states the minimum length every caller in this tree
		// actually uses.
		mid := n / 2
		if mid == 0 {
			mid = 1
		}
		if mid >= n {
			mid = n - 1
		}
		return mid
	}
	return lo + (hi-lo)/2
}

func midWordSpellCheck(marker string) string {
	runes := []rune(marker)
	split := splitPoint(runes)
	a, b := string(runes[:split]), string(runes[split:])
	return plainRun(a) + plainRun(b)
}

func languageMarkBoundary(marker string) string {
	runes := []rune(marker)
	split := splitPoint(runes)
	a, b := string(runes[:split]), string(runes[split:])
	return `<w:r><w:rPr><w:lang w:val="en-US"/></w:rPr><w:t>` + xmlcopy.EscapeText(a) + `</w:t></w:r>` +
		`<w:r><w:rPr><w:lang w:val="en-GB"/></w:rPr><w:t>` + xmlcopy.EscapeText(b) + `</w:t></w:r>`
}

func revisionSaveIDSplit(marker string) string {
	runes := []rune(marker)
	split := splitPoint(runes)
	a, b := string(runes[:split]), string(runes[split:])
	return `<w:r w:rsidR="00AB1111" w:rsidRPr="00AB1111"><w:rPr><w:b/></w:rPr><w:t>` + xmlcopy.EscapeText(a) + `</w:t></w:r>` +
		`<w:r w:rsidR="00AB2222" w:rsidRPr="00AB2222"><w:rPr><w:i/></w:rPr><w:t>` + xmlcopy.EscapeText(b) + `</w:t></w:r>`
}

func pasteBoundary(marker string) string {
	runes := []rune(marker)
	n := len(runes)
	q1, q2, q3 := n/4, n/2, (3*n)/4
	if q1 < 1 {
		q1 = 1
	}
	if q2 <= q1 {
		q2 = q1 + 1
	}
	if q3 <= q2 {
		q3 = q2 + 1
	}
	if q3 >= n {
		q3 = n - 1
	}
	p1 := string(runes[:q1])
	p2 := string(runes[q1:q2])
	p3 := string(runes[q2:q3])
	p4 := string(runes[q3:])

	// p1 and p4 each mix non-marker text ("mid" / "tail") with the match's
	// own edge, so they survive the splice as a defrag.Piece; p2 and p3 are
	// entirely inside the match and carry different formatting each, so
	// both are discarded along with it -- the multi-run middle-consumption
	// case with more than one fully-consumed run in the middle.
	return `<w:r><w:t>mid` + xmlcopy.EscapeText(p1) + `</w:t></w:r>` +
		`<w:r><w:rPr><w:b/></w:rPr><w:t>` + xmlcopy.EscapeText(p2) + `</w:t></w:r>` +
		`<w:r><w:rPr><w:i/></w:rPr><w:t>` + xmlcopy.EscapeText(p3) + `</w:t></w:r>` +
		`<w:r><w:t>` + xmlcopy.EscapeText(p4) + `tail</w:t></w:r>`
}

func acceptedTrackedChangeResidue(marker string) string {
	runes := []rune(marker)
	split := splitPoint(runes)
	a, b := string(runes[:split]), string(runes[split:])
	// An empty w:ins shell -- carrying no w:r of its own, so it contributes
	// nothing to the flattened text template/defrag.Flatten builds --
	// sitting immediately before the marker's own runs without wrapping
	// them. This is the debris a real Word document can carry after a
	// tracked insertion elsewhere in the paragraph was accepted.
	shell := `<w:ins w:id="900" w:author="Reviewer" w:date="2020-01-01T00:00:00Z"/>`
	return shell +
		`<w:r w:rsidR="00CC1111"><w:t>` + xmlcopy.EscapeText(a) + `</w:t></w:r>` +
		`<w:r w:rsidRPr="00CC2222"><w:t>` + xmlcopy.EscapeText(b) + `</w:t></w:r>`
}
