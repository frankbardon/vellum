package bind_test

// E11-S2's own coverage for RepeatTargetSlide: a real pptx-shaped fixture
// (presentation + two slides, each wired through actual [opc.Package]
// relationships) run through the full anchor.Discover -> bind.Execute
// pipeline, proving the slide-clone repeat's own part/relationship/
// content-type registration and <p:sldId> numbering. template/splice's own
// pptx_test.go covers spliceShape's own text-rendering shapes in isolation;
// this file is about the slide-clone mechanism repeat_slide.go implements.

import (
	"bytes"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/template/bind"
	"github.com/frankbardon/vellum/xmlcopy"
)

const (
	nsPresentationSL = "http://schemas.openxmlformats.org/presentationml/2006/main"
	nsDrawingMainSL  = "http://schemas.openxmlformats.org/drawingml/2006/main"
	nsRelSL          = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	relSlideSL       = nsRelSL + "/slide"
	relSlideLayoutSL = nsRelSL + "/slideLayout"
	relSlideMasterSL = nsRelSL + "/slideMaster"
	relOfficeDocSL   = nsRelSL + "/officeDocument"

	ctPresentationSL = "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
	ctSlideSL        = "application/vnd.openxmlformats-officedocument.presentationml.slide+xml"

	presPartSL = "/ppt/presentation.xml"
)

func xmlDeclSL() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"
}

