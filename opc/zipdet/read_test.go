package zipdet_test

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc/zipdet"
)

func TestRead_RoundTripsContentAndOrder(t *testing.T) {
	raw := writeFixture(t, zipdet.WriteOptions{})

	a, err := zipdet.Read(bytes.NewReader(raw), int64(len(raw)), zipdet.ReadOptions{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	want := fixture()
	if a.Len() != len(want) {
		t.Fatalf("Len = %d, want %d", a.Len(), len(want))
	}
	for i, got := range a.Entries() {
		if got.Name != want[i].Name {
			t.Errorf("entry %d name = %q, want %q; archive order must survive a read", i, got.Name, want[i].Name)
		}
		if !bytes.Equal(got.Data, want[i].Data) {
			t.Errorf("entry %q content did not round-trip", got.Name)
		}
	}
}

func TestRead_PreservesMethod(t *testing.T) {
	raw := writeFixture(t, zipdet.WriteOptions{})
	a, err := zipdet.Read(bytes.NewReader(raw), int64(len(raw)), zipdet.ReadOptions{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	// The method is preserved because a fill-mode round trip must reproduce
	// it: re-deflating a stored entry would change bytes Vellum promised not
	// to touch.
	png, ok := a.Get("word/media/image1.png")
	if !ok {
		t.Fatal("media entry missing")
	}
	if png.Method != zip.Store {
		t.Errorf("stored entry read back with method %d, want Store", png.Method)
	}

	doc, ok := a.Get("word/document.xml")
	if !ok {
		t.Fatal("document entry missing")
	}
	if doc.Method != zip.Deflate {
		t.Errorf("deflated entry read back with method %d, want Deflate", doc.Method)
	}
}

func TestRead_GetAndNames(t *testing.T) {
	raw := writeFixture(t, zipdet.WriteOptions{})
	a, err := zipdet.Read(bytes.NewReader(raw), int64(len(raw)), zipdet.ReadOptions{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if _, ok := a.Get("does/not/exist.xml"); ok {
		t.Error("Get reported a missing entry as present")
	}

	names := a.Names()
	if len(names) != a.Len() {
		t.Errorf("Names returned %d, Len is %d", len(names), a.Len())
	}
	if names[0] != "[Content_Types].xml" {
		t.Errorf("Names[0] = %q, want the content types part first", names[0])
	}
}

func TestRead_EntriesReturnsCopy(t *testing.T) {
	raw := writeFixture(t, zipdet.WriteOptions{})
	a, err := zipdet.Read(bytes.NewReader(raw), int64(len(raw)), zipdet.ReadOptions{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	first := a.Entries()
	original := first[0].Name
	first[0].Name = "mutated"

	if second := a.Entries(); second[0].Name != original {
		t.Error("Entries returned the backing slice; a caller could reorder the archive by mutating it")
	}
}

func TestRead_SkipsDirectoryEntries(t *testing.T) {
	// Authoring tools emit folder markers. Refusing a template because Word
	// added one would be pedantry rather than safety, so they are skipped.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if _, err := zw.Create("word/"); err != nil {
		t.Fatalf("Create dir: %v", err)
	}
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := w.Write([]byte("<w:p/>")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw := buf.Bytes()
	a, err := zipdet.Read(bytes.NewReader(raw), int64(len(raw)), zipdet.ReadOptions{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if a.Len() != 1 {
		t.Fatalf("Len = %d, want 1 (the directory marker should be skipped)", a.Len())
	}
	if a.Names()[0] != "word/document.xml" {
		t.Errorf("Names[0] = %q", a.Names()[0])
	}
}

func TestRead_RejectsMalformed(t *testing.T) {
	raw := writeFixture(t, zipdet.WriteOptions{})

	tests := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"truncated to nothing useful", raw[:8]},
		{"truncated central directory", raw[:len(raw)-8]},
		{"not an archive", []byte("this is plainly not a zip file at all, not even close")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := zipdet.Read(bytes.NewReader(tt.data), int64(len(tt.data)), zipdet.ReadOptions{})
			if err == nil {
				t.Fatal("Read succeeded on malformed input")
			}
			if !verr.HasCode(err, verr.VELLUM_ZIP_MALFORMED) {
				t.Errorf("error = %v, want VELLUM_ZIP_MALFORMED", err)
			}
		})
	}
}

func TestRead_RejectsTraversalNames(t *testing.T) {
	// Built with archive/zip directly, because zipdet.Write refuses to
	// produce such an archive — which is the point: the reader cannot assume
	// its input came from the writer.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("../../etc/passwd")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := w.Write([]byte("root:x:0:0")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw := buf.Bytes()
	_, err = zipdet.Read(bytes.NewReader(raw), int64(len(raw)), zipdet.ReadOptions{})
	if !verr.HasCode(err, verr.VELLUM_ZIP_ENTRY_NAME_INVALID) {
		t.Fatalf("error = %v, want VELLUM_ZIP_ENTRY_NAME_INVALID", err)
	}
}

// TestRead_RefusesBomb covers the case the bound exists for: a small archive
// whose entry expands enormously. The declared size is attacker-controlled, so
// the bound is enforced against the bytes actually read, not the declaration.
func TestRead_RefusesBomb(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("bomb.bin")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Highly compressible: 8 MiB of zeros deflates to a few kilobytes.
	if _, err := w.Write(bytes.Repeat([]byte{0}, 8<<20)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw := buf.Bytes()
	if len(raw) > 1<<20 {
		t.Fatalf("fixture is %d bytes; it was meant to compress small", len(raw))
	}

	_, err = zipdet.Read(bytes.NewReader(raw), int64(len(raw)), zipdet.ReadOptions{MaxEntryBytes: 1 << 20})
	if !verr.HasCode(err, verr.VELLUM_ZIP_TOO_LARGE) {
		t.Fatalf("error = %v, want VELLUM_ZIP_TOO_LARGE", err)
	}
}

func TestRead_EnforcesTotalBound(t *testing.T) {
	raw := writeFixture(t, zipdet.WriteOptions{})

	_, err := zipdet.Read(bytes.NewReader(raw), int64(len(raw)), zipdet.ReadOptions{MaxTotalBytes: 16})
	if !verr.HasCode(err, verr.VELLUM_ZIP_TOO_LARGE) {
		t.Fatalf("error = %v, want VELLUM_ZIP_TOO_LARGE", err)
	}
}

func TestArchive_NilReceiverIsSafe(t *testing.T) {
	var a *zipdet.Archive
	if a.Len() != 0 {
		t.Error("Len on nil archive")
	}
	if a.Entries() != nil {
		t.Error("Entries on nil archive")
	}
	if a.Names() != nil {
		t.Error("Names on nil archive")
	}
	if _, ok := a.Get("x"); ok {
		t.Error("Get on nil archive reported a hit")
	}
}

// TestRead_WriteRoundTripIsStable proves the loop that fill mode depends on:
// reading an archive and writing its entries back out reproduces the bytes.
func TestRead_WriteRoundTripIsStable(t *testing.T) {
	original := writeFixture(t, zipdet.WriteOptions{})

	a, err := zipdet.Read(bytes.NewReader(original), int64(len(original)), zipdet.ReadOptions{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	entries := make([]zipdet.Entry, 0, a.Len())
	for _, e := range a.Entries() {
		kind := zipdet.KindCompressible
		if e.Method == zip.Store {
			kind = zipdet.KindPrecompressed
		}
		entries = append(entries, zipdet.Entry{Name: e.Name, Kind: kind, Data: e.Data})
	}

	var rewritten bytes.Buffer
	if err := zipdet.Write(&rewritten, entries, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if !bytes.Equal(original, rewritten.Bytes()) {
		t.Error("read-then-write did not reproduce the original bytes")
	}
}

func TestWrite_DoesNotWriteOnFailure(t *testing.T) {
	// A failure must be detected before any bytes reach the writer for the
	// offending entry, so a caller streaming to a file does not end up with a
	// half-written archive it believes is complete.
	var buf bytes.Buffer
	err := zipdet.Write(&buf, []zipdet.Entry{
		{Name: "ok.xml", Kind: zipdet.KindCompressible, Data: []byte("<a/>")},
		{Name: "../escape.xml", Kind: zipdet.KindCompressible, Data: []byte("<b/>")},
	}, zipdet.WriteOptions{})

	if err == nil {
		t.Fatal("Write succeeded with a traversal name")
	}
	if _, rerr := zipdet.Read(bytes.NewReader(buf.Bytes()), int64(buf.Len()), zipdet.ReadOptions{}); rerr == nil {
		t.Error("a partially written archive parsed as valid; the caller could mistake it for complete")
	}
}

var _ io.Reader = (*bytes.Reader)(nil)
