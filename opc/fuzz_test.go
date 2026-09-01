package opc_test

import (
	"bytes"
	"testing"

	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
)

// FuzzOpen fuzzes the package reader.
//
// Fuzzing this is cheap now and expensive later: once fill mode exists, a
// crash in Open is a crash in a consumer's ingest path, reachable by anyone who
// can upload a template. The property under test is not correctness of the
// parse but the absence of catastrophe — no panic, no unbounded allocation,
// and nothing written outside the package.
func FuzzOpen(f *testing.F) {
	for _, name := range seedFiles(f) {
		f.Add(readSeed(f, name))
	}
	// A couple of structural edge cases the corpus does not cover.
	f.Add([]byte{})
	f.Add([]byte("PK\x03\x04"))
	f.Add([]byte("PK\x05\x06\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// The bound is deliberately small so a bomb is refused rather than
		// expanded: without it, fuzzing would find one and the process would
		// die of memory exhaustion rather than reporting a finding.
		p, err := opc.Open(bytes.NewReader(data), int64(len(data)),
			opc.WithMaxPartBytes(1<<20), opc.WithMaxTotalBytes(4<<20))
		if err != nil {
			return
		}
		if p == nil {
			t.Fatal("Open returned no package and no error")
		}

		// Anything that opened must survive the operations a caller will
		// immediately perform on it.
		_ = p.Validate()
		for _, name := range p.Names() {
			part, ok := p.Get(name)
			if !ok {
				t.Fatalf("Names listed %q but Get could not find it", name)
			}
			if _, err := part.Bytes(); err != nil {
				t.Fatalf("part %q listed but unreadable: %v", name, err)
			}
			if err := opc.ValidatePartName(name); err != nil {
				t.Fatalf("Open accepted an invalid part name %q: %v", name, err)
			}
		}

		// A package that opened must also write, and writing must not panic.
		var buf bytes.Buffer
		_ = p.WriteTo(&buf, zipdet.WriteOptions{})
	})
}

// FuzzReadWriteRoundTrip checks that anything Vellum successfully opens and
// writes can be opened again — the property fill mode relies on when it hands
// a filled package back to a consumer.
func FuzzReadWriteRoundTrip(f *testing.F) {
	for _, name := range seedFiles(f) {
		f.Add(readSeed(f, name))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		first, err := opc.Open(bytes.NewReader(data), int64(len(data)),
			opc.WithMaxPartBytes(1<<20), opc.WithMaxTotalBytes(4<<20))
		if err != nil {
			return
		}

		var buf bytes.Buffer
		if err := first.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
			return
		}
		out := buf.Bytes()

		second, err := opc.Open(bytes.NewReader(out), int64(len(out)),
			opc.WithMaxPartBytes(1<<20), opc.WithMaxTotalBytes(4<<20))
		if err != nil {
			t.Fatalf("a package Vellum wrote could not be read back: %v", err)
		}
		if got, want := second.Len(), first.Len(); got != want {
			t.Fatalf("round trip changed the part count: %d -> %d", want, got)
		}

		// Writing again must reproduce the same bytes. If it does not, the
		// writer is not idempotent and the determinism guarantee is false for
		// some input the fixtures do not cover.
		var again bytes.Buffer
		if err := second.WriteTo(&again, zipdet.WriteOptions{}); err != nil {
			t.Fatalf("second write failed: %v", err)
		}
		if !bytes.Equal(out, again.Bytes()) {
			t.Fatal("writing a package twice produced different bytes")
		}
	})
}