// slideXMLSL builds one slide part carrying a single shape anchor named
// name, whose <p:txBody> holds placeholder text.
func slideXMLSL(name string) []byte {
	return []byte(xmlDeclSL() +
		`<p:sld xmlns:a="` + nsDrawingMainSL + `" xmlns:r="` + nsRelSL +
		`" xmlns:p="` + nsPresentationSL + `">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="` + name + `"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>placeholder</a:t></a:r></a:p></p:txBody></p:sp>` +
		`</p:spTree></p:cSld>` +
		`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
		`</p:sld>`)
}

// buildSlideRepeatFixture builds a minimal but realistic .pptx-shaped
// package: one master relationship (never backed by a real part — the
// numbering rule this story cares about never looks at it, only at
// scanning <p:sldId> entries), and slideCount slides, each carrying one
// shape anchor named "title" + its own 1-based index, wired with real
// relationships including each slide's own relationship to a layout, so the
// clone's own copied-rels-part path is exercised.
//
// The package is built via opc.New and then round-tripped through
// WriteTo/opc.Open before being returned — not merely handed back as built.
// A package built directly via opc.New carries *unparsed* relationship
// sets, and execSlideRepeat's own Relationships(presPart).Freeze() call
// would renumber every entry in an unparsed set (including the master and
// original-slide relationships this fixture already baked into
// presentation.xml's own static bytes as literal r:id="rId1" text),
// desynchronising the XML from the relationship part. A real template
// opened through template.Open never has this problem — see
// [opc.Relationships.Freeze]'s own doc comment: a *parsed* set (which
// every real, previously-authored template's rels part is) is left exactly
// as it was read, and Freeze is a no-op on it. Round-tripping here is what
// makes this fixture exercise the same "parsed" code path a genuine
// template does, rather than the narrow, documented limitation
// embedAsset's own doc comment already names for an unparsed set.
func buildSlideRepeatFixture(t *testing.T, slideCount int) (*opc.Package, []string) {
	t.Helper()
	pkg := opc.New()

	if _, err := pkg.Relationships(presPartSL).Add(relSlideMasterSL, "slideMasters/slideMaster1.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add master rel: %v", err)
	}

	var slideParts []string
	slideTargets := make([]string, 0, slideCount)
	for i := 0; i < slideCount; i++ {
		partName := "/ppt/slides/slide" + itoaSL(i+1) + ".xml"
		slideParts = append(slideParts, partName)
		slideTarget := "slides/slide" + itoaSL(i+1) + ".xml"
		slideTargets = append(slideTargets, slideTarget)

		if _, err := pkg.Relationships(presPartSL).Add(relSlideSL, slideTarget, opc.TargetInternal); err != nil {
			t.Fatalf("Add slide rel: %v", err)
		}

		shapeName := "title" + itoaSL(i+1)
		if err := pkg.Put(&opc.Part{Name: partName, ContentType: ctSlideSL, Data: slideXMLSL(shapeName)}); err != nil {
			t.Fatalf("Put %s: %v", partName, err)
		}

		if _, err := pkg.Relationships(partName).Add(relSlideLayoutSL, "../slideLayouts/slideLayout1.xml", opc.TargetInternal); err != nil {
			t.Fatalf("Add layout rel for %s: %v", partName, err)
		}
	}

	// Freeze locks in final identifiers now, before any of them are baked
	// into presentation.xml's own hand-built body text below — see this
	// function's own doc comment for why a set carrying more than one
	// relationship for the same owner cannot trust Add's own immediately-
	// returned id until Freeze (or, in production, until a genuinely parsed
	// set, which never renumbers at all) has had the last word.
	pres := pkg.Relationships(presPartSL)
	pres.Freeze()
	masterRID, ok := pres.IDFor(relSlideMasterSL, "slideMasters/slideMaster1.xml")
	if !ok {
		t.Fatal("master relationship not found after Freeze")
	}
	var sldIDXML string
	for i, target := range slideTargets {
		rid, ok := pres.IDFor(relSlideSL, target)
		if !ok {
			t.Fatalf("slide relationship for %q not found after Freeze", target)
		}
		sldIDXML += `<p:sldId id="` + itoaSL(256+i) + `" r:id="` + rid + `"/>`
	}

	presXML := xmlDeclSL() +
		`<p:presentation xmlns:a="` + nsDrawingMainSL + `" xmlns:r="` + nsRelSL +
		`" xmlns:p="` + nsPresentationSL + `">` +
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="` + masterRID + `"/></p:sldMasterIdLst>` +
		`<p:sldIdLst>` + sldIDXML + `</p:sldIdLst>` +
		`</p:presentation>`
	if err := pkg.Put(&opc.Part{Name: presPartSL, ContentType: ctPresentationSL, Data: []byte(presXML)}); err != nil {
		t.Fatalf("Put presentation.xml: %v", err)
	}
	if _, err := pkg.Relationships("/").Add(relOfficeDocSL, "ppt/presentation.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add officeDocument rel: %v", err)
	}

	var buf bytes.Buffer
	if err := pkg.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	reopened, err := opc.Open(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("opc.Open on the fixture: %v", err)
	}
	return reopened, slideParts
}

func itoaSL(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func discoverPPTXFrame(t *testing.T, pkg *opc.Package) bind.Frame {
	t.Helper()
	inv, err := anchor.Discover(pkg, artifact.FormatPPTX, presPartSL)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	anchors := make(map[string]anchor.Anchor, len(inv.Anchors))
	for _, a := range inv.Anchors {
		anchors[a.Name] = a
	}
	return bind.Frame{SrcPkg: pkg, Anchors: anchors}
}

func slideRepeatStatement(anchorName string) bind.Statement {
	return bind.Statement{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
		Over: "items", As: "item", Target: bind.RepeatTargetSlide,
		Body: []bind.Statement{
			{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: anchorName, Expr: "item.name"}},
		},
	}}
}

func TestExecute_SlideRepeatThreeItems(t *testing.T) {
	pkg, slideParts := buildSlideRepeatFixture(t, 2) // slide1 (repeated), slide2 (kept as-is)
	frame := discoverPPTXFrame(t, pkg)

	repls := bind.NewReplacementSet()
	ev := bind.NewFEELEvaluator()
	stmts := []bind.Statement{slideRepeatStatement("title1")}
	items := []any{
		map[string]any{"name": "Alpha"},
		map[string]any{"name": "Beta"},
		map[string]any{"name": "Gamma"},
	}
	data := bind.Scope{"items": items}

	if err := bind.Execute(stmts, data, ev, frame, pkg, repls); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	presSrc, ok := pkg.Get(presPartSL)
	if !ok {
		t.Fatal("package missing presentation.xml")
	}
	presBytes, err := presSrc.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	appliedPres, err := xmlcopy.Apply(presBytes, repls.For(presPartSL))
	if err != nil {
		t.Fatalf("Apply(presentation.xml): %v", err)
	}
	if err := xmlcopy.Walk(appliedPres, func(xmlcopy.Element) error { return nil }); err != nil {
		t.Fatalf("filled presentation.xml does not parse: %v\n%s", err, appliedPres)
	}
	// Original slide1's r:id no longer appears in the applied bytes at all
	// (replaced wholesale), the un-repeated slide2 entry (originally sldId
	// 257, one past slide1's own 256) survives untouched, and fresh ids
	// starting one past the highest original id (257) are used for the
	// three clones: 258, 259, 260.
	sldIDCount := bytes.Count(appliedPres, []byte("<p:sldId "))
	if sldIDCount != 3+1 { // 3 clones + the un-repeated slide2
		t.Fatalf("got %d <p:sldId> entries, want 4 (3 clones + slide2): %s", sldIDCount, appliedPres)
	}
	if !bytes.Contains(appliedPres, []byte(`id="257"`)) {
		t.Errorf("slide2's own original sldId=257 did not survive: %s", appliedPres)
	}
	for _, want := range []string{`id="258"`, `id="259"`, `id="260"`} {
		if !bytes.Contains(appliedPres, []byte(want)) {
			t.Errorf("missing expected fresh sldId %s: %s", want, appliedPres)
		}
	}

	// Write the whole package out and reopen it, to prove every new part,
	// relationship and content-type override is well-formed together, not
	// merely each in isolation.
	out := pkg.Clone()
	if err := out.Put(&opc.Part{Name: presPartSL, ContentType: ctPresentationSL, Data: appliedPres}); err != nil {
		t.Fatalf("Put filled presentation.xml: %v", err)
	}

	var buf bytes.Buffer
	if err := out.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	reopened, err := opc.Open(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("the filled package does not round-trip through opc.Open: %v", err)
	}

	// Every relationship the (reopened, materialised) presentation rels part
	// carries for type "slide" must resolve to a real part in the package.
	// This story never deletes an original slide's own relationship — only
	// the <p:sldId> *entry* that used it is replaced — so the original
	// slide1 relationship (now unreferenced by any sldId, since slide1's own
	// entry was replaced wholesale) is still present alongside slide2's own
	// (kept, since this repeat never touched it) and the three new clones':
	// five in total.
	rels, ok := reopened.RelationshipsFor(presPartSL)
	if !ok {
		t.Fatal("reopened package carries no presentation.xml relationships")
	}
	slideRels := rels.ByType(relSlideSL)
	if len(slideRels) != 5 {
		t.Fatalf("got %d slide relationships, want 5 (orphaned slide1 + slide2 + 3 clones): %+v", len(slideRels), slideRels)
	}
	newPartCount := 0
	for _, rel := range slideRels {
		target := "/ppt/" + rel.Target // presPartSL's own directory is "/ppt/"
		if !reopened.Has(target) {
			t.Errorf("relationship target %q does not resolve to a part in the package", target)
		}
		if target == slideParts[0] || target == slideParts[1] {
			continue // the original slides, not a new clone
		}
		newPartCount++
		// The clone's own copied rels part is present and carries the
		// same layout relationship every slide's own rels does.
		cloneRels, ok := reopened.RelationshipsFor(target)
		if !ok {
			t.Errorf("clone %q carries no relationships part of its own", target)
			continue
		}
		if len(cloneRels.ByType(relSlideLayoutSL)) != 1 {
			t.Errorf("clone %q did not inherit its own layout relationship", target)
		}
		// The clone's own content type matches the original slide's.
		part, _ := reopened.Get(target)
		if part.ContentType != ctSlideSL {
			t.Errorf("clone %q content type = %q, want %q", target, part.ContentType, ctSlideSL)
		}
	}
	if newPartCount != 3 {
		t.Fatalf("got %d new slide parts, want 3: %+v", newPartCount, slideRels)
	}

	// The original, un-repeated slide1 template part is still present,
	// unreferenced-but-not-destroyed — see execSlideRepeat's own doc
	// comment for why it is left rather than deleted.
	if !reopened.Has(slideParts[0]) {
		t.Error("the original template slide part was deleted; execSlideRepeat's own doc says it should be left in place")
	}
	// And slide2, never touched by this repeat, is byte-identical.
	origSlide2, _ := pkg.Get(slideParts[1])
	origSlide2Bytes, _ := origSlide2.Bytes()
	reopenedSlide2, _ := reopened.Get(slideParts[1])
	reopenedSlide2Bytes, _ := reopenedSlide2.Bytes()
	if !bytes.Equal(origSlide2Bytes, reopenedSlide2Bytes) {
		t.Error("slide2 was disturbed by a repeat that never targeted it")
	}
}

func TestExecute_SlideRepeatZeroItemsRemovesTheSlideWhenAnotherRemains(t *testing.T) {
	pkg, _ := buildSlideRepeatFixture(t, 2)
	frame := discoverPPTXFrame(t, pkg)

	repls := bind.NewReplacementSet()
	ev := bind.NewFEELEvaluator()
	stmts := []bind.Statement{slideRepeatStatement("title1")}
	data := bind.Scope{"items": []any{}}

	if err := bind.Execute(stmts, data, ev, frame, pkg, repls); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	presSrc, _ := pkg.Get(presPartSL)
	presBytes, _ := presSrc.Bytes()
	applied, err := xmlcopy.Apply(presBytes, repls.For(presPartSL))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if bytes.Count(applied, []byte("<p:sldId ")) != 1 {
		t.Fatalf("want exactly slide2's own single sldId entry to remain: %s", applied)
	}
	if !bytes.Contains(applied, []byte(`id="257"`)) {
		t.Errorf("slide2's own sldId did not survive a zero-item repeat targeting slide1: %s", applied)
	}
}

func TestExecute_SlideRepeatZeroItemsOnTheOnlySlideIsRejected(t *testing.T) {
	pkg, _ := buildSlideRepeatFixture(t, 1)
	frame := discoverPPTXFrame(t, pkg)

	repls := bind.NewReplacementSet()
	ev := bind.NewFEELEvaluator()
	stmts := []bind.Statement{slideRepeatStatement("title1")}
	data := bind.Scope{"items": []any{}}

	err := bind.Execute(stmts, data, ev, frame, pkg, repls)
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_SLIDE_REPEAT_EMPTIES_DECK) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_SLIDE_REPEAT_EMPTIES_DECK", err)
	}
}

func TestExecute_SlideRepeatBodyAnchorsSpanningTwoSlidesIsRejected(t *testing.T) {
	pkg, _ := buildSlideRepeatFixture(t, 2)
	frame := discoverPPTXFrame(t, pkg)

	repls := bind.NewReplacementSet()
	ev := bind.NewFEELEvaluator()
	stmts := []bind.Statement{{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
		Over: "items", As: "item", Target: bind.RepeatTargetSlide,
		Body: []bind.Statement{
			{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "title1", Expr: "item.name"}},
			{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "title2", Expr: "item.name"}},
		},
	}}}
	data := bind.Scope{"items": []any{map[string]any{"name": "x"}}}

	err := bind.Execute(stmts, data, ev, frame, pkg, repls)
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_REPEAT_CONTAINER_INVALID) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_REPEAT_CONTAINER_INVALID", err)
	}
}

func TestExecute_SlideRepeatSldIDsStayBelowTheMasterIdentifierSpace(t *testing.T) {
	pkg, _ := buildSlideRepeatFixture(t, 2)
	frame := discoverPPTXFrame(t, pkg)

	repls := bind.NewReplacementSet()
	ev := bind.NewFEELEvaluator()
	stmts := []bind.Statement{slideRepeatStatement("title1")}
	data := bind.Scope{"items": []any{map[string]any{"name": "Alpha"}, map[string]any{"name": "Beta"}}}

	if err := bind.Execute(stmts, data, ev, frame, pkg, repls); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	presSrc, _ := pkg.Get(presPartSL)
	presBytes, _ := presSrc.Bytes()
	applied, err := xmlcopy.Apply(presBytes, repls.For(presPartSL))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Every new sldId is >= 256 (checked directly: they are 258 and 259,
	// one past the highest original id 257) and strictly below the
	// sldMasterId this fixture also declares, 2147483648 — the two spaces
	// stay disjoint. The fixture's own untouched <p:sldMasterId
	// id="2147483648"/> legitimately still appears in applied (that region
	// is never touched by this repeat's own replacement, which covers only
	// the <p:sldIdLst> entry it replaces) — the check below is scoped to
	// the sldIdLst's own entries, not the whole document, so it does not
	// mistake that survivor for a collision.
	sldIdLstStart := bytes.Index(applied, []byte("<p:sldIdLst>"))
	if sldIdLstStart < 0 {
		t.Fatalf("no <p:sldIdLst> found: %s", applied)
	}
	sldIdLst := applied[sldIdLstStart:]
	if bytes.Contains(sldIdLst, []byte(`id="2147483648"`)) {
		t.Errorf("a cloned slide's own sldId collided with the master identifier space: %s", sldIdLst)
	}
	if !bytes.Contains(sldIdLst, []byte(`id="258"`)) || !bytes.Contains(sldIdLst, []byte(`id="259"`)) {
		t.Errorf("expected fresh ids 258 and 259: %s", sldIdLst)
	}
}
