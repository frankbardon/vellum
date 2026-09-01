package numfmt_test

import (
	"testing"
	"time"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/numfmt"
)

func num(f float64) numfmt.Value    { return numfmt.Value{Kind: numfmt.KindNumber, Number: f} }
func text(s string) numfmt.Value    { return numfmt.Value{Kind: numfmt.KindText, Text: s} }
func date(t time.Time) numfmt.Value { return numfmt.Value{Kind: numfmt.KindDate, Time: t} }

func apply(t *testing.T, code string, v numfmt.Value) string {
	t.Helper()
	f, err := numfmt.Parse(code)
	if err != nil {
		t.Fatalf("Parse(%q): %v", code, err)
	}
	return f.Apply(v)
}

func TestApply_Numbers(t *testing.T) {
	cases := []struct {
		code string
		in   float64
		want string
	}{
		{"0", 1234.5678, "1235"},
		{"0.00", 1234.5678, "1234.57"},
		{"0.0", 0, "0.0"},
		{"#,##0", 1234567, "1,234,567"},
		{"#,##0.00", 1234.5, "1,234.50"},
		{"#", 0, ""},
		{"0", 0, "0"},
		{"0.0%", 0.4567, "45.7%"},
		{"0%", 1, "100%"},
		{"#,##0,", 1234567, "1,235"},
		{"0.###", 1.5, "1.5"},
		{"0.000", 1.5, "1.500"},
		{"$#,##0.00", 1234.5, "$1,234.50"},
		{`#,##0" units"`, 1200, "1,200 units"},
		// Half away from zero, as a spreadsheet rounds, rather than Go's
		// half-to-even. The difference shows on exactly the values a reviewer
		// checks by hand.
		{"0", 0.5, "1"},
		{"0", 1.5, "2"},
		{"0", 2.5, "3"},
		{"0.00", 1.005, "1.01"},
		{"0", -0.5, "-1"},
	}
	for _, tc := range cases {
		t.Run(tc.code+"/"+tc.want, func(t *testing.T) {
			if got := apply(t, tc.code, num(tc.in)); got != tc.want {
				t.Errorf("Apply(%q, %v) = %q, want %q", tc.code, tc.in, got, tc.want)
			}
		})
	}
}

// TestApply_Sections pins the positional selection rules, which are the part of
// the vocabulary most easily got subtly wrong.
func TestApply_Sections(t *testing.T) {
	cases := []struct {
		name string
		code string
		in   numfmt.Value
		want string
	}{
		{"one section formats everything", "0.0", num(-5), "-5.0"},
		// With two sections the negative arm states its own sign, so the value
		// reaches it absolute — "(5.0)", not "(-5.0)".
		{"two sections split at zero", "0.0;(0.0)", num(-5), "(5.0)"},
		{"two sections, positive", "0.0;(0.0)", num(5), "5.0"},
		{"three sections add a zero arm", `0.0;(0.0);"—"`, num(0), "—"},
		{"three sections, negative", `0.0;(0.0);"—"`, num(-5), "(5.0)"},
		{"an empty third section hides zero", "0.0;(0.0);", num(0), ""},
		{"four sections add a text arm", `0.0;(0.0);"—";"n/a: "@`, text("no base"), "n/a: no base"},
		// Fewer than four sections: text passes through rather than being
		// forced through a numeric arm that would render it as nothing.
		{"text with no text arm passes through", "0.0", text("suppressed"), "suppressed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := apply(t, tc.code, tc.in); got != tc.want {
				t.Errorf("Apply(%q) = %q, want %q", tc.code, got, tc.want)
			}
		})
	}
}

// TestApply_ConditionsOverridePosition pins that an explicit condition replaces
// the positional rule entirely, with the unconditioned arm acting as the else.
func TestApply_ConditionsOverridePosition(t *testing.T) {
	const code = `[<30]"low base";[>=30]0.0`
	if got := apply(t, code, num(12)); got != "low base" {
		t.Errorf("Apply(12) = %q, want %q", got, "low base")
	}
	if got := apply(t, code, num(45)); got != "45.0" {
		t.Errorf("Apply(45) = %q, want %q", got, "45.0")
	}
}

func TestApply_Dates(t *testing.T) {
	when := time.Date(2026, time.September, 1, 14, 5, 9, 0, time.UTC)
	cases := []struct{ code, want string }{
		{"yyyy-mm-dd", "2026-09-01"},
		{"yy-m-d", "26-9-1"},
		{"d mmmm yyyy", "1 September 2026"},
		{"ddd d mmm yy", "Tue 1 Sep 26"},
		{"dddd", "Tuesday"},
		{"mmmmm", "S"},
		{"hh:mm:ss", "14:05:09"},
		{"h:mm AM/PM", "2:05 PM"},
		{"yyyy-mm-dd hh:mm", "2026-09-01 14:05"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			if got := apply(t, tc.code, date(when)); got != tc.want {
				t.Errorf("Apply(%q) = %q, want %q", tc.code, got, tc.want)
			}
		})
	}
}

