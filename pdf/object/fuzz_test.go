package object_test

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/pdf/object"
)

// FuzzWriteDocument fuzzes the object writer.
//
// The fuzz input is a program, not a document: each byte selects an object to
// append, so the fuzzer explores the shapes a document can take rather than the
// bytes a document is made of. That matters because the writer is not a parser
// — nobody hands it a file — so fuzzing its output would test the reader in
// this file and nothing else.
//
// Three properties, and the third is the one worth having:
//
//   - it never panics, however the objects nest;
//   - what it writes parses back, so an escaping or delimiter fault is caught
//     rather than sitting in a golden nobody re-reads;
//   - writing the same document twice produces identical bytes. That is the
//     determinism guarantee restated over arbitrary input instead of over the
//     seven fixtures, and it is the one a map or a wall-clock reaching the
//     output path would break for some shape no fixture happens to have.
func FuzzWriteDocument(f *testing.F) {
	f.Add([]byte{0})
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	f.Add([]byte("dictionaries and arrays and names"))
	f.Add(bytes.Repeat([]byte{9}, 64))

	f.Fuzz(func(t *testing.T, program []byte) {
		// Bounded, so the fuzzer spends its budget on shapes rather than on
		// building one enormous flat document that proves nothing new.
		if len(program) > 256 {
			program = program[:256]
		}

		first := buildDocument(t, program)
		var a bytes.Buffer
		if err := first.Write(&a); err != nil {
			// A refusal is fine — an unfilled reservation, for one. It must
			// still be a refusal rather than a partial file.
			return
		}

		// Anything written must parse, and must carry the header and the
		// terminator a reader looks for first.
		out := a.Bytes()
		if !bytes.HasPrefix(out, []byte("%PDF-")) {
			t.Fatal("the file does not begin with a PDF header")
		}
		if !bytes.Contains(out, []byte("startxref")) || !bytes.HasSuffix(out, []byte("%%EOF\n")) {
			t.Fatal("the file has no cross-reference pointer or no terminator")
		}
		readObjects(t, out)
		assertXrefOffsetsPoint(t, out)

		second := buildDocument(t, program)
		var b bytes.Buffer
		if err := second.Write(&b); err != nil {
			t.Fatalf("the same document wrote once and refused once: %v", err)
		}
		if !bytes.Equal(a.Bytes(), b.Bytes()) {
			t.Fatal("writing the same document twice produced different bytes")
		}
	})
}

// buildDocument interprets the fuzz input as a sequence of object appends.
//
// Every opcode adds something, so the document grows with the input and the
// fuzzer's coverage feedback is about structure. Values are drawn from the
// bytes that follow, which is how strings containing PDF delimiters — the ones
// that need escaping — get produced without being seeded by hand.
func buildDocument(t *testing.T, program []byte) *object.Document {
	t.Helper()

	doc := &object.Document{}
	var refs []object.Ref

	// A reference to an object that exists, or a reference to object zero when
	// none does yet. Object zero is the free-list head and is never a target,
	// so it also exercises the writer's handling of a reference going nowhere.
	pick := func(i int) object.Ref {
		if len(refs) == 0 {
			return object.Ref{}
		}
		return refs[i%len(refs)]
	}

	for i := 0; i < len(program); i++ {
		op := program[i]
		next := byte(0)
		if i+1 < len(program) {
			next = program[i+1]
		}

		switch op % 9 {
		case 0:
			refs = append(refs, doc.Add(object.Int(int64(next)-128)))
		case 1:
			refs = append(refs, doc.Add(object.Real(int64(next)*7-1000)))
		case 2:
			refs = append(refs, doc.Add(object.Name(nameFrom(next))))
		case 3:
			refs = append(refs, doc.Add(object.String(stringFrom(program[i:]))))
		case 4:
			refs = append(refs, doc.Add(object.HexString(program[i:min(i+int(next%8)+1, len(program))])))
		case 5:
			refs = append(refs, doc.Add(object.Array{
				object.Int(int64(next)), object.Name(nameFrom(op)), pick(int(next)),
			}))
		case 6:
			d := object.NewDict(
				object.Name(nameFrom(next)), object.Int(int64(op)),
				"Ref", pick(int(op)),
			)
			refs = append(refs, doc.Add(d))
		case 7:
			ref, err := doc.AddStream(object.NewDict("Sub", object.Name(nameFrom(next))), program[i:])
			if err != nil {
				t.Fatalf("adding a stream failed: %v", err)
			}
			refs = append(refs, ref)
		case 8:
			// A reservation filled immediately. The unfilled case is covered by
			// the write refusing, above; this covers the cycle a page tree
			// needs, where a child names a parent allocated before it.
			ref := doc.Reserve()
			if err := doc.Fill(ref, object.NewDict("Self", ref, "Prev", pick(int(next)))); err != nil {
				t.Fatalf("filling a reserved object failed: %v", err)
			}
			refs = append(refs, ref)
		}
	}

	if len(refs) > 0 {
		doc.Root = refs[0]
	} else {
		doc.Root = doc.Add(object.NewDict("Type", object.Name("Catalog")))
	}
	return doc
}

