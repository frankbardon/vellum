package template_test

// TestFill_PPTX* exercise E11-S2's pptx fill mode end to end through the
// same public surface template/fill_test.go already exercises for DOCX and
// template/fill_xlsx_test.go for xlsx: template.Open then template.Fill,
// with the package a real caller writes re-opened through opc.Open and
// every part outside Result.Touched checked byte-for-byte against the
// source. For pptx specifically that check matters in a way it does not for
// the other two formats: a slide-clone repeat adds whole new OPC parts
// directly (never through Result.Touched's own xmlcopy.Apply path — see
// execSlideRepeat's own doc comment), so this file also confirms the
// original, un-repeated slide the repeat targeted survives byte-identical,
// proving the non-destructiveness guarantee holds even though this format's
// own repeat mechanism works by adding parts rather than by editing one.

import (
	"bytes"
	"testing"

	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/template"
	"github.com/frankbardon/vellum/template/bind"
)

const (
	pptxRelOfficeDocument = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	pptxRelSlide          = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide"
	pptxNSPresentation    = "http://schemas.openxmlformats.org/presentationml/2006/main"
	pptxNSDrawingMain     = "http://schemas.openxmlformats.org/drawingml/2006/main"
	pptxNSRelationships   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

	pptxCTPresentation = "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
	pptxCTSlide        = "application/vnd.openxmlformats-officedocument.presentationml.slide+xml"
	pptxCTPresProps    = "application/vnd.openxmlformats-officedocument.presentationml.presProps+xml"

	pptxPresentationPart = "/ppt/presentation.xml"
	pptxSlide1Part       = "/ppt/slides/slide1.xml"
	pptxSlide2Part       = "/ppt/slides/slide2.xml"
	pptxPresPropsPart    = "/ppt/presProps.xml"
)

func xmlDeclPPTXFill() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"
}

