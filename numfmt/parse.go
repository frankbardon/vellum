package numfmt

import (
	"strconv"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
)

// tokenKind classifies one run of a section.
type tokenKind uint8

const (
	// tokLiteral is text emitted verbatim.
	tokLiteral tokenKind = iota

	// tokDigit is a run of 0, # and ? placeholders.
	tokDigit

	// tokDecimalPoint separates the integer and fraction parts.
	tokDecimalPoint

	// tokThousands is a comma acting as a grouping separator.
	tokThousands

	// tokPercent multiplies by a hundred and emits a percent sign.
	tokPercent

	// tokDate is a date or time placeholder run.
	tokDate

	// tokText is the @ placeholder.
	tokText

	// tokGeneral is the General placeholder.
	tokGeneral

	// tokFill is an asterisk repeat. The repeated glyph is dropped rather than
	// repeated: it exists to pad a value to a cell's width, and outside a
	// spreadsheet there is no width to pad to. Dropping is the honest
	// degradation; guessing a width would be inventing a layout.
	tokFill

	// tokSkipWidth is an underscore, which reserves the width of the next
	// character. Rendered as a space, which is what it looks like.
	tokSkipWidth
)

// token is one parsed run.
type token struct {
	kind tokenKind

	// text is the literal content for tokLiteral, or the placeholder run for
	// tokDigit and tokDate.
	text string
}

// condition is an explicit [<100]-style guard on a section.
type condition struct {
	op    string
	value float64
}

func generalSection() section {
	return section{kind: kindNumber, tokens: []token{{kind: tokGeneral}}}
}

// dateLetters are the characters that make a section a date section.
const dateLetters = "yYmMdDhHsS"

// parseSection tokenises one arm.
func parseSection(src, code string, index int) (section, error) {
	s := section{kind: kindNumber}
	where := map[string]any{"format": code, "section_index": index}

	// Bracketed prefixes first: colours and conditions attach to the section
	// rather than to a position within it.
	rest := src
	for strings.HasPrefix(rest, "[") {
		close := strings.IndexByte(rest, ']')
		if close < 0 {
			return s, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_FORMAT_INVALID,
				"a number-format section has an unterminated bracket", where)
		}
		inner := rest[1:close]
		rest = rest[close+1:]

		switch {
		case isColorName(inner):
			s.color = strings.ToLower(inner)
		case isCondition(inner):
			c, err := parseCondition(inner, where)
			if err != nil {
				return s, err
			}
			s.condition = c
		case isElapsed(inner):
			// [h], [mm] and [ss] are elapsed-time placeholders. Accepted and
			// treated as their unbracketed form; a duration is a number of
			// units, not a clock time.
			s.kind = kindDate
			s.tokens = append(s.tokens, token{kind: tokDate, text: strings.ToLower(inner)})
		case strings.HasPrefix(inner, "$"):
			// A currency-and-locale prefix like [$$-409]. The symbol before the
			// hyphen is content; the locale id after it is deliberately ignored,
			// because honouring it would make output depend on a locale
			// database.
			symbol := inner[1:]
			if i := strings.IndexByte(symbol, '-'); i >= 0 {
				symbol = symbol[:i]
			}
			if symbol != "" {
				s.tokens = append(s.tokens, token{kind: tokLiteral, text: symbol})
			}
		default:
			return s, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_FORMAT_INVALID,
				"a number-format section carries an unrecognised bracketed directive",
				withValue(where, "directive", inner))
		}
	}

	isDate := containsDateLetter(rest)
	if isDate {
		s.kind = kindDate
	}

	for i := 0; i < len(rest); {
		c := rest[i]
		switch {
		case c == '\\':
			if i+1 >= len(rest) {
				return s, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_FORMAT_INVALID,
					"a number-format section ends with a dangling escape", where)
			}
			s.appendLiteral(string(rest[i+1]))
			i += 2

		case c == '"':
			close := strings.IndexByte(rest[i+1:], '"')
			if close < 0 {
				return s, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_FORMAT_INVALID,
					"a number-format section has an unterminated quoted literal", where)
			}
			s.appendLiteral(rest[i+1 : i+1+close])
			i += close + 2

		case c == '_':
			// Reserves the width of the next character; rendered as a space,
			// which is what it looks like on a page.
			if i+1 < len(rest) {
				i += 2
			} else {
				i++
			}
			s.tokens = append(s.tokens, token{kind: tokSkipWidth})

		case c == '*':
			if i+1 < len(rest) {
				i += 2
			} else {
				i++
			}
			s.tokens = append(s.tokens, token{kind: tokFill})

		case c == '@':
			s.kind = kindText
			s.tokens = append(s.tokens, token{kind: tokText})
			i++

		case c == '%':
			s.tokens = append(s.tokens, token{kind: tokPercent})
			i++

		case c == '.' && !isDate:
			s.tokens = append(s.tokens, token{kind: tokDecimalPoint})
			i++

		case c == ',' && !isDate:
			s.tokens = append(s.tokens, token{kind: tokThousands})
			i++

		case !isDate && (c == '0' || c == '#' || c == '?'):
			j := i
			for j < len(rest) && (rest[j] == '0' || rest[j] == '#' || rest[j] == '?') {
				j++
			}
			s.tokens = append(s.tokens, token{kind: tokDigit, text: rest[i:j]})
			i = j

		case isDate && strings.IndexByte(dateLetters, c) >= 0:
			j := i
			for j < len(rest) && lowerEq(rest[j], c) {
				j++
			}
			s.tokens = append(s.tokens, token{kind: tokDate, text: strings.ToLower(rest[i:j])})
			i = j

		case isDate && (c == 'A' || c == 'a') && hasAMPM(rest[i:]):
			n := ampmLength(rest[i:])
			s.tokens = append(s.tokens, token{kind: tokDate, text: "am/pm"})
			i += n

		case hasGeneralPrefix(rest[i:]):
			s.tokens = append(s.tokens, token{kind: tokGeneral})
			i += len("General")

		default:
			s.appendLiteral(string(c))
			i++
		}
	}

	if len(s.tokens) == 0 {
		// An empty section is legal and means "render nothing" — the
		// conventional way to hide zeroes is a code whose third section is
		// empty. It is not an error.
		s.tokens = nil
	}
	return s, nil
}

