package provenance_test

import (
	"bytes"
	"testing"
	"time"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf"
	"github.com/frankbardon/vellum/pdf/object"
	"github.com/frankbardon/vellum/pdf/xmp"
	"github.com/frankbardon/vellum/provenance"
)

// buildPDF writes a minimal one-page PDF/A-2b document, carrying rec's XMP
// schema as an extension when rec is non-nil — mirroring what a caller who
// populated [xmp.Metadata.Extensions] with [provenance.Record.XMPSchema]
// directly would produce, since Compose's own path does not wire this in
// yet.
func buildPDF(t *testing.T, rec *provenance.Record) []byte {
	t.Helper()
	meta := xmp.Metadata{Producer: "Vellum"}
	if rec != nil {
		meta.Extensions = []xmp.Schema{rec.XMPSchema()}
	}
	doc := &pdf.Document{
		Metadata: meta,
		Pages:    []pdf.Page{{Width: object.Points(612), Height: object.Points(792)}},
	}
	var buf bytes.Buffer
	if err := doc.WriteTo(&buf, pdf.WriteOptions{
		SourceDateEpoch: time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

// TestExtractPDF_RoundTripsTheWholeRecord pins that the PDF path is lossless,
// unlike the OOXML one: the packet carries the record's own canonical JSON,
// not a summary of it.
func TestExtractPDF_RoundTripsTheWholeRecord(t *testing.T) {
	generatedAt := time.Date(2024, time.March, 1, 12, 30, 0, 0, time.UTC)
	want := &provenance.Record{
		VellumVersion:   "1.2.3",
		GeneratedAt:     &generatedAt,
		SourceDateEpoch: time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC),
		SpecHash:        "spec-hash-0123456789abcdef",
		ThemeHash:       "theme-hash-fedcba9876543210",
		Assets: []provenance.AssetRef{
			{Handle: "logo.png", Media: "image/png", Hash: "asset-hash-1"},
		},
		Fonts: []provenance.FontRef{
			{Family: "Georgia", SubstitutedWith: "Times New Roman", SubsetProfile: "latin-basic"},
		},
		Sources: []provenance.Source{{Kind: "job", ID: "job-42"}},
	}

	raw := buildPDF(t, want)

	got, err := provenance.ExtractPDF(raw)
	if err != nil {
		t.Fatalf("ExtractPDF: %v", err)
	}
	if got == nil {
		t.Fatal("ExtractPDF reported no provenance for a document that carries one")
	}

	if got.VellumVersion != want.VellumVersion {
		t.Errorf("VellumVersion = %q, want %q", got.VellumVersion, want.VellumVersion)
	}
	if !got.SourceDateEpoch.Equal(want.SourceDateEpoch) {
		t.Errorf("SourceDateEpoch = %v, want %v", got.SourceDateEpoch, want.SourceDateEpoch)
	}
	if got.GeneratedAt == nil || !got.GeneratedAt.Equal(*want.GeneratedAt) {
		t.Errorf("GeneratedAt = %v, want %v", got.GeneratedAt, want.GeneratedAt)
	}
	if got.SpecHash != want.SpecHash || got.ThemeHash != want.ThemeHash {
		t.Errorf("hashes = %q/%q, want %q/%q", got.SpecHash, got.ThemeHash, want.SpecHash, want.ThemeHash)
	}
	if len(got.Assets) != 1 || got.Assets[0] != want.Assets[0] {
		t.Errorf("Assets = %+v, want %+v", got.Assets, want.Assets)
	}
	if len(got.Fonts) != 1 || got.Fonts[0] != want.Fonts[0] {
		t.Errorf("Fonts = %+v, want %+v (SubsetProfile included — the PDF path is lossless)", got.Fonts, want.Fonts)
	}
	if len(got.Sources) != 1 || got.Sources[0] != want.Sources[0] {
		t.Errorf("Sources = %+v, want %+v", got.Sources, want.Sources)
	}
}

// TestExtractPDF_NoProvenanceIsHonestlyAbsent covers the ordinary case: every
// PDF Vellum writes carries an XMP packet, but not every one carries the
// vellum:record property inside it.
func TestExtractPDF_NoProvenanceIsHonestlyAbsent(t *testing.T) {
	raw := buildPDF(t, nil)

	got, err := provenance.ExtractPDF(raw)
	if err != nil {
		t.Fatalf("ExtractPDF: %v", err)
	}
	if got != nil {
		t.Errorf("ExtractPDF = %+v, want nil for a document with no provenance extension", got)
	}
}

// TestExtractPDF_NotAPDFAtAllIsHonestlyAbsent covers arbitrary bytes with no
// XMP packet at all, which must not be an error either.
func TestExtractPDF_NotAPDFAtAllIsHonestlyAbsent(t *testing.T) {
	got, err := provenance.ExtractPDF([]byte("not a pdf at all"))
	if err != nil {
		t.Fatalf("ExtractPDF: %v", err)
	}
	if got != nil {
		t.Errorf("ExtractPDF = %+v, want nil", got)
	}
}

// TestExtractPDF_MalformedRecordIsCoded pins the real failure mode: a packet
// that does carry the property, with a value that is not valid JSON.
func TestExtractPDF_MalformedRecordIsCoded(t *testing.T) {
	rec := &provenance.Record{VellumVersion: "1.0.0"}
	schema := rec.XMPSchema()
	for i := range schema.Properties {
		if schema.Properties[i].Name == provenance.PropertyRecord {
			schema.Properties[i].Value = "{not valid json"
		}
	}
	doc := &pdf.Document{
		Metadata: xmp.Metadata{Producer: "Vellum", Extensions: []xmp.Schema{schema}},
		Pages:    []pdf.Page{{Width: object.Points(612), Height: object.Points(792)}},
	}
	var buf bytes.Buffer
	if err := doc.WriteTo(&buf, pdf.WriteOptions{
		SourceDateEpoch: time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	_, err := provenance.ExtractPDF(buf.Bytes())
	if err == nil {
		t.Fatal("ExtractPDF accepted a malformed record")
	}
	if !verr.HasCode(err, verr.VELLUM_PROVENANCE_MALFORMED) {
		t.Errorf("ExtractPDF error = %v, want VELLUM_PROVENANCE_MALFORMED", err)
	}
}
