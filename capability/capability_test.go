package capability_test

import (
	"testing"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/capability"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/spec"
)

// TestCapabilityMatrixComplete asserts by cardinality rather than by
// inspection, so adding a feature or a format fails the build until every cell
// is declared. A pair with no entry is the failure this whole package exists
// to prevent: a consumer discovering a gap from a support ticket.
func TestCapabilityMatrixComplete(t *testing.T) {
	features := capability.AllFeatures()
	formats := artifact.AllFormats()
	all := capability.All()

	want := len(features) * len(formats)
	if len(all) != want {
		t.Errorf("matrix has %d entries, want %d (%d features x %d formats)", len(all), want, len(features), len(formats))
	}

	seen := make(map[[2]string]int, want)
	for _, e := range all {
		seen[[2]string{string(e.Feature), string(e.Format)}]++
	}
	for _, f := range features {
		for _, fm := range formats {
			key := [2]string{string(f), string(fm)}
			switch seen[key] {
			case 1:
			case 0:
				t.Errorf("no entry declares what %q does in %q", f, fm)
			default:
				t.Errorf("%q in %q is declared %d times", f, fm, seen[key])
			}
		}
	}
}

// TestCapabilityEveryBlockKindHasAFeature keeps the two registries in step: a
// new block kind must arrive with a feature covering it.
func TestCapabilityEveryBlockKindHasAFeature(t *testing.T) {
	for _, kind := range spec.AllBlockKinds() {
		f := capability.FeatureForBlockKind(kind)
		found := false
		for _, known := range capability.AllFeatures() {
			if known == f {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("block kind %q maps to feature %q, which is not in the feature registry", kind, f)
		}
	}
}

// TestCapabilityCodesRegistered checks that every code the matrix names is a
// real one. A matrix row promising an error that does not exist would fail at
// the worst possible moment.
func TestCapabilityCodesRegistered(t *testing.T) {
	for _, e := range capability.All() {
		if e.Code == "" {
			continue
		}
		if _, ok := verr.ParseCode(string(e.Code)); !ok {
			t.Errorf("%q in %q names an unregistered code %q", e.Feature, e.Format, e.Code)
		}
	}
}

// TestCapabilityOutcomesAreWellFormed pins the invariants that make an entry
// actionable. "Degrades" on its own tells a consumer nothing they can plan
// around; naming the alternative does.
func TestCapabilityOutcomesAreWellFormed(t *testing.T) {
	for _, e := range capability.All() {
		switch e.Outcome {
		case capability.Renders:
			if e.Degrade != "" {
				t.Errorf("%q in %q renders but names a degradation", e.Feature, e.Format)
			}
			if e.Code != "" {
				t.Errorf("%q in %q renders but names a code", e.Feature, e.Format)
			}
		case capability.Degrades:
			if e.Degrade == "" {
				t.Errorf("%q in %q degrades but does not say what to; a consumer cannot plan around \"degrades\"", e.Feature, e.Format)
			}
			if e.Code == "" {
				t.Errorf("%q in %q degrades but names no warning code", e.Feature, e.Format)
			}
		case capability.Rejects:
			if e.Code == "" {
				t.Errorf("%q in %q is rejected but names no error code", e.Feature, e.Format)
			}
		default:
			t.Errorf("%q in %q has an unknown outcome %q", e.Feature, e.Format, e.Outcome)
		}
	}
}

// TestCapabilityKnownDecisions pins the outcomes the design argued for
// explicitly. These are the ones a future change is most likely to get wrong
// by accident.
func TestCapabilityKnownDecisions(t *testing.T) {
	tests := []struct {
		feature capability.Feature
		format  artifact.Format
		outcome capability.Outcome
		why     string
	}{
		{capability.FeatureBlockNotes, artifact.FormatPPTX, capability.Renders,
			"a deck is the only format with a native speaker-note channel"},
		{capability.FeatureBlockNotes, artifact.FormatDOCX, capability.Degrades,
			"a flowing document has no note channel, so a note becomes a footnote"},
		{capability.FeatureBlockNotes, artifact.FormatXLSX, capability.Degrades,
			"a workbook turns a note into a cell comment"},
		{capability.FeatureBlockAsset, artifact.FormatXLSX, capability.Rejects,
			"a workbook is where a reader goes for the numbers behind a chart, not for the chart"},
		{capability.FeatureAssetSVG, artifact.FormatPDF, capability.Rejects,
			"PDF has no SVG mechanism, and Vellum will neither rasterise nor ship a second renderer"},
		{capability.FeatureAssetSVG, artifact.FormatDOCX, capability.Degrades,
			"Word reads an SVG only alongside a raster blip, which the caller supplies"},
		{capability.FeatureFill, artifact.FormatPDF, capability.Rejects,
			"fill edits an OPC package surgically, and a PDF is not one"},
		{capability.FeatureOverflowContinue, artifact.FormatPPTX, capability.Renders,
			"a table longer than a slide continues with its headers repeated"},
		{capability.FeatureOverflowContinue, artifact.FormatDOCX, capability.Degrades,
			"a flowing format paginates itself; a split Vellum computed would disagree with Word's"},
		{capability.FeatureFontEmbedSubset, artifact.FormatPDF, capability.Renders,
			"PDF/A-2b requires every font embedded"},
		{capability.FeatureFontOutlinesCFF, artifact.FormatPDF, capability.Degrades,
			"Vellum subsets glyf and not CFF, so a CFF face is embedded whole"},
	}
	for _, tt := range tests {
		e, ok := capability.Lookup(tt.feature, tt.format)
		if !ok {
			t.Errorf("no entry for %q in %q", tt.feature, tt.format)
			continue
		}
		if e.Outcome != tt.outcome {
			t.Errorf("%q in %q = %q, want %q — %s", tt.feature, tt.format, e.Outcome, tt.outcome, tt.why)
		}
	}
}

func TestAcceptedMedia(t *testing.T) {
	tests := []struct {
		format artifact.Format
		want   []string
	}{
		{artifact.FormatDOCX, []string{"image/jpeg", "image/png", "image/svg+xml"}},
		{artifact.FormatPPTX, []string{"image/jpeg", "image/png", "image/svg+xml"}},
		{artifact.FormatPDF, []string{"image/jpeg", "image/png"}},
		{artifact.FormatXLSX, nil},
	}
	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			got := capability.AcceptedMedia(tt.format)
			if len(got) != len(tt.want) {
				t.Fatalf("AcceptedMedia = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("AcceptedMedia[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestProfile_IsDerivedFromTheMatrix(t *testing.T) {
	for _, format := range artifact.AllFormats() {
		profile := capability.Profile(format)
		for _, f := range profile {
			e, ok := capability.Lookup(f, format)
			if !ok || e.Outcome != capability.Renders {
				t.Errorf("Profile(%q) includes %q, which the matrix does not say renders", format, f)
			}
		}
		for _, e := range capability.ForFormat(format) {
			if e.Outcome != capability.Renders {
				continue
			}
			found := false
			for _, f := range profile {
				if f == e.Feature {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Profile(%q) omits %q, which the matrix says renders", format, e.Feature)
			}
		}
	}
}

func TestCheck_AcceptsWhatEachFormatRenders(t *testing.T) {
	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Sections: []spec.Section{{Blocks: []spec.Block{
			{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 1, Content: "Title"}},
			{Kind: spec.BlockText, Text: &spec.Text{Content: "Prose"}},
			{Kind: spec.BlockTable, Table: &spec.Table{
				ColumnHeaders: spec.HeaderTree{{Label: "A"}, {Label: "B"}},
				Body:          [][]spec.Cell{{{Text: "1"}, {Text: "2"}}},
			}},
		}}},
	}
	for _, format := range artifact.AllFormats() {
		res, err := capability.Check(s, format)
		if err != nil {
			t.Fatalf("Check(%q): %v", format, err)
		}
		if format == artifact.FormatXLSX {
			// Headings and prose degrade into cells in a workbook.
			if len(res.Degradations) == 0 {
				t.Errorf("Check(xlsx) reported no degradations for a heading and a paragraph")
			}
			continue
		}
		if !res.OK() {
			t.Errorf("Check(%q) rejected a document of headings, prose and a flat table: %v", format, res.Rejections)
		}
	}
}

// TestCheck_RejectsAssetsInWorkbooks exercises the whole loop: a declared
// rejection surfaces as a located, coded error before any writer runs.
func TestCheck_RejectsAssetsInWorkbooks(t *testing.T) {
	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Sections: []spec.Section{
			{ID: "intro", Blocks: []spec.Block{{Kind: spec.BlockText, Text: &spec.Text{Content: "x"}}}},
			{ID: "charts", Blocks: []spec.Block{
				{Kind: spec.BlockText, Text: &spec.Text{Content: "y"}},
				{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: "chart-1"}},
			}},
		},
	}

	res, err := capability.Check(s, artifact.FormatXLSX)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.OK() {
		t.Fatal("Check accepted an asset block in a workbook")
	}
	if len(res.Rejections) != 1 {
		t.Fatalf("got %d rejections, want 1: %v", len(res.Rejections), res.Rejections)
	}

	f := res.Rejections[0]
	if f.Feature != capability.FeatureBlockAsset {
		t.Errorf("feature = %q", f.Feature)
	}
	if f.SectionIndex != 1 || f.BlockIndex != 1 || f.SectionID != "charts" {
		t.Errorf("fault located at section %d block %d (%q), want section 1 block 1 (\"charts\")", f.SectionIndex, f.BlockIndex, f.SectionID)
	}

	if !verr.HasCode(res.Err(), verr.VELLUM_CAPABILITY_REJECTED) {
		t.Errorf("Err() = %v, want VELLUM_CAPABILITY_REJECTED", res.Err())
	}
}

// TestCheck_ReportsEveryRejectionAtOnce keeps a caller from fixing a document
// one refusal per run.
func TestCheck_ReportsEveryRejectionAtOnce(t *testing.T) {
	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Sections: []spec.Section{{Blocks: []spec.Block{
			{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: "a"}},
			{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: "b"}},
			{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: "c"}},
		}}},
	}
	res, err := capability.Check(s, artifact.FormatXLSX)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(res.Rejections) != 3 {
		t.Errorf("got %d rejections, want 3", len(res.Rejections))
	}
}

