// Package ingest turns a Pulse crosstab result into a [spec.Table], at the
// protocol boundary and nowhere else.
//
// # Coupling at the wire, not at the package
//
// Vellum does not import Pulse. The shapes this package decodes are Vellum's
// own — chosen to match the JSON a Pulse crosstab result carries at its
// "matrix" payload, verified by hand against Pulse's own types.MatrixPayload,
// and never against Pulse's Go structs. That is the whole of the coupling: two
// projects agreeing on a wire shape, not one importing the other's package. A
// change to Pulse's internal representation that leaves this JSON shape
// unchanged asks nothing of Vellum, and Pulse gains no vocabulary from doing
// so — TestNoPulseCodes holds the reverse of that boundary, refusing a
// PULSE_* error code anywhere in this tree.
//
// # What is decoded
//
// The payload this package expects, matching a Pulse crosstab result's
// "matrix" field:
//
//	{
//	  "row_header":    {"fields": [...], "types": [...]},
//	  "column_header": {"fields": [...], "types": [...]},
//	  "row_keys":      [[...], [...], ...],
//	  "column_keys":   [[...], [...], ...],
//	  "cells":         [[{"value": ..., "present": true}, ...], ...],
//	  "row_margins":    [{"value": ..., "present": true}, ...],
//	  "column_margins": [{"value": ..., "present": true}, ...],
//	  "grand_total":    {"value": ..., "present": true},
//	  "cell_label":       "count",
//	  "normalize_applied": "row"
//	}
//
// Decoding is strict: an unrecognised field is [errors.VELLUM_INGEST_INVALID]
// rather than silently ignored, for the reason spec decoding is strict —
// silent tolerance of a field this package does not yet know about is poison
// when the caller cannot see what was dropped.
//
// # What a table cell may carry
//
// A cell's value must be a number, a string or a boolean. Pulse's rich
// aggregators — a per-label frequency map from AGG_SET_FREQUENCY, a Welford
// triple's structured payload — have no scalar form this package will invent
// one for; a payload carrying one is [errors.VELLUM_INGEST_VALUE_UNSUPPORTED].
// Guessing a string representation would give a table cell content nobody
// asked for and no signal that the real value was dropped.
//
// # Row and column keys build the header trees
//
// A Pulse axis is a flat list of tuples — [][]any, one tuple per row or
// column, sorted — rather than the nested tree [spec.Table] wants for its
// banners. This package rebuilds the tree by collapsing consecutive tuples
// that share a prefix at each level, which is the inverse of how the tuples
// were produced: a grouped result's keys are already grouped, only flattened.
//
// # Margins
//
// A per-row margin becomes a trailing "Total" column; a per-column margin
// becomes a trailing "Total" row; a grand total, when either is present,
// becomes their corner cell. Every margin cell carries
// [spec.CellMargin], and the grand total carries [spec.CellTotal] — the same
// distinction [spec.Table]'s own documentation draws.
package ingest

import (
	"bytes"
	"encoding/json"
	"strconv"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/spec"
)

// axisHeader names the fields making up one axis tuple, mirroring Pulse's
// types.AxisHeader.
type axisHeader struct {
	Fields []string `json:"fields"`
	Types  []string `json:"types"`
}

// axisKey is one row or column tuple, mirroring Pulse's types.AxisKey.
type axisKey []any

// matrixCell is one entry of the dense cell grid, mirroring Pulse's
// types.MatrixCell.
type matrixCell struct {
	Value   any  `json:"value"`
	Present bool `json:"present"`
}

// matrixPayload is the whole decoded shape, mirroring Pulse's
// types.MatrixPayload.
type matrixPayload struct {
	RowHeader    axisHeader `json:"row_header"`
	ColumnHeader axisHeader `json:"column_header"`

	RowKeys    []axisKey `json:"row_keys"`
	ColumnKeys []axisKey `json:"column_keys"`

	Cells [][]matrixCell `json:"cells"`

	RowMargins    []matrixCell `json:"row_margins"`
	ColumnMargins []matrixCell `json:"column_margins"`
	GrandTotal    matrixCell   `json:"grand_total"`

	CellLabel        string `json:"cell_label"`
	NormalizeApplied string `json:"normalize_applied"`
}

// marginColumnLabel and marginRowLabel name the trailing summary column and
// row a margin becomes.
const (
	marginColumnLabel = "Total"
	marginRowLabel    = "Total"
)

