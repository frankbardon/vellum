package sfnt_test

import "golang.org/x/image/math/fixed"

// fixedInt renders a whole number as the 26.6 fixed-point value the independent
// parser measures against.
func fixedInt(n int) fixed.Int26_6 { return fixed.Int26_6(n << 6) }
