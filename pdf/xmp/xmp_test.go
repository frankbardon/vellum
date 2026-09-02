package xmp_test

import (
	"strings"
	"testing"
	"time"

	"github.com/frankbardon/vellum/pdf/object"
	"github.com/frankbardon/vellum/pdf/xmp"
)

func sample() xmp.Metadata {
	return xmp.Metadata{
		Title:    "Quarterly Findings",
		Author:   "Research",
		Subject:  "Awareness by region",
		Keywords: "awareness, region",
		Creator:  "Arc",
		Producer: "Vellum",
		Date:     time.Date(2026, time.March, 4, 15, 30, 45, 0, time.UTC),
	}
}

// TestDatesAgree is the property the whole package exists for.
//
// PDF states its dates twice, in two syntaxes, and ISO 19005 requires them to
// match. Generating both from one value is what makes disagreement impossible
// to express; this test pins that the two renderings describe the same instant
// rather than merely both being present.
func TestDatesAgree(t *testing.T) {
	m := sample()
	info := m.InfoEntries()

	created, ok := info.Get("CreationDate")
	if !ok {
		t.Fatal("the information dictionary has no CreationDate")
	}
	modified, ok := info.Get("ModDate")
	if !ok {
		t.Fatal("the information dictionary has no ModDate")
	}
	if string(created.AppendPDF(nil)) != string(modified.AppendPDF(nil)) {
		t.Error("CreationDate and ModDate differ; Vellum writes files and never updates them")
	}

	// D:20260304153045+00'00' against 2026-03-04T15:30:45+00:00 — the same
	// instant in the two syntaxes.
	wantInfo := "(D:20260304153045+00'00')"
	if got := string(created.AppendPDF(nil)); got != wantInfo {
		t.Errorf("CreationDate is %s, want %s", got, wantInfo)
	}

	packet := string(m.Packet())
	for _, want := range []string{
		"<xmp:CreateDate>2026-03-04T15:30:45+00:00</xmp:CreateDate>",
		"<xmp:ModifyDate>2026-03-04T15:30:45+00:00</xmp:ModifyDate>",
		"<xmp:MetadataDate>2026-03-04T15:30:45+00:00</xmp:MetadataDate>",
	} {
		if !strings.Contains(packet, want) {
			t.Errorf("the packet does not carry %s", want)
		}
	}
}

// TestDatesAreUTCRegardlessOfTheZoneGiven pins that where the file was written
// does not reach the bytes.
func TestDatesAreUTCRegardlessOfTheZoneGiven(t *testing.T) {
	utc := xmp.Metadata{Date: time.Date(2026, time.March, 4, 15, 30, 45, 0, time.UTC)}
	east := xmp.Metadata{Date: utc.Date.In(time.FixedZone("UTC+9", 9*3600))}

	if string(utc.Packet()) != string(east.Packet()) {
		t.Error("the same instant in two zones produced different packets")
	}
	a, _ := utc.InfoEntries().Get("CreationDate")
	b, _ := east.InfoEntries().Get("CreationDate")
	if string(a.AppendPDF(nil)) != string(b.AppendPDF(nil)) {
		t.Error("the same instant in two zones produced different information dictionaries")
	}
}

func TestPacket_ClaimsPDFA2B(t *testing.T) {
	packet := string(sample().Packet())

	for _, want := range []string{
		"<?xpacket begin=\"\uFEFF\" id=\"" + xmp.PacketID + "\"?>",
		"<pdfaid:part>2</pdfaid:part>",
		"<pdfaid:conformance>B</pdfaid:conformance>",
		"<dc:format>application/pdf</dc:format>",
		`<?xpacket end="w"?>`,
	} {
		if !strings.Contains(packet, want) {
			t.Errorf("the packet does not carry %q", want)
		}
	}
}

// TestInfoEntries_OmitsWhatWasNotSet pins the difference between making no
// claim and claiming emptiness.
func TestInfoEntries_OmitsWhatWasNotSet(t *testing.T) {
	info := xmp.Metadata{Producer: "Vellum"}.InfoEntries()

	for _, key := range []object.Name{"Title", "Author", "Subject", "Keywords", "Creator"} {
		if _, ok := info.Get(key); ok {
			t.Errorf("%s is present although it was never set", key)
		}
	}
	if _, ok := info.Get("Producer"); !ok {
		t.Error("Producer was set and is absent")
	}
}

