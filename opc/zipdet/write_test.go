package zipdet_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"strings"
	"testing"
	"time"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc/zipdet"
)

func fixture() []zipdet.Entry {
	return []zipdet.Entry{
		{Name: "[Content_Types].xml", Kind: zipdet.KindCompressible, Data: []byte(`<?xml version="1.0"?><Types/>`)},
		{Name: "_rels/.rels", Kind: zipdet.KindCompressible, Data: []byte(`<?xml version="1.0"?><Relationships/>`)},
		{Name: "word/document.xml", Kind: zipdet.KindCompressible, Data: bytes.Repeat([]byte("<w:p><w:r><w:t>text</w:t></w:r></w:p>"), 64)},
		{Name: "word/media/image1.png", Kind: zipdet.KindPrecompressed, Data: []byte("\x89PNG\r\n\x1a\n not really a png")},
	}
}

func writeFixture(t *testing.T, opts zipdet.WriteOptions) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := zipdet.Write(&buf, fixture(), opts); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return buf.Bytes()
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// TestWrite_ByteIdenticalAcrossRuns is the core claim of the package. A
// thousand writes in one process must produce exactly one digest.
func TestWrite_ByteIdenticalAcrossRuns(t *testing.T) {
	seen := make(map[string]int)
	for range 1000 {
		seen[digest(writeFixture(t, zipdet.WriteOptions{}))]++
	}
	if len(seen) != 1 {
		t.Fatalf("1000 writes produced %d distinct digests, want 1: %v", len(seen), seen)
	}
}

// TestWrite_NoExtraFields is the regression test for the trap this package
// exists to avoid: a non-zero FileHeader.Modified makes archive/zip append an
// Info-ZIP extended timestamp extra field, so the header bytes carry a second
// copy of the time. Reading the raw local headers is the only way to see it —
// the archive/zip reader hides it.
func TestWrite_NoExtraFields(t *testing.T) {
	raw := writeFixture(t, zipdet.WriteOptions{})

	for _, h := range localHeaders(t, raw) {
		if h.extraLen != 0 {
			t.Errorf("entry %q has a %d-byte extra field; leave FileHeader.Modified zero and write the MS-DOS fields directly",
				h.name, h.extraLen)
		}
	}
}

// TestWrite_NoDataDescriptors pins that sizes and CRC are written up front.
// Bit 3 of the general-purpose flags means "sizes follow the payload in a data
// descriptor", which the streaming Create path sets and CreateRaw does not.
func TestWrite_NoDataDescriptors(t *testing.T) {
	raw := writeFixture(t, zipdet.WriteOptions{})

	for _, h := range localHeaders(t, raw) {
		if h.flags&0x08 != 0 {
			t.Errorf("entry %q sets the data-descriptor flag; use CreateRaw with sizes known up front", h.name)
		}
		if h.compressedSize == 0 && h.uncompressedSize == 0 && len(h.name) > 0 {
			t.Errorf("entry %q has zero sizes in its local header", h.name)
		}
	}
}

// TestWrite_TimestampsArePinned checks the actual encoded MS-DOS fields rather
// than trusting the reader's interpretation.
func TestWrite_TimestampsArePinned(t *testing.T) {
	raw := writeFixture(t, zipdet.WriteOptions{})

	// 1980-01-01 00:00:00 encodes as date = (0<<9)|(1<<5)|1 = 33, time = 0.
	const wantDate, wantTime = 33, 0
	for _, h := range localHeaders(t, raw) {
		if h.modDate != wantDate || h.modTime != wantTime {
			t.Errorf("entry %q has date=%d time=%d, want date=%d time=%d (the pinned 1980 epoch)",
				h.name, h.modDate, h.modTime, wantDate, wantTime)
		}
	}
}

func TestWrite_SourceDateEpochIsStableAndDistinct(t *testing.T) {
	epoch := time.Date(2020, time.June, 15, 12, 30, 0, 0, time.UTC)

	a := digest(writeFixture(t, zipdet.WriteOptions{SourceDateEpoch: epoch}))
	b := digest(writeFixture(t, zipdet.WriteOptions{SourceDateEpoch: epoch}))
	if a != b {
		t.Error("the same explicit epoch produced different bytes")
	}

	if pinned := digest(writeFixture(t, zipdet.WriteOptions{})); a == pinned {
		t.Error("an explicit epoch produced the same bytes as the pinned default; the option has no effect")
	}
}

