package template_test

// TestFill_* exercise E10-S3's public orchestration entry point,
// template.Fill, end to end: a template carrying a genuine mix of plain
// binds, an if and a row repeat, filled through the same surface a real
// caller uses (template.Open then template.Fill), with the package the
// caller actually writes re-opened through opc.Open and every part outside
// Result.Touched checked byte-for-byte against the source — the same
// non-destructiveness property TestNonDestructiveCorpus already proves at
// the lower splice/xmlcopy layer, proved again here at the surface a real
// caller uses.

import (
	"bytes"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/template"
	"github.com/frankbardon/vellum/template/bind"
)

const (
	fillRelOfficeDocument = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	fillCTMainDocument    = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	fillCTStyles          = "application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"
	fillNSWord            = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
)

// fillFixtureBody is a mix deliberately shaped like a real template: an
// untouched opening paragraph, a marker bound directly, an if-driven
// marker, a table repeated by row, and an untouched closing paragraph.
const fillFixtureBody = `<w:p><w:r><w:t>Statement of account</w:t></w:r></w:p>` +
	`<w:p><w:r><w:t>Dear {{customer_name}},</w:t></w:r></w:p>` +
	`<w:p><w:r><w:t>{{vip_note}}</w:t></w:r></w:p>` +
	`<w:tbl><w:tblPr/><w:tblGrid><w:gridCol/><w:gridCol/></w:tblGrid>` +
	`<w:tr><w:tc><w:p><w:r><w:t>Item</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Qty</w:t></w:r></w:p></w:tc></w:tr>` +
	`<w:tr><w:tc><w:p><w:r><w:t>{{item_name}}</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>{{item_qty}}</w:t></w:r></w:p></w:tc></w:tr>` +
	`</w:tbl>` +
	`<w:p><w:r><w:t>Thank you for your business.</w:t></w:r></w:p>`

func fillDocumentXML() []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<w:document xmlns:w="` + fillNSWord + `"><w:body>` + fillFixtureBody + `</w:body></w:document>`)
}

const fillStylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<w:styles xmlns:w="` + fillNSWord + `"/>`

// buildFillFixture returns a two-part .docx-shaped package: document.xml
// (the mix described above) and an unrelated styles.xml, so
// non-destructiveness can be checked against a part Fill has no reason to
// touch.
func buildFillFixture(t *testing.T) []byte {
	t.Helper()
	pkg := opc.New()
	if err := pkg.Put(&opc.Part{Name: "/word/document.xml", ContentType: fillCTMainDocument, Data: fillDocumentXML()}); err != nil {
		t.Fatalf("Put document.xml: %v", err)
	}
	if err := pkg.Put(&opc.Part{Name: "/word/styles.xml", ContentType: fillCTStyles, Data: []byte(fillStylesXML)}); err != nil {
		t.Fatalf("Put styles.xml: %v", err)
	}
	if _, err := pkg.Relationships("/").Add(fillRelOfficeDocument, "word/document.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add officeDocument relationship: %v", err)
	}

	var buf bytes.Buffer
	if err := pkg.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

func fillFixtureBinding() *bind.Binding {
	return &bind.Binding{
		FormatVersion: bind.FormatVersion,
		Statements: []bind.Statement{
			{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "customer_name", Expr: "customer.name"}},
			{Kind: bind.StatementIf, If: &bind.If{
				When: "customer.vip",
				Then: []bind.Statement{{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "vip_note", Expr: `"VIP customer — priority handling."`}}},
				Else: []bind.Statement{{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "vip_note", Expr: `""`}}},
			}},
			{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
				Over: "items", As: "item", Target: bind.RepeatTargetRow,
				Body: []bind.Statement{
					{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_name", Expr: "item.name"}},
					{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_qty", Expr: "item.qty"}},
				},
			}},
		},
	}
}

func fillFixtureData() bind.Scope {
	return bind.Scope{
		"customer": map[string]any{"name": "Acme & Co.", "vip": true},
		"items": []any{
			map[string]any{"name": "Widget", "qty": 3.0},
			map[string]any{"name": "Gadget", "qty": 5.0},
		},
	}
}

