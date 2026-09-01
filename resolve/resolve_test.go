package resolve_test

import (
	"context"
	"encoding/base64"
	stderrors "errors"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/asset"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/resolve"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/theme"
)

var onePixelPNG = mustDecode(`iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==`)

func mustDecode(s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// wideAsset is 640x360, so a 16:9 ratio is visible in the placed height.
const wideSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 360"/>`

func doc(blocks ...spec.Block) *spec.Spec {
	return &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Title:         "Resolved",
		Sections:      []spec.Section{{ID: "s1", Blocks: blocks}},
	}
}

func heading(level int, content string, marks ...string) spec.Block {
	return spec.Block{Kind: spec.BlockHeading, Marks: marks,
		Heading: &spec.Heading{Level: level, Content: content}}
}

func text(content string, marks ...string) spec.Block {
	return spec.Block{Kind: spec.BlockText, Marks: marks, Text: &spec.Text{Content: content}}
}

func resolveDoc(t *testing.T, s *spec.Spec, opts resolve.Options) *resolve.Result {
	t.Helper()
	res, err := resolve.Resolve(context.Background(), s, opts)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return res
}

func hasWarning(res *resolve.Result, code verr.Code) bool {
	for _, w := range res.Warnings {
		if w.Code == code {
			return true
		}
	}
	return false
}

// TestResolve_WireNothingWorks is the epic's slice: a caller with no provider,
// no resolver and no theme gets a fully resolved document.
func TestResolve_WireNothingWorks(t *testing.T) {
	res := resolveDoc(t, doc(heading(1, "Title"), text("Body.")),
		resolve.Options{Format: artifact.FormatDOCX})

	if res.Doc.ThemeID != theme.BuiltinID {
		t.Errorf("ThemeID = %q, want %q", res.Doc.ThemeID, theme.BuiltinID)
	}
	if len(res.Doc.Sections) != 1 || len(res.Doc.Sections[0].Blocks) != 2 {
		t.Fatalf("resolved %d sections", len(res.Doc.Sections))
	}
	if len(res.Doc.Fonts) != len(theme.AllFontRoles()) {
		t.Errorf("font manifest has %d faces, want %d", len(res.Doc.Fonts), len(theme.AllFontRoles()))
	}

	// Everything the specification left open is closed: a concrete colour, a
	// size in EMU, and a face resolved by index rather than by name.
	run := res.Doc.Sections[0].Blocks[0].Paragraph.Runs[0]
	if run.Style.Color == "" {
		t.Error("the run carries no resolved colour")
	}
	if run.Style.SizeEMU <= 0 {
		t.Errorf("SizeEMU = %d, want a positive measurement", run.Style.SizeEMU)
	}
	if run.Style.FaceIndex < 0 || run.Style.FaceIndex >= len(res.Doc.Fonts) {
		t.Errorf("FaceIndex = %d, outside the manifest", run.Style.FaceIndex)
	}
}

// TestResolve_HeadingSizesComeFromTheThemeScale pins that the type scale is
// consulted rather than a table baked into a writer — which is what E4 would
// otherwise have gone on doing.
func TestResolve_HeadingSizesComeFromTheThemeScale(t *testing.T) {
	res := resolveDoc(t, doc(heading(1, "One"), heading(2, "Two"), heading(9, "Deep")),
		resolve.Options{Format: artifact.FormatDOCX})

	blocks := res.Doc.Sections[0].Blocks
	one := blocks[0].Paragraph.Runs[0].Style.SizeEMU
	two := blocks[1].Paragraph.Runs[0].Style.SizeEMU
	deep := blocks[2].Paragraph.Runs[0].Style.SizeEMU

	if one <= two {
		t.Errorf("level 1 (%d EMU) is not larger than level 2 (%d EMU)", one, two)
	}
	// A level past the end of the scale clamps rather than failing: an outline
	// deeper than the theme anticipated is a document that should still render.
	th, _ := theme.Builtin()
	last, _ := th.Type.Headings[len(th.Type.Headings)-1].EMU()
	if deep != last {
		t.Errorf("level 9 = %d EMU, want the deepest declared size %d", deep, last)
	}
}