func TestPacket_OmitsWhatWasNotSet(t *testing.T) {
	packet := string(xmp.Metadata{Producer: "Vellum"}.Packet())

	for _, unwanted := range []string{"<dc:title>", "<dc:creator>", "<dc:description>", "<xmp:CreatorTool>"} {
		if strings.Contains(packet, unwanted) {
			t.Errorf("the packet carries %s although it was never set", unwanted)
		}
	}
}

// TestPacket_EscapesMarkup pins that a title cannot break the packet.
func TestPacket_EscapesMarkup(t *testing.T) {
	m := xmp.Metadata{Title: `A & B <script> "quoted" 'single'`}
	packet := string(m.Packet())

	if strings.Contains(packet, "<script>") {
		t.Error("markup in the title reached the packet unescaped")
	}
	for _, want := range []string{"&amp;", "&lt;script&gt;", "&quot;", "&apos;"} {
		if !strings.Contains(packet, want) {
			t.Errorf("the packet does not carry the escape %q", want)
		}
	}
}

func TestPacket_IsDeterministic(t *testing.T) {
	m := sample()
	first := string(m.Packet())
	for range 25 {
		if string(m.Packet()) != first {
			t.Fatal("two identical packets differ")
		}
	}
}

// TestValueType_AcceptsRejectsGosBooleanSyntax pins the specific mistake that
// reached a validator.
//
// XMP booleans are "True" and "False". strconv.FormatBool gives "true" and
// "false", which every reader displays perfectly and ISO 19005-2 clause
// 6.6.2.3.1 refuses. The constructor is the fix; this is the check for a schema
// somebody else built.
func TestValueType_AcceptsRejectsGosBooleanSyntax(t *testing.T) {
	cases := []struct {
		kind  xmp.ValueType
		value string
		want  bool
	}{
		{xmp.TypeBoolean, "True", true},
		{xmp.TypeBoolean, "False", true},
		{xmp.TypeBoolean, "true", false},
		{xmp.TypeBoolean, "false", false},
		{xmp.TypeBoolean, "1", false},
		{xmp.TypeInteger, "-42", true},
		{xmp.TypeInteger, "4.2", false},
		{xmp.TypeDate, "1980-01-01T00:00:00+00:00", true},
		{xmp.TypeDate, "1980-01-01", false},
		// Text accepts anything, which is what makes it the type to reach for
		// when a value has no syntax of its own.
		{xmp.TypeText, "anything at all", true},
		{xmp.ValueType("Rational"), "1/2", false},
	}

	for _, c := range cases {
		if got := c.kind.Accepts(c.value); got != c.want {
			t.Errorf("%s.Accepts(%q) = %v, want %v", c.kind, c.value, got, c.want)
		}
	}
}

// TestBool_IsXMPsSyntaxAndNotGos is the constructor half of the same pin.
func TestBool_IsXMPsSyntaxAndNotGos(t *testing.T) {
	if got := xmp.Bool(true); got != "True" {
		t.Errorf("Bool(true) = %q, want %q", got, "True")
	}
	if got := xmp.Bool(false); got != "False" {
		t.Errorf("Bool(false) = %q, want %q", got, "False")
	}
}

// TestDate_IsTheSameFormatterThePacketUses keeps a packet from carrying two
// date syntaxes.
func TestDate_IsTheSameFormatterThePacketUses(t *testing.T) {
	stamp := time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	rendered := xmp.Date(stamp)

	packet := string(xmp.Metadata{Date: stamp}.Packet())
	if !strings.Contains(packet, "<xmp:CreateDate>"+rendered+"</xmp:CreateDate>") {
		t.Errorf("the packet's own date is not %q:\n%s", rendered, packet)
	}
	if got := xmp.Date(time.Time{}); got != "" {
		t.Errorf("Date(zero) = %q, want the empty string", got)
	}
}
