package fragtest_test

import (
	"bytes"
	"testing"

	"github.com/frankbardon/vellum/internal/fragtest"
	"github.com/frankbardon/vellum/xmlcopy"
)

const (
	nsWordprocessing = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	nsRelationships  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
)

// TestFragment_DeterministicAndWellFormed exercises the binding
// determinism constraint directly: the same Strategy and marker produce
// byte-identical output on every call, and the produced paragraph is
// well-formed XML once wrapped in a minimal document.
func TestFragment_DeterministicAndWellFormed(t *testing.T) {
	const marker = "{{customer_name}}"
	for _, strategy := range fragtest.All() {
		t.Run(strategy.String(), func(t *testing.T) {
			first := fragtest.Fragment(strategy, marker)
			for i := 0; i < 25; i++ {
				again := fragtest.Fragment(strategy, marker)
				if !bytes.Equal(first, again) {
					t.Fatalf("Fragment(%s, %q) is not deterministic: call 0 = %s, call %d = %s",
						strategy, marker, first, i+1, again)
				}
			}

			doc := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
				`<w:document xmlns:w="` + nsWordprocessing + `" xmlns:r="` + nsRelationships + `">` +
				`<w:body>`)
			doc = append(doc, first...)
			doc = append(doc, []byte(`</w:body></w:document>`)...)

			if err := xmlcopy.Walk(doc, func(xmlcopy.Element) error { return nil }); err != nil {
				t.Fatalf("Fragment(%s, %q) produced XML that does not parse: %v\n%s", strategy, marker, err, doc)
			}
		})
	}
}

// TestFragment_MarkerTextNeverAppearsContiguous confirms the whole point of
// generating these fixtures instead of hand-typing a marker: for every
// strategy that splits the marker itself (all but the surrounding "See "/"
// today." wrapper, which is never split), the literal marker string does not
// appear as a contiguous substring of the raw XML bytes -- a naive
// regex-over-raw-XML matcher would find nothing.
func TestFragment_MarkerTextNeverAppearsContiguous(t *testing.T) {
	const marker = "{{customer_name}}"
	for _, strategy := range fragtest.All() {
		t.Run(strategy.String(), func(t *testing.T) {
			out := fragtest.Fragment(strategy, marker)
			if bytes.Contains(out, []byte(marker)) {
				t.Errorf("Fragment(%s, %q) left the marker contiguous in the raw XML; that defeats the point of this generator: %s",
					strategy, marker, out)
			}
		})
	}
}