// TestResolve_FontPolicy is the whole of E3-S3 in one place.
func TestResolve_FontPolicy(t *testing.T) {
	t.Run("non-embeddable substitutes and warns", func(t *testing.T) {
		// The built-in theme declares every face non-embeddable with a
		// substitute, so this is its default behaviour.
		res := resolveDoc(t, doc(text("x")), resolve.Options{Format: artifact.FormatDOCX})

		if !hasWarning(res, verr.VELLUM_FONT_SUBSTITUTED) {
			t.Error("a substitution produced no warning; a silent one is how a spec renders differently on two machines")
		}
		for _, f := range res.Doc.Fonts {
			if !f.Substituted {
				t.Errorf("face %q is not marked substituted", f.Role)
			}
			if f.Family == f.Requested {
				t.Errorf("face %q kept the requested family %q", f.Role, f.Family)
			}
			if f.Embed != fragment.EmbedNone {
				t.Errorf("face %q has embed plan %q, want none", f.Role, f.Embed)
			}
		}
	})

	t.Run("embeddable in a format that cannot embed degrades and warns", func(t *testing.T) {
		th := embeddableTheme(t, theme.EmbedAuto)
		p, err := theme.NewStaticProvider(th)
		if err != nil {
			t.Fatalf("NewStaticProvider: %v", err)
		}
		s := doc(text("x"))
		s.Theme = th.ID

		res := resolveDoc(t, s, resolve.Options{
			Format: artifact.FormatDOCX,
			Themes: p,
			Assets: fontStore(),
		})
		if !hasWarning(res, verr.VELLUM_CAPABILITY_DEGRADED) {
			t.Error("a format that carries no font programs must say so")
		}
		for _, f := range res.Doc.Fonts {
			if f.Embed != fragment.EmbedNone {
				t.Errorf("face %q has embed plan %q in docx, want none", f.Role, f.Embed)
			}
		}
	})

	t.Run("an explicit embed mode in a format that carries no programs still degrades", func(t *testing.T) {
		// An embed mode is a licence condition on how a program may be
		// embedded — subset only, or unmodified. Not embedding it cannot
		// violate a condition about embedding it, so the demand is reported as
		// the degradation it is rather than refused. The refusal belongs where
		// the format does embed and Vellum cannot honour the mode.
		for _, mode := range []theme.EmbedMode{theme.EmbedSubset, theme.EmbedWhole} {
			th := embeddableTheme(t, mode)
			p, err := theme.NewStaticProvider(th)
			if err != nil {
				t.Fatalf("NewStaticProvider: %v", err)
			}
			s := doc(text("x"))
			s.Theme = th.ID

			res, err := resolve.Resolve(context.Background(), s, resolve.Options{
				Format: artifact.FormatDOCX,
				Themes: p,
				Assets: fontStore(),
			})
			if err != nil {
				t.Fatalf("embed %q: %v", mode, err)
			}
			if !hasWarning(res, verr.VELLUM_CAPABILITY_DEGRADED) {
				t.Errorf("embed %q: the degradation was not reported", mode)
			}
			for _, f := range res.Doc.Fonts {
				if f.Embed != fragment.EmbedNone {
					t.Errorf("embed %q: face %q has plan %q, want none", mode, f.Role, f.Embed)
				}
			}
		}
	})

	t.Run("embeddable in PDF embeds and carries the program", func(t *testing.T) {
		th := embeddableTheme(t, theme.EmbedAuto)
		p, err := theme.NewStaticProvider(th)
		if err != nil {
			t.Fatalf("NewStaticProvider: %v", err)
		}
		s := doc(text("x"))
		s.Theme = th.ID

		res := resolveDoc(t, s, resolve.Options{
			Format: artifact.FormatPDF, Themes: p, Assets: fontStore(),
		})
		for _, f := range res.Doc.Fonts {
			if f.Embed != fragment.EmbedSubset {
				t.Errorf("face %q has embed plan %q in pdf, want subset", f.Role, f.Embed)
			}
			if f.AssetIndex < 0 {
				t.Errorf("face %q is embedded but names no font program", f.Role)
			}
		}
		if len(res.Doc.Assets) == 0 {
			t.Error("no font program reached the asset manifest")
		}
	})

	t.Run("a non-embeddable theme cannot be resolved for PDF", func(t *testing.T) {
		// The one row that decides whether a theme is usable for PDF at all.
		// The built-in theme fails it, deliberately: Vellum ships no font
		// program, so its three faces name families and carry nothing.
		s := doc(text("x"))

		_, err := resolve.Resolve(context.Background(), s, resolve.Options{Format: artifact.FormatPDF})
		if !verr.HasCode(err, verr.VELLUM_FONT_EMBED_UNSUPPORTED) {
			t.Fatalf("error = %v, want VELLUM_FONT_EMBED_UNSUPPORTED", err)
		}

		// The same theme is fine for a target that resolves families by name,
		// which is what makes this a per-format answer rather than a bad theme.
		if _, err := resolve.Resolve(context.Background(), s,
			resolve.Options{Format: artifact.FormatDOCX}); err != nil {
			t.Fatalf("the built-in theme failed for docx as well: %v", err)
		}
	})

	t.Run("a font handle the resolver cannot produce is a font error", func(t *testing.T) {
		th := embeddableTheme(t, theme.EmbedAuto)
		p, err := theme.NewStaticProvider(th)
		if err != nil {
			t.Fatalf("NewStaticProvider: %v", err)
		}
		s := doc(text("x"))
		s.Theme = th.ID

		// A missing font and a missing picture are the same failure with
		// different consequences, so the code distinguishes them.
		_, err = resolve.Resolve(context.Background(), s, resolve.Options{
			Format: artifact.FormatPDF, Themes: p,
		})
		if !verr.HasCode(err, verr.VELLUM_FONT_UNAVAILABLE) {
			t.Fatalf("error = %v, want VELLUM_FONT_UNAVAILABLE", err)
		}
	})
}

