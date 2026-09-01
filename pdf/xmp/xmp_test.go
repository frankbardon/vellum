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
