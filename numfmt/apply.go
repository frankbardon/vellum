package numfmt

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// Value is what a format code is applied to.
//
// A closed struct rather than an any, for the same reason [spec.Value] is one:
// an interface would arrive from JSON as whatever the decoder felt like, and a
// number that had been through float64 is not the number the consumer wrote.
type Value struct {
	// Kind names which arm carries content.
	Kind ValueKind

	// Number is set when Kind is KindNumber.
	Number float64

	// Text is set when Kind is KindText.
	Text string

	// Bool is set when Kind is KindBool.
	Bool bool

	// Time is set when Kind is KindDate.
	Time time.Time
}

// ValueKind names a value's arm.
type ValueKind uint8

const (
	// KindEmpty is no value.
	KindEmpty ValueKind = iota
	// KindNumber is numeric.
	KindNumber
	// KindText is textual.
	KindText
	// KindBool is boolean.
	KindBool
	// KindDate is a date or time.
	KindDate
)

// Apply renders a value through the format.
//
// Deterministic by construction: no locale, no clock, no ambient state. The
// same value and the same code produce the same string on every machine, which
// is what lets a rendered table participate in byte-identical output.
func (f *Format) Apply(v Value) string {
	if f == nil || len(f.sections) == 0 {
		return general(v)
	}

	s := f.selectSection(v)
	if s == nil {
		// A code can deliberately select nothing — a third section that is
		// empty is the conventional way to hide zeroes.
		return ""
	}
	switch {
	case v.Kind == KindEmpty:
		return ""
	case s.kind == kindDate && v.Kind == KindDate:
		return applyDate(*s, v.Time)
	case s.kind == kindText || v.Kind == KindText || v.Kind == KindBool:
		return applyText(*s, textOf(v))
	case v.Kind == KindNumber:
		return applyNumber(*s, v.Number, f.negatesImplicitly())
	case v.Kind == KindDate:
		// A date fed through a numeric section renders as its serial number,
		// which is what a spreadsheet does and is at least honest about the
		// mismatch.
		return applyNumber(*s, serialOf(v.Time), f.negatesImplicitly())
	}
	return general(v)
}

// selectSection picks the arm for a value.
//
// Explicit conditions win over position, and the positional rules follow the
// section count: one section formats everything; two split at zero, with the
// second arm rendering the absolute value of a negative; three add a zero arm;
// four add a text arm.
func (f *Format) selectSection(v Value) *section {
	if v.Kind == KindText || v.Kind == KindBool {
		if len(f.sections) >= 4 {
			return &f.sections[3]
		}
		// A code that is itself a text section applies to text even without a
		// fourth arm — `"["@"]"` is one section and must still bracket its
		// value.
		if len(f.sections) > 0 && f.sections[0].kind == kindText {
			return &f.sections[0]
		}
		// Otherwise text passes through unformatted rather than being forced
		// through a numeric arm, which would render it as nothing.
		return &section{kind: kindText, tokens: []token{{kind: tokText}}}
	}

	n := numberOf(v)

	// An explicit condition on either of the first two sections replaces the
	// positional rule entirely, and the last section becomes the else arm.
	if f.hasConditions() {
		for i := range f.sections {
			c := f.sections[i].condition
			if c == nil {
				continue
			}
			if c.matches(n) {
				return &f.sections[i]
			}
		}
		for i := range f.sections {
			if f.sections[i].condition == nil {
				return &f.sections[i]
			}
		}
		return nil
	}

	switch len(f.sections) {
	case 1:
		return &f.sections[0]
	case 2:
		if n < 0 {
			return &f.sections[1]
		}
		return &f.sections[0]
	default:
		switch {
		case n > 0:
			return &f.sections[0]
		case n < 0:
			return &f.sections[1]
		default:
			return &f.sections[2]
		}
	}
}

func (f *Format) hasConditions() bool {
	for i := range f.sections {
		if f.sections[i].condition != nil {
			return true
		}
	}
	return false
}

// negatesImplicitly reports whether the format must supply its own minus sign.
//
// A single-section code has no negative arm, so the sign is written for it. A
// code with two or more sections states the negative form explicitly — usually
// as parentheses — so the value reaches that arm absolute, and the result is
// "(1.00)" rather than "(-1.00)".
func (f *Format) negatesImplicitly() bool { return len(f.sections) <= 1 }