// appendLiteral merges adjacent literals, so a run of escaped characters is one
// token rather than one per character.
func (s *section) appendLiteral(text string) {
	if n := len(s.tokens); n > 0 && s.tokens[n-1].kind == tokLiteral {
		s.tokens[n-1].text += text
		return
	}
	s.tokens = append(s.tokens, token{kind: tokLiteral, text: text})
}

func lowerEq(a, b byte) bool { return toLower(a) == toLower(b) }

func toLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

func containsDateLetter(s string) bool {
	inQuote := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\':
			i++
		case s[i] == '"':
			inQuote = !inQuote
		case inQuote:
		case strings.IndexByte(dateLetters, s[i]) >= 0:
			return true
		}
	}
	return false
}

func hasGeneralPrefix(s string) bool {
	const g = "General"
	return len(s) >= len(g) && strings.EqualFold(s[:len(g)], g)
}

func hasAMPM(s string) bool { return ampmLength(s) > 0 }

func ampmLength(s string) int {
	for _, form := range []string{"AM/PM", "am/pm", "A/P", "a/p"} {
		if len(s) >= len(form) && strings.EqualFold(s[:len(form)], form) {
			return len(form)
		}
	}
	return 0
}

var colorNames = []string{
	"black", "blue", "cyan", "green", "magenta", "red", "white", "yellow",
}

func isColorName(s string) bool {
	l := strings.ToLower(s)
	for _, c := range colorNames {
		if l == c {
			return true
		}
	}
	// [Color1] through [Color56] are the indexed palette forms.
	if strings.HasPrefix(l, "color") {
		if n, err := strconv.Atoi(l[len("color"):]); err == nil && n >= 1 && n <= 56 {
			return true
		}
	}
	return false
}

func isCondition(s string) bool {
	return strings.HasPrefix(s, "<") || strings.HasPrefix(s, ">") || strings.HasPrefix(s, "=")
}

func isElapsed(s string) bool {
	l := strings.ToLower(s)
	return l == "h" || l == "hh" || l == "m" || l == "mm" || l == "s" || l == "ss"
}

func parseCondition(s string, where map[string]any) (*condition, error) {
	ops := []string{"<=", ">=", "<>", "<", ">", "="}
	for _, op := range ops {
		if strings.HasPrefix(s, op) {
			v, err := strconv.ParseFloat(strings.TrimSpace(s[len(op):]), 64)
			if err != nil {
				return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_FORMAT_INVALID,
					"a number-format condition has a value that is not a number",
					withValue(where, "condition", s))
			}
			return &condition{op: op, value: v}, nil
		}
	}
	return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_FORMAT_INVALID,
		"a number-format condition has an unrecognised operator",
		withValue(where, "condition", s))
}

func (c *condition) matches(v float64) bool {
	switch c.op {
	case "<":
		return v < c.value
	case "<=":
		return v <= c.value
	case ">":
		return v > c.value
	case ">=":
		return v >= c.value
	case "=":
		return v == c.value
	case "<>":
		return v != c.value
	}
	return false
}

func withValue(where map[string]any, key string, value any) map[string]any {
	out := make(map[string]any, len(where)+1)
	for k, v := range where {
		out[k] = v
	}
	out[key] = value
	return out
}