// Table decodes a Pulse crosstab result's matrix payload into a [spec.Table].
//
// raw is the JSON object at that result's "matrix" field — not the whole
// envelope, and not the whole crosstab result, both of which carry fields this
// package has no use for and no opinion about. A caller holding a full Pulse
// envelope extracts that field before calling Table; see the package doc for
// the shape expected here.
func Table(raw []byte) (*spec.Table, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	dec.UseNumber()

	var p matrixPayload
	if err := dec.Decode(&p); err != nil {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_INGEST_INVALID,
			"the payload does not decode as the documented matrix shape",
			map[string]any{"decode_error": err.Error()})
	}
	if dec.More() {
		return nil, verr.NewCodedError(verr.VELLUM_INGEST_INVALID,
			"the payload carries more than one JSON value")
	}

	if err := p.validate(); err != nil {
		return nil, err
	}

	rowDepth := len(p.RowHeader.Fields)
	colDepth := len(p.ColumnHeader.Fields)

	body := make([][]spec.Cell, len(p.RowKeys))
	for i := range p.RowKeys {
		row := make([]spec.Cell, len(p.ColumnKeys))
		for j := range p.ColumnKeys {
			cell, err := cellFrom(p.Cells[i][j])
			if err != nil {
				return nil, verr.Annotate(err, map[string]any{"row": i, "column": j})
			}
			row[j] = cell
		}
		body[i] = row
	}

	rowHeaders := buildHeaderTree(p.RowKeys, rowDepth)
	columnHeaders := buildHeaderTree(p.ColumnKeys, colDepth)

	haveRowMargins := len(p.RowMargins) > 0
	haveColumnMargins := len(p.ColumnMargins) > 0
	haveGrandTotal := p.GrandTotal.Present

	// A grand total needs both a margin row and a margin column to sit at the
	// corner of. One that arrived without either is given empty ones to sit
	// in, rather than being dropped — a value the source explicitly marked
	// present is a value a caller wrote code to compute, and this package does
	// not discard content nobody asked it to.
	if haveGrandTotal && !haveRowMargins {
		haveRowMargins = true
	}
	if haveGrandTotal && !haveColumnMargins {
		haveColumnMargins = true
	}

	if haveRowMargins {
		columnHeaders = append(columnHeaders, spec.HeaderNode{Label: marginColumnLabel, Span: 1})
		for i := range body {
			cell := spec.Cell{Class: spec.CellMargin}
			if i < len(p.RowMargins) {
				c, err := cellFrom(p.RowMargins[i])
				if err != nil {
					return nil, verr.Annotate(err, map[string]any{"row_margin": i})
				}
				c.Class = spec.CellMargin
				cell = c
			}
			body[i] = append(body[i], cell)
		}
	}

	if haveColumnMargins {
		rowHeaders = append(rowHeaders, spec.HeaderNode{Label: marginRowLabel, Span: 1})
		width := len(p.ColumnKeys)
		if haveRowMargins {
			width++
		}
		last := make([]spec.Cell, width)
		for j := range p.ColumnKeys {
			cell := spec.Cell{Class: spec.CellMargin}
			if j < len(p.ColumnMargins) {
				c, err := cellFrom(p.ColumnMargins[j])
				if err != nil {
					return nil, verr.Annotate(err, map[string]any{"column_margin": j})
				}
				c.Class = spec.CellMargin
				cell = c
			}
			last[j] = cell
		}
		if haveRowMargins {
			corner := spec.Cell{Class: spec.CellTotal}
			if haveGrandTotal {
				c, err := cellFrom(p.GrandTotal)
				if err != nil {
					return nil, verr.Annotate(err, map[string]any{"grand_total": true})
				}
				c.Class = spec.CellTotal
				corner = c
			}
			last[width-1] = corner
		}
		body = append(body, last)
	}

	return &spec.Table{
		RowHeaders:    rowHeaders,
		ColumnHeaders: columnHeaders,
		Body:          body,
		Caption:       captionOf(p),
	}, nil
}

// validate checks the payload is internally consistent before any of it is
// projected onto a table.
func (p matrixPayload) validate() error {
	if len(p.RowHeader.Fields) == 0 {
		return verr.NewCodedError(verr.VELLUM_INGEST_INVALID, "row_header names no fields")
	}
	if len(p.ColumnHeader.Fields) == 0 {
		return verr.NewCodedError(verr.VELLUM_INGEST_INVALID, "column_header names no fields")
	}
	for i, k := range p.RowKeys {
		if len(k) != len(p.RowHeader.Fields) {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_INGEST_INVALID,
				"a row key's length does not match row_header's field count",
				map[string]any{"row": i, "key_length": len(k), "fields": len(p.RowHeader.Fields)})
		}
	}
	for j, k := range p.ColumnKeys {
		if len(k) != len(p.ColumnHeader.Fields) {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_INGEST_INVALID,
				"a column key's length does not match column_header's field count",
				map[string]any{"column": j, "key_length": len(k), "fields": len(p.ColumnHeader.Fields)})
		}
	}
	if len(p.Cells) != len(p.RowKeys) {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_INGEST_INVALID,
			"the cell grid's row count does not match row_keys",
			map[string]any{"grid_rows": len(p.Cells), "row_keys": len(p.RowKeys)})
	}
	for i, row := range p.Cells {
		if len(row) != len(p.ColumnKeys) {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_INGEST_INVALID,
				"the cell grid's column count does not match column_keys",
				map[string]any{"row": i, "grid_columns": len(row), "column_keys": len(p.ColumnKeys)})
		}
	}
	if n := len(p.RowMargins); n > 0 && n != len(p.RowKeys) {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_INGEST_INVALID,
			"row_margins' length does not match row_keys",
			map[string]any{"row_margins": n, "row_keys": len(p.RowKeys)})
	}
	if n := len(p.ColumnMargins); n > 0 && n != len(p.ColumnKeys) {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_INGEST_INVALID,
			"column_margins' length does not match column_keys",
			map[string]any{"column_margins": n, "column_keys": len(p.ColumnKeys)})
	}
	return nil
}

