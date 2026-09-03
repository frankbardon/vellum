package bind_test

// Shared fixture-building and assertion helpers for exec_test.go and
// repeat_test.go: a minimal WordprocessingML package, anchor discovery
// wired into a bind.Frame, and byte-level helpers for reading back a
// splice's own result without re-deriving offsets by hand.

import (
	"strings"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/template/bind"
	"github.com/frankbardon/vellum/xmlcopy"
)

const (
	nsWord         = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	ctMainDocument = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	execMainPart   = "/word/document.xml"
	execSecondPart = "/word/styles.xml"
	execSecondXML  = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" + `<w:styles xmlns:w="` + nsWord + `"/>`
	execSecondCT   = "application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"
)

// wordDocument wraps body in a realistic WordprocessingML root element and
// declaration, mirroring template/anchor's own fixture builder.
func wordDocument(body string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<w:document xmlns:w="` + nsWord + `">` +
		`<w:body>` + body + `</w:body>` +
		`</w:document>`
}

// buildExecPkg returns a package carrying document.xml (built from body) plus
// one unrelated second part, so a test can assert the second part is never
// touched by anything exec.go or repeat.go does.
func buildExecPkg(t *testing.T, body string) *opc.Package {
	t.Helper()
	pkg := opc.New()
	if err := pkg.Put(&opc.Part{
		Name:        execMainPart,
		ContentType: ctMainDocument,
		Data:        []byte(wordDocument(body)),
	}); err != nil {
		t.Fatalf("Put document.xml: %v", err)
	}
	if err := pkg.Put(&opc.Part{
		Name:        execSecondPart,
		ContentType: execSecondCT,
		Data:        []byte(execSecondXML),
	}); err != nil {
		t.Fatalf("Put styles.xml: %v", err)
	}
	return pkg
}

// discoverFrame runs anchor.Discover against pkg's main part and wraps the
// result as the bind.Frame Execute needs at the top level.
func discoverFrame(t *testing.T, pkg *opc.Package) bind.Frame {
	t.Helper()
	inv, err := anchor.Discover(pkg, artifact.FormatDOCX, execMainPart)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	anchors := make(map[string]anchor.Anchor, len(inv.Anchors))
	for _, a := range inv.Anchors {
		anchors[a.Name] = a
	}
	return bind.Frame{SrcPkg: pkg, Anchors: anchors}
}

// applyToPart reads part's own source bytes from pkg and applies every
// replacement repls accumulated against it, returning the result.
func applyToPart(t *testing.T, pkg *opc.Package, part string, repls *bind.ReplacementSet) []byte {
	t.Helper()
	p, ok := pkg.Get(part)
	if !ok {
		t.Fatalf("part %q missing from package", part)
	}
	src, err := p.Bytes()
	if err != nil {
		t.Fatalf("Bytes(%q): %v", part, err)
	}
	out, err := xmlcopy.Apply(src, repls.For(part))
	if err != nil {
		t.Fatalf("Apply(%q): %v", part, err)
	}
	return out
}

// runExec discovers pkg's anchors, executes stmts against data with a fresh
// FEEL evaluator, applies the result against document.xml and returns the
// filled bytes alongside the frame Execute ran against (for tests that also
// want to inspect the ReplacementSet or the discovered anchors directly).
func runExec(t *testing.T, pkg *opc.Package, stmts []bind.Statement, data bind.Scope) []byte {
	t.Helper()
	frame := discoverFrame(t, pkg)
	repls := bind.NewReplacementSet()
	ev := bind.NewFEELEvaluator()
	if err := bind.Execute(stmts, data, ev, frame, pkg, repls); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return applyToPart(t, pkg, execMainPart, repls)
}

// collectElements walks doc once and returns every element named local in
// the WordprocessingML namespace, in document order.
func collectElements(t *testing.T, doc []byte, local string) []xmlcopy.Element {
	t.Helper()
	var out []xmlcopy.Element
	if err := xmlcopy.Walk(doc, func(e xmlcopy.Element) error {
		if e.Name.Space == nsWord && e.Name.Local == local {
			out = append(out, e)
		}
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	return out
}

// rowTexts returns, for every <w:tr> in doc, the concatenation of every
// <w:t> nested inside it, in document order — the whole row's text, cell
// boundaries not distinguished, which is enough to check what a repeated
// row's cells actually read without re-deriving byte offsets by hand.
func rowTexts(t *testing.T, doc []byte) []string {
	t.Helper()
	trs := collectElements(t, doc, "tr")
	texts := collectElements(t, doc, "t")
	out := make([]string, len(trs))
	for i, tr := range trs {
		var b strings.Builder
		for _, tx := range texts {
			if tx.Content.Start >= tr.Content.Start && tx.Content.End <= tr.Content.End {
				b.Write(doc[tx.Content.Start:tx.Content.End])
			}
		}
		out[i] = b.String()
	}
	return out
}

// buildRowTable returns a <w:tbl> with a header row and one templated data
// row carrying three markers, one per cell.
func buildRowTable() string {
	header := `<w:tr><w:tc><w:p><w:r><w:t>Name</w:t></w:r></w:p></w:tc>` +
		`<w:tc><w:p><w:r><w:t>Qty</w:t></w:r></w:p></w:tc>` +
		`<w:tc><w:p><w:r><w:t>Status</w:t></w:r></w:p></w:tc></w:tr>`
	templated := `<w:tr><w:tc><w:p><w:r><w:t>{{item_name}}</w:t></w:r></w:p></w:tc>` +
		`<w:tc><w:p><w:r><w:t>{{item_qty}}</w:t></w:r></w:p></w:tc>` +
		`<w:tc><w:p><w:r><w:t>{{item_status}}</w:t></w:r></w:p></w:tc></w:tr>`
	return `<w:tbl><w:tblPr/><w:tblGrid><w:gridCol/><w:gridCol/><w:gridCol/></w:tblGrid>` + header + templated + `</w:tbl>`
}
