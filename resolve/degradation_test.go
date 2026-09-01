package resolve_test

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/asset"
	"github.com/frankbardon/vellum/capability"
	"github.com/frankbardon/vellum/resolve"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/theme"
)

// exercise is how a declared degradation is provoked and checked.
//
// Either a specification that triggers it, or a stated reason no specification
// can. There is no third option, and that is the point: a row saying a feature
// degrades is a promise to the consumer that they will be told, and a promise
// nothing can keep should not be in the matrix without saying so.
type exercise struct {
	// spec builds a specification that uses the feature, or is nil when the
	// row is unreportable.
	spec func(t *testing.T) (*spec.Spec, resolve.Options)

	// unreportable states why no specification can provoke a warning for this
	// row. Set only when spec is nil.
	unreportable string
}

// TestCapabilityDegradationsAreReported walks every row of the matrix whose
// outcome is "degrades" and requires that a consumer would learn about it.
//
// This is the gate behind "declared, not emergent". The matrix already promises
// that every observable behaviour is a row before it is code; this is the other
// half — that a row promising a degradation produces a warning a consumer can
// actually read, rather than a sentence in a table nothing enforces.
//
// A row with neither an exercise nor a stated reason fails the build. That is
// deliberately awkward: adding a degrading row is a commitment to reporting it,
// and the awkward moment belongs at the point the row is added rather than at
// the point a consumer notices their chart became a raster and nobody said so.
func TestCapabilityDegradationsAreReported(t *testing.T) {
	exercises := degradationExercises()

	for _, entry := range capability.All() {
		if entry.Outcome != capability.Degrades {
			continue
		}

		key := string(entry.Feature) + "/" + string(entry.Format)
		ex, ok := exercises[key]
		if !ok {
			t.Errorf("%s degrades in %s and nothing here exercises it.\n"+
				"A degrading row is a promise that the consumer will be told. Add a specification that "+
				"provokes the warning, or state why none can.", entry.Feature, entry.Format)
			continue
		}

		t.Run(key, func(t *testing.T) {
			if ex.spec == nil {
				if ex.unreportable == "" {
					t.Fatal("neither exercised nor explained")
				}
				t.Skipf("unreportable: %s", ex.unreportable)
			}

			s, opts := ex.spec(t)
			opts.Format = entry.Format

			res, err := resolve.Resolve(context.Background(), s, opts)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if !warnsAbout(res, entry.Feature) {
				t.Fatalf("no warning names %q.\n\nWhat was reported:\n%s",
					entry.Feature, warningSummary(res))
			}
		})
	}

	assertNoOrphanExercises(t, exercises)
}

// warnsAbout reports whether any warning names the feature.
//
// By the feature name in the details rather than by the code, because several
// degradations share VELLUM_CAPABILITY_DEGRADED and a check on the code alone
// would pass on somebody else's warning.
func warnsAbout(res *resolve.Result, feature capability.Feature) bool {
	for _, w := range res.Warnings {
		if value, ok := w.Detail("feature"); ok {
			if name, isString := value.(string); isString && name == string(feature) {
				return true
			}
		}
	}
	return false
}

