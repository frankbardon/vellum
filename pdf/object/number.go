package object

import "strconv"

// RealScale is the fixed-point denominator: a [Real] counts thousandths.
//
// A thousandth of a point is finer than any device resolves and finer than the
// three decimal places PDF producers conventionally emit, so nothing is lost by
// working in it.
const RealScale = 1000

// Real is a PDF real number held as fixed point, in thousandths.
//
// Fixed point rather than float64, for the same reason lengths elsewhere in
// Vellum are EMU: a float64 that has been added, scaled and rounded reaches the
// output as one of several nearby values depending on the order the operations
// happened to occur in, and the bytes then differ between runs that computed
// the same thing by different routes. An integer cannot do that.
//
// This also removes float formatting from the output path entirely. Go's
// shortest-representation formatting is deterministic, but it is deterministic
// about a value that already varies, which is the wrong end of the problem.
type Real int64

// Points returns the Real for a whole number of points.
func Points(p int64) Real { return Real(p * RealScale) }

// Thousandths returns the Real for a count of thousandths.
func Thousandths(t int64) Real { return Real(t) }

// Ratio returns num/den as a Real, rounded half away from zero.
//
// Half away from zero rather than Go's truncation, so a value and its negation
// round symmetrically. Truncation biases toward zero, which over a page of
// positioned glyphs accumulates into a visible leftward drift.
func Ratio(num, den int64) Real {
	if den == 0 {
		return 0
	}
	n := num * RealScale
	if (n < 0) != (den < 0) {
		return Real((n - den/2) / den)
	}
	return Real((n + den/2) / den)
}

// String renders the number in PDF syntax: no exponent, no trailing zeros, and
// a leading zero on values below one.
//
// PDF permits a bare ".5", and several producers emit it, but a leading zero is
// what every reader is certain to accept and costs one byte.
func (r Real) String() string {
	return string(r.AppendPDF(nil))
}

// AppendPDF implements [Object].
func (r Real) AppendPDF(dst []byte) []byte {
	whole, frac := int64(r)/RealScale, int64(r)%RealScale
	if frac == 0 {
		return strconv.AppendInt(dst, whole, 10)
	}
	if frac < 0 {
		frac = -frac
		if whole == 0 {
			dst = append(dst, '-')
		}
	}
	dst = strconv.AppendInt(dst, whole, 10)
	dst = append(dst, '.')

	// Thousandths, most significant first, stopping once the remainder is zero
	// so "1.500" is written "1.5".
	for div := int64(RealScale / 10); div > 0 && frac > 0; div /= 10 {
		dst = append(dst, byte('0'+frac/div))
		frac %= div
	}
	return dst
}

// Int is a PDF integer.
type Int int64

// AppendPDF implements [Object].
func (i Int) AppendPDF(dst []byte) []byte { return strconv.AppendInt(dst, int64(i), 10) }
