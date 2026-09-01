package doc_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/doc"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/spec"
)

func skeletonSpec() *spec.Spec {
	return &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Title:         "Walking Skeleton",
		Sections: []spec.Section{{
			ID: "intro",
			Blocks: []spec.Block{
				{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 1, Content: "First Heading"}},
				{Kind: spec.BlockText, Text: &spec.Text{Content: "A paragraph of prose, with an ampersand & a < angle bracket."}},
			},
		}},
	}
}

func emit(t *testing.T, s *spec.Spec) []byte {
	t.Helper()
	d, err := doc.Lower(s)
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	var buf bytes.Buffer
	if err := d.WriteTo(&buf, doc.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

func TestWriteTo_ProducesExpectedParts(t *testing.T) {
	raw := emit(t, skeletonSpec())

	a, err := zipdet.Read(bytes.NewReader(raw), int64(len(raw)), zipdet.ReadOptions{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	want := []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"docProps/app.xml",
		"docProps/core.xml",
		"word/document.xml",
		"word/_rels/document.xml.rels",
	}
	got := a.Names()
	if len(got) != len(want) {
		t.Fatalf("parts = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("part %d = %q, want %q\nfull: %v", i, got[i], want[i], got)
		}
	}
}

func TestWriteTo_DocumentXMLShape(t *testing.T) {
	raw := emit(t, skeletonSpec())
	body := partText(t, raw, "word/document.xml")

	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`,
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`,
		`<w:outlineLvl w:val="0"/>`,
		`<w:t xml:space="preserve">First Heading</w:t>`,
		`ampersand &amp; a &lt; angle bracket.`,
		`<w:sectPr>`,
		`<w:pgSz w:w="11906" w:h="16838"/>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("document.xml is missing %q\n---\n%s", want, body)
		}
	}
}

// TestWriteTo_PreservesWhitespace covers the reason xml:space is unconditional:
// leading and trailing whitespace is content, and deciding case by case
// whether to preserve it is how a writer comes to eat a space it was given.
func TestWriteTo_PreservesWhitespace(t *testing.T) {
	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Sections: []spec.Section{{Blocks: []spec.Block{
			{Kind: spec.BlockText, Text: &spec.Text{Content: "  leading and trailing  "}},
		}}},
	}
	body := partText(t, emit(t, s), "word/document.xml")
	if !strings.Contains(body, `<w:t xml:space="preserve">  leading and trailing  </w:t>`) {
		t.Errorf("whitespace was not preserved:\n%s", body)
	}
}

// TestLower_RejectsUnsupportedBlockKinds is the first enforcement of "never
// silently drop content". The error must name the kind and where it was, so
// the caller can act rather than go looking.
func TestLower_RejectsUnsupportedBlockKinds(t *testing.T) {
	for _, kind := range []spec.BlockKind{
		spec.BlockAsset, spec.BlockTable, spec.BlockPageBreak, spec.BlockNotes, spec.BlockSpacer,
	} {
		t.Run(string(kind), func(t *testing.T) {
			s := &spec.Spec{
				FormatVersion: spec.FormatVersion,
				Sections: []spec.Section{{
					ID: "s1",
					Blocks: []spec.Block{
						{Kind: spec.BlockText, Text: &spec.Text{Content: "before"}},
						{Kind: kind},
					},
				}},
			}
			_, err := doc.Lower(s)
			if !verr.HasCode(err, verr.VELLUM_DOC_BLOCK_UNSUPPORTED) {
				t.Fatalf("error = %v, want VELLUM_DOC_BLOCK_UNSUPPORTED", err)
			}

			var ce *verr.CodedError
			if !asCoded(err, &ce) {
				t.Fatal("error is not a CodedError")
			}
			if got, _ := ce.Detail("kind"); got != string(kind) {
				t.Errorf("detail kind = %v, want %q", got, kind)
			}
			if got, _ := ce.Detail("block_index"); got != 1 {
				t.Errorf("detail block_index = %v, want 1", got)
			}
			if got, _ := ce.Detail("section_id"); got != "s1" {
				t.Errorf("detail section_id = %v, want \"s1\"", got)
			}
		})
	}
}

func TestLower_RejectsInvalidSpecs(t *testing.T) {
	tests := []struct {
		name string
		spec *spec.Spec
		code verr.Code
	}{
		{"nil", nil, verr.VELLUM_SPEC_INVALID},
		{"no sections", &spec.Spec{}, verr.VELLUM_SPEC_INVALID},
		{"empty section", &spec.Spec{Sections: []spec.Section{{}}}, verr.VELLUM_SPEC_INVALID},
		{
			name: "heading with no content arm",
			spec: &spec.Spec{Sections: []spec.Section{{Blocks: []spec.Block{{Kind: spec.BlockHeading}}}}},
			code: verr.VELLUM_SPEC_INVALID,
		},
		{
			name: "heading level zero",
			spec: &spec.Spec{Sections: []spec.Section{{Blocks: []spec.Block{
				{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 0, Content: "x"}},
			}}}},
			code: verr.VELLUM_SPEC_INVALID,
		},
		{
			name: "unknown kind",
			spec: &spec.Spec{Sections: []spec.Section{{Blocks: []spec.Block{{Kind: "sidebar"}}}}},
			code: verr.VELLUM_SPEC_BLOCK_KIND_UNKNOWN,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := doc.Lower(tt.spec); !verr.HasCode(err, tt.code) {
				t.Errorf("error = %v, want %s", err, tt.code)
			}
		})
	}
}

func TestWriteTo_TimestampsArePinned(t *testing.T) {
	core := partText(t, emit(t, skeletonSpec()), "docProps/core.xml")

	if !strings.Contains(core, `<dcterms:created xsi:type="dcterms:W3CDTF">1980-01-01T00:00:00Z</dcterms:created>`) {
		t.Errorf("core properties do not carry the pinned epoch:\n%s", core)
	}
	if strings.Contains(core, "202") {
		t.Errorf("core properties appear to contain a current-year date; the clock must not reach the document:\n%s", core)
	}
}

func TestWriteTo_IsDeterministic(t *testing.T) {
	seen := make(map[string]bool)
	for range 200 {
		sum := sha256.Sum256(emit(t, skeletonSpec()))
		seen[hex.EncodeToString(sum[:])] = true
	}
	if len(seen) != 1 {
		t.Fatalf("200 emissions produced %d distinct digests, want 1", len(seen))
	}
}

func TestHeadingLevels_ClampRatherThanPanic(t *testing.T) {
	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Sections: []spec.Section{{Blocks: []spec.Block{
			{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 99, Content: "Deep"}},
		}}},
	}
	body := partText(t, emit(t, s), "word/document.xml")
	if !strings.Contains(body, `<w:outlineLvl w:val="98"/>`) {
		t.Errorf("a deep heading did not survive lowering:\n%s", body)
	}
}

func partText(t *testing.T, raw []byte, name string) string {
	t.Helper()
	a, err := zipdet.Read(bytes.NewReader(raw), int64(len(raw)), zipdet.ReadOptions{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	e, ok := a.Get(name)
	if !ok {
		t.Fatalf("part %q not found; parts are %v", name, a.Names())
	}
	return string(e.Data)
}

func asCoded(err error, target **verr.CodedError) bool {
	for err != nil {
		if ce, ok := err.(*verr.CodedError); ok {
			*target = ce
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