// captionOf summarises what the cell values are, when the payload said
// anything about it. Empty when neither field carries anything worth
// surfacing, which is the ordinary case for an unnormalised count.
func captionOf(p matrixPayload) string {
	switch {
	case p.CellLabel != "" && p.NormalizeApplied != "" && p.NormalizeApplied != "none":
		return p.CellLabel + ", normalized by " + p.NormalizeApplied
	case p.CellLabel != "":
		return p.CellLabel
	}
	return ""
}

// cellFrom projects one matrix cell onto a table cell.
//
// An absent cell (Present false) becomes an empty one rather than a zero: a
// structurally missing combination — no record matched the row x column
// tuple — is not the same claim as a value of zero, and numfmt.Value's own
// closed shape has an empty arm for exactly this.
func cellFrom(c matrixCell) (spec.Cell, error) {
	if !c.Present {
		return spec.Cell{}, nil
	}

	switch v := c.Value.(type) {
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return spec.Cell{}, verr.NewCodedErrorWithDetails(verr.VELLUM_INGEST_INVALID,
				"a cell's numeric value does not parse", map[string]any{"value": v.String()})
		}
		return spec.Cell{Value: &spec.Value{Kind: spec.ValueNumber, Number: f}}, nil
	case string:
		return spec.Cell{Value: &spec.Value{Kind: spec.ValueText, Text: v}}, nil
	case bool:
		return spec.Cell{Value: &spec.Value{Kind: spec.ValueBool, Bool: v}}, nil
	case nil:
		return spec.Cell{}, nil
	}

	return spec.Cell{}, verr.NewCodedErrorWithDetails(verr.VELLUM_INGEST_VALUE_UNSUPPORTED,
		"a cell carries a value ingest cannot represent as a table cell",
		map[string]any{"go_type": goTypeName(c.Value)})
}

// goTypeName names a decoded JSON value's Go type for an error detail, without
// reflection: json.Unmarshal into `any` produces exactly these five shapes and
// no others.
func goTypeName(v any) string {
	switch v.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case json.Number:
		return "number"
	case string:
		return "string"
	case bool:
		return "boolean"
	}
	return "unknown"
}

// buildHeaderTree collapses a flat, sorted list of axis tuples into the nested
// tree [spec.Table]'s banners want.
//
// A grouped result's keys arrive already grouped — every tuple sharing a
// prefix with its neighbour is adjacent, because that is what "sorted by
// tuple" means — so collapsing consecutive equal prefixes, level by level,
// exactly inverts however the tuples were flattened in the first place.
//
// Every node's Span is set explicitly, on leaves as well as parents, rather
// than left at zero for [spec.HeaderNode]'s documented "derive it from the
// children" default. That default is honoured by [spec.Table.Validate]'s own
// width arithmetic but not by resolve.resolveHeaders, which copies Span
// verbatim into the resolved tree — so a node built with it unset renders
// every writer's banner cell at width one regardless of how many leaves it
// actually covers. Setting it here is a workaround for that gap, not a
// preference; every table fixture elsewhere in this codebase does the same by
// hand for the same reason.
func buildHeaderTree(keys []axisKey, depth int) spec.HeaderTree {
	return headerLevel(keys, 0, depth)
}

func headerLevel(keys []axisKey, level, depth int) spec.HeaderTree {
	if level >= depth || len(keys) == 0 {
		return nil
	}

	var out spec.HeaderTree
	for i := 0; i < len(keys); {
		j := i + 1
		for j < len(keys) && keys[j][level] == keys[i][level] {
			j++
		}
		node := spec.HeaderNode{Label: labelOf(keys[i][level]), Span: j - i}
		if level+1 < depth {
			node.Children = headerLevel(keys[i:j], level+1, depth)
		}
		out = append(out, node)
		i = j
	}
	return out
}

// labelOf renders one axis key element as banner text.
//
// A key element is always one of the scalars encoding/json produces for `any`
// under UseNumber: a number, a string, a boolean or null — Pulse's own
// documentation for AxisKey states numeric bins stay numeric and categorical
// or date keys stay strings, so this is never asked to render a nested value.
func labelOf(v any) string {
	switch t := v.(type) {
	case json.Number:
		return t.String()
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case nil:
		return ""
	}
	return ""
}
