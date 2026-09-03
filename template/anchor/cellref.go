package anchor

import (
	"strconv"
	"strings"
)

// ColumnNumber converts spreadsheet column letters ("A".."XFD", uppercase
// only — the form every OOXML writer, including Vellum's own sheet package,
// emits) to an absolute 1-based column number. It returns 0 for an empty
// string or one containing anything outside A-Z.
//
// Exported so template/splice — which shares this package's xlsx cell
// arithmetic rather than re-deriving it — can locate a table column's cell by
// the same rule discovery used to number it.
func ColumnNumber(letters string) int {
	if letters == "" {
		return 0
	}
	n := 0
	for i := 0; i < len(letters); i++ {
		c := letters[i]
		if c < 'A' || c > 'Z' {
			return 0
		}
		n = n*26 + int(c-'A'+1)
	}
	return n
}

// ColumnLetters converts an absolute 1-based column number to its letter
// form (1 -> "A", 26 -> "Z", 27 -> "AA"). n <= 0 returns "".
func ColumnLetters(n int) string {
	if n <= 0 {
		return ""
	}
	var b []byte
	for n > 0 {
		n--
		b = append([]byte{byte('A' + n%26)}, b...)
		n /= 26
	}
	return string(b)
}

// ParseSimpleCellRef splits a single-cell reference into its column letters
// and row number, tolerating (and ignoring) an absolute "$" before either
// part — so it parses "B2", "$B2", "B$2" and "$B$2" alike, which lets the
// same function serve both a defined name's formula (always $-prefixed for
// the shape this package supports) and a bare table ref corner ("A1", never
// $-prefixed) without two near-duplicate parsers. ok is false for anything
// else: empty input, no column letters, no row digits, a row of "0", or
// trailing garbage after the digits.
func ParseSimpleCellRef(s string) (colLetters string, row int, ok bool) {
	s = strings.ReplaceAll(s, "$", "")

	i := 0
	for i < len(s) && s[i] >= 'A' && s[i] <= 'Z' {
		i++
	}
	if i == 0 || i == len(s) {
		return "", 0, false
	}
	colLetters = s[:i]

	digits := s[i:]
	n := 0
	for j := 0; j < len(digits); j++ {
		c := digits[j]
		if c < '0' || c > '9' {
			return "", 0, false
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 {
		return "", 0, false
	}
	return colLetters, n, true
}

// CellRef renders an absolute worksheet column and row as a bare reference
// ("A1"), the inverse of [ParseSimpleCellRef] minus the "$" it tolerates on
// the way in — every reference this package writes back out is bare, never
// $-prefixed, matching what sheet's own writer and every plain (non-defined-
// name) formula in an authored workbook already looks like.
func CellRef(col, row int) string {
	return ColumnLetters(col) + strconv.Itoa(row)
}
