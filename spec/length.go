package spec

import (
	"math"

	verr "github.com/frankbardon/vellum/errors"
)

// Unit is a length unit accepted at the specification boundary.
//
// EMU — English Metric Units, 914400 to the inch — is Vellum's internal unit
// everywhere, but requiring an author to write EMU would be hostile. Units are
// converted once, at this boundary, and never again.
type Unit string

const (
	// UnitPoint is a typographic point, 1/72 inch.
	UnitPoint Unit = "pt"

	// UnitMillimetre is a millimetre.
	UnitMillimetre Unit = "mm"

	// UnitInch is an inch.
	UnitInch Unit = "in"

	// UnitEMU is an English Metric Unit, Vellum's internal unit. Accepted so a
	// caller that has already converted need not convert back.
	UnitEMU Unit = "emu"
)

// EMUPerInch is the number of English Metric Units in one inch.
const EMUPerInch = 914400

// Length is a measurement with an explicit unit.
//
// The unit is explicit rather than implied because a bare number is the
// classic source of a document that is correct on one machine and wrong on
// another, and because a specification is authored by people and by models,
// neither of whom should have to remember a convention.
type Length struct {
	Value float64 `json:"value"`
	Unit  Unit    `json:"unit"`
}

// Points returns a length in typographic points.
func Points(v float64) Length { return Length{Value: v, Unit: UnitPoint} }

// Millimetres returns a length in millimetres.
func Millimetres(v float64) Length { return Length{Value: v, Unit: UnitMillimetre} }

// Inches returns a length in inches.
func Inches(v float64) Length { return Length{Value: v, Unit: UnitInch} }

// IsZero reports whether the length is unset.
func (l Length) IsZero() bool { return l.Value == 0 && l.Unit == "" }

// EMU converts the length to English Metric Units.
//
// The result is an integer because every downstream measurement is an integer:
// accumulating layout in floating point is how two runs of the same document
// come to differ in the last decimal place, and integers make that
// unrepresentable rather than unlikely.
func (l Length) EMU() (int64, error) {
	var perUnit float64
	switch l.Unit {
	case UnitPoint:
		perUnit = EMUPerInch / 72.0
	case UnitMillimetre:
		perUnit = EMUPerInch / 25.4
	case UnitInch:
		perUnit = EMUPerInch
	case UnitEMU:
		perUnit = 1
	default:
		return 0, verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_INVALID,
			"length has an unknown unit",
			map[string]any{"unit": string(l.Unit), "value": l.Value})
	}

	if math.IsNaN(l.Value) || math.IsInf(l.Value, 0) {
		return 0, verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_INVALID,
			"length is not a finite number",
			map[string]any{"unit": string(l.Unit), "value": l.Value})
	}
	// Round half away from zero, so a length is never silently truncated
	// toward the origin and a negative length rounds symmetrically with its
	// positive counterpart.
	return int64(math.Round(l.Value * perUnit)), nil
}

// AllUnits returns the accepted units, in declaration order.
func AllUnits() []Unit {
	return []Unit{UnitPoint, UnitMillimetre, UnitInch, UnitEMU}
}
