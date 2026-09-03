package template_test

// TestNonDestructiveCorpus_PPTX is E11-S3's pptx counterpart to E9-S5's
// TestNonDestructiveCorpus and this story's own
// TestNonDestructiveCorpus_XLSX: the same "every part outside the fill's own
// touched parts is byte-identical to source" property, proved against a
// richer pptx fixture than fill_pptx_test.go's own minimal one. It reuses
// that file's slide/binding/data shape (a plain shape bind on slide1, a
// RepeatTargetSlide over slide2) and adds a third slide the repeat never
// touches: speaker notes reached through its own relationship, embedded
// media reached through another, and a custom XML part. It also proves the
// property fill_pptx_test.go's own doc comment names but does not itself
// check in this much depth: that a slide-clone repeat adding whole new OPC
// parts and relationships elsewhere in the deck does not disturb an
// unrelated slide's own relationship graph — asserted here by resolving
// slide3's own notesSlide and image relationships in the *reopened* output
// package, not merely by comparing its own part's bytes.

import (
	"bytes"
	"testing"

	"github.com/frankbardon/vellum/internal/imagetest"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/template"
)

const (
	pndRelNotesSlide     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesSlide"
	pndRelImage          = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"
	pndRelCustomXML      = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/customXml"
	pndRelCustomXMLProps = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/customXmlProps"
	pndNSCustomXML       = "http://schemas.openxmlformats.org/officeDocument/2006/customXml"

	pndCTNotesSlide     = "application/vnd.openxmlformats-officedocument.presentationml.notesSlide+xml"
	pndCTPNG            = "image/png"
	pndCTCustomXML      = "application/xml"
	pndCTCustomXMLProps = "application/vnd.openxmlformats-officedocument.customXmlProperties+xml"

	pndSlide3Part         = "/ppt/slides/slide3.xml"
	pndNotesSlidePart     = "/ppt/notesSlides/notesSlide1.xml"
	pndMediaPart          = "/ppt/media/image1.png"
	pndCustomXMLPart      = "/customXml/item1.xml"
	pndCustomXMLPropsPart = "/customXml/itemProps1.xml"

	pndNotesText = "Speaker notes that must survive the fill untouched."
)

// pptxSlide3XML is the second, entirely untouched slide: no named shape (so
// it contributes no anchor at all — see template/anchor/pptx.go's own doc
// comment on an empty name simply not being discovered), one picture
// reaching embedded media through its own relationship.
func pptxSlide3XML(imageRID string) []byte {
	return []byte(xmlDeclPPTXFill() +
		`<p:sld xmlns:a="` + pptxNSDrawingMain + `" xmlns:r="` + pptxNSRelationships +
		`" xmlns:p="` + pptxNSPresentation + `">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/>` +
		`<p:pic>` +
		`<p:nvPicPr><p:cNvPr id="2" name="Picture 2" descr="An unrelated picture nothing in the binding touches"/>` +
		`<p:cNvPicPr/><p:nvPr/></p:nvPicPr>` +
		`<p:blipFill><a:blip r:embed="` + imageRID + `"/><a:stretch><a:fillRect/></a:stretch></p:blipFill>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="914400" cy="914400"/></a:xfrm>` +
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>` +
		`</p:pic>` +
		`</p:spTree></p:cSld>` +
		`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
		`</p:sld>`)
}

// pptxNotesSlide1XML is slide3's own speaker notes.
const pptxNotesSlide1XML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<p:notes xmlns:a="` + pptxNSDrawingMain + `" xmlns:r="` + pptxNSRelationships +
	`" xmlns:p="` + pptxNSPresentation + `">` +
	`<p:cSld><p:spTree>` +
	`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr/>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Notes Placeholder 2"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>` +
	`<p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr>` +
	`<p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>` + pndNotesText + `</a:t></a:r></a:p></p:txBody></p:sp>` +
	`</p:spTree></p:cSld>` +
	`</p:notes>`

