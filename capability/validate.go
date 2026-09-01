package capability

import (
	"github.com/frankbardon/vellum/artifact"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/spec"
)

// Fault is one problem found while checking a specification against a format.
type Fault struct {
	// Feature is what the specification used.
	Feature Feature `json:"feature"`

	// Outcome is what the matrix says the format does with it.
	Outcome Outcome `json:"outcome"`

	// Degrade names the alternative, when the outcome is Degrades.
	Degrade string `json:"degrade,omitempty"`

	// Code is the error or warning code the matrix declared.
	Code verr.Code `json:"code"`

	// Where locates the offending block.
	SectionIndex int    `json:"section_index"`
	SectionID    string `json:"section_id,omitempty"`
	BlockIndex   int    `json:"block_index"`
	Kind         string `json:"kind"`
}

// Result is the outcome of checking a specification against a format.
type Result struct {
	// Rejections are hard failures. A specification with any of these cannot
	// be rendered to the format.
	Rejections []Fault `json:"rejections,omitempty"`

	// Degradations are the features that will become something else. They are
	// not failures; they are the warnings the envelope carries, reported so
	// that no degradation is ever silent.
	Degradations []Fault `json:"degradations,omitempty"`
}

// OK reports whether the specification can be rendered to the format.
func (r Result) OK() bool { return len(r.Rejections) == 0 }

// Err returns a coded error describing every rejection, or nil.
//
// Every rejection is reported at once rather than the first. A caller fixing a
// document one refusal per run is a caller having a bad afternoon, and an
// agent doing it is burning a turn apiece.
func (r Result) Err() error {
	if r.OK() {
		return nil
	}
	faults := make([]any, 0, len(r.Rejections))
	for _, f := range r.Rejections {
		faults = append(faults, map[string]any{
			"feature":       string(f.Feature),
			"kind":          f.Kind,
			"section_index": f.SectionIndex,
			"section_id":    f.SectionID,
			"block_index":   f.BlockIndex,
			"code":          string(f.Code),
		})
	}
	return verr.NewCodedErrorWithDetails(verr.VELLUM_CAPABILITY_REJECTED,
		"the specification uses features the target format does not render",
		map[string]any{"rejections": faults, "rejection_count": len(faults)})
}

// Check walks a specification against the matrix for one format.
//
// It answers before a render is attempted and without invoking a writer, which
// is the point: a scheduled job can ask whether it will work, rather than
// finding out when it does not.
func Check(s *spec.Spec, format artifact.Format) (Result, error) {
	var res Result
	if s == nil {
		return res, verr.NewCodedError(verr.VELLUM_SPEC_INVALID, "specification is nil")
	}
	if !artifact.ValidFormat(format) {
		return res, verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_INVALID,
			"unknown output format", map[string]any{"format": string(format)})
	}

	for si := range s.Sections {
		sec := &s.Sections[si]
		for bi := range sec.Blocks {
			b := &sec.Blocks[bi]
			for _, feature := range featuresUsedBy(b) {
				entry, ok := Lookup(feature, format)
				if !ok {
					return res, verr.NewCodedErrorWithDetails(verr.VELLUM_CAPABILITY_UNDECLARED,
						"the capability matrix has no entry for this feature and format",
						map[string]any{"feature": string(feature), "format": string(format)})
				}
				if entry.Outcome == Renders {
					continue
				}
				fault := Fault{
					Feature:      feature,
					Outcome:      entry.Outcome,
					Degrade:      entry.Degrade,
					Code:         entry.Code,
					SectionIndex: si,
					SectionID:    sec.ID,
					BlockIndex:   bi,
					Kind:         string(b.Kind),
				}
				if entry.Outcome == Rejects {
					res.Rejections = append(res.Rejections, fault)
				} else {
					res.Degradations = append(res.Degradations, fault)
				}
			}
		}
	}
	return res, nil
}

// featuresUsedBy reports which features a block actually exercises.
//
// A table block does not use every table feature — it uses the ones its
// content requires. Reporting a hierarchical-header degradation for a flat
// table would be a warning the consumer cannot act on and would teach them to
// ignore the channel.
func featuresUsedBy(b *spec.Block) []Feature {
	out := []Feature{FeatureForBlockKind(b.Kind)}

	if b.Kind != spec.BlockTable || b.Table == nil {
		return out
	}
	t := b.Table

	if hasChildren(t.ColumnHeaders) || hasChildren(t.RowHeaders) {
		out = append(out, FeatureTableHierarchicalHeaders)
	}

	var annotated, margined, spanned bool
	for r := range t.Body {
		for c := range t.Body[r] {
			cell := &t.Body[r][c]
			if len(cell.Annotations) > 0 {
				annotated = true
			}
			if cell.Class == spec.CellMargin || cell.Class == spec.CellTotal {
				margined = true
			}
			if cell.RowSpan > 1 || cell.ColSpan > 1 {
				spanned = true
			}
		}
	}
	if annotated {
		out = append(out, FeatureTableCellAnnotation)
	}
	if margined {
		out = append(out, FeatureTableMargins)
	}
	if spanned {
		out = append(out, FeatureTableCellSpan)
	}
	return out
}

func hasChildren(t spec.HeaderTree) bool {
	for i := range t {
		if len(t[i].Children) > 0 {
			return true
		}
	}
	return false
}
