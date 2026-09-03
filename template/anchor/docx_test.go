package anchor_test

import (
	"testing"

	"github.com/frankbardon/vellum/artifact"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/template/anchor"
)

const ctMainDocument = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"

// wordDocument wraps body in a realistic WordprocessingML root element and
// declaration, mirroring the shape xmlcopy's own fixtures use — a hand-rolled
// toy element tree would not exercise the namespace-prefix handling real Word
// output carries.
func wordDocument(body string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body>` + body + `</w:body>` +
		`</w:document>`
}

// buildDOCX returns an opc.Package carrying document.xml as its main part,
// and the part name to pass to anchor.Discover.
func buildDOCX(t *testing.T, body string) (*opc.Package, string) {
	t.Helper()
	p := opc.New()
	if err := p.Put(&opc.Part{
		Name:        "/word/document.xml",
		ContentType: ctMainDocument,
		Data:        []byte(wordDocument(body)),
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	return p, "/word/document.xml"
}

func discover(t *testing.T, body string) (*anchor.Inventory, error) {
	t.Helper()
	pkg, mainPart := buildDOCX(t, body)
	return anchor.Discover(pkg, artifact.FormatDOCX, mainPart)
}

func TestDiscover_NativeAnchorWithTagAndAlias(t *testing.T) {
	inv, err := discover(t, `<w:p>`+
		`<w:sdt>`+
		`<w:sdtPr><w:alias w:val="Client Name"/><w:tag w:val="client_name"/><w:id w:val="123"/></w:sdtPr>`+
		`<w:sdtContent><w:r><w:t>Acme &amp; Co.</w:t></w:r></w:sdtContent>`+
		`</w:sdt>`+
		`</w:p>`)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Anchors) != 1 {
		t.Fatalf("got %d anchors, want 1: %+v", len(inv.Anchors), inv.Anchors)
	}
	a := inv.Anchors[0]
	if a.Kind != anchor.KindNative {
		t.Errorf("kind = %q, want native", a.Kind)
	}
	if a.Name != "client_name" {
		t.Errorf("name = %q, want client_name", a.Name)
	}
	if a.Alias != "Client Name" {
		t.Errorf("alias = %q, want %q", a.Alias, "Client Name")
	}
	if a.Part != "/word/document.xml" {
		t.Errorf("part = %q", a.Part)
	}
	if a.Span.Start == 0 && a.Span.End == 0 {
		t.Error("span is zero-valued")
	}
}

func TestDiscover_NativeAnchorWithNoTagIsSkipped(t *testing.T) {
	// Word inserts an untagged w:sdt for things like the built-in
	// table-of-contents field. It carries no name to bind against.
	inv, err := discover(t, `<w:p>`+
		`<w:sdt>`+
		`<w:sdtPr><w:id w:val="123"/></w:sdtPr>`+
		`<w:sdtContent><w:r><w:t>Table of Contents</w:t></w:r></w:sdtContent>`+
		`</w:sdt>`+
		`</w:p>`)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Anchors) != 0 {
		t.Fatalf("got %d anchors, want 0: %+v", len(inv.Anchors), inv.Anchors)
	}
}

func TestDiscover_NestedNativeAnchorsAreBothDiscovered(t *testing.T) {
	inv, err := discover(t, `<w:p>`+
		`<w:sdt>`+
		`<w:sdtPr><w:tag w:val="outer"/></w:sdtPr>`+
		`<w:sdtContent>`+
		`<w:sdt>`+
		`<w:sdtPr><w:tag w:val="inner"/></w:sdtPr>`+
		`<w:sdtContent><w:r><w:t>nested</w:t></w:r></w:sdtContent>`+
		`</w:sdt>`+
		`</w:sdtContent>`+
		`</w:sdt>`+
		`</w:p>`)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Anchors) != 2 {
		t.Fatalf("got %d anchors, want 2: %+v", len(inv.Anchors), inv.Anchors)
	}
	names := map[string]bool{}
	for _, a := range inv.Anchors {
		names[a.Name] = true
		if a.Kind != anchor.KindNative {
			t.Errorf("anchor %q: kind = %q, want native", a.Name, a.Kind)
		}
	}
	if !names["outer"] || !names["inner"] {
		t.Errorf("names = %v, want outer and inner both present", names)
	}
}

func TestDiscover_TwoNativeAnchorsSharingATagIsRejected(t *testing.T) {
	_, err := discover(t, `<w:p><w:sdt><w:sdtPr><w:tag w:val="dup"/></w:sdtPr>`+
		`<w:sdtContent><w:r><w:t>one</w:t></w:r></w:sdtContent></w:sdt></w:p>`+
		`<w:p><w:sdt><w:sdtPr><w:tag w:val="dup"/></w:sdtPr>`+
		`<w:sdtContent><w:r><w:t>two</w:t></w:r></w:sdtContent></w:sdt></w:p>`)
	if !verr.HasCode(err, verr.VELLUM_ANCHOR_DUPLICATE) {
		t.Fatalf("err = %v, want VELLUM_ANCHOR_DUPLICATE", err)
	}
}

func TestDiscover_UnfragmentedMarker(t *testing.T) {
	inv, err := discover(t, `<w:p><w:r><w:t>Dear {{customer_name}}, thanks.</w:t></w:r></w:p>`)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Anchors) != 1 {
		t.Fatalf("got %d anchors, want 1: %+v", len(inv.Anchors), inv.Anchors)
	}
	a := inv.Anchors[0]
	if a.Kind != anchor.KindMarker {
		t.Errorf("kind = %q, want marker", a.Kind)
	}
	if a.Name != "customer_name" {
		t.Errorf("name = %q, want customer_name", a.Name)
	}
}

func TestDiscover_MarkerFragmentedAcrossRunsMidWord(t *testing.T) {
	// A spell-checker-style split: the marker's own braces and name are torn
	// across three separate w:r runs. Detection relies only on flattening
	// w:t content in document order, not on any run-level position map.
	inv, err := discover(t, `<w:p>`+
		`<w:r><w:t>Dear {{cust</w:t></w:r>`+
		`<w:r><w:t>omer_na</w:t></w:r>`+
		`<w:r><w:t>me}}, thanks.</w:t></w:r>`+
		`</w:p>`)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Anchors) != 1 {
		t.Fatalf("got %d anchors, want 1: %+v", len(inv.Anchors), inv.Anchors)
	}
	a := inv.Anchors[0]
	if a.Kind != anchor.KindMarker {
		t.Errorf("kind = %q, want marker", a.Kind)
	}
	if a.Name != "customer_name" {
		t.Errorf("name = %q, want customer_name", a.Name)
	}
}

func TestDiscover_UnterminatedMarkerIsRejected(t *testing.T) {
	_, err := discover(t, `<w:p><w:r><w:t>Dear {{customer_name, thanks.</w:t></w:r></w:p>`)
	if !verr.HasCode(err, verr.VELLUM_ANCHOR_MARKER_MALFORMED) {
		t.Fatalf("err = %v, want VELLUM_ANCHOR_MARKER_MALFORMED", err)
	}
}

func TestDiscover_EmptyMarkerIsRejected(t *testing.T) {
	_, err := discover(t, `<w:p><w:r><w:t>Dear {{}}, thanks.</w:t></w:r></w:p>`)
	if !verr.HasCode(err, verr.VELLUM_ANCHOR_MARKER_MALFORMED) {
		t.Fatalf("err = %v, want VELLUM_ANCHOR_MARKER_MALFORMED", err)
	}
}

func TestDiscover_WhitespaceOnlyMarkerIsRejected(t *testing.T) {
	_, err := discover(t, `<w:p><w:r><w:t>Dear {{   }}, thanks.</w:t></w:r></w:p>`)
	if !verr.HasCode(err, verr.VELLUM_ANCHOR_MARKER_MALFORMED) {
		t.Fatalf("err = %v, want VELLUM_ANCHOR_MARKER_MALFORMED", err)
	}
}

func TestDiscover_MarkerCollidingWithNativeTagIsRejected(t *testing.T) {
	_, err := discover(t,
		`<w:p><w:sdt><w:sdtPr><w:tag w:val="customer_name"/></w:sdtPr>`+
			`<w:sdtContent><w:r><w:t>Acme</w:t></w:r></w:sdtContent></w:sdt></w:p>`+
			`<w:p><w:r><w:t>Dear {{customer_name}},</w:t></w:r></w:p>`)
	if !verr.HasCode(err, verr.VELLUM_ANCHOR_DUPLICATE) {
		t.Fatalf("err = %v, want VELLUM_ANCHOR_DUPLICATE", err)
	}
}

func TestDiscover_TwoMarkersSharingANameAreRejected(t *testing.T) {
	_, err := discover(t,
		`<w:p><w:r><w:t>Dear {{customer_name}},</w:t></w:r></w:p>`+
			`<w:p><w:r><w:t>Again, {{customer_name}}.</w:t></w:r></w:p>`)
	if !verr.HasCode(err, verr.VELLUM_ANCHOR_DUPLICATE) {
		t.Fatalf("err = %v, want VELLUM_ANCHOR_DUPLICATE", err)
	}
}

func TestDiscover_ZeroAnchorsIsNotAnError(t *testing.T) {
	inv, err := discover(t, `<w:p><w:r><w:t>No anchors in this document.</w:t></w:r></w:p>`)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Anchors) != 0 {
		t.Fatalf("got %d anchors, want 0: %+v", len(inv.Anchors), inv.Anchors)
	}
}

func TestDiscover_AnchorsAreInDocumentOrder(t *testing.T) {
	inv, err := discover(t,
		`<w:p><w:r><w:t>{{first}}</w:t></w:r></w:p>`+
			`<w:p><w:sdt><w:sdtPr><w:tag w:val="second"/></w:sdtPr>`+
			`<w:sdtContent><w:r><w:t>x</w:t></w:r></w:sdtContent></w:sdt></w:p>`+
			`<w:p><w:r><w:t>{{third}}</w:t></w:r></w:p>`)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Anchors) != 3 {
		t.Fatalf("got %d anchors, want 3: %+v", len(inv.Anchors), inv.Anchors)
	}
	want := []string{"first", "second", "third"}
	for i, name := range want {
		if inv.Anchors[i].Name != name {
			t.Errorf("anchor %d: name = %q, want %q", i, inv.Anchors[i].Name, name)
		}
	}
}

// TestDiscover_UnsupportedFormatIsRejectedLoudly proves a format with no
// discoverer wired — PDF, which is not an OPC package to begin with — fails
// loudly rather than returning an empty inventory. DOCX (E9-S2), XLSX
// (E11-S1) and PPTX (E11-S2) all have one now; see
// template/anchor/xlsx_test.go and template/anchor/pptx_test.go for their
// own discovery coverage.
func TestDiscover_UnsupportedFormatIsRejectedLoudly(t *testing.T) {
	pkg, mainPart := buildDOCX(t, `<w:p><w:r><w:t>hello</w:t></w:r></w:p>`)
	_, err := anchor.Discover(pkg, artifact.FormatPDF, mainPart)
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_FORMAT_UNSUPPORTED) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_FORMAT_UNSUPPORTED", err)
	}
}
