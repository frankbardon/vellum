package errors_test

import (
	"strings"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
)

func TestCodeFormat_AllCodesWellFormed(t *testing.T) {
	codes := verr.AllCodes()
	if len(codes) == 0 {
		t.Fatal("AllCodes returned nothing; the registry in codes.go is empty or allCodes was not updated")
	}
	for _, c := range codes {
		s := string(c)
		switch {
		case s == "":
			t.Error("a code is the empty string")
		case !strings.HasPrefix(s, "VELLUM_"):
			t.Errorf("code %q does not start with VELLUM_; codes are VELLUM_<AREA>_<CATEGORY>", s)
		case s != strings.ToUpper(s):
			t.Errorf("code %q is not upper case", s)
		case strings.ContainsAny(s, " \t\n"):
			t.Errorf("code %q contains whitespace", s)
		}
		if c.Domain() == "" {
			t.Errorf("code %q has no extractable domain; it is missing the <AREA> segment", s)
		}
	}
}

func TestCodeUniqueness_NoDuplicates(t *testing.T) {
	seen := make(map[verr.Code]bool)
	for _, c := range verr.AllCodes() {
		if seen[c] {
			t.Errorf("code %q appears twice in allCodes; remove the duplicate entry in codes.go", c)
		}
		seen[c] = true
	}
}

func TestAllCodes_ReturnsCopy(t *testing.T) {
	first := verr.AllCodes()
	if len(first) == 0 {
		t.Fatal("AllCodes returned nothing")
	}
	original := first[0]
	first[0] = "VELLUM_MUTATED_BY_CALLER"

	second := verr.AllCodes()
	if second[0] != original {
		t.Error("AllCodes returned the backing slice; a caller mutating it would move the manifest and the payload schema")
	}
}

func TestParseCode_RoundTripsAndRejects(t *testing.T) {
	for _, c := range verr.AllCodes() {
		got, ok := verr.ParseCode(string(c))
		if !ok {
			t.Errorf("ParseCode(%q) failed for a registered code", c)
			continue
		}
		if got != c {
			t.Errorf("ParseCode(%q) = %q, want %q", c, got, c)
		}
	}

	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"unknown", "VELLUM_NOT_A_REAL_CODE"},
		{"lower case", "vellum_zip_malformed"},
		{"missing prefix", "ZIP_MALFORMED"},
		{"padded", " VELLUM_ZIP_MALFORMED "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := verr.ParseCode(tt.input); ok {
				t.Errorf("ParseCode(%q) succeeded; it must reject anything that is not an exact registered code", tt.input)
			}
		})
	}
}

func TestCodeDomain_Extraction(t *testing.T) {
	tests := []struct {
		name string
		code verr.Code
		want string
	}{
		{"opc", verr.VELLUM_OPC_INVALID, "OPC"},
		{"zip", verr.VELLUM_ZIP_MALFORMED, "ZIP"},
		{"font", verr.VELLUM_FONT_SUBSTITUTED, "FONT"},
		{"internal", verr.VELLUM_INTERNAL_INVARIANT, "INTERNAL"},
		{"not a vellum code", verr.Code("PULSE_IMPORT_ROW_ERROR"), ""},
		{"prefix only", verr.Code("VELLUM_"), ""},
		{"no category segment", verr.Code("VELLUM_OPC"), ""},
		{"empty area", verr.Code("VELLUM__X"), ""},
		{"empty", verr.Code(""), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.code.Domain(); got != tt.want {
				t.Errorf("Domain() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestByDomain_CoversEveryCode(t *testing.T) {
	total := 0
	for _, d := range verr.AllDomains() {
		codes := verr.ByDomain(d)
		if len(codes) == 0 {
			t.Errorf("AllDomains reported %q but ByDomain(%q) is empty", d, d)
		}
		for _, c := range codes {
			if c.Domain() != d {
				t.Errorf("ByDomain(%q) returned %q, whose domain is %q", d, c, c.Domain())
			}
		}
		total += len(codes)
	}
	if want := len(verr.AllCodes()); total != want {
		t.Errorf("domains partition %d codes, want %d; a code has a domain AllDomains did not report", total, want)
	}
}

// TestNoPulseCodes guards against predecessor vocabulary leaking in when code
// is adapted from the sibling library.
func TestNoPulseCodes(t *testing.T) {
	for _, c := range verr.AllCodes() {
		if strings.Contains(string(c), "PULSE") {
			t.Errorf("code %q carries predecessor-project vocabulary; Vellum codes are VELLUM_*", c)
		}
	}
}
