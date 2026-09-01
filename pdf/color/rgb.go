package color

import (
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/object"
)

// RGB is a colour in the sRGB space the output intent declares.
//
// Eight-bit channels, because that is what a theme carries and what a
// specification can express. The conversion to PDF's zero-to-one components
// happens once, on the way into a content stream, through [object.Ratio] — so
// nothing here passes through a float and the same colour writes the same bytes
// every time.
//
// The zero value is black, which is the right default for text: a run whose
// colour was never resolved draws in the colour a reader expects rather than
// invisibly in white.
type RGB struct {
	R, G, B uint8
}

// Black is the default text colour.
var Black = RGB{}

// Components returns the colour as PDF operands.
func (c RGB) Components() (r, g, b object.Real) {
	return object.Ratio(int64(c.R), 255),
		object.Ratio(int64(c.G), 255),
		object.Ratio(int64(c.B), 255)
}

// Hex renders the colour in the form a theme writes it: six uppercase
// hexadecimal digits, no leading hash.
func (c RGB) Hex() string {
	const digits = "0123456789ABCDEF"
	out := make([]byte, 6)
	for i, v := range [3]uint8{c.R, c.G, c.B} {
		out[i*2] = digits[v>>4]
		out[i*2+1] = digits[v&0x0F]
	}
	return string(out)
}

// ParseHex reads the form a resolved fragment carries.
//
// Six hexadecimal digits, with or without a leading hash, in either case. A
// three-digit shorthand is accepted because it is what a person writes in a
// theme by hand and expanding it is unambiguous.
//
// Anything else is a coded error rather than a default. A colour that silently
// became black is a document that renders and is wrong, and the theme that
// produced it keeps producing it.
func ParseHex(s string) (RGB, error) {
	raw := s
	if len(raw) > 0 && raw[0] == '#' {
		raw = raw[1:]
	}

	var nibbles [6]uint8
	switch len(raw) {
	case 6:
		for i := range 6 {
			n, ok := hexNibble(raw[i])
			if !ok {
				return RGB{}, badColor(s)
			}
			nibbles[i] = n
		}
	case 3:
		for i := range 3 {
			n, ok := hexNibble(raw[i])
			if !ok {
				return RGB{}, badColor(s)
			}
			nibbles[i*2], nibbles[i*2+1] = n, n
		}
	default:
		return RGB{}, badColor(s)
	}

	return RGB{
		R: nibbles[0]<<4 | nibbles[1],
		G: nibbles[2]<<4 | nibbles[3],
		B: nibbles[4]<<4 | nibbles[5],
	}, nil
}

func hexNibble(c byte) (uint8, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

func badColor(s string) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
		"a colour is not a hexadecimal sRGB triplet",
		map[string]any{"value": s, "expected": "RRGGBB or RGB, with an optional leading hash"})
}
