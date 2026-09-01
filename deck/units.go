package deck

// PresentationML measures geometry in EMU directly, which is the one mercy this
// format offers over WordprocessingML. Font sizes and a few spacings are not in
// EMU, and getting one wrong produces a deck that opens and is silently the
// wrong size, so the conversions live here rather than at each call site.
const (
	// emuPerInch is the EMU definition. Everything below derives from it.
	emuPerInch = 914400

	// emuPerPoint is 1/72 inch.
	emuPerPoint = emuPerInch / 72 // 12700

	// emuPerHundredthPoint is what a:rPr/@sz carries.
	emuPerHundredthPoint = emuPerInch / 7200 // 127
)

// hundredthPoints converts EMU to hundredths of a point, the unit a:rPr/@sz and
// a:spcPts/@val carry.
func hundredthPoints(emu int64) int64 { return roundDiv(emu, emuPerHundredthPoint) }

// percent converts a multiple to the thousandths-of-a-percent unit DrawingML
// uses for line spacing and shading: 1.2 becomes 120000.
func percent(multiple float64) int64 {
	if multiple <= 0 {
		return 0
	}
	return int64(multiple*100000 + 0.5)
}

// roundDiv divides rounding half away from zero.
//
// Go's integer division truncates toward zero, which loses half a unit on every
// conversion and accumulates visibly over a deck's worth of spacing.
func roundDiv(n, d int64) int64 {
	if d == 0 {
		return 0
	}
	if (n < 0) != (d < 0) {
		return (n - d/2) / d
	}
	return (n + d/2) / d
}

// Cell insets, in EMU. DrawingML's own defaults, and written explicitly on
// every table cell rather than left to the reader.
//
// The distinction matters because the overflow split is computed from them. A
// writer that computes a row height from an assumed inset and then lets the
// reader apply its own has computed a capacity for a table it did not write —
// which is how a split that fits on paper overflows on screen.
const (
	cellMarginV = 45720 // 0.05 inch, top and bottom
	cellMarginH = 91440 // 0.10 inch, left and right
)
