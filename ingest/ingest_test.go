package ingest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/doc"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/ingest"
	"github.com/frankbardon/vellum/resolve"
	"github.com/frankbardon/vellum/spec"
)

// simple is a one-level-by-one-level crosstab: two regions by two genders,
// counts only, no margins.
const simple = `{
  "row_header": {"fields": ["region"], "types": ["GROUP_CATEGORY"]},
  "column_header": {"fields": ["gender"], "types": ["GROUP_CATEGORY"]},
  "row_keys": [["North"], ["South"]],
  "column_keys": [["Female"], ["Male"]],
  "cells": [
    [{"value": 12, "present": true}, {"value": 18, "present": true}],
    [{"value": 30, "present": true}, {"value": 40, "present": true}]
  ],
  "row_margins": [],
  "column_margins": [],
  "grand_total": {"present": false},
  "cell_label": "count",
  "normalize_applied": "none"
}`

func TestTable_TheSimplestShape(t *testing.T) {
	got, err := ingest.Table([]byte(simple))
	if err != nil {
		t.Fatalf("Table: %v", err)
	}

	if len(got.RowHeaders) != 2 || got.RowHeaders[0].Label != "North" || got.RowHeaders[1].Label != "South" {
		t.Fatalf("RowHeaders = %+v", got.RowHeaders)
	}
	if len(got.ColumnHeaders) != 2 || got.ColumnHeaders[0].Label != "Female" || got.ColumnHeaders[1].Label != "Male" {
		t.Fatalf("ColumnHeaders = %+v", got.ColumnHeaders)
	}
	if len(got.Body) != 2 || len(got.Body[0]) != 2 {
		t.Fatalf("Body shape = %d x %d, want 2 x 2", len(got.Body), len(got.Body[0]))
	}
	if got.Body[0][0].Value == nil || got.Body[0][0].Value.Kind != spec.ValueNumber || got.Body[0][0].Value.Number != 12 {
		t.Errorf("Body[0][0] = %+v, want 12", got.Body[0][0])
	}
	if got.Body[1][1].Value.Number != 40 {
		t.Errorf("Body[1][1] = %+v, want 40", got.Body[1][1])
	}
	if got.Caption != "count" {
		t.Errorf("Caption = %q, want %q", got.Caption, "count")
	}
}

// hierarchical carries a two-level column banner (region > gender) over a
// one-level row stub, which is the shape that actually exercises the
// prefix-collapsing algorithm rather than degenerating to one leaf per key.
const hierarchical = `{
  "row_header": {"fields": ["age_band"], "types": ["GROUP_CATEGORY"]},
  "column_header": {"fields": ["region", "gender"], "types": ["GROUP_CATEGORY", "GROUP_CATEGORY"]},
  "row_keys": [["18-34"], ["35+"]],
  "column_keys": [["North", "Female"], ["North", "Male"], ["South", "Female"], ["South", "Male"]],
  "cells": [
    [{"value": 1, "present": true}, {"value": 2, "present": true}, {"value": 3, "present": true}, {"value": 4, "present": true}],
    [{"value": 5, "present": true}, {"value": 6, "present": true}, {"value": 7, "present": true}, {"value": 8, "present": true}]
  ],
  "row_margins": [],
  "column_margins": [],
  "grand_total": {"present": false},
  "cell_label": "count"
}`

// TestTable_HierarchicalKeysCollapseIntoTheBanner is non-vacuous by
// construction: a decoder that failed to group consecutive keys would produce
// four top-level column nodes named "North", "North", "South", "South" rather
// than two named "North" and "South", each carrying two children.
func TestTable_HierarchicalKeysCollapseIntoTheBanner(t *testing.T) {
	got, err := ingest.Table([]byte(hierarchical))
	if err != nil {
		t.Fatalf("Table: %v", err)
	}

	if len(got.ColumnHeaders) != 2 {
		t.Fatalf("ColumnHeaders has %d top-level nodes, want 2 (North, South): %+v",
			len(got.ColumnHeaders), got.ColumnHeaders)
	}
	north, south := got.ColumnHeaders[0], got.ColumnHeaders[1]
	if north.Label != "North" || south.Label != "South" {
		t.Fatalf("ColumnHeaders = [%q, %q], want [North, South]", north.Label, south.Label)
	}
	if len(north.Children) != 2 || north.Children[0].Label != "Female" || north.Children[1].Label != "Male" {
		t.Errorf("North's children = %+v, want [Female, Male]", north.Children)
	}
	if len(south.Children) != 2 || south.Children[0].Label != "Female" || south.Children[1].Label != "Male" {
		t.Errorf("South's children = %+v, want [Female, Male]", south.Children)
	}

	// The body stays flat over the four leaf columns regardless of the
	// banner's shape: North/Male is column index 1, not nested access.
	if got.Body[0][1].Value.Number != 2 {
		t.Errorf("Body[0][1] (North, Male) = %+v, want 2", got.Body[0][1])
	}
	if got.Body[1][2].Value.Number != 7 {
		t.Errorf("Body[1][2] (South, Female) = %+v, want 7", got.Body[1][2])
	}
}

