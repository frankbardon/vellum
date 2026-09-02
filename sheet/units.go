package sheet

import "strconv"

// emuPerPoint is the EMU definition: 914400 per inch, 72 points per inch.
const emuPerPoint = 12700

// pointTenths converts EMU to tenths of a point, rounding half away from
// zero.
//
// Integer arithmetic throughout. A font size that round-trips through float64
// accumulates error that shows as a one-tenth-of-a-point disagreement between
// two runs of the same specification — a determinism failure wearing a
// rounding bug's clothes. The tenth-of-a-point granularity is Excel's own:
// `<sz val="10.5"/>` is a value it writes, and nothing finer is meaningful in
// a styles part nobody measures text against.
func pointTenths(emu int64) int64 {
	n := emu * 10
	const d = emuPerPoint
	if n < 0 {
		return (n - d/2) / d
	}
	return (n + d/2) / d
}

// formatPoints renders a point value from tenths, dropping the fraction when
// it is zero so an ordinary size reads "11" rather than "11.0" — which is
// cosmetic, but it is also what an authored file looks like, and a Vellum
// workbook sitting beside one should not be picked out by that alone.
func formatPoints(tenths int64) string {
	whole, frac := tenths/10, tenths%10
	if frac < 0 {
		frac = -frac
	}
	if frac == 0 {
		return strconv.FormatInt(whole, 10)
	}
	return strconv.FormatInt(whole, 10) + "." + strconv.FormatInt(frac, 10)
}

// columnLetter renders a 1-based column number in A1 notation: 1 is "A", 26 is
// "Z", 27 is "AA".
func columnLetter(col int) string {
	if col < 1 {
		col = 1
	}
	var b []byte
	for col > 0 {
		col--
		b = append([]byte{byte('A' + col%26)}, b...)
		col /= 26
	}
	return string(b)
}

// cellRef renders a 1-based (row, column) pair as an A1 reference.
func cellRef(row, col int) string {
	return columnLetter(col) + strconv.Itoa(row)
}

// rangeRef renders a rectangular block of cells as an A1:B2 range reference.
func rangeRef(fromRow, fromCol, toRow, toCol int) string {
	return cellRef(fromRow, fromCol) + ":" + cellRef(toRow, toCol)
}
