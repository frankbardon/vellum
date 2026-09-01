// Package numfmt is the number-format code engine.
//
// One formatting vocabulary for all four targets. The codes are xlsx's, and
// they are used to render a value in a document and a deck and a PDF as well as
// in a workbook — so there is no second dialect to learn, and no opportunity for
// a number to read one way in the spreadsheet and another in the report built
// from the same specification.
//
// # What this is not
//
// It is not locale-aware and will not become so. Locale-correct formatting means
// CLDR, which means a locale database, which means the same value renders
// differently depending on which machine ran the job — the exact property
// byte-identical output exists to deny. A consumer needing locale rendering
// formats the value themselves and supplies the result as the cell's text;
// Vellum takes it verbatim.
//
// It also computes nothing. A format code decides how a number is written, never
// what the number is.
package numfmt

import (
	"strings"

	verr "github.com/frankbardon/vellum/errors"
)

// Format is a parsed number-format code.
//
// A code carries up to four sections, separated by semicolons, selected by the
// value: positive, negative, zero and text. Fewer sections is the common case
// and the selection rules below say what each count means.
type Format struct {
	// Code is the original source, retained verbatim so a writer that must
	// embed the code — xlsx does, in its styles part — emits exactly what the
	// consumer wrote rather than a reconstruction of it.
	Code string

	// sections are the parsed arms, in source order.
	sections []section
}

// section is one semicolon-separated arm.
type section struct {
	// tokens are the literal and placeholder runs, in order.
	tokens []token

	// condition, when present, replaces the positional selection rule for this
	// arm. A code may carry explicit conditions like [<100] on its first two
	// sections, and an explicit condition always wins over position.
	condition *condition

	// color is a declared colour name, retained but not applied here: what a
	// colour means is a theme question, and this package does not have a theme.
	color string

	// kind records what the arm formats, which decides how a value is fed
	// through its tokens.
	kind sectionKind
}

type sectionKind uint8

const (
	// kindNumber is the ordinary case: digits, separators and literals.
	kindNumber sectionKind = iota

	// kindDate carries date or time tokens. A section is one or the other,
	// never both, because a token like "m" means month next to a year and
	// minute next to an hour, and the disambiguation is per-section.
	kindDate

	// kindText carries the @ placeholder.
	kindText
)

// Parse reads a number-format code.
//
// A code that does not parse is [verr.VELLUM_TABLE_FORMAT_INVALID] rather than a
// silently ignored one. Ignoring it would render the value in some default the
// consumer did not choose, which is a wrong answer wearing a right answer's
// clothes.
func Parse(code string) (*Format, error) {
	if code == "" {
		return &Format{Code: "", sections: []section{generalSection()}}, nil
	}
	if strings.EqualFold(code, "General") {
		return &Format{Code: code, sections: []section{generalSection()}}, nil
	}

	raw, err := splitSections(code)
	if err != nil {
		return nil, err
	}
	if len(raw) > 4 {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_FORMAT_INVALID,
			"a number-format code carries at most four sections",
			map[string]any{"format": code, "section_count": len(raw)})
	}

	f := &Format{Code: code, sections: make([]section, 0, len(raw))}
	for i, r := range raw {
		s, err := parseSection(r, code, i)
		if err != nil {
			return nil, err
		}
		f.sections = append(f.sections, s)
	}
	return f, nil
}

// MustParse is Parse for a code known at compile time.
func MustParse(code string) *Format {
	f, err := Parse(code)
	if err != nil {
		panic(err)
	}
	return f
}

// Valid reports whether a code parses.
func Valid(code string) bool {
	_, err := Parse(code)
	return err == nil
}

// splitSections divides a code on unquoted semicolons.
//
// Quoted literals and bracketed sequences are skipped, because a semicolon
// inside either is content rather than a separator — "a;b" is one literal, and
// splitting it would silently turn one section into two.
func splitSections(code string) ([]string, error) {
	var out []string
	var cur strings.Builder

	inQuote := false
	depth := 0

	for i := 0; i < len(code); i++ {
		c := code[i]
		switch {
		case c == '\\' && i+1 < len(code):
			// A backslash escapes the next character, which then cannot act as
			// a separator or a bracket.
			cur.WriteByte(c)
			cur.WriteByte(code[i+1])
			i++
			continue
		case c == '"':
			inQuote = !inQuote
		case inQuote:
			// Inside a literal, nothing is structural.
		case c == '[':
			depth++
		case c == ']':
			if depth == 0 {
				return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_FORMAT_INVALID,
					"a number-format code closes a bracket it never opened",
					map[string]any{"format": code, "offset": i})
			}
			depth--
		case c == ';' && depth == 0:
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}

	if inQuote {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_FORMAT_INVALID,
			"a number-format code has an unterminated quoted literal",
			map[string]any{"format": code})
	}
	if depth != 0 {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_FORMAT_INVALID,
			"a number-format code has an unterminated bracket",
			map[string]any{"format": code})
	}
	out = append(out, cur.String())
	return out, nil
}