// TestTable_RowMarginsBecomeATrailingColumn checks the row-margin placement.
func TestTable_RowMarginsBecomeATrailingColumn(t *testing.T) {
	payload := `{
  "row_header": {"fields": ["region"], "types": ["GROUP_CATEGORY"]},
  "column_header": {"fields": ["gender"], "types": ["GROUP_CATEGORY"]},
  "row_keys": [["North"], ["South"]],
  "column_keys": [["Female"], ["Male"]],
  "cells": [
    [{"value": 12, "present": true}, {"value": 18, "present": true}],
    [{"value": 30, "present": true}, {"value": 40, "present": true}]
  ],
  "row_margins": [{"value": 30, "present": true}, {"value": 70, "present": true}],
  "column_margins": [],
  "grand_total": {"present": false},
  "cell_label": "count"
}`
	got, err := ingest.Table([]byte(payload))
	if err != nil {
		t.Fatalf("Table: %v", err)
	}

	if len(got.ColumnHeaders) != 3 || got.ColumnHeaders[2].Label != "Total" {
		t.Fatalf("ColumnHeaders = %+v, want a trailing Total", got.ColumnHeaders)
	}
	if len(got.Body[0]) != 3 || got.Body[0][2].Value.Number != 30 || got.Body[0][2].Class != spec.CellMargin {
		t.Errorf("Body[0] trailing cell = %+v, want {30, CellMargin}", got.Body[0][2])
	}
	if got.Body[1][2].Value.Number != 70 {
		t.Errorf("Body[1] trailing cell = %+v, want 70", got.Body[1][2])
	}
	// Row shape did not grow: this is still two rows, not three.
	if len(got.Body) != 2 {
		t.Errorf("Body has %d rows, want 2 — row margins add a column, not a row", len(got.Body))
	}
}

// TestTable_ColumnMarginsAndGrandTotalBecomeATrailingRow checks the
// column-margin and grand-total placement together, since the grand total
// needs both axes' margins to have a corner to sit in.
func TestTable_ColumnMarginsAndGrandTotalBecomeATrailingRow(t *testing.T) {
	payload := `{
  "row_header": {"fields": ["region"], "types": ["GROUP_CATEGORY"]},
  "column_header": {"fields": ["gender"], "types": ["GROUP_CATEGORY"]},
  "row_keys": [["North"], ["South"]],
  "column_keys": [["Female"], ["Male"]],
  "cells": [
    [{"value": 12, "present": true}, {"value": 18, "present": true}],
    [{"value": 30, "present": true}, {"value": 40, "present": true}]
  ],
  "row_margins": [{"value": 30, "present": true}, {"value": 70, "present": true}],
  "column_margins": [{"value": 42, "present": true}, {"value": 58, "present": true}],
  "grand_total": {"value": 100, "present": true},
  "cell_label": "count"
}`
	got, err := ingest.Table([]byte(payload))
	if err != nil {
		t.Fatalf("Table: %v", err)
	}

	if len(got.RowHeaders) != 3 || got.RowHeaders[2].Label != "Total" {
		t.Fatalf("RowHeaders = %+v, want a trailing Total", got.RowHeaders)
	}
	if len(got.Body) != 3 {
		t.Fatalf("Body has %d rows, want 3 (two data rows plus the margin row)", len(got.Body))
	}
	last := got.Body[2]
	if len(last) != 3 {
		t.Fatalf("the margin row has %d cells, want 3 (two column margins plus the corner)", len(last))
	}
	if last[0].Value.Number != 42 || last[0].Class != spec.CellMargin {
		t.Errorf("last[0] = %+v, want {42, CellMargin}", last[0])
	}
	if last[1].Value.Number != 58 || last[1].Class != spec.CellMargin {
		t.Errorf("last[1] = %+v, want {58, CellMargin}", last[1])
	}
	if last[2].Value.Number != 100 || last[2].Class != spec.CellTotal {
		t.Errorf("the corner cell = %+v, want {100, CellTotal}", last[2])
	}
}