// blipEmbedID extracts the r:embed value from a slide's own <a:blip>
// element by a plain substring search — this file's fixtures are built by
// hand with a known, single blip, so a full xmlcopy.Walk is more machinery
// than the assertion needs.
func blipEmbedID(t *testing.T, slideXML []byte) string {
	t.Helper()
	const marker = `r:embed="`
	i := bytes.Index(slideXML, []byte(marker))
	if i < 0 {
		t.Fatalf("slide XML carries no r:embed attribute: %s", slideXML)
	}
	rest := slideXML[i+len(marker):]
	j := bytes.IndexByte(rest, '"')
	if j < 0 {
		t.Fatalf("slide XML's r:embed attribute is not properly terminated: %s", slideXML)
	}
	return string(rest[:j])
}

// buildRicherFixturePPTX assembles a more realistic .pptx-shaped package than
// fill_pptx_test.go's own buildFillFixturePPTX: everything that fixture
// carries (slide1's plain shape anchor, slide2's shape anchor that a
// RepeatTargetSlide statement clones, presProps.xml), plus a third slide the
// repeat never names — carrying speaker notes reached through its own
// notesSlide relationship, an embedded picture reached through its own image
// relationship — and a custom XML part.
func buildRicherFixturePPTX(t *testing.T) []byte {
	t.Helper()
	pkg := opc.New()

	presRels := pkg.Relationships(pptxPresentationPart)
	if _, err := presRels.Add(pptxRelSlide, "slides/slide1.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add slide1 rel: %v", err)
	}
	if _, err := presRels.Add(pptxRelSlide, "slides/slide2.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add slide2 rel: %v", err)
	}
	if _, err := presRels.Add(pptxRelSlide, "slides/slide3.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add slide3 rel: %v", err)
	}
	// Three relationships of the same type on one owner: canonicalisation
	// resorts by (Type, Mode, Target) at Freeze/marshal time, so the
	// identifiers embedded in presentation.xml's own text below must be read
	// back with Freeze+IDFor rather than trusted from Add's own return
	// value. See nondestructive_xlsx_test.go's own comment on the same
	// requirement for sheet1's relationships — same mechanism, same reason.
	presRels.Freeze()
	slide1RID, ok := presRels.IDFor(pptxRelSlide, "slides/slide1.xml")
	if !ok {
		t.Fatal("slide1 relationship not found after Freeze")
	}
	slide2RID, ok := presRels.IDFor(pptxRelSlide, "slides/slide2.xml")
	if !ok {
		t.Fatal("slide2 relationship not found after Freeze")
	}
	slide3RID, ok := presRels.IDFor(pptxRelSlide, "slides/slide3.xml")
	if !ok {
		t.Fatal("slide3 relationship not found after Freeze")
	}

	pres := xmlDeclPPTXFill() +
		`<p:presentation xmlns:a="` + pptxNSDrawingMain + `" xmlns:r="` + pptxNSRelationships +
		`" xmlns:p="` + pptxNSPresentation + `">` +
		`<p:sldIdLst>` +
		`<p:sldId id="256" r:id="` + slide1RID + `"/>` +
		`<p:sldId id="257" r:id="` + slide2RID + `"/>` +
		`<p:sldId id="258" r:id="` + slide3RID + `"/>` +
		`</p:sldIdLst>` +
		`<p:sldSz cx="12192000" cy="6858000"/>` +
		`<p:notesSz cx="6858000" cy="9144000"/>` +
		`</p:presentation>`
	if err := pkg.Put(&opc.Part{Name: pptxPresentationPart, ContentType: pptxCTPresentation, Data: []byte(pres)}); err != nil {
		t.Fatalf("Put presentation.xml: %v", err)
	}
	if _, err := pkg.Relationships("/").Add(pptxRelOfficeDocument, "ppt/presentation.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add officeDocument rel: %v", err)
	}
	if _, err := pkg.Relationships("/").Add(pndRelCustomXML, "customXml/item1.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add customXml rel: %v", err)
	}

	if err := pkg.Put(&opc.Part{Name: pptxSlide1Part, ContentType: pptxCTSlide, Data: pptxSlideXMLFill("customer_name")}); err != nil {
		t.Fatalf("Put slide1.xml: %v", err)
	}
	if err := pkg.Put(&opc.Part{Name: pptxSlide2Part, ContentType: pptxCTSlide, Data: pptxSlideXMLFill("item_name")}); err != nil {
		t.Fatalf("Put slide2.xml: %v", err)
	}

	// slide3's own relationships: its notes and its picture — two different
	// relationship types on one owner, so (per the same Freeze+IDFor
	// requirement explained on presRels above) imageRID must be read back
	// after Freeze rather than trusted from Add's own return value: sorted
	// by Type, ".../relationships/image" < ".../relationships/notesSlide",
	// so canonicalisation actually reorders these two relative to insertion
	// order here, not merely in principle.
	slide3Rels := pkg.Relationships(pndSlide3Part)
	if _, err := slide3Rels.Add(pndRelNotesSlide, "../notesSlides/notesSlide1.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add notesSlide rel: %v", err)
	}
	if _, err := slide3Rels.Add(pndRelImage, "../media/image1.png", opc.TargetInternal); err != nil {
		t.Fatalf("Add image rel: %v", err)
	}
	slide3Rels.Freeze()
	imageRID, ok := slide3Rels.IDFor(pndRelImage, "../media/image1.png")
	if !ok {
		t.Fatal("image relationship not found after Freeze")
	}
	if err := pkg.Put(&opc.Part{Name: pndSlide3Part, ContentType: pptxCTSlide, Data: pptxSlide3XML(imageRID)}); err != nil {
		t.Fatalf("Put slide3.xml: %v", err)
	}
	if err := pkg.Put(&opc.Part{Name: pndNotesSlidePart, ContentType: pndCTNotesSlide, Data: []byte(pptxNotesSlide1XML)}); err != nil {
		t.Fatalf("Put notesSlide1.xml: %v", err)
	}
	if err := pkg.Put(&opc.Part{Name: pndMediaPart, ContentType: pndCTPNG, Data: imagetest.RGB()}); err != nil {
		t.Fatalf("Put image1.png: %v", err)
	}

	if err := pkg.Put(&opc.Part{Name: pptxPresPropsPart, ContentType: pptxCTPresProps, Data: []byte(pptxPresPropsXML)}); err != nil {
		t.Fatalf("Put presProps.xml: %v", err)
	}

	// customXml/item1.xml + itemProps1.xml, the same kind of consumer-owned
	// part nondestructive_fixture_test.go's DOCX fixture and this story's own
	// XLSX fixture carry.
	itemXML := xmlDeclPPTXFill() + `<consumerData xmlns="http://example.com/consumer-schema"><field>value</field></consumerData>`
	if err := pkg.Put(&opc.Part{Name: pndCustomXMLPart, ContentType: pndCTCustomXML, Data: []byte(itemXML)}); err != nil {
		t.Fatalf("Put customXml/item1.xml: %v", err)
	}
	itemPropsXML := xmlDeclPPTXFill() +
		`<ds:datastoreItem ds:itemID="{12345678-1234-1234-1234-123456789012}" xmlns:ds="` + pndNSCustomXML + `">` +
		`<ds:schemaRefs><ds:schemaRef ds:uri="http://example.com/consumer-schema"/></ds:schemaRefs>` +
		`</ds:datastoreItem>`
	if err := pkg.Put(&opc.Part{Name: pndCustomXMLPropsPart, ContentType: pndCTCustomXMLProps, Data: []byte(itemPropsXML)}); err != nil {
		t.Fatalf("Put customXml/itemProps1.xml: %v", err)
	}
	if _, err := pkg.Relationships(pndCustomXMLPart).Add(pndRelCustomXMLProps, "itemProps1.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add customXml item rel: %v", err)
	}

	var buf bytes.Buffer
	if err := pkg.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

