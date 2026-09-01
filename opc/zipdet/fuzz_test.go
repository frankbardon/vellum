package zipdet_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/opc/zipdet"
)

// FuzzRead fuzzes the archive reader.
//
// The reader sees untrusted input the moment a consumer accepts a
// user-supplied template. The property under test is the absence of
// catastrophe rather than the correctness of any particular parse: no panic,
// no unbounded allocation, and no entry name that escapes the package.
func FuzzRead(f *testing.F) {
	f.Add(writeSeed(f))
	f.Add([]byte{})
	f.Add([]byte("PK\x03\x04"))
	f.Add([]byte("PK\x05\x06\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"))
	f.Add(bytes.Repeat([]byte("PK\x01\x02"), 64))

	f.Fuzz(func(t *testing.T, data []byte) {
		// A small bound so a decompression bomb is refused rather than
		// expanded. Without it the fuzzer would find one and the process would
		// die of memory exhaustion instead of reporting a finding.
		a, err := zipdet.Read(bytes.NewReader(data), int64(len(data)), zipdet.ReadOptions{
			MaxEntryBytes: 1 << 20,
			MaxTotalBytes: 4 << 20,
		})
		if err != nil {
			return
		}

		for _, e := range a.Entries() {
			if e.Name == "" {
				t.Fatal("Read produced an entry with an empty name")
			}
			if e.Name[0] == '/' {
				t.Fatalf("Read produced an absolute entry name %q", e.Name)
			}
			// Segment-wise, not substring: "[Content_Types..000" contains ".."
			// and is a perfectly legal relative name. Only a segment that *is*
			// ".." escapes the package, and conflating the two is how a
			// traversal check comes to reject valid input.
			for _, seg := range strings.Split(e.Name, "/") {
				if seg == ".." || seg == "." || seg == "" {
					t.Fatalf("Read produced an entry name with an unsafe segment %q: %q", seg, e.Name)
				}
			}
			if _, ok := a.Get(e.Name); !ok {
				t.Fatalf("entry %q was listed but Get could not find it", e.Name)
			}
		}
	})
}

// FuzzWriteNames fuzzes entry-name validation from the writing side, where a
// caller-supplied name reaches the archive directly.
func FuzzWriteNames(f *testing.F) {
	for _, s := range []string{
		"word/document.xml",
		"/absolute.xml",
		"../escape.xml",
		`back\slash.xml`,
		"trailing/",
		"",
		"C:/drive.xml",
		"a//b.xml",
		"unicode/\u00e9\u4e2d.xml",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, name string) {
		var buf bytes.Buffer
		err := zipdet.Write(&buf, []zipdet.Entry{
			{Name: name, Kind: zipdet.KindCompressible, Data: []byte("x")},
		}, zipdet.WriteOptions{})
		if err != nil {
			return
		}

		// Anything the writer accepted must read back, and must read back with
		// the same name — a writer that accepted a name the reader then
		// rejects would produce archives Vellum itself cannot open.
		raw := buf.Bytes()
		a, rerr := zipdet.Read(bytes.NewReader(raw), int64(len(raw)), zipdet.ReadOptions{})
		if rerr != nil {
			t.Fatalf("Write accepted name %q but Read rejected the result: %v", name, rerr)
		}
		if got := a.Names(); len(got) != 1 || got[0] != name {
			t.Fatalf("round trip changed the entry name: wrote %q, read %v", name, got)
		}
	})
}

// writeSeed produces a valid archive for the fuzz corpus.
func writeSeed(f *testing.F) []byte {
	f.Helper()
	var buf bytes.Buffer
	if err := zipdet.Write(&buf, []zipdet.Entry{
		{Name: "[Content_Types].xml", Kind: zipdet.KindCompressible, Data: []byte(`<?xml version="1.0"?><Types/>`)},
		{Name: "word/document.xml", Kind: zipdet.KindCompressible, Data: []byte(`<?xml version="1.0"?><w:document/>`)},
		{Name: "word/media/image1.png", Kind: zipdet.KindPrecompressed, Data: []byte("\x89PNG\r\n\x1a\nseed")},
	}, zipdet.WriteOptions{}); err != nil {
		f.Fatalf("building the seed archive: %v", err)
	}
	return buf.Bytes()
}