// TestTable_AnAbsentCellIsEmptyNotZero pins the distinction the matrix
// payload itself is documented to draw: Present=false means no record
// matched the tuple, which is not the same claim as a value of zero.
func TestTable_AnAbsentCellIsEmptyNotZero(t *testing.T) {
	payload := `{
  "row_header": {"fields": ["region"], "types": ["GROUP_CATEGORY"]},
  "column_header": {"fields": ["gender"], "types": ["GROUP_CATEGORY"]},
  "row_keys": [["North"]],
  "column_keys": [["Female"], ["Male"]],
  "cells": [[{"present": false}, {"value": 1, "present": true}]],
  "row_margins": [],
  "column_margins": [],
  "grand_total": {"present": false}
}`
	got, err := ingest.Table([]byte(payload))
	if err != nil {
		t.Fatalf("Table: %v", err)
	}
	if got.Body[0][0].Value != nil {
		t.Errorf("an absent cell carries Value = %+v, want nil", got.Body[0][0].Value)
	}
}

// TestTable_ATextAndABooleanCell covers the other two scalar kinds ingest
// accepts, so numbers are not the only value shape ever exercised.
func TestTable_ATextAndABooleanCell(t *testing.T) {
	payload := `{
  "row_header": {"fields": ["region"], "types": ["GROUP_CATEGORY"]},
  "column_header": {"fields": ["metric"], "types": ["GROUP_CATEGORY"]},
  "row_keys": [["North"]],
  "column_keys": [["label"], ["flag"]],
  "cells": [[{"value": "leads", "present": true}, {"value": true, "present": true}]],
  "row_margins": [],
  "column_margins": [],
  "grand_total": {"present": false}
}`
	got, err := ingest.Table([]byte(payload))
	if err != nil {
		t.Fatalf("Table: %v", err)
	}
	if got.Body[0][0].Value.Kind != spec.ValueText || got.Body[0][0].Value.Text != "leads" {
		t.Errorf("Body[0][0] = %+v, want text %q", got.Body[0][0], "leads")
	}
	if got.Body[0][1].Value.Kind != spec.ValueBool || !got.Body[0][1].Value.Bool {
		t.Errorf("Body[0][1] = %+v, want bool true", got.Body[0][1])
	}
}

// TestTable_ARichAggregatorPayloadIsRefused pins that a non-scalar value is a
// coded error rather than a silently stringified guess.
func TestTable_ARichAggregatorPayloadIsRefused(t *testing.T) {
	payload := `{
  "row_header": {"fields": ["region"], "types": ["GROUP_CATEGORY"]},
  "column_header": {"fields": ["gender"], "types": ["GROUP_CATEGORY"]},
  "row_keys": [["North"]],
  "column_keys": [["Female"]],
  "cells": [[{"value": {"a": 1, "b": 2}, "present": true}]]
}`
	_, err := ingest.Table([]byte(payload))
	if !verr.HasCode(err, verr.VELLUM_INGEST_VALUE_UNSUPPORTED) {
		t.Fatalf("error = %v, want VELLUM_INGEST_VALUE_UNSUPPORTED", err)
	}
}

// TestTable_AnUnknownFieldIsRefused pins strict decoding: a field this
// package does not know about is refused rather than silently dropped, the
// same discipline spec decoding holds to.
func TestTable_AnUnknownFieldIsRefused(t *testing.T) {
	payload := `{
  "row_header": {"fields": ["region"], "types": ["GROUP_CATEGORY"]},
  "column_header": {"fields": ["gender"], "types": ["GROUP_CATEGORY"]},
  "row_keys": [["North"]],
  "column_keys": [["Female"]],
  "cells": [[{"value": 1, "present": true}]],
  "components": {"cell_counts": [[1]]}
}`
	_, err := ingest.Table([]byte(payload))
	if !verr.HasCode(err, verr.VELLUM_INGEST_INVALID) {
		t.Fatalf("error = %v, want VELLUM_INGEST_INVALID", err)
	}
}

