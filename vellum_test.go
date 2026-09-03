package vellum_test

import (
	"bytes"
	"context"
	"testing"

	vellum "github.com/frankbardon/vellum"
	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/asset"
	"github.com/frankbardon/vellum/capability"
	"github.com/frankbardon/vellum/deck"
	"github.com/frankbardon/vellum/doc"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/pdf"
	"github.com/frankbardon/vellum/resolve"
	"github.com/frankbardon/vellum/sheet"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/template"
	"github.com/frankbardon/vellum/template/bind"
	"github.com/frankbardon/vellum/theme"
	"golang.org/x/image/font/gofont/goregular"
)

// minimalSpec is a small, valid specification every format renders natively.
func minimalSpec() *spec.Spec {
	return &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Title:         "Facade smoke",
		Sections: []spec.Section{{
			ID: "s1",
			Blocks: []spec.Block{
				{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 1, Content: "Title"}},
				{Kind: spec.BlockText, Text: &spec.Text{Content: "Body."}},
			},
		}},
	}
}

// degradingSpec carries a notes block, which FeatureBlockNotes degrades in
// DOCX (capability/matrix.go), so resolving it against FormatDOCX always
// raises exactly the one warning this file's Validate test looks for.
func degradingSpec() *spec.Spec {
	s := minimalSpec()
	s.Sections[0].Blocks = append(s.Sections[0].Blocks, spec.Block{
		Kind:  spec.BlockNotes,
		Notes: &spec.Notes{Content: "A note."},
	})
	return s
}

// pdfThemeID names the theme pdfTheme builds.
const pdfThemeID = "vellum-test-pdf"

// pdfTheme is a theme whose faces can actually be embedded.
//
// The built-in theme cannot target PDF and that is deliberate rather than an
// oversight: its three faces are declared non-embeddable, because Vellum
// ships no font program, and PDF/A-2b requires every font embedded —
// font.embed.none in the capability matrix, refused before any bytes exist.
// A caller targeting PDF supplies a theme like this one, mirroring
// internal/dettest's own composeTheme.
func pdfTheme(t *testing.T) *theme.Theme {
	t.Helper()
	th, err := theme.Builtin()
	if err != nil {
		t.Fatalf("theme.Builtin: %v", err)
	}
	th.ID = pdfThemeID
	for i := range th.Fonts {
		th.Fonts[i].Family = "Go Regular"
		th.Fonts[i].Embeddable = true
		th.Fonts[i].Substitute = ""
		th.Fonts[i].Embed = theme.EmbedSubset
		th.Fonts[i].Handle = "font/go-regular"
	}
	return th
}

// pdfAssets serves the one font program pdfTheme names.
func pdfAssets() asset.Resolver {
	return asset.NewMap(map[string]asset.Asset{
		"font/go-regular": {MediaType: "font/ttf", Bytes: goregular.TTF},
	})
}

