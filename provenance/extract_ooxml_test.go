package provenance_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/frankbardon/vellum/doc"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/provenance"
)

// buildDOCX writes a minimal one-section DOCX carrying rec as its
// provenance, and returns the package byte-for-byte as [doc.Document.WriteTo]
// produced it.
func buildDOCX(t *testing.T, rec *provenance.Record) []byte {
	t.Helper()
	d := &doc.Document{
		Sections:   []doc.Section{{Page: doc.A4Portrait()}},
		Provenance: rec,
	}
	var buf bytes.Buffer
	if err := d.WriteTo(&buf, doc.WriteOptions{SourceDateEpoch: zipdet.PinnedEpoch}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

func openPackage(t *testing.T, raw []byte) *opc.Package {
	t.Helper()
	pkg, err := opc.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("opc.Open: %v", err)
	}
	return pkg
}

// TestExtract_RoundTripsWhatTheFlatteningPreserves is the round-trip the
// story asks for: a DOCX built with a manually-set Provenance field, proving
// Extract is the actual inverse of provenanceProperties rather than merely
// plausible. Every field provenanceProperties writes losslessly is checked
// for exact equality; Assets and Fonts, which it summarises, are checked
// against what the summary can actually carry back.
func TestExtract_RoundTripsWhatTheFlatteningPreserves(t *testing.T) {
	generatedAt := time.Date(2024, time.March, 1, 12, 30, 0, 0, time.UTC)
	want := &provenance.Record{
		VellumVersion:   "1.2.3",
		GeneratedAt:     &generatedAt,
		SourceDateEpoch: time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC),
		SpecHash:        "spec-hash-0123456789abcdef",
		ThemeHash:       "theme-hash-fedcba9876543210",
		BindingHash:     "binding-hash-aaaaaaaaaaaaaaaa",
		TemplateHash:    "template-hash-bbbbbbbbbbbbbbbb",
		Assets: []provenance.AssetRef{
			{Handle: "logo.png", Media: "image/png", Hash: "asset-hash-1"},
			{Handle: "chart.png", Media: "image/png", Hash: "asset-hash-2"},
		},
		Fonts: []provenance.FontRef{
			{Family: "Georgia", Embedded: true},
			{Family: "Calibri", SubstitutedWith: "Carlito", SubsetProfile: "latin-basic"},
		},
		Sources: []provenance.Source{
			{Kind: "job", ID: "job-42"},
			{Kind: "session", ID: "sess-7"},
		},
	}

	raw := buildDOCX(t, want)
	pkg := openPackage(t, raw)

	got, err := provenance.Extract(pkg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got == nil {
		t.Fatal("Extract reported no provenance for a package that carries one")
	}

	// Fields provenanceProperties carries as their own scalar property
	// round-trip exactly.
	if got.VellumVersion != want.VellumVersion {
		t.Errorf("VellumVersion = %q, want %q", got.VellumVersion, want.VellumVersion)
	}
	if !got.SourceDateEpoch.Equal(want.SourceDateEpoch) {
		t.Errorf("SourceDateEpoch = %v, want %v", got.SourceDateEpoch, want.SourceDateEpoch)
	}
	if got.GeneratedAt == nil || !got.GeneratedAt.Equal(*want.GeneratedAt) {
		t.Errorf("GeneratedAt = %v, want %v", got.GeneratedAt, want.GeneratedAt)
	}
	if got.SpecHash != want.SpecHash {
		t.Errorf("SpecHash = %q, want %q", got.SpecHash, want.SpecHash)
	}
	if got.ThemeHash != want.ThemeHash {
		t.Errorf("ThemeHash = %q, want %q", got.ThemeHash, want.ThemeHash)
	}
	if got.BindingHash != want.BindingHash {
		t.Errorf("BindingHash = %q, want %q", got.BindingHash, want.BindingHash)
	}
	if got.TemplateHash != want.TemplateHash {
		t.Errorf("TemplateHash = %q, want %q", got.TemplateHash, want.TemplateHash)
	}

	// Sources round-trip exactly: each one already had its own scalar
	// property, so nothing about it was summarised.
	if len(got.Sources) != len(want.Sources) {
		t.Fatalf("Sources = %v, want %v", got.Sources, want.Sources)
	}
	wantSources := map[string]string{}
	for _, s := range want.Sources {
		wantSources[s.Kind] = s.ID
	}
	for _, s := range got.Sources {
		if wantSources[s.Kind] != s.ID {
			t.Errorf("source %q = %q, want %q", s.Kind, s.ID, wantSources[s.Kind])
		}
	}

	// Assets round-trip only their hash: Handle and Media are lost in the
	// flattening, which joins hashes alone into VellumAssetHashes.
	if len(got.Assets) != len(want.Assets) {
		t.Fatalf("Assets = %v, want %d entries", got.Assets, len(want.Assets))
	}
	for i, a := range got.Assets {
		if a.Handle != "" || a.Media != "" {
			t.Errorf("Assets[%d] carries Handle=%q Media=%q; the flattening never wrote either", i, a.Handle, a.Media)
		}
		if a.Hash != want.Assets[i].Hash {
			t.Errorf("Assets[%d].Hash = %q, want %q", i, a.Hash, want.Assets[i].Hash)
		}
	}

	// Fonts round-trip Family, SubstitutedWith and Embedded; SubsetProfile is
	// lost the same way.
	if len(got.Fonts) != len(want.Fonts) {
		t.Fatalf("Fonts = %v, want %d entries", got.Fonts, len(want.Fonts))
	}
	for i, f := range got.Fonts {
		w := want.Fonts[i]
		if f.Family != w.Family {
			t.Errorf("Fonts[%d].Family = %q, want %q", i, f.Family, w.Family)
		}
		if f.SubstitutedWith != w.SubstitutedWith {
			t.Errorf("Fonts[%d].SubstitutedWith = %q, want %q", i, f.SubstitutedWith, w.SubstitutedWith)
		}
		if f.Embedded != w.Embedded {
			t.Errorf("Fonts[%d].Embedded = %v, want %v", i, f.Embedded, w.Embedded)
		}
		if f.SubsetProfile != "" {
			t.Errorf("Fonts[%d] carries SubsetProfile=%q; the flattening never wrote it", i, f.SubsetProfile)
		}
	}
}

// TestExtract_NoProvenanceIsHonestlyAbsent pins the "most artifacts have
// none" case: a plain compose-mode package carries no docProps/custom.xml at
// all, and Extract reports that as a nil record and no error, not a failure.
func TestExtract_NoProvenanceIsHonestlyAbsent(t *testing.T) {
	raw := buildDOCX(t, nil)
	pkg := openPackage(t, raw)

	got, err := provenance.Extract(pkg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got != nil {
		t.Errorf("Extract = %+v, want nil for a package with no provenance", got)
	}
}

// TestExtract_NilPackageIsHonestlyAbsent guards the nil-receiver path a CLI
// caller who failed to open a package might otherwise panic on.
func TestExtract_NilPackageIsHonestlyAbsent(t *testing.T) {
	got, err := provenance.Extract(nil)
	if err != nil {
		t.Fatalf("Extract(nil): %v", err)
	}
	if got != nil {
		t.Errorf("Extract(nil) = %+v, want nil", got)
	}
}

// TestExtract_MalformedTimestampIsCoded pins the one real failure mode: a
// property that is recognisably Vellum's own but does not parse.
func TestExtract_MalformedTimestampIsCoded(t *testing.T) {
	pkg := opc.New()
	custom := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/custom-properties" ` +
		`xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">` +
		`<property fmtid="{D5CDD505-2E9C-101B-9397-08002B2CF9AE}" pid="2" name="VellumSourceDateEpoch">` +
		`<vt:lpwstr>not-a-timestamp</vt:lpwstr></property>` +
		`</Properties>`)
	if err := pkg.Put(&opc.Part{Name: "/docProps/custom.xml", ContentType: "application/vnd.openxmlformats-officedocument.custom-properties+xml", Data: custom}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	_, err := provenance.Extract(pkg)
	if err == nil {
		t.Fatal("Extract accepted a malformed timestamp")
	}
	if !verr.HasCode(err, verr.VELLUM_PROVENANCE_MALFORMED) {
		t.Errorf("Extract error = %v, want VELLUM_PROVENANCE_MALFORMED", err)
	}
}