// TestCheck_OnlyReportsFeaturesActuallyUsed covers a warning-channel failure
// mode: reporting a hierarchical-header degradation for a flat table would be
// a warning nobody can act on, and would teach consumers to ignore the channel.
func TestCheck_OnlyReportsFeaturesActuallyUsed(t *testing.T) {
	flat := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Sections: []spec.Section{{Blocks: []spec.Block{{Kind: spec.BlockTable, Table: &spec.Table{
			ColumnHeaders: spec.HeaderTree{{Label: "A"}, {Label: "B"}},
			Body:          [][]spec.Cell{{{Text: "1"}, {Text: "2"}}},
		}}}}},
	}
	res, err := capability.Check(flat, artifact.FormatXLSX)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, d := range res.Degradations {
		if d.Feature == capability.FeatureTableCellAnnotation {
			t.Error("a flat, unannotated table reported an annotation degradation")
		}
	}

	annotated := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Sections: []spec.Section{{Blocks: []spec.Block{{Kind: spec.BlockTable, Table: &spec.Table{
			ColumnHeaders: spec.HeaderTree{{Label: "A"}, {Label: "B"}},
			Body: [][]spec.Cell{{
				{Text: "1", Annotations: []spec.Annotation{{Text: "a"}}},
				{Text: "2"},
			}},
		}}}}},
	}
	res, err = capability.Check(annotated, artifact.FormatXLSX)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	found := false
	for _, d := range res.Degradations {
		if d.Feature == capability.FeatureTableCellAnnotation {
			found = true
			if d.Degrade == "" {
				t.Error("the annotation degradation does not name its alternative")
			}
		}
	}
	if !found {
		t.Error("an annotated table in a workbook reported no annotation degradation")
	}
}