// embeddableTheme returns the built-in theme with every face declared
// embeddable against a handle the font store serves.
func embeddableTheme(t *testing.T, mode theme.EmbedMode) *theme.Theme {
	t.Helper()
	th, err := theme.Builtin()
	if err != nil {
		t.Fatalf("Builtin: %v", err)
	}
	th.ID = "embeddable"
	for i := range th.Fonts {
		th.Fonts[i].Embeddable = true
		th.Fonts[i].Substitute = ""
		th.Fonts[i].Embed = mode
		th.Fonts[i].Handle = "font/" + string(th.Fonts[i].Role)
	}
	return th
}

// fontStore serves a font program for each role. The bytes are not a real font
// — nothing in resolve parses one, and a fixture that pretended to be one would
// be claiming a guarantee this layer does not make.
func fontStore() asset.Resolver {
	entries := map[string]asset.Asset{}
	for _, role := range theme.AllFontRoles() {
		entries["font/"+string(role)] = asset.Asset{
			MediaType: "font/ttf",
			Bytes:     []byte("font program for " + string(role)),
		}
	}
	return asset.NewMap(entries)
}

func TestResolve_MarksAreTheConsumersVocabulary(t *testing.T) {
	// "flagged" is styled by the built-in theme; "stale" is not.
	res := resolveDoc(t, doc(text("moved", "flagged"), text("also moved", "stale"), text("again", "stale")),
		resolve.Options{Format: artifact.FormatDOCX})

	styled := res.Doc.Sections[0].Blocks[0].Paragraph.Runs[0].Style
	if !styled.Italic {
		t.Error("the styled mark did not reach the run")
	}

	if !hasWarning(res, verr.VELLUM_MARK_UNKNOWN) {
		t.Fatal("an unstyled mark produced no warning; an invisible flag is indistinguishable from no flag")
	}
	// Once per distinct name, however many times it is used.
	count := 0
	for _, w := range res.Warnings {
		if w.Code == verr.VELLUM_MARK_UNKNOWN {
			count++
		}
	}
	if count != 1 {
		t.Errorf("got %d VELLUM_MARK_UNKNOWN warnings for one distinct mark, want 1", count)
	}
}

// TestResolve_MarksLayerInOrder pins the conflict rule: later wins, as in a
// stylesheet, which is what an author will assume.
func TestResolve_MarksLayerInOrder(t *testing.T) {
	res := resolveDoc(t, doc(text("x", "muted", "flagged")),
		resolve.Options{Format: artifact.FormatDOCX})

	th, _ := theme.Builtin()
	accent, _ := th.LookupColor(theme.ColorAccent)
	if got := res.Doc.Sections[0].Blocks[0].Paragraph.Runs[0].Style.Color; got != accent {
		t.Errorf("colour = %q, want the later mark's %q", got, accent)
	}
}

