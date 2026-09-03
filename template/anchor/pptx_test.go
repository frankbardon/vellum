package anchor_test

// Real-pptx-shaped fixtures for E11-S2's discoverPPTX: a genuine
// ppt/presentation.xml plus ppt/slides/slideN.xml, wired together through
// actual [opc.Package] relationships rather than a hardcoded
// "ppt/slides/*.xml" glob — the same resolve-through-the-relationship-graph
// discipline anchor/xlsx_test.go already exercises for discoverXLSX.

import (
	"testing"

	"github.com/frankbardon/vellum/artifact"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/template/anchor"
)

const (
	nsPresentationT   = "http://schemas.openxmlformats.org/presentationml/2006/main"
	nsDrawingMainT    = "http://schemas.openxmlformats.org/drawingml/2006/main"
	nsRelationshipsT2 = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	relSlideT         = nsRelationshipsT2 + "/slide"
	relOfficeDocT     = nsRelationshipsT2 + "/officeDocument"

	ctPresentationT = "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
	ctSlideT        = "application/vnd.openxmlformats-officedocument.presentationml.slide+xml"
)

func xmlDeclP() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"
}

// presentationXMLFixture builds a minimal ppt/presentation.xml carrying one
// <p:sldId> entry per rID given, in order.
func presentationXMLFixture(rIDs ...string) string {
	var sldIDs string
	for i, rid := range rIDs {
		sldIDs += `<p:sldId id="` + itoaT(256+i) + `" r:id="` + rid + `"/>`
	}
	return xmlDeclP() +
		`<p:presentation xmlns:a="` + nsDrawingMainT + `" xmlns:r="` + nsRelationshipsT2 +
		`" xmlns:p="` + nsPresentationT + `">` +
		`<p:sldIdLst>` + sldIDs + `</p:sldIdLst>` +
		`</p:presentation>`
}

// slideXMLFixture builds one slide part carrying shapesXML verbatim inside
// its own <p:spTree>.
func slideXMLFixture(shapesXML string) string {
	return xmlDeclP() +
		`<p:sld xmlns:a="` + nsDrawingMainT + `" xmlns:r="` + nsRelationshipsT2 +
		`" xmlns:p="` + nsPresentationT + `">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/>` +
		shapesXML +
		`</p:spTree></p:cSld>` +
		`</p:sld>`
}

// shapeXML builds one <p:sp> text-frame shape with the given name/descr and
// text content, mirroring what a real deck's placeholder or textbox shape
// looks like.
func shapeXML(id int, name, descr, text string) string {
	descrAttr := ""
	if descr != "" {
		descrAttr = ` descr="` + descr + `"`
	}
	return `<p:sp><p:nvSpPr><p:cNvPr id="` + itoaT(id) + `" name="` + name + `"` + descrAttr + `/>` +
		`<p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr/>` +
		`<p:txBody><a:bodyPr/><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></p:txBody>` +
		`</p:sp>`
}

// pptxFixture assembles a minimal but realistic .pptx-shaped package: one
// presentation part wired to N slide parts through real relationships.
type pptxFixture struct {
	t              *testing.T
	pkg            *opc.Package
	presentation   string
	slideParts     []string
	slideShapesXML []string
}

func newPPTXFixture(t *testing.T) *pptxFixture {
	t.Helper()
	return &pptxFixture{t: t, pkg: opc.New(), presentation: "/ppt/presentation.xml"}
}

// addSlide adds one slide carrying shapesXML and returns its own part name.
func (f *pptxFixture) addSlide(shapesXML string) string {
	f.t.Helper()
	idx := len(f.slideParts) + 1
	name := "/ppt/slides/slide" + itoaT(idx) + ".xml"
	f.slideParts = append(f.slideParts, name)
	f.slideShapesXML = append(f.slideShapesXML, shapesXML)
	return name
}

// build wires every added slide into the presentation part's own
// <p:sldIdLst>, in the order they were added, and returns the package plus
// the presentation part's own name.
func (f *pptxFixture) build() (*opc.Package, string) {
	f.t.Helper()

	var rIDs []string
	for i, part := range f.slideParts {
		rid, err := f.pkg.Relationships(f.presentation).Add(relSlideT, "slides/slide"+itoaT(i+1)+".xml", opc.TargetInternal)
		if err != nil {
			f.t.Fatalf("Add slide relationship: %v", err)
		}
		rIDs = append(rIDs, rid)
		if err := f.pkg.Put(&opc.Part{Name: part, ContentType: ctSlideT, Data: []byte(slideXMLFixture(f.slideShapesXML[i]))}); err != nil {
			f.t.Fatalf("Put %s: %v", part, err)
		}
	}

	if err := f.pkg.Put(&opc.Part{Name: f.presentation, ContentType: ctPresentationT, Data: []byte(presentationXMLFixture(rIDs...))}); err != nil {
		f.t.Fatalf("Put presentation.xml: %v", err)
	}
	if _, err := f.pkg.Relationships("/").Add(relOfficeDocT, "ppt/presentation.xml", opc.TargetInternal); err != nil {
		f.t.Fatalf("Add officeDocument relationship: %v", err)
	}

	return f.pkg, f.presentation
}