func applyNumber(s section, n float64, signed bool) string {
	// Percent scaling happens before digits are counted, because a code of
	// "0.0%" means one decimal of the scaled value.
	scale := 1.0
	for _, t := range s.tokens {
		if t.kind == tokPercent {
			scale *= 100
		}
	}
	n *= scale

	// Trailing commas immediately before the decimal point or the end of the
	// integer run scale down by a thousand each — the "#,##0," idiom for
	// thousands.
	n /= math.Pow(1000, float64(trailingScaleCommas(s)))

	intMask, fracMask, grouped := digitMasks(s)
	if intMask == "" && fracMask == "" {
		// No digit placeholders at all: the section is literals, a General
		// placeholder, or both.
		return renderLiterals(s, n)
	}

	negative := signed && math.Signbit(n) && n != 0
	if !signed {
		n = math.Abs(n)
	}
	abs := math.Abs(n)

	fracDigits := countDigits(fracMask)
	rounded := roundHalfAwayFromZero(abs, fracDigits)

	intPart, fracPart := splitParts(rounded, fracDigits)
	intStr := padInteger(intPart, intMask)
	if grouped {
		intStr = group(intStr)
	}
	fracStr := padFraction(fracPart, fracMask)

	var b strings.Builder
	if negative {
		b.WriteByte('-')
	}

	// The whole integer run is written at the first integer-side digit token
	// and the whole fraction run at the first fraction-side one. Deciding by
	// position rather than by the token's own text matters: "#,##0.00" carries
	// three digit tokens, two of them on the integer side, and matching them by
	// content put the fraction into the middle of the integer.
	afterPoint := false
	wroteInt, wroteFrac := false, false

	for _, t := range s.tokens {
		switch t.kind {
		case tokLiteral:
			b.WriteString(t.text)
		case tokSkipWidth:
			b.WriteByte(' ')
		case tokFill:
			// Dropped: it pads to a cell width, and outside a spreadsheet
			// there is no width to pad to.
		case tokPercent:
			b.WriteByte('%')
		case tokGeneral:
			b.WriteString(trimFloat(n))
		case tokThousands:
			// Consumed by grouping and by the trailing-comma scale.
		case tokDecimalPoint:
			afterPoint = true
			if fracStr != "" {
				b.WriteByte('.')
			}
		case tokDigit:
			if afterPoint {
				if !wroteFrac {
					b.WriteString(fracStr)
					wroteFrac = true
				}
				continue
			}
			if !wroteInt {
				b.WriteString(intStr)
				wroteInt = true
			}
		}
	}
	return b.String()
}

func applyText(s section, text string) string {
	var b strings.Builder
	for _, t := range s.tokens {
		switch t.kind {
		case tokText, tokGeneral:
			b.WriteString(text)
		case tokLiteral:
			b.WriteString(t.text)
		case tokSkipWidth:
			b.WriteByte(' ')
		}
	}
	if b.Len() == 0 {
		return text
	}
	return b.String()
}