func TestCheck_RejectsUnknownFormat(t *testing.T) {
	s := &spec.Spec{Sections: []spec.Section{{Blocks: []spec.Block{
		{Kind: spec.BlockText, Text: &spec.Text{Content: "x"}},
	}}}}
	if _, err := capability.Check(s, artifact.Format("rtf")); !verr.HasCode(err, verr.VELLUM_SPEC_INVALID) {
		t.Errorf("error = %v, want VELLUM_SPEC_INVALID", err)
	}
}

func TestFormat_ParseAndClassify(t *testing.T) {
	for _, in := range []string{"docx", "DOCX", ".docx", " .DocX "} {
		got, ok := artifact.ParseFormat(in)
		if !ok || got != artifact.FormatDOCX {
			t.Errorf("ParseFormat(%q) = %q, %v", in, got, ok)
		}
	}
	if _, ok := artifact.ParseFormat("rtf"); ok {
		t.Error("ParseFormat accepted an unknown format")
	}

	for _, f := range []artifact.Format{artifact.FormatDOCX, artifact.FormatXLSX, artifact.FormatPPTX} {
		if !f.IsOOXML() {
			t.Errorf("%q should be an OOXML format", f)
		}
	}
	if artifact.FormatPDF.IsOOXML() {
		t.Error("PDF is not an OOXML format; it shares none of the packaging substrate")
	}
}
