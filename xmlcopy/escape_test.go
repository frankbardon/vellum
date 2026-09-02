package xmlcopy_test

import (
	"bytes"
	"encoding/xml"
	"testing"

	"github.com/frankbardon/vellum/xmlcopy"
)

func TestEscapeText(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"plain", "plain"},
		{"Acme & Co.", "Acme &amp; Co."},
		{"a < b > c", "a &lt; b &gt; c"},
		{`quotes "stay" 'as-is'`, `quotes "stay" 'as-is'`},
		{"&<>&<>", "&amp;&lt;&gt;&amp;&lt;&gt;"},
		{"tabs\tand\nnewlines\rstay literal", "tabs\tand\nnewlines\rstay literal"},
	}
	for _, tt := range cases {
		if got := xmlcopy.EscapeText(tt.in); got != tt.want {
			t.Errorf("EscapeText(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEscapeAttr(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"plain", "plain"},
		{"Acme & Co.", "Acme &amp; Co."},
		{`say "hi"`, "say &quot;hi&quot;"},
		{"it's mine", "it&apos;s mine"},
		{`<a & 'b' "c">`, "&lt;a &amp; &apos;b&apos; &quot;c&quot;&gt;"},
	}
	for _, tt := range cases {
		if got := xmlcopy.EscapeAttr(tt.in); got != tt.want {
			t.Errorf("EscapeAttr(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestEscape_RoundTripsThroughAWalkedElement proves the escapers produce
// something that decodes back to the original string when parsed by the same
// decoder Walk uses — the property that actually matters for splice: a
// replacement built with EscapeText survives a reader parsing it back.
func TestEscape_RoundTripsThroughAWalkedElement(t *testing.T) {
	originals := []string{
		"",
		"plain text",
		"Acme & Co.",
		"<tag> looking text & more",
		"quotes \" and apostrophes '",
		"unicode: café — 世界",
	}
	for _, original := range originals {
		wrapped := []byte("<a>" + xmlcopy.EscapeText(original) + "</a>")

		var decoded string
		var v struct {
			XMLName xml.Name `xml:"a"`
			Text    string   `xml:",chardata"`
		}
		if err := xml.Unmarshal(wrapped, &v); err != nil {
			t.Fatalf("EscapeText(%q) produced XML that does not parse: %v (%s)", original, err, wrapped)
		}
		decoded = v.Text
		if decoded != original {
			t.Errorf("round trip for %q: decoded to %q", original, decoded)
		}

		// It also holds when walked, not just when unmarshalled: the
		// element's own Content span is exactly the escaped bytes.
		if err := xmlcopy.Walk(wrapped, func(el xmlcopy.Element) error {
			if el.Name.Local != "a" {
				return nil
			}
			got := wrapped[el.Content.Start:el.Content.End]
			if !bytes.Equal(got, []byte(xmlcopy.EscapeText(original))) {
				t.Errorf("Content span for %q = %q, want the escaped text unchanged", original, got)
			}
			return nil
		}); err != nil {
			t.Fatalf("Walk: %v", err)
		}
	}
}

// TestEscapeAttr_RoundTripsThroughAnAttribute mirrors the text case for
// attribute values, which additionally need the quote characters escaped.
func TestEscapeAttr_RoundTripsThroughAnAttribute(t *testing.T) {
	originals := []string{
		"",
		`say "hi" to O'Brien`,
		"Acme & <Co>",
	}
	for _, original := range originals {
		wrapped := []byte(`<a v="` + xmlcopy.EscapeAttr(original) + `"/>`)

		var v struct {
			XMLName xml.Name `xml:"a"`
			V       string   `xml:"v,attr"`
		}
		if err := xml.Unmarshal(wrapped, &v); err != nil {
			t.Fatalf("EscapeAttr(%q) produced XML that does not parse: %v (%s)", original, err, wrapped)
		}
		if v.V != original {
			t.Errorf("round trip for %q: decoded to %q", original, v.V)
		}
	}
}