// pdfVellum is a *vellum.Vellum configured with a theme provider serving
// pdfTheme, so Compose/Write against artifact.FormatPDF succeed.
func pdfVellum(t *testing.T) *vellum.Vellum {
	t.Helper()
	provider, err := theme.NewStaticProvider(pdfTheme(t))
	if err != nil {
		t.Fatalf("theme.NewStaticProvider: %v", err)
	}
	v, err := vellum.New(vellum.Options{Themes: provider, Assets: pdfAssets()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return v
}

// pdfSpec is minimalSpec targeted at pdfTheme.
func pdfSpec() *spec.Spec {
	s := minimalSpec()
	s.Theme = pdfThemeID
	return s
}

func TestNew_ZeroValueOptionsWorks(t *testing.T) {
	v, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if v == nil {
		t.Fatal("New returned a nil *Vellum with a nil error")
	}
}

func TestCompose_DOCXProducesOpenableBytes(t *testing.T) {
	v, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var buf bytes.Buffer
	report, err := v.Compose(context.Background(), minimalSpec(), artifact.FormatDOCX, &buf)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if report.Format != artifact.FormatDOCX {
		t.Errorf("report.Format = %q, want %q", report.Format, artifact.FormatDOCX)
	}
	if buf.Len() == 0 {
		t.Error("Compose wrote no bytes")
	}
}

// TestCompose_PDFProducesOpenableBytes exercises the PDF branch of Compose's
// dispatch. It needs a theme whose faces can be embedded — see pdfTheme —
// because the built-in theme's faces cannot be, by design.
func TestCompose_PDFProducesOpenableBytes(t *testing.T) {
	v := pdfVellum(t)
	var buf bytes.Buffer
	report, err := v.Compose(context.Background(), pdfSpec(), artifact.FormatPDF, &buf)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if report.Format != artifact.FormatPDF {
		t.Errorf("report.Format = %q, want %q", report.Format, artifact.FormatPDF)
	}
	if buf.Len() == 0 {
		t.Error("Compose wrote no bytes")
	}
}

func TestCompose_XLSXAndPPTXProduceOpenableBytes(t *testing.T) {
	v, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	s := minimalSpec()

	for _, format := range []artifact.Format{artifact.FormatXLSX, artifact.FormatPPTX} {
		var buf bytes.Buffer
		report, err := v.Compose(ctx, s, format, &buf)
		if err != nil {
			t.Fatalf("Compose(%s): %v", format, err)
		}
		if report.Format != format {
			t.Errorf("report.Format = %q, want %q", report.Format, format)
		}
		if buf.Len() == 0 {
			t.Errorf("Compose(%s) wrote no bytes", format)
		}
	}
}

func TestCompose_InvalidSpecReturnsCodedErrorAndWritesNothing(t *testing.T) {
	v, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var buf bytes.Buffer
	_, err = v.Compose(context.Background(), &spec.Spec{}, artifact.FormatDOCX, &buf)
	if err == nil {
		t.Fatal("Compose with no sections: want an error, got nil")
	}
	if _, ok := verr.CodeOf(err); !ok {
		t.Errorf("Compose error is not coded: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("Compose wrote %d bytes despite the rejection", buf.Len())
	}
}

func TestValidate_SurfacesADegradationWithoutWritingBytes(t *testing.T) {
	v, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	warnings, err := v.Validate(context.Background(), degradingSpec(), artifact.FormatDOCX)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	found := false
	for _, w := range warnings {
		if w.Code == verr.VELLUM_CAPABILITY_DEGRADED {
			found = true
		}
	}
	if !found {
		t.Errorf("Validate warnings = %+v, want one naming %s", warnings, verr.VELLUM_CAPABILITY_DEGRADED)
	}
}

func TestValidate_MatchesComposesWarnings(t *testing.T) {
	v, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	s := degradingSpec()

	warnings, err := v.Validate(ctx, s, artifact.FormatDOCX)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	var buf bytes.Buffer
	report, err := v.Compose(ctx, s, artifact.FormatDOCX, &buf)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}

	if len(warnings) != len(report.Warnings) {
		t.Fatalf("Validate produced %d warnings, Compose produced %d", len(warnings), len(report.Warnings))
	}
	for i := range warnings {
		if warnings[i].Code != report.Warnings[i].Code {
			t.Errorf("warning %d: Validate code %s, Compose code %s", i, warnings[i].Code, report.Warnings[i].Code)
		}
	}
}

func TestBoxes_AnsweredWithNoSpec(t *testing.T) {
	v, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	boxes, err := v.Boxes(context.Background(), theme.BuiltinID, artifact.FormatDOCX)
	if err != nil {
		t.Fatalf("Boxes: %v", err)
	}
	if len(boxes) == 0 {
		t.Error("Boxes returned no boxes for the built-in theme")
	}

	// An empty theme id selects the same built-in theme.
	same, err := v.Boxes(context.Background(), "", artifact.FormatDOCX)
	if err != nil {
		t.Fatalf("Boxes(\"\"): %v", err)
	}
	if len(same) != len(boxes) {
		t.Errorf("Boxes(\"\") = %d boxes, Boxes(BuiltinID) = %d", len(same), len(boxes))
	}
}

func TestBoxes_UnknownFormatIsCoded(t *testing.T) {
	v, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = v.Boxes(context.Background(), theme.BuiltinID, artifact.Format("bogus"))
	if err == nil {
		t.Fatal("Boxes with an unknown format: want an error, got nil")
	}
	if _, ok := verr.CodeOf(err); !ok {
		t.Errorf("Boxes error is not coded: %v", err)
	}
}

func TestCapabilities_MatchesForFormat(t *testing.T) {
	v, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, format := range artifact.AllFormats() {
		got := v.Capabilities(format)
		want := capability.ForFormat(format)
		if len(got) != len(want) {
			t.Fatalf("Capabilities(%s) = %d rows, ForFormat = %d rows", format, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("Capabilities(%s)[%d] = %+v, want %+v", format, i, got[i], want[i])
			}
		}
	}
}

// TestArtifactName_StableAndNoRender uses a spec with no sections, which
// Compose and Validate both reject outright ([spec.Spec.Validate] requires
// at least one). ArtifactName succeeding against exactly that spec is the
// evidence that it never resolves or renders anything: the only method it
// calls on s is Hash, which needs no section either.
func TestArtifactName_StableAndNoRender(t *testing.T) {
	v, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// An invalid spec (no sections) would fail Compose and Validate outright.
	// ArtifactName must still succeed, because it never resolves or renders.
	s := &spec.Spec{}

	name1 := v.ArtifactName(s, []string{"b-hash", "a-hash"})
	name2 := v.ArtifactName(s, []string{"a-hash", "b-hash"})
	if name1 != name2 {
		t.Errorf("ArtifactName is order-sensitive over asset hashes: %q vs %q", name1, name2)
	}
	if name1 == "" {
		t.Error("ArtifactName returned an empty string")
	}

	nameNoAssets := v.ArtifactName(s, nil)
	if nameNoAssets == name1 {
		t.Error("ArtifactName did not change when the asset hash set changed")
	}

	// Stable across a second, independently constructed Vellum.
	v2, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := v2.ArtifactName(s, []string{"a-hash", "b-hash"}); got != name1 {
		t.Errorf("ArtifactName is not stable across Vellum instances: %q vs %q", got, name1)
	}
}

// resolvedModel resolves and lowers s for format, giving the concrete
// per-format model Write's "compose from a format model" path expects.
func resolvedModel(t *testing.T, format artifact.Format) any {
	t.Helper()

	s := minimalSpec()
	opts := resolve.Options{Format: format}
	if format == artifact.FormatPDF {
		// The built-in theme's faces are not embeddable, and PDF/A-2b
		// requires every font embedded; see pdfTheme.
		provider, err := theme.NewStaticProvider(pdfTheme(t))
		if err != nil {
			t.Fatalf("theme.NewStaticProvider: %v", err)
		}
		s = pdfSpec()
		opts.Themes = provider
		opts.Assets = pdfAssets()
	}

	res, err := resolve.Resolve(context.Background(), s, opts)
	if err != nil {
		t.Fatalf("resolve.Resolve(%s): %v", format, err)
	}
	switch format {
	case artifact.FormatDOCX:
		m, err := doc.Lower(res.Doc)
		if err != nil {
			t.Fatalf("doc.Lower: %v", err)
		}
		return m
	case artifact.FormatXLSX:
		m, err := sheet.Lower(res.Doc)
		if err != nil {
			t.Fatalf("sheet.Lower: %v", err)
		}
		return m
	case artifact.FormatPPTX:
		m, err := deck.Lower(res.Doc)
		if err != nil {
			t.Fatalf("deck.Lower: %v", err)
		}
		return m
	case artifact.FormatPDF:
		m, err := pdf.Lower(res.Doc)
		if err != nil {
			t.Fatalf("pdf.Lower: %v", err)
		}
		return m
	default:
		t.Fatalf("no lowering for %s", format)
		return nil
	}
}

func TestWrite_EachFormatModelDispatchesCorrectly(t *testing.T) {
	v, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	for _, format := range artifact.AllFormats() {
		model := resolvedModel(t, format)
		var buf bytes.Buffer
		report, err := v.Write(ctx, model, &buf)
		if err != nil {
			t.Fatalf("Write(%s): %v", format, err)
		}
		if report.Format != format {
			t.Errorf("Write(%s) report.Format = %q", format, report.Format)
		}
		if buf.Len() == 0 {
			t.Errorf("Write(%s) wrote no bytes", format)
		}
		if report.Warnings != nil {
			t.Errorf("Write(%s) report carries warnings from a path that never resolves: %+v", format, report.Warnings)
		}
	}
}

func TestWrite_UnsupportedModelIsCoded(t *testing.T) {
	v, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var buf bytes.Buffer
	_, err = v.Write(context.Background(), "not a format model", &buf)
	if err == nil {
		t.Fatal("Write with an unrecognised model: want an error, got nil")
	}
	code, ok := verr.CodeOf(err)
	if !ok || code != verr.VELLUM_ARTIFACT_MODEL_UNSUPPORTED {
		t.Errorf("Write error code = %v, ok=%v, want %s", code, ok, verr.VELLUM_ARTIFACT_MODEL_UNSUPPORTED)
	}
	if buf.Len() != 0 {
		t.Errorf("Write wrote %d bytes despite the rejection", buf.Len())
	}
}

// --- Fill mode -------------------------------------------------------------

const (
	fillNSWord   = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	fillRelDoc   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	fillCTMain   = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	fillCTStyles = "application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"
	fillXMLDecl  = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"
)

// buildFillTemplate is a minimal docx-shaped template carrying one {{name}}
// marker, built directly against opc the same way template/fill_test.go and
// internal/dettest/fill_fixtures.go already build theirs.
func buildFillTemplate(t *testing.T) []byte {
	t.Helper()
	pkg := opc.New()

	body := `<w:p><w:r><w:t>Hello, {{name}}.</w:t></w:r></w:p>`
	docXML := fillXMLDecl + `<w:document xmlns:w="` + fillNSWord + `"><w:body>` + body + `</w:body></w:document>`
	if err := pkg.Put(&opc.Part{Name: "/word/document.xml", ContentType: fillCTMain, Data: []byte(docXML)}); err != nil {
		t.Fatalf("Put document.xml: %v", err)
	}
	stylesXML := fillXMLDecl + `<w:styles xmlns:w="` + fillNSWord + `"/>`
	if err := pkg.Put(&opc.Part{Name: "/word/styles.xml", ContentType: fillCTStyles, Data: []byte(stylesXML)}); err != nil {
		t.Fatalf("Put styles.xml: %v", err)
	}
	if _, err := pkg.Relationships("/").Add(fillRelDoc, "word/document.xml", opc.TargetInternal); err != nil {
		t.Fatalf("Add relationship: %v", err)
	}

	var buf bytes.Buffer
	if err := pkg.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

func fillBinding() *bind.Binding {
	return &bind.Binding{
		FormatVersion: bind.FormatVersion,
		Statements: []bind.Statement{
			{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "name", Expr: "customer_name"}},
		},
	}
}

func TestInspect_ReportsTheDeclaredAnchor(t *testing.T) {
	v, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	raw := buildFillTemplate(t)
	report, err := v.Inspect(context.Background(), bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(report.Anchors) != 1 || report.Anchors[0].Name != "name" {
		t.Errorf("Inspect anchors = %+v, want one named %q", report.Anchors, "name")
	}
}

func TestFill_BindsDataIntoTheTemplate(t *testing.T) {
	v, err := vellum.New(vellum.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	raw := buildFillTemplate(t)
	data := bind.Scope{"customer_name": "Acme & Co."}

	res, err := v.Fill(context.Background(), bytes.NewReader(raw), int64(len(raw)), fillBinding(), data)
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if len(res.Touched) == 0 {
		t.Error("Fill touched no parts")
	}
	if res.Package == nil {
		t.Fatal("Fill returned a nil package")
	}

	var buf bytes.Buffer
	if err := res.Package.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("Package.WriteTo: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("the filled package wrote no bytes")
	}
}

// TestAliases_Compile is not a runtime assertion; it is evidence the root
// aliases in aliases.go actually name the types the facade's methods use, so
// a caller importing only "github.com/frankbardon/vellum" can write every
// line below without a second import.
func TestAliases_Compile(t *testing.T) {
	var (
		_ vellum.Spec       = spec.Spec{}
		_ vellum.Format     = artifact.FormatDOCX
		_ vellum.Report     = artifact.Report{}
		_ vellum.Matrix     = capability.Matrix{}
		_ vellum.BoxSet     = theme.BoxSet{}
		_ vellum.CodedError = verr.CodedError{}
		_ vellum.Binding    = bind.Binding{}
		_ vellum.Scope      = bind.Scope{}
		_ vellum.FillResult = template.Result{}
		_ vellum.Evaluator  = bind.NewFEELEvaluator()
	)
	var _ *vellum.InspectReport = &template.InspectReport{}
	_ = vellum.FormatXLSX
	_ = vellum.FormatPPTX
	_ = vellum.FormatPDF
}