func TestDiscoverPPTX_ShapeNameIsTheBindingKey(t *testing.T) {
	f := newPPTXFixture(t)
	f.addSlide(shapeXML(2, "customer_name", "", "placeholder"))
	pkg, mainPart := f.build()

	inv, err := anchor.Discover(pkg, artifact.FormatPPTX, mainPart)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Anchors) != 1 {
		t.Fatalf("got %d anchors, want 1: %+v", len(inv.Anchors), inv.Anchors)
	}
	a := inv.Anchors[0]
	if a.Name != "customer_name" || a.Kind != anchor.KindShape || a.Alias != "" {
		t.Errorf("anchor = %+v, want Name=customer_name Kind=shape Alias=\"\"", a)
	}
	if a.Part != "/ppt/slides/slide1.xml" {
		t.Errorf("Part = %q, want /ppt/slides/slide1.xml", a.Part)
	}
}

func TestDiscoverPPTX_AltTextIsCarriedAsAliasNotAsAFallbackName(t *testing.T) {
	f := newPPTXFixture(t)
	f.addSlide(shapeXML(2, "headline", "A short headline describing the quarter", "placeholder"))
	pkg, mainPart := f.build()

	inv, err := anchor.Discover(pkg, artifact.FormatPPTX, mainPart)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Anchors) != 1 {
		t.Fatalf("got %d anchors, want 1", len(inv.Anchors))
	}
	a := inv.Anchors[0]
	if a.Name != "headline" {
		t.Errorf("Name = %q, want %q (never the descr)", a.Name, "headline")
	}
	if a.Alias != "A short headline describing the quarter" {
		t.Errorf("Alias = %q, want the shape's own descr", a.Alias)
	}
}

func TestDiscoverPPTX_EmptyShapeNameIsNotDiscovered(t *testing.T) {
	f := newPPTXFixture(t)
	f.addSlide(shapeXML(2, "", "some alt text", "placeholder") + shapeXML(3, "real_anchor", "", "placeholder"))
	pkg, mainPart := f.build()

	inv, err := anchor.Discover(pkg, artifact.FormatPPTX, mainPart)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Anchors) != 1 {
		t.Fatalf("got %d anchors, want 1 (the empty-named shape must not surface even via its alt text): %+v", len(inv.Anchors), inv.Anchors)
	}
	if inv.Anchors[0].Name != "real_anchor" {
		t.Errorf("Name = %q, want real_anchor", inv.Anchors[0].Name)
	}
}

func TestDiscoverPPTX_ShapeNestedInAGroupIsNotDiscovered(t *testing.T) {
	f := newPPTXFixture(t)
	grouped := `<p:grpSp><p:nvGrpSpPr><p:cNvPr id="5" name="Group 5"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/>` + shapeXML(6, "nested_shape", "", "placeholder") + `</p:grpSp>`
	f.addSlide(grouped + shapeXML(2, "top_level", "", "placeholder"))
	pkg, mainPart := f.build()

	inv, err := anchor.Discover(pkg, artifact.FormatPPTX, mainPart)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Anchors) != 1 || inv.Anchors[0].Name != "top_level" {
		t.Fatalf("got %+v, want exactly one anchor named top_level (a grouped shape is out of v1 scope)", inv.Anchors)
	}
}

func TestDiscoverPPTX_TwoSlidesInPresentationOrder(t *testing.T) {
	f := newPPTXFixture(t)
	f.addSlide(shapeXML(2, "slide_one_title", "", "placeholder"))
	f.addSlide(shapeXML(2, "slide_two_title", "", "placeholder"))
	pkg, mainPart := f.build()

	inv, err := anchor.Discover(pkg, artifact.FormatPPTX, mainPart)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Anchors) != 2 {
		t.Fatalf("got %d anchors, want 2", len(inv.Anchors))
	}
	if inv.Anchors[0].Name != "slide_one_title" || inv.Anchors[0].Part != "/ppt/slides/slide1.xml" {
		t.Errorf("anchor 0 = %+v, want slide_one_title on slide1.xml", inv.Anchors[0])
	}
	if inv.Anchors[1].Name != "slide_two_title" || inv.Anchors[1].Part != "/ppt/slides/slide2.xml" {
		t.Errorf("anchor 1 = %+v, want slide_two_title on slide2.xml", inv.Anchors[1])
	}
}

func TestDiscoverPPTX_DuplicateShapeNameAcrossSlidesIsRejected(t *testing.T) {
	f := newPPTXFixture(t)
	f.addSlide(shapeXML(2, "title", "", "placeholder"))
	f.addSlide(shapeXML(2, "title", "", "placeholder"))
	pkg, mainPart := f.build()

	_, err := anchor.Discover(pkg, artifact.FormatPPTX, mainPart)
	if err == nil {
		t.Fatal("Discover succeeded despite two slides sharing one shape name")
	}
	code, ok := verr.CodeOf(err)
	if !ok || code != verr.VELLUM_ANCHOR_DUPLICATE {
		t.Errorf("code = %v, ok=%v, want VELLUM_ANCHOR_DUPLICATE", code, ok)
	}
}

func TestDiscoverPPTX_NoSlidesIsEmptyNotAnError(t *testing.T) {
	f := newPPTXFixture(t)
	pkg, mainPart := f.build()

	inv, err := anchor.Discover(pkg, artifact.FormatPPTX, mainPart)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Anchors) != 0 {
		t.Fatalf("got %d anchors, want 0", len(inv.Anchors))
	}
}