// TestTable_MismatchedDimensionsAreRefused covers the structural
// cross-checks: a cell grid that disagrees with its own declared axes is
// refused rather than silently truncated or padded.
func TestTable_MismatchedDimensionsAreRefused(t *testing.T) {
	cases := map[string]string{
		"row key too short": `{
  "row_header": {"fields": ["region", "age"], "types": ["GROUP_CATEGORY", "GROUP_CATEGORY"]},
  "column_header": {"fields": ["gender"], "types": ["GROUP_CATEGORY"]},
  "row_keys": [["North"]],
  "column_keys": [["Female"]],
  "cells": [[{"value": 1, "present": true}]]
}`,
		"grid has too few columns": `{
  "row_header": {"fields": ["region"], "types": ["GROUP_CATEGORY"]},
  "column_header": {"fields": ["gender"], "types": ["GROUP_CATEGORY"]},
  "row_keys": [["North"]],
  "column_keys": [["Female"], ["Male"]],
  "cells": [[{"value": 1, "present": true}]]
}`,
		"row_margins length disagrees with row_keys": `{
  "row_header": {"fields": ["region"], "types": ["GROUP_CATEGORY"]},
  "column_header": {"fields": ["gender"], "types": ["GROUP_CATEGORY"]},
  "row_keys": [["North"], ["South"]],
  "column_keys": [["Female"]],
  "cells": [[{"value": 1, "present": true}], [{"value": 2, "present": true}]],
  "row_margins": [{"value": 1, "present": true}]
}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ingest.Table([]byte(payload))
			if !verr.HasCode(err, verr.VELLUM_INGEST_INVALID) {
				t.Fatalf("error = %v, want VELLUM_INGEST_INVALID", err)
			}
		})
	}
}

// TestTable_CaptionNamesTheCellAndTheNormalization covers both halves of
// what the caption reports.
func TestTable_CaptionNamesTheCellAndTheNormalization(t *testing.T) {
	payload := `{
  "row_header": {"fields": ["region"], "types": ["GROUP_CATEGORY"]},
  "column_header": {"fields": ["gender"], "types": ["GROUP_CATEGORY"]},
  "row_keys": [["North"]],
  "column_keys": [["Female"]],
  "cells": [[{"value": 0.5, "present": true}]],
  "cell_label": "count",
  "normalize_applied": "row"
}`
	got, err := ingest.Table([]byte(payload))
	if err != nil {
		t.Fatalf("Table: %v", err)
	}
	if !strings.Contains(got.Caption, "count") || !strings.Contains(got.Caption, "row") {
		t.Errorf("Caption = %q, want it to name both the cell label and the normalization", got.Caption)
	}
}

// TestTable_NoErrorsHaveEmptyDetails is a light sanity sweep: every coded
// error this package raises names something concrete, not just a bare
// message.
func TestTable_MalformedJSONIsRefused(t *testing.T) {
	_, err := ingest.Table([]byte(`{not json`))
	if !verr.HasCode(err, verr.VELLUM_INGEST_INVALID) {
		t.Fatalf("error = %v, want VELLUM_INGEST_INVALID", err)
	}
}

// TestTable_ProducesATableTheRestOfVellumAccepts is the integration check: a
// table this package builds is not merely shaped correctly by its own
// reading, it is one spec.Spec.Validate, resolve.Resolve and a real writer
// all accept — including the case that stresses the row-header tree the most,
// a margin row mixed in beside the ordinary category nodes at a shallower
// depth than its siblings.
func TestTable_ProducesATableTheRestOfVellumAccepts(t *testing.T) {
	payload := `{
  "row_header": {"fields": ["age_band"], "types": ["GROUP_CATEGORY"]},
  "column_header": {"fields": ["region", "gender"], "types": ["GROUP_CATEGORY", "GROUP_CATEGORY"]},
  "row_keys": [["18-34"], ["35+"]],
  "column_keys": [["North", "Female"], ["North", "Male"], ["South", "Female"], ["South", "Male"]],
  "cells": [
    [{"value": 1, "present": true}, {"value": 2, "present": true}, {"value": 3, "present": true}, {"value": 4, "present": true}],
    [{"value": 5, "present": true}, {"value": 6, "present": true}, {"value": 7, "present": true}, {"value": 8, "present": true}]
  ],
  "row_margins": [{"value": 10, "present": true}, {"value": 26, "present": true}],
  "column_margins": [{"value": 6, "present": true}, {"value": 8, "present": true}, {"value": 10, "present": true}, {"value": 12, "present": true}],
  "grand_total": {"value": 36, "present": true},
  "cell_label": "count",
  "normalize_applied": "none"
}`
	table, err := ingest.Table([]byte(payload))
	if err != nil {
		t.Fatalf("Table: %v", err)
	}
	if err := table.Validate(); err != nil {
		t.Fatalf("the produced table does not validate: %v", err)
	}

	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Title:         "Ingested",
		Sections: []spec.Section{{
			ID:     "s",
			Blocks: []spec.Block{{Kind: spec.BlockTable, Table: table}},
		}},
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("the specification carrying the ingested table does not validate: %v", err)
	}

	res, err := resolve.Resolve(context.Background(), s, resolve.Options{Format: artifact.FormatDOCX})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := doc.Lower(res.Doc); err != nil {
		t.Fatalf("doc.Lower: %v", err)
	}
}