func TestResolve_AssetPlacedInTheThemeBox(t *testing.T) {
	handle := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(wideSVG))
	s := doc(spec.Block{Kind: spec.BlockAsset,
		Asset: &spec.Asset{Handle: handle, Role: "asset.full", AltText: "a chart"}})

	res := resolveDoc(t, s, resolve.Options{Format: artifact.FormatDOCX})
	ref := res.Doc.Sections[0].Blocks[0].Asset
	if ref == nil {
		t.Fatal("the asset block did not resolve")
	}

	th, _ := theme.Builtin()
	layout, _ := th.LayoutFor(artifact.FormatDOCX, "")
	box, _ := layout.BoxFor("asset.full")
	wantWidth, _ := box.Width.EMU()
	if ref.WidthEMU != wantWidth {
		t.Errorf("WidthEMU = %d, want the theme box's %d", ref.WidthEMU, wantWidth)
	}

	// The box declares an intrinsic height, so the asset's 16:9 ratio decides
	// it — and it is decided here, so a writer never asks an asset how tall it
	// is.
	want := int64(float64(wantWidth) / (640.0 / 360.0))
	if diff := ref.HeightEMU - want; diff > 1 || diff < -1 {
		t.Errorf("HeightEMU = %d, want %d from the asset's aspect ratio", ref.HeightEMU, want)
	}
}

// TestResolve_PDFRejectsSVG is the format-aware media policy reaching all the
// way through a real resolve, rather than being tested only at its own seam.
func TestResolve_PDFRejectsSVG(t *testing.T) {
	handle := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(wideSVG))
	s := doc(spec.Block{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: handle}})

	// An embeddable theme, because the built-in one cannot be resolved for PDF
	// at all: its three faces are declared non-embeddable and PDF/A-2b requires
	// every font embedded. Using it here would fail on the font before reaching
	// the asset, and the test would pass for the wrong reason.
	th := embeddableTheme(t, theme.EmbedAuto)
	p, err := theme.NewStaticProvider(th)
	if err != nil {
		t.Fatalf("NewStaticProvider: %v", err)
	}
	s.Theme = th.ID

	_, err = resolve.Resolve(context.Background(), s, resolve.Options{
		Format: artifact.FormatPDF, Themes: p, Assets: fontStore(),
	})
	if !verr.HasCode(err, verr.VELLUM_ASSET_MEDIA_UNSUPPORTED) {
		t.Fatalf("error = %v, want VELLUM_ASSET_MEDIA_UNSUPPORTED", err)
	}

	var coded *verr.CodedError
	if stderrors.As(err, &coded) {
		if _, ok := coded.Details["accepted"]; !ok {
			t.Error("the error names no accepted set, so it is not actionable")
		}
		// The failure is located, because a document with forty assets needs
		// to say which one.
		if _, ok := coded.Details["block_index"]; !ok {
			t.Error("the error carries no block index")
		}
	}
}

// TestResolve_AssetsDeduplicateByContent pins that a logo used six times is one
// asset. Ordering by content is what makes an OOXML package's media parts a
// function of what is in them rather than of the order they were mentioned.
func TestResolve_AssetsDeduplicateByContent(t *testing.T) {
	png := "data:image/png;base64," + base64.StdEncoding.EncodeToString(onePixelPNG)
	blocks := make([]spec.Block, 0, 6)
	for range 6 {
		blocks = append(blocks, spec.Block{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: png}})
	}

	res := resolveDoc(t, doc(blocks...), resolve.Options{Format: artifact.FormatDOCX})
	if len(res.Doc.Assets) != 1 {
		t.Fatalf("six references to one picture produced %d assets", len(res.Doc.Assets))
	}
	for i, b := range res.Doc.Sections[0].Blocks {
		if b.Asset.AssetIndex != 0 {
			t.Errorf("block %d points at asset %d, want 0", i, b.Asset.AssetIndex)
		}
	}
}