// TestWrite_PreEpochTimeIsClamped covers the MS-DOS range floor. A wrapped
// year is a plausible-looking wrong answer; a clamped one is obviously pinned.
func TestWrite_PreEpochTimeIsClamped(t *testing.T) {
	old := time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)
	got := digest(writeFixture(t, zipdet.WriteOptions{SourceDateEpoch: old}))
	want := digest(writeFixture(t, zipdet.WriteOptions{}))
	if got != want {
		t.Error("a pre-1980 epoch was not clamped to the pinned epoch; MS-DOS timestamps cannot represent it")
	}
}

// TestWrite_MethodFollowsKind pins that compression is a rule over the
// declared kind, not a decision at the call site and not content sniffing.
func TestWrite_MethodFollowsKind(t *testing.T) {
	raw := writeFixture(t, zipdet.WriteOptions{})
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	want := map[string]uint16{
		"[Content_Types].xml":   zip.Deflate,
		"_rels/.rels":           zip.Deflate,
		"word/document.xml":     zip.Deflate,
		"word/media/image1.png": zip.Store,
	}
	for _, f := range zr.File {
		w, ok := want[f.Name]
		if !ok {
			t.Errorf("unexpected entry %q", f.Name)
			continue
		}
		if f.Method != w {
			t.Errorf("entry %q method = %d, want %d", f.Name, f.Method, w)
		}
	}
}

func TestWrite_UncompressedStoresEverything(t *testing.T) {
	raw := writeFixture(t, zipdet.WriteOptions{Uncompressed: true})
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	for _, f := range zr.File {
		if f.Method != zip.Store {
			t.Errorf("entry %q method = %d, want Store under Uncompressed", f.Name, f.Method)
		}
	}
}

func TestWrite_OrderIsExactlyAsGiven(t *testing.T) {
	raw := writeFixture(t, zipdet.WriteOptions{})
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	want := []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml", "word/media/image1.png"}
	if len(zr.File) != len(want) {
		t.Fatalf("got %d entries, want %d", len(zr.File), len(want))
	}
	for i, f := range zr.File {
		if f.Name != want[i] {
			t.Errorf("entry %d = %q, want %q; Write must not reorder — canonical ordering is an OPC concern", i, f.Name, want[i])
		}
	}
}

func TestWrite_LazySourceMatchesInlineData(t *testing.T) {
	payload := []byte("<w:p/>")

	var inline bytes.Buffer
	if err := zipdet.Write(&inline, []zipdet.Entry{
		{Name: "a.xml", Kind: zipdet.KindCompressible, Data: payload},
	}, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("Write inline: %v", err)
	}

	var lazy bytes.Buffer
	if err := zipdet.Write(&lazy, []zipdet.Entry{
		{Name: "a.xml", Kind: zipdet.KindCompressible, Open: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(payload)), nil
		}},
	}, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("Write lazy: %v", err)
	}

	if !bytes.Equal(inline.Bytes(), lazy.Bytes()) {
		t.Error("a lazily-opened entry produced different bytes from the same inline data")
	}
}

func TestWrite_RejectsBadEntries(t *testing.T) {
	tests := []struct {
		name    string
		entries []zipdet.Entry
		code    verr.Code
	}{
		{
			name:    "absolute name",
			entries: []zipdet.Entry{{Name: "/word/document.xml", Data: []byte("x")}},
			code:    verr.VELLUM_ZIP_ENTRY_NAME_INVALID,
		},
		{
			name:    "parent traversal",
			entries: []zipdet.Entry{{Name: "word/../../etc/passwd", Data: []byte("x")}},
			code:    verr.VELLUM_ZIP_ENTRY_NAME_INVALID,
		},
		{
			name:    "backslash separator",
			entries: []zipdet.Entry{{Name: `word\document.xml`, Data: []byte("x")}},
			code:    verr.VELLUM_ZIP_ENTRY_NAME_INVALID,
		},
		{
			name:    "drive letter",
			entries: []zipdet.Entry{{Name: "C:/document.xml", Data: []byte("x")}},
			code:    verr.VELLUM_ZIP_ENTRY_NAME_INVALID,
		},
		{
			name:    "trailing slash",
			entries: []zipdet.Entry{{Name: "word/", Data: []byte("x")}},
			code:    verr.VELLUM_ZIP_ENTRY_NAME_INVALID,
		},
		{
			name:    "empty name",
			entries: []zipdet.Entry{{Name: "", Data: []byte("x")}},
			code:    verr.VELLUM_ZIP_ENTRY_NAME_INVALID,
		},
		{
			name: "duplicate names",
			entries: []zipdet.Entry{
				{Name: "a.xml", Data: []byte("x")},
				{Name: "a.xml", Data: []byte("y")},
			},
			code: verr.VELLUM_ZIP_ENTRY_DUPLICATE,
		},
		{
			name:    "neither data nor source",
			entries: []zipdet.Entry{{Name: "a.xml"}},
			code:    verr.VELLUM_INTERNAL_INVARIANT,
		},
		{
			name: "both data and source",
			entries: []zipdet.Entry{{Name: "a.xml", Data: []byte("x"), Open: func() (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader("y")), nil
			}}},
			code: verr.VELLUM_INTERNAL_INVARIANT,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := zipdet.Write(io.Discard, tt.entries, zipdet.WriteOptions{})
			if err == nil {
				t.Fatal("Write succeeded; it must refuse rather than sanitise")
			}
			if !verr.HasCode(err, tt.code) {
				t.Errorf("error = %v, want code %s", err, tt.code)
			}
		})
	}
}