func warningSummary(res *resolve.Result) string {
	if len(res.Warnings) == 0 {
		return "    (nothing at all)"
	}
	var b strings.Builder
	for _, w := range res.Warnings {
		b.WriteString("    " + string(w.Code))
		if value, ok := w.Detail("feature"); ok {
			b.WriteString(" feature=")
			if name, isString := value.(string); isString {
				b.WriteString(name)
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// assertNoOrphanExercises fails when the table names a row the matrix no longer
// carries.
//
// A row that stopped degrading would otherwise leave its exercise behind, still
// listed, still passing, checking nothing.
func assertNoOrphanExercises(t *testing.T, exercises map[string]exercise) {
	t.Helper()

	live := make(map[string]bool)
	for _, entry := range capability.All() {
		if entry.Outcome == capability.Degrades {
			live[string(entry.Feature)+"/"+string(entry.Format)] = true
		}
	}
	for key := range exercises {
		if !live[key] {
			t.Errorf("%s is exercised here and no longer degrades in the matrix", key)
		}
	}
}

// degradationExercises is the table. One entry per degrading row.
func degradationExercises() map[string]exercise {
	// A block whose kind alone provokes the degradation. Every block-kind row
	// is reported by the capability check before resolution begins, so the
	// block is all these need.
	byBlock := func(b spec.Block) func(*testing.T) (*spec.Spec, resolve.Options) {
		return func(*testing.T) (*spec.Spec, resolve.Options) {
			return doc(b), resolve.Options{}
		}
	}

	notes := spec.Block{Kind: spec.BlockNotes, Notes: &spec.Notes{Content: "n"}}
	pageBreak := spec.Block{Kind: spec.BlockPageBreak, PageBreak: &spec.PageBreak{}}
	spacer := spec.Block{Kind: spec.BlockSpacer, Spacer: &spec.Spacer{Height: spec.Points(12)}}

	// The embed rows need a theme whose faces are embeddable and a format that
	// carries no font programs.
	embed := func(mode theme.EmbedMode) func(*testing.T) (*spec.Spec, resolve.Options) {
		return func(t *testing.T) (*spec.Spec, resolve.Options) {
			th := embeddableTheme(t, mode)
			p, err := theme.NewStaticProvider(th)
			if err != nil {
				t.Fatalf("NewStaticProvider: %v", err)
			}
			s := doc(text("x"))
			s.Theme = th.ID
			return s, resolve.Options{Themes: p, Assets: fontStore()}
		}
	}

	// The CFF row needs a program that declares CFF outlines, in a format that
	// embeds one at all.
	cff := func(t *testing.T) (*spec.Spec, resolve.Options) {
		th := embeddableTheme(t, theme.EmbedAuto)
		p, err := theme.NewStaticProvider(th)
		if err != nil {
			t.Fatalf("NewStaticProvider: %v", err)
		}
		s := doc(text("x"))
		s.Theme = th.ID
		return s, resolve.Options{Themes: p, Assets: openTypeFontStore()}
	}

	svg := func(*testing.T) (*spec.Spec, resolve.Options) {
		return doc(
			spec.Block{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: svgURI()}},
		), resolve.Options{}
	}

	table := func(*testing.T) (*spec.Spec, resolve.Options) {
		return doc(spec.Block{Kind: spec.BlockTable, Table: &spec.Table{
			ColumnHeaders: spec.HeaderTree{{Label: "h"}},
			Body:          [][]spec.Cell{{{Text: "1"}}},
		}}), resolve.Options{}
	}

	annotated := func(*testing.T) (*spec.Spec, resolve.Options) {
		return doc(spec.Block{Kind: spec.BlockTable, Table: &spec.Table{
			ColumnHeaders: spec.HeaderTree{{Label: "h"}},
			Body: [][]spec.Cell{{{Text: "1",
				Annotations: []spec.Annotation{{Text: "a"}}}}},
		}}), resolve.Options{}
	}

	// The unreportable reason shared by the OOXML CFF rows.
	const cffUnreportable = "no font program is loaded for a format that embeds none, " +
		"so the outline format is never seen — and the degradation the row names, " +
		"the family referenced by name, is the one font.embed.* already reports"

	return map[string]exercise{
		// ---- DOCX ----
		"block.notes/docx":                      {spec: byBlock(notes)},
		"asset.media.image/svg+xml/docx":        {spec: svg},
		"font.embed.subset/docx":                {spec: embed(theme.EmbedSubset)},
		"font.embed.whole/docx":                 {spec: embed(theme.EmbedWhole)},
		"font.outlines.cff/docx":                {unreportable: cffUnreportable},
		"overflow.continue_repeat_headers/docx": {spec: table},

		// ---- XLSX ----
		"block.heading/xlsx":                    {spec: byBlock(heading(1, "h"))},
		"block.text/xlsx":                       {spec: byBlock(text("t"))},
		"block.page_break/xlsx":                 {spec: byBlock(pageBreak)},
		"block.notes/xlsx":                      {spec: byBlock(notes)},
		"block.spacer/xlsx":                     {spec: byBlock(spacer)},
		"table.cell_annotation/xlsx":            {spec: annotated},
		"font.embed.subset/xlsx":                {spec: embed(theme.EmbedSubset)},
		"font.embed.whole/xlsx":                 {spec: embed(theme.EmbedWhole)},
		"font.outlines.cff/xlsx":                {unreportable: cffUnreportable},
		"overflow.continue_repeat_headers/xlsx": {spec: table},

		// ---- PPTX ----
		"block.page_break/pptx":          {spec: byBlock(pageBreak)},
		"asset.media.image/svg+xml/pptx": {spec: svg},
		"font.embed.subset/pptx":         {spec: embed(theme.EmbedSubset)},
		"font.embed.whole/pptx":          {spec: embed(theme.EmbedWhole)},
		"font.outlines.cff/pptx":         {unreportable: cffUnreportable},

		// ---- PDF ----
		"block.notes/pdf": {spec: func(t *testing.T) (*spec.Spec, resolve.Options) {
			s, opts := pdfReady(t)
			s.Sections[0].Blocks = []spec.Block{notes}
			return s, opts
		}},
		"asset.alt_text/pdf": {spec: func(t *testing.T) (*spec.Spec, resolve.Options) {
			s, opts := pdfReady(t)
			s.Sections[0].Blocks = []spec.Block{
				{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: pngURI(), AltText: "a chart"}},
			}
			return s, opts
		}},
		"text.bold/pdf": {spec: func(t *testing.T) (*spec.Spec, resolve.Options) {
			s, opts := pdfReady(t)
			s.Sections[0].Blocks = []spec.Block{heading(1, "bold by the theme")}
			return s, opts
		}},
		"text.italic/pdf": {spec: func(t *testing.T) (*spec.Spec, resolve.Options) {
			s, opts := pdfReady(t)
			s.Sections[0].Blocks = []spec.Block{text("emphasised", "flagged")}
			return s, opts
		}},
		"font.outlines.cff/pdf": {spec: cff},
	}
}

// pdfReady is a specification and options a PDF render will accept: a theme
// whose faces are embeddable, because PDF/A-2b refuses one whose faces are not.
func pdfReady(t *testing.T) (*spec.Spec, resolve.Options) {
	t.Helper()

	th := embeddableTheme(t, theme.EmbedAuto)
	p, err := theme.NewStaticProvider(th)
	if err != nil {
		t.Fatalf("NewStaticProvider: %v", err)
	}
	s := doc(text("x"))
	s.Theme = th.ID
	return s, resolve.Options{Themes: p, Assets: mixedStore()}
}

// mixedStore serves both the font programs a theme names and an inline picture.
func mixedStore() asset.Resolver {
	entries := map[string]asset.Asset{}
	for _, role := range theme.AllFontRoles() {
		entries["font/"+string(role)] = asset.Asset{
			MediaType: "font/ttf",
			Bytes:     []byte("font program for " + string(role)),
		}
	}
	return asset.NewMap(entries)
}

// openTypeFontStore serves programs that declare CFF outlines.
//
// Four bytes of tag and nothing else. Resolution reads the tag and no more, so
// a fixture carrying a real CFF table would be claiming a guarantee this layer
// does not make.
func openTypeFontStore() asset.Resolver {
	entries := map[string]asset.Asset{}
	for _, role := range theme.AllFontRoles() {
		entries["font/"+string(role)] = asset.Asset{
			MediaType: "font/otf",
			Bytes:     append([]byte("OTTO"), []byte(" outlines for "+string(role))...),
		}
	}
	return asset.NewMap(entries)
}

func svgURI() string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(wideSVG))
}

func pngURI() string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(onePixelPNG)
}