func TestResolve_TableCellsCarryTextAndValue(t *testing.T) {
	table := &spec.Table{
		ColumnHeaders: spec.HeaderTree{{Label: "Share", Span: 1}},
		Body: [][]spec.Cell{{{
			Value:       &spec.Value{Kind: spec.ValueNumber, Number: 0.4567},
			Format:      "0.0%",
			Annotations: []spec.Annotation{{Text: "a"}},
		}}},
		Caption: "Table 1",
	}
	res := resolveDoc(t, doc(spec.Block{Kind: spec.BlockTable, Table: table}),
		resolve.Options{Format: artifact.FormatDOCX})

	cell := res.Doc.Sections[0].Blocks[0].Table.Body[0][0]
	if cell.Text != "45.7%" {
		t.Errorf("Text = %q, want %q", cell.Text, "45.7%")
	}
	// Both are carried: a flowing target writes the text, a workbook writes the
	// value and the code so the cell stays live.
	if cell.Value == nil || cell.Value.Number != 0.4567 {
		t.Errorf("Value = %+v, want the unformatted number", cell.Value)
	}
	if cell.FormatCode != "0.0%" {
		t.Errorf("FormatCode = %q, want it retained verbatim", cell.FormatCode)
	}
	// An annotation attaches to the value rather than replacing it.
	if len(cell.Annotations) != 1 || cell.Annotations[0].Text != "a" {
		t.Errorf("Annotations = %+v", cell.Annotations)
	}
	if cell.Annotations[0].Position != spec.AnnotationSuperscript {
		t.Errorf("Position = %q, want the conventional default", cell.Annotations[0].Position)
	}
	if res.Doc.Sections[0].Blocks[0].Table.Caption == nil {
		t.Error("the caption did not resolve")
	}
}

// TestResolve_ConsumerTextWinsOverTheFormattedValue pins the boundary: the
// consumer has already decided how a number should read, and Vellum carries
// that decision rather than second-guessing it.
func TestResolve_ConsumerTextWinsOverTheFormattedValue(t *testing.T) {
	table := &spec.Table{
		ColumnHeaders: spec.HeaderTree{{Label: "Share", Span: 1}},
		Body: [][]spec.Cell{{{
			Value:  &spec.Value{Kind: spec.ValueNumber, Number: 0.02},
			Format: "0.0%",
			Text:   "*",
		}}},
	}
	res := resolveDoc(t, doc(spec.Block{Kind: spec.BlockTable, Table: table}),
		resolve.Options{Format: artifact.FormatDOCX})

	cell := res.Doc.Sections[0].Blocks[0].Table.Body[0][0]
	if cell.Text != "*" {
		t.Errorf("Text = %q, want the consumer's own %q", cell.Text, "*")
	}
	// The typed value survives, so a workbook target can still write a live
	// cell behind the suppressed display.
	if cell.Value == nil || cell.Value.Number != 0.02 {
		t.Error("the consumer's text discarded the typed value")
	}
}

func TestResolve_BadFormatCodeIsLocated(t *testing.T) {
	table := &spec.Table{
		ColumnHeaders: spec.HeaderTree{{Label: "x", Span: 1}},
		Body:          [][]spec.Cell{{{Value: &spec.Value{Kind: spec.ValueNumber}, Format: `0.0"`}}},
	}
	_, err := resolve.Resolve(context.Background(),
		doc(spec.Block{Kind: spec.BlockTable, Table: table}),
		resolve.Options{Format: artifact.FormatDOCX})

	if !verr.HasCode(err, verr.VELLUM_TABLE_FORMAT_INVALID) {
		t.Fatalf("error = %v, want VELLUM_TABLE_FORMAT_INVALID", err)
	}
	var coded *verr.CodedError
	if stderrors.As(err, &coded) {
		for _, key := range []string{"row", "column", "block_index"} {
			if _, ok := coded.Details[key]; !ok {
				t.Errorf("the error carries no %s", key)
			}
		}
	}
}

func TestResolve_DatesParseOnceHere(t *testing.T) {
	for _, in := range []string{"2026-09-01T00:00:00Z", "2026-09-01"} {
		table := &spec.Table{
			ColumnHeaders: spec.HeaderTree{{Label: "When", Span: 1}},
			Body:          [][]spec.Cell{{{Value: &spec.Value{Kind: spec.ValueDate, Date: in}, Format: "d mmmm yyyy"}}},
		}
		res := resolveDoc(t, doc(spec.Block{Kind: spec.BlockTable, Table: table}),
			resolve.Options{Format: artifact.FormatDOCX})

		cell := res.Doc.Sections[0].Blocks[0].Table.Body[0][0]
		if cell.Text != "1 September 2026" {
			t.Errorf("Date %q rendered as %q, want %q", in, cell.Text, "1 September 2026")
		}
	}
}