func TestWrite_EnforcesEntryBound(t *testing.T) {
	entries := []zipdet.Entry{{Name: "big.bin", Kind: zipdet.KindCompressible, Data: bytes.Repeat([]byte{0}, 4096)}}

	err := zipdet.Write(io.Discard, entries, zipdet.WriteOptions{MaxEntryBytes: 1024})
	if !verr.HasCode(err, verr.VELLUM_ZIP_TOO_LARGE) {
		t.Fatalf("error = %v, want VELLUM_ZIP_TOO_LARGE", err)
	}

	if err := zipdet.Write(io.Discard, entries, zipdet.WriteOptions{MaxEntryBytes: 4096}); err != nil {
		t.Errorf("an entry exactly at the bound was rejected: %v", err)
	}
}

func TestWrite_EnforcesBoundOnLazySource(t *testing.T) {
	// A lazy source that lies about its size is the interesting case: the
	// bound must be enforced while reading, not from a declaration.
	entries := []zipdet.Entry{{Name: "big.bin", Kind: zipdet.KindCompressible, Open: func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{0}, 4096))), nil
	}}}

	err := zipdet.Write(io.Discard, entries, zipdet.WriteOptions{MaxEntryBytes: 1024})
	if !verr.HasCode(err, verr.VELLUM_ZIP_TOO_LARGE) {
		t.Fatalf("error = %v, want VELLUM_ZIP_TOO_LARGE", err)
	}
}

// localHeader is the subset of a ZIP local file header this test file needs.
type localHeader struct {
	name             string
	flags            uint16
	modTime          uint16
	modDate          uint16
	compressedSize   uint32
	uncompressedSize uint32
	extraLen         uint16
}

// localHeaders walks the raw archive bytes header by header. Going to the raw
// bytes is the point: archive/zip's reader normalises away exactly the fields
// under test.
func localHeaders(t *testing.T, raw []byte) []localHeader {
	t.Helper()

	var out []localHeader
	off := 0
	for off+30 <= len(raw) {
		if !bytes.Equal(raw[off:off+4], []byte("PK\x03\x04")) {
			break // central directory reached
		}
		h := localHeader{
			flags:            binary.LittleEndian.Uint16(raw[off+6:]),
			modTime:          binary.LittleEndian.Uint16(raw[off+10:]),
			modDate:          binary.LittleEndian.Uint16(raw[off+12:]),
			compressedSize:   binary.LittleEndian.Uint32(raw[off+18:]),
			uncompressedSize: binary.LittleEndian.Uint32(raw[off+22:]),
			extraLen:         binary.LittleEndian.Uint16(raw[off+28:]),
		}
		nameLen := int(binary.LittleEndian.Uint16(raw[off+26:]))
		start := off + 30
		if start+nameLen > len(raw) {
			t.Fatalf("truncated local header at offset %d", off)
		}
		h.name = string(raw[start : start+nameLen])
		out = append(out, h)
		off = start + nameLen + int(h.extraLen) + int(h.compressedSize)
	}
	if len(out) == 0 {
		t.Fatal("no local file headers found")
	}
	return out
}