// pptxSlideXMLFill builds one slide part carrying a single shape anchor
// named shapeName.
func pptxSlideXMLFill(shapeName string) []byte {
	return []byte(xmlDeclPPTXFill() +
		`<p:sld xmlns:a="` + pptxNSDrawingMain + `" xmlns:r="` + pptxNSRelationships +
		`" xmlns:p="` + pptxNSPresentation + `">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="` + shapeName + `"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>placeholder</a:t></a:r></a:p></p:txBody></p:sp>` +
		`</p:spTree></p:cSld>` +
		`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
		`</p:sld>`)
}

const pptxPresPropsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<p:presentationPr xmlns:p="` + pptxNSPresentation + `"/>`

// buildFillFixturePPTX builds a two-slide .pptx-shaped package: slide1
// carries a plain shape anchor ("customer_name") and slide2 carries a shape
// anchor ("item_name") that a RepeatTargetSlide statement clones. presProps
// is an unrelated part Fill has no reason to touch, mirroring
// fillStylesXML's/xlsxStylesXML's own role in the DOCX and xlsx fixtures.
func buildFillFixturePPTX(t *testing.T) []byte {
	t.Helper()
	pkg := opc.New()

	slide1RID, err := pkg.Relationships(pptxPresentationPart).Add(pptxRelSlide, "slides/slide1.xml", opc.TargetInternal)
	if err != nil {
		t.Fatalf("Add slide1 rel: %v", err)
	}
	slide2RID, err := pkg.Relationships(pptxPresentationPart).Add(pptxRelSlide, "slides/slide2.xml", opc.TargetInternal)
	if err != nil {
		t.Fatalf("Add slide2 rel: %v", err)
	}

	pres := xmlDeclPPTXFill() +
		`<p:presentation xmlns:a="` + pptxNSDrawingMain + `" xmlns:r="` + pptxNSRelationships +
		`" xmlns:p="` + pptxNSPresentation + `">` +
		`<p:sldIdLst>` +
		`<p:sldId id="256" r:id="` + slide1RID + `"/>` +
		`<p:sldId id="257" r:id="` + slide2RID + `"/>` +
		`</p:sldIdLst>` +
		`</p:presentation>`
	if err := pkg.Put(&opc.Part{Name: pptxPresentationPart, ContentType: pptxCTPresentation, Data: []byte(pres)}); err != nil {
		t.Fatalf("Put presentation.xml: %v", err)
	}
	if _, err := pkg.Relationships("/").Add(pptxRelOfficeDocument, "ppt/presentation.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add officeDocument rel: %v", err)
	}

	if err := pkg.Put(&opc.Part{Name: pptxSlide1Part, ContentType: pptxCTSlide, Data: pptxSlideXMLFill("customer_name")}); err != nil {
		t.Fatalf("Put slide1.xml: %v", err)
	}
	if err := pkg.Put(&opc.Part{Name: pptxSlide2Part, ContentType: pptxCTSlide, Data: pptxSlideXMLFill("item_name")}); err != nil {
		t.Fatalf("Put slide2.xml: %v", err)
	}
	if err := pkg.Put(&opc.Part{Name: pptxPresPropsPart, ContentType: pptxCTPresProps, Data: []byte(pptxPresPropsXML)}); err != nil {
		t.Fatalf("Put presProps.xml: %v", err)
	}

	var buf bytes.Buffer
	if err := pkg.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

func fillFixtureBindingPPTX() *bind.Binding {
	return &bind.Binding{
		FormatVersion: bind.FormatVersion,
		Statements: []bind.Statement{
			{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "customer_name", Expr: "customer.name"}},
			{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
				Over: "items", As: "item", Target: bind.RepeatTargetSlide,
				Body: []bind.Statement{
					{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_name", Expr: "item.name"}},
				},
			}},
		},
	}
}

func fillFixtureDataPPTX() bind.Scope {
	return bind.Scope{
		"customer": map[string]any{"name": "Acme & Co."},
		"items": []any{
			map[string]any{"name": "Widget"},
			map[string]any{"name": "Gadget"},
			map[string]any{"name": "Sprocket"},
		},
	}
}

func TestFill_PPTXEndToEndPlainBindAndSlideRepeat(t *testing.T) {
	raw := buildFillFixturePPTX(t)
	srcPkg, err := opc.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("opc.Open on the fixture: %v", err)
	}
	srcPresProps := mustPartBytes(t, srcPkg, pptxPresPropsPart)
	srcSlide2 := mustPartBytes(t, srcPkg, pptxSlide2Part)

	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	res, err := template.Fill(tpl, fillFixtureBindingPPTX(), fillFixtureDataPPTX())
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}

	// --- Touched receipt: only the parts actually spliced by xmlcopy.Apply -
	// slide1 (the plain bind) and presentation.xml (the repeat's own
	// sldIdLst replacement) — never the brand-new cloned slide parts, which
	// land directly in the output package rather than through the
	// Touched/xmlcopy.Apply path. See this file's own doc comment.
	wantTouched := map[string]bool{pptxSlide1Part: true, pptxPresentationPart: true}
	if len(res.Touched) != len(wantTouched) {
		t.Fatalf("Touched = %v, want exactly %v", res.Touched, wantTouched)
	}
	for _, name := range res.Touched {
		if !wantTouched[name] {
			t.Errorf("unexpected touched part %q", name)
		}
	}

	// --- slide1 carries the filled content ---------------------------------
	slide1 := mustPartBytes(t, res.Package, pptxSlide1Part)
	if !bytes.Contains(slide1, []byte("Acme &amp; Co.")) {
		t.Errorf("slide1 missing filled customer name: %s", slide1)
	}
	if bytes.Contains(slide1, []byte("placeholder")) {
		t.Errorf("slide1 still carries raw placeholder text: %s", slide1)
	}

	// --- Three new slide parts, each carrying one item's own name ----------
	names := res.Package.Names()
	var newSlideParts []string
	for _, n := range names {
		if n != pptxSlide1Part && n != pptxSlide2Part && n != pptxPresentationPart && n != pptxPresPropsPart &&
			!bytesHasSuffixDotRels(n) {
			newSlideParts = append(newSlideParts, n)
		}
	}
	if len(newSlideParts) != 3 {
		t.Fatalf("got %d new slide parts, want 3: %v", len(newSlideParts), newSlideParts)
	}
	wantItems := map[string]bool{"Widget": true, "Gadget": true, "Sprocket": true}
	found := map[string]bool{}
	for _, part := range newSlideParts {
		data := mustPartBytes(t, res.Package, part)
		for item := range wantItems {
			if bytes.Contains(data, []byte(item)) {
				found[item] = true
			}
		}
		if bytes.Contains(data, []byte("placeholder")) {
			t.Errorf("clone %q still carries raw placeholder text: %s", part, data)
		}
	}
	for item := range wantItems {
		if !found[item] {
			t.Errorf("no cloned slide carried item %q", item)
		}
	}

	// --- presentation.xml's own sldIdLst reflects the repeat -------------
	pres := mustPartBytes(t, res.Package, pptxPresentationPart)
	if bytes.Count(pres, []byte("<p:sldId ")) != 1+3 { // slide1 + 3 clones of slide2
		t.Errorf("presentation.xml carries the wrong number of <p:sldId> entries: %s", pres)
	}

	// --- Non-destructiveness -------------------------------------------
	filledPresProps := mustPartBytes(t, res.Package, pptxPresPropsPart)
	if !bytes.Equal(srcPresProps, filledPresProps) {
		t.Errorf("presProps.xml changed even though it was never in Touched:\nsource: %s\nfilled: %s", srcPresProps, filledPresProps)
	}
	// The original, un-repeated slide2 template part is left in place,
	// byte-identical to the source — execSlideRepeat's own doc comment
	// explains why it is not deleted.
	filledSlide2 := mustPartBytes(t, res.Package, pptxSlide2Part)
	if !bytes.Equal(srcSlide2, filledSlide2) {
		t.Errorf("the original slide2 template part changed even though the repeat only ever reads it, never writes it:\nsource: %s\nfilled: %s", srcSlide2, filledSlide2)
	}

	// --- The result round-trips through opc.Open ----------------------------
	var out bytes.Buffer
	if err := res.Package.WriteTo(&out, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	reopened, err := opc.Open(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatalf("the filled package does not round-trip through opc.Open: %v", err)
	}
	// Every relationship of type "slide" from presentation.xml resolves to
	// a real, present part.
	rels, ok := reopened.RelationshipsFor(pptxPresentationPart)
	if !ok {
		t.Fatal("reopened package carries no presentation.xml relationships")
	}
	for _, rel := range rels.ByType(pptxRelSlide) {
		target := "/ppt/" + rel.Target
		if !reopened.Has(target) {
			t.Errorf("relationship target %q does not resolve to a part in the package", target)
		}
	}

	// --- The originally opened Template is untouched -----------------------
	origSlide1 := mustPartBytes(t, tpl.Package(), pptxSlide1Part)
	if !bytes.Contains(origSlide1, []byte("placeholder")) {
		t.Error("Fill mutated the *Template it was called against; the original placeholder text should still be there")
	}
}

// bytesHasSuffixDotRels reports whether name addresses a relationships part
// — a cheap local check (this file's fixtures never nest ".rels" inside a
// deeper directory in a way this would mis-detect) so the new-part scan
// above does not miscount a clone's own copied _rels/*.rels part as a
// fourth "new slide".
func bytesHasSuffixDotRels(name string) bool {
	return len(name) > 5 && name[len(name)-5:] == ".rels"
}