func TestNonDestructiveCorpus_PPTX(t *testing.T) {
	raw := buildRicherFixturePPTX(t)

	srcPkg, err := opc.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("opc.Open on the fixture: %v", err)
	}

	tpl, err := template.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("template.Open: %v", err)
	}

	res, err := template.Fill(tpl, fillFixtureBindingPPTX(), fillFixtureDataPPTX())
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}

	// --- Touched receipt: only slide1 and presentation.xml, exactly as -----
	// fill_pptx_test.go's own end-to-end test — the new slide3/notes/media/
	// customXml content changes nothing about what a plain bind and a
	// slide-clone repeat actually rewrite.
	wantTouched := map[string]bool{pptxSlide1Part: true, pptxPresentationPart: true}
	if len(res.Touched) != len(wantTouched) {
		t.Fatalf("Touched = %v, want exactly %v", res.Touched, wantTouched)
	}
	touched := make(map[string]bool, len(res.Touched))
	for _, name := range res.Touched {
		touched[name] = true
		if !wantTouched[name] {
			t.Errorf("unexpected touched part %q", name)
		}
	}

	// --- Every part outside Touched is byte-identical to the source, ------
	// except the brand-new cloned slide parts (and their own .rels), which
	// exist in the filled package but not in the source at all — those are
	// asserted separately below, the same split fill_pptx_test.go's own test
	// already makes.
	srcNames := make(map[string]bool)
	for _, n := range srcPkg.Names() {
		srcNames[n] = true
	}
	for name := range srcNames {
		if touched[name] {
			continue
		}
		srcBytes := mustPartBytes(t, srcPkg, name)
		filledBytes := mustPartBytes(t, res.Package, name)
		if !bytes.Equal(srcBytes, filledBytes) {
			t.Errorf("part %q changed even though it was never in Touched; "+
				"every part outside the fill's own touched parts must survive untouched.\n"+
				"source (%d bytes):\n%s\nfilled (%d bytes):\n%s",
				name, len(srcBytes), srcBytes, len(filledBytes), filledBytes)
		}
	}

	// --- Named, specific checks for the new content this fixture adds ------
	for _, part := range []string{
		pndSlide3Part, pndNotesSlidePart, pndMediaPart,
		pndCustomXMLPart, pndCustomXMLPropsPart, pptxPresPropsPart,
	} {
		if touched[part] {
			t.Fatalf("part %q was touched by the fill; this fixture's whole point is that it must not be", part)
		}
		if !srcPkg.Has(part) {
			t.Fatalf("fixture is missing its own %q part", part)
		}
	}
	filledNotes := mustPartBytes(t, res.Package, pndNotesSlidePart)
	if !bytes.Contains(filledNotes, []byte(pndNotesText)) {
		t.Errorf("slide3's own speaker notes did not survive the fill: %s", filledNotes)
	}
	filledMedia := mustPartBytes(t, res.Package, pndMediaPart)
	if !bytes.Equal(filledMedia, imagetest.RGB()) {
		t.Error("slide3's own embedded media changed even though nothing in the binding ever names it")
	}

	// --- The original slide2 template part is untouched, and every new ----
	// clone carries one item's own name — the same assertions
	// fill_pptx_test.go's own test makes, repeated here because this
	// fixture's own slide numbering (three slides, not two) is different
	// enough to be worth re-proving rather than assumed to still hold.
	srcSlide2 := mustPartBytes(t, srcPkg, pptxSlide2Part)
	filledSlide2 := mustPartBytes(t, res.Package, pptxSlide2Part)
	if !bytes.Equal(srcSlide2, filledSlide2) {
		t.Errorf("the original slide2 template part changed even though the repeat only ever reads it, never writes it:\nsource: %s\nfilled: %s", srcSlide2, filledSlide2)
	}

	names := res.Package.Names()
	var newSlideParts []string
	knownParts := map[string]bool{
		pptxSlide1Part: true, pptxSlide2Part: true, pndSlide3Part: true,
		pptxPresentationPart: true, pptxPresPropsPart: true,
		pndNotesSlidePart: true, pndMediaPart: true,
		pndCustomXMLPart: true, pndCustomXMLPropsPart: true,
	}
	for _, n := range names {
		if !knownParts[n] && !bytesHasSuffixDotRels(n) {
			newSlideParts = append(newSlideParts, n)
		}
	}
	if len(newSlideParts) != 3 {
		t.Fatalf("got %d new slide parts, want 3 (one clone per item): %v", len(newSlideParts), newSlideParts)
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
	}
	for item := range wantItems {
		if !found[item] {
			t.Errorf("no cloned slide carried item %q", item)
		}
	}

	// --- The result round-trips through opc.Open, and slide3's own -------
	// relationship graph — its notes and its media, both reached through
	// relationships nothing about the slide-clone repeat touches — still
	// resolves in the reopened output package. This is the assertion
	// fill_pptx_test.go's own doc comment names but does not itself make:
	// that adding whole new parts and relationships elsewhere in the deck
	// (the three slide clones, presentation.xml's rewritten sldIdLst) does
	// not disturb an unrelated slide's own relationship graph.
	var out bytes.Buffer
	if err := res.Package.WriteTo(&out, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	reopened, err := opc.Open(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatalf("the filled package does not round-trip through opc.Open: %v", err)
	}
	if !reopened.Has(pndSlide3Part) {
		t.Fatal("reopened package lost the third, untouched slide")
	}
	slide3Rels, ok := reopened.RelationshipsFor(pndSlide3Part)
	if !ok {
		t.Fatal("reopened package carries no slide3.xml relationships")
	}
	sawNotes, sawImage := false, false
	for _, rel := range slide3Rels.ByType(pndRelNotesSlide) {
		sawNotes = true
		target := "/ppt/" + rel.Target[len("../"):]
		if !reopened.Has(target) {
			t.Errorf("slide3's notesSlide relationship target %q does not resolve to a part in the reopened package", target)
		}
	}
	for _, rel := range slide3Rels.ByType(pndRelImage) {
		sawImage = true
		target := "/ppt/" + rel.Target[len("../"):]
		if !reopened.Has(target) {
			t.Errorf("slide3's image relationship target %q does not resolve to a part in the reopened package", target)
		}
	}
	if !sawNotes {
		t.Error("slide3's own notesSlide relationship did not survive the fill")
	}
	if !sawImage {
		t.Error("slide3's own image relationship did not survive the fill")
	}

	// The stronger form of the image check: not merely that some
	// relationship of type image exists, but that the specific r:id
	// slide3.xml's own <a:blip r:embed="..."/> names resolves, through
	// slide3's own relationships, to that image — not to the notesSlide
	// relationship or to nothing at all. This is the check that would have
	// caught buildRicherFixturePPTX's own construction bug during this
	// story's development: [opc.Relationships] renumbers a built set by a
	// sorted (Type, Mode, Target) walk at Freeze/marshal time, not left in
	// insertion order, so an r:id captured from Add's own return value
	// before Freeze can silently point at the wrong relationship once the
	// package is actually serialised — exactly the class of defect
	// CLAUDE.md's own "external reader oracles" section describes: bytes
	// that assemble and open fine while pointing at the wrong thing.
	filledSlide3 := mustPartBytes(t, res.Package, pndSlide3Part)
	embedID := blipEmbedID(t, filledSlide3)
	rel, ok := slide3Rels.Resolve(embedID)
	if !ok {
		t.Fatalf("slide3.xml's own r:embed=%q does not resolve to any relationship slide3 owns", embedID)
	}
	if rel.Type != pndRelImage {
		t.Errorf("slide3.xml's own r:embed=%q resolves to a %q relationship, want %q — "+
			"the picture points at the wrong part", embedID, rel.Type, pndRelImage)
	}

	// Every relationship of type "slide" from presentation.xml resolves to a
	// real, present part — including slide3's, unaffected by the repeat.
	presRels, ok := reopened.RelationshipsFor(pptxPresentationPart)
	if !ok {
		t.Fatal("reopened package carries no presentation.xml relationships")
	}
	for _, rel := range presRels.ByType(pptxRelSlide) {
		target := "/ppt/" + rel.Target
		if !reopened.Has(target) {
			t.Errorf("presentation.xml relationship target %q does not resolve to a part in the package", target)
		}
	}

	// --- The originally opened Template is untouched -----------------------
	origSlide1 := mustPartBytes(t, tpl.Package(), pptxSlide1Part)
	if !bytes.Contains(origSlide1, []byte("placeholder")) {
		t.Error("Fill mutated the *Template it was called against; the original placeholder text should still be there")
	}
}
