package template_test

import (
	"bytes"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/template"
)

const (
	relOfficeDocument = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"

	ctMainDocument     = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	ctWorkbook         = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"
	ctPresentation     = "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
	ctSomethingElse    = "application/octet-stream"
	minimalDocumentXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body><w:p><w:r><w:t>hello</w:t></w:r></w:p></w:body>` +
		`</w:document>`
)

// buildPackage assembles a minimal one-part OPC package whose root
// officeDocument relationship (when addRel is true) points at a part
// carrying contentType, and returns it serialised.
func buildPackage(t *testing.T, partName, contentType string, addRel bool) []byte {
	t.Helper()
	p := opc.New()
	if err := p.Put(&opc.Part{
		Name:        partName,
		ContentType: contentType,
		Data:        []byte(minimalDocumentXML),
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if addRel {
		target := partName[1:] // relative to package root, no leading slash
		if _, err := p.Relationships("/").Add(relOfficeDocument, target, opc.TargetInternal); err != nil {
			t.Fatalf("Add rel: %v", err)
		}
	}

	var buf bytes.Buffer
	if err := p.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

func TestOpen_DOCXIsDetectedByRelationshipAndContentType(t *testing.T) {
	raw := buildPackage(t, "/word/document.xml", ctMainDocument, true)
	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if tpl.Format() != artifact.FormatDOCX {
		t.Errorf("format = %q, want docx", tpl.Format())
	}
	if tpl.MainPart() != "/word/document.xml" {
		t.Errorf("main part = %q", tpl.MainPart())
	}
}

func TestOpen_XLSXIsDetected(t *testing.T) {
	raw := buildPackage(t, "/xl/workbook.xml", ctWorkbook, true)
	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if tpl.Format() != artifact.FormatXLSX {
		t.Errorf("format = %q, want xlsx", tpl.Format())
	}
}

func TestOpen_PPTXIsDetected(t *testing.T) {
	raw := buildPackage(t, "/ppt/presentation.xml", ctPresentation, true)
	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if tpl.Format() != artifact.FormatPPTX {
		t.Errorf("format = %q, want pptx", tpl.Format())
	}
}

func TestOpen_NoOfficeDocumentRelationshipIsRejected(t *testing.T) {
	raw := buildPackage(t, "/word/document.xml", ctMainDocument, false)
	_, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_INVALID) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_INVALID", err)
	}
}

// TestOpen_UnrecognizedMainContentTypeIsRejected covers "PDF-shaped": a
// package that resolves an officeDocument relationship to a real part, whose
// declared content type names none of the three recognised OOXML main parts.
// A real PDF is not a zip at all and fails inside opc.Open with a ZIP-domain
// error before ever reaching this check; this is the case CLAUDE.md's
// "handing template.Open a PDF ... is the same VELLUM_TEMPLATE_INVALID-class
// rejection" describes for a package shaped like something else entirely.
func TestOpen_UnrecognizedMainContentTypeIsRejected(t *testing.T) {
	raw := buildPackage(t, "/word/document.xml", ctSomethingElse, true)
	_, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_INVALID) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_INVALID", err)
	}
}

func TestOpen_DanglingOfficeDocumentTargetIsRejected(t *testing.T) {
	p := opc.New()
	if err := p.Put(&opc.Part{
		Name:        "/word/document.xml",
		ContentType: ctMainDocument,
		Data:        []byte(minimalDocumentXML),
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Points at a part the package never carries.
	if _, err := p.Relationships("/").Add(relOfficeDocument, "word/missing.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add rel: %v", err)
	}
	var buf bytes.Buffer
	if err := p.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	_, err := template.Open(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_INVALID) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_INVALID", err)
	}
}

func TestOpen_NotAZipIsRejected(t *testing.T) {
	raw := []byte("%PDF-1.7\n...not a zip...")
	_, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err == nil {
		t.Fatal("Open succeeded on non-zip bytes, want an error")
	}
}

func TestInspect_DOCXDiscoversAnchors(t *testing.T) {
	raw := buildPackage(t, "/word/document.xml", ctMainDocument, true)
	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	inv, err := template.Inspect(tpl)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(inv.Anchors) != 0 {
		t.Errorf("got %d anchors in an anchor-free fixture, want 0", len(inv.Anchors))
	}
}

func TestInspect_XLSXIsRejectedNotEmpty(t *testing.T) {
	raw := buildPackage(t, "/xl/workbook.xml", ctWorkbook, true)
	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, err = template.Inspect(tpl)
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_FORMAT_UNSUPPORTED) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_FORMAT_UNSUPPORTED", err)
	}
}

func TestInspect_PPTXIsRejectedNotEmpty(t *testing.T) {
	raw := buildPackage(t, "/ppt/presentation.xml", ctPresentation, true)
	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, err = template.Inspect(tpl)
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_FORMAT_UNSUPPORTED) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_FORMAT_UNSUPPORTED", err)
	}
}

func TestOpen_MaxPartBytesIsEnforced(t *testing.T) {
	raw := buildPackage(t, "/word/document.xml", ctMainDocument, true)
	_, err := template.Open(bytes.NewReader(raw), int64(len(raw)), template.WithMaxPartBytes(4))
	if err == nil {
		t.Fatal("Open succeeded despite a part exceeding MaxPartBytes")
	}
}
