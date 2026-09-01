// Package capability declares what each feature does in each output format.
//
// There are exactly three legitimate outcomes: a feature renders, it degrades
// to a stated alternative, or it is rejected at validate time. The matrix is
// data, queryable before a render is attempted, and it is the contract the
// format writers are implemented against — not a document written afterwards
// to describe whatever they happened to do.
//
// # Why this is a registry and not prose
//
// A consumer scheduling an unattended job needs to know whether notes will
// drop, become footnotes, or fail, *before* the job runs. If they learn it from
// a support ticket, this package has failed at its only purpose.
//
// A completeness gate asserts the matrix has an entry for every (feature,
// format) pair, by cardinality rather than by inspection, so adding a feature
// fails the build until every format declares what it does with it.
package capability

import (
	"sort"
	"strings"

	"github.com/frankbardon/vellum/artifact"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/spec"
)

// Outcome is the declared behaviour of one feature in one format.
type Outcome string

const (
	// Renders means the feature is native to the format.
	Renders Outcome = "renders"

	// Degrades means the feature becomes the stated alternative, with a
	// warning naming it.
	Degrades Outcome = "degrades"

	// Rejects means the feature is a hard error at validate time, before any
	// bytes are written.
	Rejects Outcome = "rejects"
)

// AllOutcomes returns the outcomes, in declaration order.
func AllOutcomes() []Outcome { return []Outcome{Renders, Degrades, Rejects} }

// Feature is the axis the matrix is indexed by.
//
// It is wider than the block vocabulary, because the same declaration question
// applies to table properties, asset media types, font embedding, overflow and
// fill. Anything a consumer can observe belongs here.
type Feature string

// Block features, one per block kind. Named by convention so a feature can be
// derived from a kind without a lookup table that could fall out of step.
const (
	FeatureBlockHeading   Feature = "block.heading"
	FeatureBlockText      Feature = "block.text"
	FeatureBlockAsset     Feature = "block.asset"
	FeatureBlockTable     Feature = "block.table"
	FeatureBlockPageBreak Feature = "block.page_break"
	FeatureBlockNotes     Feature = "block.notes"
	FeatureBlockSpacer    Feature = "block.spacer"
)

// Table features. A table is not one capability: a format may render a flat
// grid perfectly and have nowhere to put a nested banner.
const (
	FeatureTableHierarchicalHeaders Feature = "table.hierarchical_headers"
	FeatureTableCellAnnotation      Feature = "table.cell_annotation"
	FeatureTableMargins             Feature = "table.margins"
	FeatureTableCellSpan            Feature = "table.cell_span"
)

// Asset media features. These are matrix rows rather than a hard-coded list
// because the answer genuinely differs per format — PDF has no SVG mechanism
// at all, and XLSX takes no assets.
const (
	FeatureAssetPNG  Feature = "asset.media.image/png"
	FeatureAssetJPEG Feature = "asset.media.image/jpeg"
	FeatureAssetSVG  Feature = "asset.media.image/svg+xml"
)

// Font features.
//
// The two embed modes are what a theme asks for. The outline format is a
// property of the face itself, and it is a separate row because it changes the
// answer independently: a PDF honours a subset request for TrueType outlines
// and cannot honour one for CFF, and no single outcome on font.embed.subset
// states both.
const (
	FeatureFontEmbedSubset Feature = "font.embed.subset"
	FeatureFontEmbedWhole  Feature = "font.embed.whole"
	FeatureFontOutlinesCFF Feature = "font.outlines.cff"
)

// Overflow and fill.
const (
	FeatureOverflowContinue Feature = "overflow.continue_repeat_headers"
	FeatureFill             Feature = "fill"
)

// allFeatures is the registry, in declaration order.
var allFeatures = []Feature{
	FeatureBlockHeading,
	FeatureBlockText,
	FeatureBlockAsset,
	FeatureBlockTable,
	FeatureBlockPageBreak,
	FeatureBlockNotes,
	FeatureBlockSpacer,

	FeatureTableHierarchicalHeaders,
	FeatureTableCellAnnotation,
	FeatureTableMargins,
	FeatureTableCellSpan,

	FeatureAssetPNG,
	FeatureAssetJPEG,
	FeatureAssetSVG,

	FeatureFontEmbedSubset,
	FeatureFontEmbedWhole,
	FeatureFontOutlinesCFF,

	FeatureOverflowContinue,
	FeatureFill,
}

// AllFeatures returns a copy of the feature registry.
func AllFeatures() []Feature {
	out := make([]Feature, len(allFeatures))
	copy(out, allFeatures)
	return out
}

// FeatureForBlockKind returns the feature covering a block kind.
func FeatureForBlockKind(k spec.BlockKind) Feature { return Feature("block." + string(k)) }

// Entry is one cell of the matrix.
type Entry struct {
	// Feature and Format identify the cell.
	Feature Feature         `json:"feature"`
	Format  artifact.Format `json:"format"`

	// Outcome is the declared behaviour.
	Outcome Outcome `json:"outcome"`

	// Degrade names what the feature becomes, and is required when Outcome is
	// Degrades. "Degrades" on its own tells a consumer nothing they can plan
	// around; "becomes a footnote" does.
	Degrade string `json:"degrade,omitempty"`

	// Code is the error raised on Rejects or the warning emitted on Degrades.
	// Always a registered code, checked by a gate.
	Code verr.Code `json:"code,omitempty"`

	// Note is prose for a human reading the matrix.
	Note string `json:"note,omitempty"`
}

// Matrix is the full set of entries, sorted by format then feature.
type Matrix []Entry

// All returns a copy of the matrix.
func All() Matrix {
	out := make(Matrix, len(matrix))
	copy(out, matrix)
	return out
}

// Lookup returns the declared entry for a pair.
func Lookup(f Feature, format artifact.Format) (Entry, bool) {
	for _, e := range matrix {
		if e.Feature == f && e.Format == format {
			return e, true
		}
	}
	return Entry{}, false
}

// ForFormat returns every entry for one format, in feature declaration order.
func ForFormat(format artifact.Format) Matrix {
	out := make(Matrix, 0, len(allFeatures))
	for _, f := range allFeatures {
		if e, ok := Lookup(f, format); ok {
			out = append(out, e)
		}
	}
	return out
}

// Profile returns the features a format renders natively — the conformance
// allowlist, projected out of the matrix rather than maintained beside it.
//
// One registry, not a matrix and a separate set of profiles that can disagree.
func Profile(format artifact.Format) []Feature {
	var out []Feature
	for _, f := range allFeatures {
		if e, ok := Lookup(f, format); ok && e.Outcome == Renders {
			out = append(out, f)
		}
	}
	return out
}

// AcceptedMedia returns the asset media types a format can embed, in
// declaration order.
//
// Derived from the matrix rather than listed separately, so the answer a
// resolver is given and the answer the matrix publishes cannot diverge.
func AcceptedMedia(format artifact.Format) []string {
	const prefix = "asset.media."
	var out []string
	for _, f := range allFeatures {
		media, ok := strings.CutPrefix(string(f), prefix)
		if !ok {
			continue
		}
		if e, ok := Lookup(f, format); ok && e.Outcome != Rejects {
			out = append(out, media)
		}
	}
	sort.Strings(out)
	return out
}
