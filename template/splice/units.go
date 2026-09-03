package splice

// EMU-to-WordprocessingML unit conversions, this package's own copy of
// doc/units.go's arithmetic (read-only reference, not an import — see
// xml.go's comment for why). Only the two conversions splice actually needs:
// twips, for table geometry and paragraph spacing, and lineRule, for the
// line-height a paragraph's w:spacing carries.
const (
	emuPerInch   = 914400
	twipsPerInch = 1440
	emuPerTwip   = emuPerInch / twipsPerInch // 635
)

// twips converts EMU to twentieths of a point, rounding half away from zero.
// Integer arithmetic throughout, for the same reason doc/units.go's twips is:
// a measurement round-tripped through float64 accumulates error that shows as
// a one-twip disagreement between two runs of the same input.
func twips(emu int64) int { return int(roundDiv(emu, emuPerTwip)) }

// lineRule converts a line-height multiple to the w:line value carried with
// w:lineRule="auto", which is in 240ths of a line rather than in twips. 240 is
// single spacing.
func lineRule(multiple float64) int {
	if multiple <= 0 {
		return 240
	}
	return int(multiple*240 + 0.5)
}

func roundDiv(n, d int64) int64 {
	if d == 0 {
		return 0
	}
	if (n < 0) != (d < 0) {
		return (n - d/2) / d
	}
	return (n + d/2) / d
}

// sum totals a slice of EMU lengths, for a table's own width from its grid.
func sum(v []int64) int64 {
	var total int64
	for _, n := range v {
		total += n
	}
	return total
}