// nameFrom produces a PDF name from a byte, including ones needing escapes.
//
// A name is written with #-escapes for delimiters and whitespace, and a name
// that never contains one is a name whose escaping was never exercised.
func nameFrom(b byte) string {
	const alphabet = "AZaz09" + "#()<>[]{}/% \t\r\n" + "\x00\x7f\xff"
	i := int(b) % len(alphabet)
	return "N" + alphabet[i:i+1]
}

// stringFrom produces a literal string from the input, bounded.
func stringFrom(b []byte) string {
	if len(b) > 24 {
		b = b[:24]
	}
	return string(b)
}

// assertXrefOffsetsPoint checks each cross-reference entry lands on its object.
//
// The offsets are the one part of the file a reader trusts absolutely and no
// other assertion touches: a document whose objects are all correct and whose
// table is off by one byte opens as an empty document in some readers and as a
// damaged one in others. Everything else here would pass on that file.
func assertXrefOffsetsPoint(t *testing.T, out []byte) {
	t.Helper()

	at := bytes.LastIndex(out, []byte("\nxref\n"))
	if at < 0 {
		t.Fatal("the file has no cross-reference table")
	}
	lines := strings.Split(string(out[at+len("\nxref\n"):]), "\n")

	// The subsection header, then one fixed-width entry per object starting at
	// zero — so the entry for object n is the line after the one for n-1, and
	// the free-list head occupies the first.
	header := strings.Fields(lines[0])
	if len(header) != 2 || header[0] != "0" {
		t.Fatalf("the cross-reference subsection header is %q, want \"0 <count>\"", lines[0])
	}
	count, err := strconv.Atoi(header[1])
	if err != nil {
		t.Fatalf("the cross-reference count %q does not parse", header[1])
	}
	if count+1 > len(lines) {
		t.Fatalf("the table declares %d entries and carries %d lines", count, len(lines)-1)
	}

	for n := 1; n < count; n++ {
		entry := lines[n+1]
		if len(entry) < 18 || entry[17] != 'n' {
			continue
		}
		offset, err := strconv.Atoi(entry[:10])
		if err != nil {
			t.Fatalf("entry %d has an unparseable offset %q", n, entry[:10])
		}
		want := []byte(strconv.Itoa(n) + " 0 obj\n")
		if offset < 0 || offset+len(want) > len(out) || !bytes.Equal(out[offset:offset+len(want)], want) {
			end := min(offset+24, len(out))
			if offset < 0 || offset > len(out) {
				end = 0
				offset = 0
			}
			t.Fatalf("the cross-reference entry for object %d points at offset %d, "+
				"which is not where that object starts.\nWhat is there: %q",
				n, offset, out[offset:end])
		}
	}
}
