package spec_test

import (
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/spec"
)

func TestHeaderTree_Width(t *testing.T) {
	tests := []struct {
		name string
		tree spec.HeaderTree
		want int
	}{
		{"empty", nil, 0},
		{"flat", spec.HeaderTree{{Label: "A"}, {Label: "B"}, {Label: "C"}}, 3},
		{
			name: "one banner level",
			tree: spec.HeaderTree{
				{Label: "Region", Children: []spec.HeaderNode{{Label: "North"}, {Label: "South"}}},
				{Label: "Total"},
			},
			want: 3,
		},
		{
			name: "two banner levels on the same axis",
			tree: spec.HeaderTree{{Label: "Gender", Children: []spec.HeaderNode{
				{Label: "Male", Children: []spec.HeaderNode{{Label: "18-34"}, {Label: "35+"}}},
				{Label: "Female", Children: []spec.HeaderNode{{Label: "18-34"}, {Label: "35+"}}},
			}}},
			want: 4,
		},
		{
			name: "explicit leaf span",
			tree: spec.HeaderTree{{Label: "Wide", Span: 3}, {Label: "Narrow"}},
			want: 4,
		},
		{
			name: "explicit parent span agreeing with children",
			tree: spec.HeaderTree{{Label: "Region", Span: 2, Children: []spec.HeaderNode{{Label: "N"}, {Label: "S"}}}},
			want: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.tree.Width()
			if err != nil {
				t.Fatalf("Width: %v", err)
			}
			if got != tt.want {
				t.Errorf("Width = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestHeaderTree_SpanMismatch is the case the model exists to catch. A banner
// that does not tile its axis renders as a table with a hole in it, and a hole
// reads as deliberate in a way a refusal does not.
func TestHeaderTree_SpanMismatch(t *testing.T) {
	tree := spec.HeaderTree{{
		Label:    "Region",
		Span:     3,
		Children: []spec.HeaderNode{{Label: "North"}, {Label: "South"}},
	}}

	_, err := tree.Width()
	if !verr.HasCode(err, verr.VELLUM_TABLE_HEADER_SPAN_MISMATCH) {
		t.Fatalf("error = %v, want VELLUM_TABLE_HEADER_SPAN_MISMATCH", err)
	}

	ce, _ := err.(*verr.CodedError)
	if ce == nil {
		t.Fatal("error is not a CodedError")
	}
	if got, _ := ce.Detail("header_path"); got != "Region" {
		t.Errorf("detail header_path = %v, want %q", got, "Region")
	}
	if got, _ := ce.Detail("declared_span"); got != 3 {
		t.Errorf("detail declared_span = %v, want 3", got)
	}
	if got, _ := ce.Detail("children_span"); got != 2 {
		t.Errorf("detail children_span = %v, want 2", got)
	}
}

func TestTable_ValidateAcceptsRealisticShapes(t *testing.T) {
	tests := []struct {
		name  string
		table *spec.Table
	}{
		{
			name: "flat table",
			table: &spec.Table{
				ColumnHeaders: spec.HeaderTree{{Label: "Brand"}, {Label: "Awareness"}},
				Body: [][]spec.Cell{
					{{Text: "Acme"}, {Value: num(42)}},
					{{Text: "Globex"}, {Value: num(37)}},
				},
			},
		},
		{
			name: "banner with a margin column",
			table: &spec.Table{
				ColumnHeaders: spec.HeaderTree{
					{Label: "Region", Children: []spec.HeaderNode{{Label: "North"}, {Label: "South"}}},
					{Label: "Total"},
				},
				RowHeaders: spec.HeaderTree{{Label: "Awareness"}, {Label: "Consideration"}},
				Body: [][]spec.Cell{
					{{Value: num(10)}, {Value: num(20)}, {Value: num(30), Class: spec.CellMargin}},
					{{Value: num(11)}, {Value: num(21)}, {Value: num(32), Class: spec.CellMargin}},
				},
			},
		},
		{
			name: "crosstab nested on both axes with significance letters",
			table: &spec.Table{
				ColumnHeaders: spec.HeaderTree{{Label: "Gender", Children: []spec.HeaderNode{
					{Label: "Male"}, {Label: "Female"},
				}}},
				RowHeaders: spec.HeaderTree{{Label: "Age", Children: []spec.HeaderNode{
					{Label: "18-34"}, {Label: "35+"},
				}}},
				Body: [][]spec.Cell{
					{
						{Value: num(51), Annotations: []spec.Annotation{{Text: "b"}}},
						{Value: num(49)},
					},
					{
						{Value: num(44)},
						{Value: num(56), Annotations: []spec.Annotation{{Text: "a", Position: spec.AnnotationSuperscript}}},
					},
				},
			},
		},
		{
			name: "row span carrying a stub label down two rows",
			table: &spec.Table{
				ColumnHeaders: spec.HeaderTree{{Label: "Segment"}, {Label: "Metric"}, {Label: "Value"}},
				Body: [][]spec.Cell{
					{{Text: "Core", RowSpan: 2, Class: spec.CellHeader}, {Text: "Awareness"}, {Value: num(10)}},
					{{Text: "Consideration"}, {Value: num(20)}},
				},
			},
		},
		{
			name: "no column headers, width inferred from the body",
			table: &spec.Table{
				Body: [][]spec.Cell{
					{{Text: "a"}, {Text: "b"}},
					{{Text: "c"}, {Text: "d"}},
				},
			},
		},
		{
			name: "grand total spanning the whole width",
			table: &spec.Table{
				ColumnHeaders: spec.HeaderTree{{Label: "A"}, {Label: "B"}},
				Body: [][]spec.Cell{
					{{Value: num(1)}, {Value: num(2)}},
					{{Text: "Total", ColSpan: 2, Class: spec.CellTotal}},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.table.Validate(); err != nil {
				t.Errorf("a well-formed table was rejected: %v", err)
			}
		})
	}
}

func TestTable_ValidateRejects(t *testing.T) {
	tests := []struct {
		name  string
		table *spec.Table
		code  verr.Code
	}{
		{"nil", nil, verr.VELLUM_TABLE_INVALID},
		{"no body", &spec.Table{ColumnHeaders: spec.HeaderTree{{Label: "A"}}}, verr.VELLUM_TABLE_INVALID},
		{
			name: "row too short for the header width",
			table: &spec.Table{
				ColumnHeaders: spec.HeaderTree{{Label: "A"}, {Label: "B"}, {Label: "C"}},
				Body:          [][]spec.Cell{{{Text: "1"}, {Text: "2"}}},
			},
			code: verr.VELLUM_TABLE_ROW_ARITY,
		},
		{
			name: "row too long for the header width",
			table: &spec.Table{
				ColumnHeaders: spec.HeaderTree{{Label: "A"}, {Label: "B"}},
				Body:          [][]spec.Cell{{{Text: "1"}, {Text: "2"}, {Text: "3"}}},
			},
			code: verr.VELLUM_TABLE_SPAN_OVERLAP,
		},
		{
			name: "column span runs past the right edge",
			table: &spec.Table{
				ColumnHeaders: spec.HeaderTree{{Label: "A"}, {Label: "B"}},
				Body:          [][]spec.Cell{{{Text: "wide", ColSpan: 3}}},
			},
			code: verr.VELLUM_TABLE_SPAN_OVERLAP,
		},
		{
			name: "row span runs past the last row",
			table: &spec.Table{
				ColumnHeaders: spec.HeaderTree{{Label: "A"}},
				Body:          [][]spec.Cell{{{Text: "tall", RowSpan: 3}}},
			},
			code: verr.VELLUM_TABLE_SPAN_OVERLAP,
		},
		{
			name: "two cells claiming the same position",
			table: &spec.Table{
				ColumnHeaders: spec.HeaderTree{{Label: "A"}, {Label: "B"}},
				Body: [][]spec.Cell{
					{{Text: "spans down", RowSpan: 2}, {Text: "b"}},
					{{Text: "c"}, {Text: "d"}},
				},
			},
			code: verr.VELLUM_TABLE_SPAN_OVERLAP,
		},
		{
			name: "negative span",
			table: &spec.Table{
				ColumnHeaders: spec.HeaderTree{{Label: "A"}},
				Body:          [][]spec.Cell{{{Text: "x", ColSpan: -1}}},
			},
			code: verr.VELLUM_TABLE_INVALID,
		},
		{
			name: "negative header span",
			table: &spec.Table{
				ColumnHeaders: spec.HeaderTree{{Label: "A", Span: -2}},
				Body:          [][]spec.Cell{{{Text: "x"}}},
			},
			code: verr.VELLUM_TABLE_INVALID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.table.Validate(); !verr.HasCode(err, tt.code) {
				t.Errorf("error = %v, want %s", err, tt.code)
			}
		})
	}
}

// TestTable_RowSpanFlowsAroundOccupiedCells checks the reading model: a
// spanned cell occupies the position beneath it and the next row's cells flow
// around it, rather than the next row starting at column zero regardless.
func TestTable_RowSpanFlowsAroundOccupiedCells(t *testing.T) {
	table := &spec.Table{
		ColumnHeaders: spec.HeaderTree{{Label: "A"}, {Label: "B"}, {Label: "C"}},
		Body: [][]spec.Cell{
			{{Text: "a1", RowSpan: 2}, {Text: "b1"}, {Text: "c1"}},
			{{Text: "b2"}, {Text: "c2"}},
		},
	}
	if err := table.Validate(); err != nil {
		t.Errorf("a table whose second row flows around a row span was rejected: %v", err)
	}
}

func TestAllRegistries_ReturnStableVocabularies(t *testing.T) {
	if got := len(spec.AllCellClasses()); got != 4 {
		t.Errorf("AllCellClasses returned %d entries, want 4", got)
	}
	if got := len(spec.AllValueKinds()); got != 5 {
		t.Errorf("AllValueKinds returned %d entries, want 5", got)
	}
	if got := len(spec.AllAnnotationPositions()); got != 4 {
		t.Errorf("AllAnnotationPositions returned %d entries, want 4", got)
	}
	if got := len(spec.AllUnits()); got != 4 {
		t.Errorf("AllUnits returned %d entries, want 4", got)
	}
}

func num(v float64) *spec.Value {
	return &spec.Value{Kind: spec.ValueNumber, Number: v}
}
