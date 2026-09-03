package xmlcopy_test

import (
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/xmlcopy"
)

func TestDecodeText(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"plain", "plain text", "plain text"},
		{"amp", "Acme &amp; Co.", "Acme & Co."},
		{"lt gt", "a &lt;tag&gt; looking string", "a <tag> looking string"},
		{"quotes", "&quot;quoted&quot; and &apos;apos&apos;", `"quoted" and 'apos'`},
		{"numeric decimal", "&#38;", "&"},
		{"numeric hex", "&#x26;", "&"},
		{"mixed", "Widgets &amp; &lt;Gadgets&gt; Ltd.", "Widgets & <Gadgets> Ltd."},
		{"unicode passthrough", "café — 世界", "café — 世界"},
		{"leading/trailing whitespace preserved", "  spaced text  ", "  spaced text  "},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := xmlcopy.DecodeText([]byte(tt.in))
			if err != nil {
				t.Fatalf("DecodeText(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("DecodeText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestDecodeText_RoundTripsWithEscapeText proves DecodeText is EscapeText's
// inverse for arbitrary text, which is the property splice's resplice depends
// on: decode the source, possibly slice it, then re-escape and re-splice it
// without drift.
func TestDecodeText_RoundTripsWithEscapeText(t *testing.T) {
	originals := []string{
		"",
		"plain",
		"Acme & Co.",
		"<tag> & \"quoted\" & 'apos'",
		"  leading and trailing space  ",
		"unicode: café — 世界",
	}
	for _, original := range originals {
		escaped := xmlcopy.EscapeText(original)
		got, err := xmlcopy.DecodeText([]byte(escaped))
		if err != nil {
			t.Fatalf("DecodeText(EscapeText(%q)): %v", original, err)
		}
		if got != original {
			t.Errorf("round trip for %q: got %q", original, got)
		}
	}
}

func TestDecodeText_MalformedIsCodedError(t *testing.T) {
	_, err := xmlcopy.DecodeText([]byte("unterminated & entity &amp"))
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_XML_MALFORMED) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_XML_MALFORMED", err)
	}
}
