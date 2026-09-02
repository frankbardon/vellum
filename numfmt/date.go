package numfmt

import (
	"strconv"
	"strings"
	"time"
)

// serialEpoch is the zero of the spreadsheet date serial, 1899-12-30.
//
// Not 1900-01-01, and not a mistake. The 1900 system deliberately reproduces a
// Lotus 1-2-3 bug that treated 1900 as a leap year, so serial 60 is a date that
// never existed and every serial after it is one greater than the true day
// count. Anchoring two days early makes the arithmetic come out right for every
// date after that point, which is every date a document will ever carry.
var serialEpoch = time.Date(1899, time.December, 30, 0, 0, 0, 0, time.UTC)

// serialOf converts a time to its spreadsheet serial number.
func serialOf(t time.Time) float64 {
	t = t.UTC()
	days := t.Sub(serialEpoch).Hours() / 24
	return days
}

// Serial returns the 1900-system spreadsheet date serial for t.
//
// Exported for xlsx, which is the one target that writes a date as this number
// rather than as formatted text: a cell holding a date is a live value with a
// number-format code applied to it, not a string, and it is what keeps a
// workbook's dates sortable and computable rather than merely legible. Every
// other writer calls [Format.Apply] instead, because a document or a deck has
// no live-cell concept for the number to stay behind.
//
// Carries the same 1900-leap-year bug [Format.Apply] does, deliberately: a
// value written here and one rendered as text by the same format code must
// name the same day, and the spreadsheet application on the other end assumes
// the bug is present.
func Serial(t time.Time) float64 { return serialOf(t) }

// monthNames and dayNames are fixed English, deliberately.
//
// Locale-aware names would mean a locale database, and a value that renders
// differently depending on which machine ran the job is the exact property
// byte-identical output exists to deny. A consumer needing another language
// formats the date themselves and supplies the result as text.
var (
	monthNames  = [...]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	monthAbbrev = [...]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	dayNames    = [...]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	dayAbbrev   = [...]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
)

// applyDate renders a time through a date section.
func applyDate(s section, t time.Time) string {
	t = t.UTC()
	twelveHour := sectionHasAMPM(s)

	var b strings.Builder
	for i, tok := range s.tokens {
		switch tok.kind {
		case tokLiteral:
			b.WriteString(tok.text)
		case tokSkipWidth:
			b.WriteByte(' ')
		case tokFill:
		case tokGeneral:
			b.WriteString(trimFloat(serialOf(t)))
		case tokDate:
			b.WriteString(renderDateToken(tok.text, t, twelveHour, minuteContext(s.tokens, i)))
		}
	}
	return b.String()
}

// minuteContext decides whether an m-run means month or minute.
//
// The rule is positional and is the one every spreadsheet uses: m directly
// after an h-run, or directly before an s-run, is minutes; otherwise it is
// months. The ambiguity is inherent to the code vocabulary, so it is resolved
// here once rather than guessed at by each caller.
func minuteContext(tokens []token, i int) bool {
	prev := previousDateToken(tokens, i)
	if strings.HasPrefix(prev, "h") {
		return true
	}
	next := nextDateToken(tokens, i)
	return strings.HasPrefix(next, "s")
}

func previousDateToken(tokens []token, i int) string {
	for j := i - 1; j >= 0; j-- {
		if tokens[j].kind == tokDate {
			return tokens[j].text
		}
		if tokens[j].kind == tokLiteral && strings.TrimSpace(tokens[j].text) != "" &&
			tokens[j].text != ":" {
			return ""
		}
	}
	return ""
}

func nextDateToken(tokens []token, i int) string {
	for j := i + 1; j < len(tokens); j++ {
		if tokens[j].kind == tokDate {
			return tokens[j].text
		}
		if tokens[j].kind == tokLiteral && strings.TrimSpace(tokens[j].text) != "" &&
			tokens[j].text != ":" {
			return ""
		}
	}
	return ""
}

func sectionHasAMPM(s section) bool {
	for _, t := range s.tokens {
		if t.kind == tokDate && t.text == "am/pm" {
			return true
		}
	}
	return false
}

func renderDateToken(tok string, t time.Time, twelveHour, minutes bool) string {
	switch tok {
	case "yy":
		return pad2(t.Year() % 100)
	case "y", "yyy", "yyyy", "yyyyy":
		return strconv.Itoa(t.Year())

	case "m":
		if minutes {
			return strconv.Itoa(t.Minute())
		}
		return strconv.Itoa(int(t.Month()))
	case "mm":
		if minutes {
			return pad2(t.Minute())
		}
		return pad2(int(t.Month()))
	case "mmm":
		return monthAbbrev[int(t.Month())-1]
	case "mmmm":
		return monthNames[int(t.Month())-1]
	case "mmmmm":
		return monthNames[int(t.Month())-1][:1]

	case "d":
		return strconv.Itoa(t.Day())
	case "dd":
		return pad2(t.Day())
	case "ddd":
		return dayAbbrev[int(t.Weekday())]
	case "dddd", "ddddd":
		return dayNames[int(t.Weekday())]

	case "h":
		return strconv.Itoa(hourIn(t, twelveHour))
	case "hh":
		return pad2(hourIn(t, twelveHour))

	case "s":
		return strconv.Itoa(t.Second())
	case "ss":
		return pad2(t.Second())

	case "am/pm":
		if t.Hour() < 12 {
			return "AM"
		}
		return "PM"
	}

	// An unrecognised run of a known letter — "dddddd", say. Rendered as the
	// longest form it resembles rather than dropped, because dropping loses a
	// field the author asked for.
	switch tok[0] {
	case 'y':
		return strconv.Itoa(t.Year())
	case 'm':
		if minutes {
			return pad2(t.Minute())
		}
		return monthNames[int(t.Month())-1]
	case 'd':
		return dayNames[int(t.Weekday())]
	case 'h':
		return pad2(hourIn(t, twelveHour))
	case 's':
		return pad2(t.Second())
	}
	return ""
}

func hourIn(t time.Time, twelveHour bool) int {
	if !twelveHour {
		return t.Hour()
	}
	h := t.Hour() % 12
	if h == 0 {
		h = 12
	}
	return h
}

func pad2(n int) string {
	if n < 10 && n >= 0 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