// TestApply_MOnItsOwnIsMonthsOrMinutesByPosition pins the one genuine ambiguity
// in the vocabulary. The rule is positional and is the one every spreadsheet
// uses: m after an h-run or before an s-run is minutes, otherwise months.
func TestApply_MOnItsOwnIsMonthsOrMinutesByPosition(t *testing.T) {
	// September, 5 past 2.
	when := time.Date(2026, time.September, 1, 14, 5, 9, 0, time.UTC)

	if got := apply(t, "m", date(when)); got != "9" {
		t.Errorf("bare m = %q, want the month %q", got, "9")
	}
	if got := apply(t, "h:m", date(when)); got != "14:5" {
		t.Errorf("m after h = %q, want minutes %q", got, "14:5")
	}
	if got := apply(t, "m:s", date(when)); got != "5:9" {
		t.Errorf("m before s = %q, want minutes %q", got, "5:9")
	}
	if got := apply(t, "m/d", date(when)); got != "9/1" {
		t.Errorf("m before d = %q, want the month %q", got, "9/1")
	}
}

// TestSerial_HonoursTheLotusLeapYearBug pins the epoch. 1899-12-30 is not a
// mistake: the 1900 system reproduces a Lotus 1-2-3 bug that treated 1900 as a
// leap year, so anchoring two days early makes every later date come out right.
func TestSerial_HonoursTheLotusLeapYearBug(t *testing.T) {
	f := numfmt.MustParse("0")
	for _, tc := range []struct {
		when time.Time
		want string
	}{
		{time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC), "2"},
		{time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC), "46266"},
	} {
		if got := f.Apply(date(tc.when)); got != tc.want {
			t.Errorf("serial of %s = %q, want %q", tc.when.Format("2006-01-02"), got, tc.want)
		}
	}
}

func TestApply_TextAndBool(t *testing.T) {
	cases := []struct {
		code string
		in   numfmt.Value
		want string
	}{
		{"@", text("hello"), "hello"},
		{`"["@"]"`, text("note"), "[note]"},
		{"", text("hello"), "hello"},
		{"General", num(42), "42"},
		{"General", text("hi"), "hi"},
		{"@", numfmt.Value{Kind: numfmt.KindBool, Bool: true}, "TRUE"},
		{"@", numfmt.Value{Kind: numfmt.KindBool}, "FALSE"},
		{"0.0", numfmt.Value{Kind: numfmt.KindEmpty}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.code+"/"+tc.want, func(t *testing.T) {
			if got := apply(t, tc.code, tc.in); got != tc.want {
				t.Errorf("Apply(%q) = %q, want %q", tc.code, got, tc.want)
			}
		})
	}
}

func TestParse_Rejects(t *testing.T) {
	cases := []struct{ name, code string }{
		{"unterminated literal", `0.0"units`},
		{"unterminated bracket", `[Red0.0`},
		{"unmatched close bracket", `0.0]`},
		{"five sections", `0;0;0;0;0`},
		{"unknown bracketed directive", `[Sparkline]0.0`},
		{"condition with a non-numeric value", `[<abc]0.0`},
		{"condition with an unknown operator", `[~5]0.0`},
		{"dangling escape", `0.0\`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := numfmt.Parse(tc.code)
			if !verr.HasCode(err, verr.VELLUM_TABLE_FORMAT_INVALID) {
				t.Fatalf("Parse(%q) error = %v, want VELLUM_TABLE_FORMAT_INVALID", tc.code, err)
			}
		})
	}
}

// TestParse_AcceptsRealWorldCodes checks the vocabulary against codes a
// consumer will actually write, rather than against the subset the parser was
// designed around.
func TestParse_AcceptsRealWorldCodes(t *testing.T) {
	codes := []string{
		"General", "0", "0.00", "#,##0", "#,##0.00", "0%", "0.00%",
		"0.00E+00", "# ?/?", "mm-dd-yy", "d-mmm-yy", "d-mmm", "mmm-yy",
		"h:mm AM/PM", "h:mm:ss AM/PM", "h:mm", "h:mm:ss", "m/d/yy h:mm",
		"#,##0 ;(#,##0)", "#,##0 ;[Red](#,##0)", "#,##0.00;[Red]#,##0.00",
		`_("$"* #,##0.00_);_("$"* \(#,##0.00\);_("$"* "-"??_);_(@_)`,
		"[$€-407]#,##0.00", "[h]:mm:ss", `0.0"pp"`, `[<1000]0;[>=1000]#,##0,"k"`,
	}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			if _, err := numfmt.Parse(code); err != nil {
				t.Errorf("Parse(%q) = %v, want it to parse", code, err)
			}
		})
	}
}

// TestApply_IsDeterministic is the property that lets a formatted table
// participate in byte-identical output: no locale, no clock, no ambient state.
func TestApply_IsDeterministic(t *testing.T) {
	f := numfmt.MustParse(`#,##0.00;[Red](#,##0.00);"—";@`)
	values := []numfmt.Value{num(1234.567), num(-1234.567), num(0), text("n/a")}

	for _, v := range values {
		first := f.Apply(v)
		for range 200 {
			if got := f.Apply(v); got != first {
				t.Fatalf("Apply is not stable: %q then %q", first, got)
			}
		}
	}
}

// TestCodeIsRetainedVerbatim pins that the original source survives parsing.
// xlsx embeds the code in its styles part, and emitting a reconstruction rather
// than what the consumer wrote would make the workbook disagree with the
// specification that produced it.
func TestCodeIsRetainedVerbatim(t *testing.T) {
	const code = `_("$"* #,##0.00_)`
	f := numfmt.MustParse(code)
	if f.Code != code {
		t.Errorf("Code = %q, want %q", f.Code, code)
	}
}
