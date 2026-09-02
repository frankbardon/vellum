package sheet_test

import (
	"archive/zip"
	"bytes"
	"context"
	"strconv"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/resolve"
	"github.com/frankbardon/vellum/sheet"
	"github.com/frankbardon/vellum/spec"
)

// resolved runs the resolve pass without lowering, which is the input every
// test below starts from — the same door a caller composing from blocks goes
// through. XLSX needs no embeddable theme, unlike PDF: every face is
// referenced by name, so the built-in theme is enough.
func resolved(t *testing.T, blocks ...spec.Block) *fragment.Doc {
	t.Helper()
	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Title:         "Report",
		Sections:      []spec.Section{{ID: "s1", Blocks: blocks}},
	}
	res, err := resolve.Resolve(context.Background(), s, resolve.Options{Format: artifact.FormatXLSX})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return res.Doc
}

// lower resolves a specification and lowers it.
func lower(t *testing.T, blocks ...spec.Block) *sheet.Workbook {
	t.Helper()
	d := resolved(t, blocks...)
	wb, err := sheet.Lower(d)
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	return wb
}

// lowerSpec resolves and lowers a specification a test built directly, for
// the cases a flat block list cannot express — multiple sections, in
// particular.
func lowerSpec(t *testing.T, s *spec.Spec) *sheet.Workbook {
	t.Helper()
	res, err := resolve.Resolve(context.Background(), s, resolve.Options{Format: artifact.FormatXLSX})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wb, err := sheet.Lower(res.Doc)
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	return wb
}

// resolveErr runs resolve.Resolve and returns its error without failing the
// test, for a case that expects one.
func resolveErr(t *testing.T, s *spec.Spec) (*fragment.Doc, error) {
	t.Helper()
	res, err := resolve.Resolve(context.Background(), s, resolve.Options{Format: artifact.FormatXLSX})
	if err != nil {
		return nil, err
	}
	return res.Doc, nil
}

func heading(level int, content string) spec.Block {
	return spec.Block{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: level, Content: content}}
}

func text(content string) spec.Block {
	return spec.Block{Kind: spec.BlockText, Text: &spec.Text{Content: content}}
}

var pageBreak = spec.Block{Kind: spec.BlockPageBreak, PageBreak: &spec.PageBreak{}}

var notes = spec.Block{Kind: spec.BlockNotes, Notes: &spec.Notes{Content: "Base: all respondents."}}

var spacer = spec.Block{Kind: spec.BlockSpacer, Spacer: &spec.Spacer{Height: spec.Points(12)}}

// write emits a workbook and returns the package bytes.
func write(t *testing.T, wb *sheet.Workbook) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := wb.WriteTo(&buf, sheet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

// part returns one part's bytes from a written package.
func part(t *testing.T, raw []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		defer rc.Close()
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return buf.String()
	}
	t.Fatalf("the package has no part named %s", name)
	return ""
}

func sheetPart(t *testing.T, raw []byte, index int) string {
	t.Helper()
	return part(t, raw, "xl/worksheets/sheet"+strconv.Itoa(index+1)+".xml")
}

// crosstab builds a small analytical table: a two-level column banner, a
// merged row-header stub, a total row, a percent-formatted body and an
// annotated cell.
func crosstab() spec.Block {
	return spec.Block{Kind: spec.BlockTable, Table: &spec.Table{
		ColumnHeaders: spec.HeaderTree{
			{Label: "Region", Span: 2, Children: spec.HeaderTree{
				{Label: "North"}, {Label: "South"},
			}},
			{Label: "Total"},
		},
		RowHeaders: spec.HeaderTree{{Label: "Age", Children: spec.HeaderTree{
			{Label: "18-34"}, {Label: "35-54"},
		}}},
		Body: [][]spec.Cell{
			{
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 0.41}, Format: "0.0%",
					Annotations: []spec.Annotation{{Text: "a"}}},
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 0.59}, Format: "0.0%"},
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 1}, Format: "0.0%"},
			},
			{
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 0.52}, Format: "0.0%", Class: spec.CellTotal},
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 0.48}, Format: "0.0%", Class: spec.CellTotal},
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 1}, Format: "0.0%", Class: spec.CellTotal},
			},
		},
		Caption: "Percentages. Base: all respondents.",
	}}
}
