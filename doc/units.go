package doc

// WordprocessingML measures in three units, and which one applies depends on
// where in the document the number sits. Getting one wrong produces a file that
// opens and is silently the wrong size, so the conversions live here rather
// than being written out at each call site.
const (
	// emuPerInch is the EMU definition. Everything below derives from it.
	emuPerInch = 914400

	// twipsPerInch is twentieths of a point: 20 x 72.
	twipsPerInch = 1440

	// emuPerTwip is what page geometry, margins and spacing are written in.
	emuPerTwip = emuPerInch / twipsPerInch // 635

	// emuPerHalfPoint is what font sizes are written in.
	emuPerHalfPoint = emuPerInch / 144 // 6350
)

// twips converts EMU to twentieths of a point, rounding half away from zero.
//
// Integer arithmetic throughout. A measurement that round-trips through float64
// accumulates error that shows as a one-twip disagreement between two runs of
// the same specification — a determinism failure wearing a rounding bug's
// clothes.
func twips(emu int64) int { return int(roundDiv(emu, emuPerTwip)) }

// halfPoints converts EMU to half-points, the unit w:sz carries.
func halfPoints(emu int64) int { return int(roundDiv(emu, emuPerHalfPoint)) }

// lineRule converts a line-height multiple and a type size to the w:line value,
// which is in twips when w:lineRule is "auto"... except that it is not: with
// lineRule="auto" the value is in 240ths of a line. 240 is single spacing.
func lineRule(multiple float64) int {
	if multiple <= 0 {
		return 240
	}
	return int(multiple*240 + 0.5)
}

// roundDiv divides rounding half away from zero, which is what a reader
// comparing a Vellum document against an authored one would expect. Go's
// integer division truncates toward zero, which loses half a unit on every
// conversion and accumulates visibly over a page of spacing.
func roundDiv(n, d int64) int64 {
	if d == 0 {
		return 0
	}
	if (n < 0) != (d < 0) {
		return (n - d/2) / d
	}
	return (n + d/2) / d
}