// TestResolve_CapabilityRejectionsComeBeforeAnyWork pins the ordering the
// library is arranged around: a rejected specification costs nothing beyond the
// check, and learns about the gap before any bytes exist.
func TestResolve_CapabilityRejectionsComeBeforeAnyWork(t *testing.T) {
	png := "data:image/png;base64," + base64.StdEncoding.EncodeToString(onePixelPNG)
	s := doc(spec.Block{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: png}})

	// xlsx rejects assets outright, as the matrix declares.
	_, err := resolve.Resolve(context.Background(), s, resolve.Options{Format: artifact.FormatXLSX})
	if !verr.HasCode(err, verr.VELLUM_CAPABILITY_REJECTED) {
		t.Fatalf("error = %v, want VELLUM_CAPABILITY_REJECTED", err)
	}
}

func TestResolve_RequiresAFormat(t *testing.T) {
	_, err := resolve.Resolve(context.Background(), doc(text("x")), resolve.Options{})
	if !verr.HasCode(err, verr.VELLUM_SPEC_INVALID) {
		t.Fatalf("error = %v, want VELLUM_SPEC_INVALID", err)
	}
}

func TestResolve_UnknownLayoutIsLocated(t *testing.T) {
	s := doc(text("x"))
	s.Sections[0].Layout = "no-such-layout"

	_, err := resolve.Resolve(context.Background(), s, resolve.Options{Format: artifact.FormatDOCX})
	if !verr.HasCode(err, verr.VELLUM_THEME_LAYOUT_NOT_FOUND) {
		t.Fatalf("error = %v, want VELLUM_THEME_LAYOUT_NOT_FOUND", err)
	}
	var coded *verr.CodedError
	if stderrors.As(err, &coded) && coded.Details["section_id"] != "s1" {
		t.Errorf("section_id = %v, want s1", coded.Details["section_id"])
	}
}

// TestResolve_IsDeterministic is the property everything downstream rests on.
// Warnings are included because they reach the envelope, and the envelope is
// compared byte for byte.
func TestResolve_IsDeterministic(t *testing.T) {
	png := "data:image/png;base64," + base64.StdEncoding.EncodeToString(onePixelPNG)
	s := doc(
		heading(1, "Title"),
		text("Body.", "stale", "flagged"),
		spec.Block{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: png}},
		spec.Block{Kind: spec.BlockNotes, Notes: &spec.Notes{Content: "A note."}},
		spec.Block{Kind: spec.BlockSpacer, Spacer: &spec.Spacer{Height: spec.Points(12)}},
		spec.Block{Kind: spec.BlockPageBreak, PageBreak: &spec.PageBreak{}},
	)

	first := describe(t, resolveDoc(t, s, resolve.Options{Format: artifact.FormatDOCX}))
	for range 200 {
		again := describe(t, resolveDoc(t, s, resolve.Options{Format: artifact.FormatDOCX}))
		if again != first {
			t.Fatalf("resolve is not stable:\n%s\n---\n%s", first, again)
		}
	}
}

// describe renders a result to a comparable string, warnings included.
func describe(t *testing.T, res *resolve.Result) string {
	t.Helper()
	out := res.Doc.ThemeID + "|"
	for _, f := range res.Doc.Fonts {
		out += string(f.Role) + ":" + f.Family + ":" + string(f.Embed) + ";"
	}
	for _, a := range res.Doc.Assets {
		out += a.Hash + ":" + a.MediaType + ";"
	}
	for _, s := range res.Doc.Sections {
		for _, b := range s.Blocks {
			out += string(b.Kind) + ","
			if b.Paragraph != nil {
				for _, r := range b.Paragraph.Runs {
					out += r.Text + "/" + r.Style.Color + ";"
				}
			}
			if b.Asset != nil {
				out += string(rune(b.Asset.AssetIndex)) + ";"
			}
		}
	}
	out += "|"
	for _, w := range res.Warnings {
		out += string(w.Code) + ";"
	}
	return out
}