func renderLiterals(s section, n float64) string {
	var b strings.Builder
	for _, t := range s.tokens {
		switch t.kind {
		case tokLiteral:
			b.WriteString(t.text)
		case tokGeneral:
			b.WriteString(trimFloat(n))
		case tokPercent:
			b.WriteByte('%')
		case tokSkipWidth:
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// digitMasks splits the digit placeholders into the integer and fraction runs,
// and reports whether the section groups thousands.
//
// Grouping is a comma *between* digit placeholders. A comma after them is a
// scale factor instead, which is why the two are separated here rather than at
// the point of use.
func digitMasks(s section) (intMask, fracMask string, grouped bool) {
	afterPoint := false
	for i, t := range s.tokens {
		switch t.kind {
		case tokDecimalPoint:
			afterPoint = true
		case tokDigit:
			if afterPoint {
				fracMask += t.text
			} else {
				intMask += t.text
			}
		case tokThousands:
			if !afterPoint && hasDigitAfter(s.tokens[i+1:]) {
				grouped = true
			}
		}
	}
	return intMask, fracMask, grouped
}

func hasDigitAfter(tokens []token) bool {
	for _, t := range tokens {
		if t.kind == tokDigit {
			return true
		}
	}
	return false
}

// trailingScaleCommas counts commas that follow the last digit placeholder of
// the integer run — each divides by a thousand.
func trailingScaleCommas(s section) int {
	count := 0
	seenDigit := false
	for _, t := range s.tokens {
		switch t.kind {
		case tokDigit:
			seenDigit = true
			count = 0
		case tokThousands:
			if seenDigit {
				count++
			}
		case tokDecimalPoint:
			return count
		}
	}
	return count
}

func countDigits(mask string) int { return len(mask) }

// roundHalfAwayFromZero rounds to a fixed number of decimals.
//
// Half away from zero rather than Go's half-to-even, because that is what a
// spreadsheet does and what a reader comparing the two would expect. The
// difference shows on exactly the values a reviewer checks by hand.
func roundHalfAwayFromZero(v float64, decimals int) float64 {
	pow := math.Pow(10, float64(decimals))
	scaled := v * pow
	// Nudge by one unit in the last place before rounding, so a value that is
	// a hair under .5 from binary representation still rounds up. Without it,
	// 1.005 at two decimals renders as 1.00 because it is stored as
	// 1.00499999999999989.
	scaled = nextAfterUp(scaled)
	return math.Floor(scaled+0.5) / pow
}

func nextAfterUp(v float64) float64 {
	if v == 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		return v
	}
	return math.Nextafter(v, math.Inf(1))
}

func splitParts(v float64, decimals int) (string, string) {
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// padInteger applies the integer mask: 0 forces a digit, ? forces a space, #
// forces nothing.
func padInteger(digits, mask string) string {
	if digits == "0" && !strings.ContainsAny(mask, "0?") {
		// "#" alone renders zero as nothing, which is the documented behaviour
		// and the reason "#,##0" exists as the common idiom.
		return ""
	}
	if len(digits) >= len(mask) {
		return digits
	}
	pad := len(mask) - len(digits)
	var b strings.Builder
	for i := 0; i < pad; i++ {
		switch mask[i] {
		case '0':
			b.WriteByte('0')
		case '?':
			b.WriteByte(' ')
		}
	}
	b.WriteString(digits)
	return b.String()
}

// padFraction applies the fraction mask, trimming digits that only # asked for.
func padFraction(digits, mask string) string {
	if mask == "" {
		return ""
	}
	out := []byte(digits)
	for len(out) < len(mask) {
		out = append(out, '0')
	}
	// Trim from the right while the mask says the position is optional.
	for i := len(out) - 1; i >= 0 && i < len(mask); i-- {
		if mask[i] == '#' && out[i] == '0' {
			out = out[:i]
			continue
		}
		if mask[i] == '?' && out[i] == '0' {
			out[i] = ' '
			continue
		}
		break
	}
	return string(out)
}

// group inserts thousands separators into an already-padded integer string.
func group(s string) string {
	lead := 0
	for lead < len(s) && (s[lead] == ' ') {
		lead++
	}
	digits := s[lead:]
	if len(digits) <= 3 {
		return s
	}
	var b strings.Builder
	b.WriteString(s[:lead])
	first := len(digits) % 3
	if first == 0 {
		first = 3
	}
	b.WriteString(digits[:first])
	for i := first; i < len(digits); i += 3 {
		b.WriteByte(',')
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

func numberOf(v Value) float64 {
	switch v.Kind {
	case KindNumber:
		return v.Number
	case KindDate:
		return serialOf(v.Time)
	case KindBool:
		if v.Bool {
			return 1
		}
	}
	return 0
}

func textOf(v Value) string {
	switch v.Kind {
	case KindText:
		return v.Text
	case KindBool:
		if v.Bool {
			return "TRUE"
		}
		return "FALSE"
	case KindNumber:
		return trimFloat(v.Number)
	case KindDate:
		return v.Time.UTC().Format("2006-01-02")
	}
	return ""
}

// general renders a value with no format code.
func general(v Value) string {
	switch v.Kind {
	case KindEmpty:
		return ""
	case KindNumber:
		return trimFloat(v.Number)
	case KindDate:
		return v.Time.UTC().Format("2006-01-02")
	default:
		return textOf(v)
	}
}

// trimFloat renders a float without a trailing .0, and without exponent
// notation for magnitudes a document would ever show.
func trimFloat(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