func TestFill_EndToEndMixOfBindIfAndRepeat(t *testing.T) {
	raw := buildFillFixture(t)
	srcPkg, err := opc.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("opc.Open on the fixture: %v", err)
	}
	srcStyles := mustPartBytes(t, srcPkg, "/word/styles.xml")

	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	res, err := template.Fill(tpl, fillFixtureBinding(), fillFixtureData())
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}

	// --- Touched receipt --------------------------------------------------
	if len(res.Touched) != 1 || res.Touched[0] != "/word/document.xml" {
		t.Fatalf("Touched = %v, want exactly [/word/document.xml]", res.Touched)
	}

	// --- document.xml carries the filled content ---------------------------
	doc := mustPartBytes(t, res.Package, "/word/document.xml")
	for _, want := range []string{"Acme &amp; Co.", "VIP customer", "Widget", "3", "Gadget", "5"} {
		if !bytes.Contains(doc, []byte(want)) {
			t.Errorf("filled document missing %q: %s", want, doc)
		}
	}
	for _, stale := range []string{"{{customer_name}}", "{{vip_note}}", "{{item_name}}", "{{item_qty}}"} {
		if bytes.Contains(doc, []byte(stale)) {
			t.Errorf("filled document still carries raw marker %q: %s", stale, doc)
		}
	}
	// Untouched surrounding content survives.
	for _, want := range []string{"Statement of account", "Thank you for your business."} {
		if !bytes.Contains(doc, []byte(want)) {
			t.Errorf("filled document lost untouched content %q: %s", want, doc)
		}
	}

	// --- Non-destructiveness: styles.xml is byte-identical -----------------
	filledStyles := mustPartBytes(t, res.Package, "/word/styles.xml")
	if !bytes.Equal(srcStyles, filledStyles) {
		t.Errorf("styles.xml changed even though it was never in Touched:\nsource: %s\nfilled: %s", srcStyles, filledStyles)
	}

	// --- The result round-trips through opc.Open ----------------------------
	var out bytes.Buffer
	if err := res.Package.WriteTo(&out, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if _, err := opc.Open(bytes.NewReader(out.Bytes()), int64(out.Len())); err != nil {
		t.Fatalf("the filled package does not round-trip through opc.Open: %v", err)
	}

	// --- The originally opened Template is untouched ------------------------
	origDoc := mustPartBytes(t, tpl.Package(), "/word/document.xml")
	if !bytes.Contains(origDoc, []byte("{{customer_name}}")) {
		t.Error("Fill mutated the *Template it was called against; the original marker text should still be there")
	}
}

func TestFill_SameTemplateFilledTwiceWithDifferentDataDoesNotInterfere(t *testing.T) {
	raw := buildFillFixture(t)
	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	data1 := bind.Scope{
		"customer": map[string]any{"name": "Acme & Co.", "vip": false},
		"items":    []any{map[string]any{"name": "Widget", "qty": 1.0}},
	}
	data2 := bind.Scope{
		"customer": map[string]any{"name": "Globex Corp.", "vip": true},
		"items":    []any{map[string]any{"name": "Gizmo", "qty": 2.0}},
	}

	res1, err := template.Fill(tpl, fillFixtureBinding(), data1)
	if err != nil {
		t.Fatalf("Fill #1: %v", err)
	}
	res2, err := template.Fill(tpl, fillFixtureBinding(), data2)
	if err != nil {
		t.Fatalf("Fill #2: %v", err)
	}

	if res1.Package == res2.Package {
		t.Fatal("both fills returned the same *opc.Package; Clone isolation is broken")
	}

	doc1 := mustPartBytes(t, res1.Package, "/word/document.xml")
	doc2 := mustPartBytes(t, res2.Package, "/word/document.xml")

	if !bytes.Contains(doc1, []byte("Acme")) || bytes.Contains(doc1, []byte("Globex")) {
		t.Errorf("fill #1's document does not reflect data1 in isolation: %s", doc1)
	}
	if !bytes.Contains(doc2, []byte("Globex")) || bytes.Contains(doc2, []byte("Acme")) {
		t.Errorf("fill #2's document does not reflect data2 in isolation: %s", doc2)
	}
	if !bytes.Contains(doc1, []byte("Widget")) || bytes.Contains(doc1, []byte("Gizmo")) {
		t.Errorf("fill #1's repeated row does not reflect data1's own items: %s", doc1)
	}
	if !bytes.Contains(doc2, []byte("Gizmo")) || bytes.Contains(doc2, []byte("Widget")) {
		t.Errorf("fill #2's repeated row does not reflect data2's own items: %s", doc2)
	}

	// The originally opened template is untouched by either fill.
	origDoc := mustPartBytes(t, tpl.Package(), "/word/document.xml")
	if !bytes.Contains(origDoc, []byte("{{customer_name}}")) {
		t.Error("a fill mutated the *Template it was called against")
	}
}

func TestFill_UnknownAnchorIsCodedError(t *testing.T) {
	raw := buildFillFixture(t)
	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	b := &bind.Binding{FormatVersion: bind.FormatVersion, Statements: []bind.Statement{
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "does_not_exist", Expr: `"x"`}},
	}}
	_, err = template.Fill(tpl, b, nil)
	if !verr.HasCode(err, verr.VELLUM_BIND_ANCHOR_UNKNOWN) {
		t.Fatalf("err = %v, want VELLUM_BIND_ANCHOR_UNKNOWN", err)
	}
}

func mustPartBytes(t *testing.T, pkg *opc.Package, name string) []byte {
	t.Helper()
	p, ok := pkg.Get(name)
	if !ok {
		t.Fatalf("package is missing part %q", name)
	}
	b, err := p.Bytes()
	if err != nil {
		t.Fatalf("Bytes(%q): %v", name, err)
	}
	return b
}
