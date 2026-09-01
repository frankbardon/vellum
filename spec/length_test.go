package spec_test

import (
	"math"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/spec"
)

func TestLength_EMU(t *testing.T) {
	tests := []struct {
		name   string
		length spec.Length
		want   int64
	}{
		{"one inch", spec.Inches(1), 914400},
		{"one point", spec.Points(1), 12700},
		{"twelve points", spec.Points(12), 152400},
		{"one millimetre", spec.Millimetres(1), 36000},
		{"raw emu passes through", spec.Length{Value: 12345, Unit: spec.UnitEMU}, 12345},
		{"zero", spec.Length{Value: 0, Unit: spec.UnitPoint}, 0},
		{"negative rounds symmetrically", spec.Points(-12), -152400},
		{"fractional point rounds half away from zero", spec.Points(0.5), 6350},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.length.EMU()
			if err != nil {
				t.Fatalf("EMU: %v", err)
			}
			if got != tt.want {
				t.Errorf("EMU = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLength_Rejects(t *testing.T) {
	tests := []struct {
		name   string
		length spec.Length
	}{
		{"unknown unit", spec.Length{Value: 1, Unit: "furlong"}},
		{"empty unit", spec.Length{Value: 1}},
		{"NaN", spec.Length{Value: math.NaN(), Unit: spec.UnitPoint}},
		{"positive infinity", spec.Length{Value: math.Inf(1), Unit: spec.UnitPoint}},
		{"negative infinity", spec.Length{Value: math.Inf(-1), Unit: spec.UnitInch}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.length.EMU(); !verr.HasCode(err, verr.VELLUM_SPEC_INVALID) {
				t.Errorf("error = %v, want VELLUM_SPEC_INVALID", err)
			}
		})
	}
}

// TestLength_UnitsAgree checks the conversions against each other rather than
// only against constants, which is what catches a factor that is right in
// isolation and wrong relative to its neighbours.
func TestLength_UnitsAgree(t *testing.T) {
	inch, err := spec.Inches(1).EMU()
	if err != nil {
		t.Fatal(err)
	}
	points, err := spec.Points(72).EMU()
	if err != nil {
		t.Fatal(err)
	}
	if inch != points {
		t.Errorf("72pt = %d EMU but 1in = %d EMU; they are the same length", points, inch)
	}

	mm, err := spec.Millimetres(25.4).EMU()
	if err != nil {
		t.Fatal(err)
	}
	if mm != inch {
		t.Errorf("25.4mm = %d EMU but 1in = %d EMU; they are the same length", mm, inch)
	}
}

func TestLength_IsZero(t *testing.T) {
	if !(spec.Length{}).IsZero() {
		t.Error("the zero Length does not report itself as zero")
	}
	if spec.Points(0).IsZero() {
		t.Error("an explicit zero-point length reports as unset; a stated zero is not the same as an omission")
	}
}
