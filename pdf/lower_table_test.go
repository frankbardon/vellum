package pdf_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/pdf"
	"github.com/frankbardon/vellum/pdf/object"
	"github.com/frankbardon/vellum/resolve"
	"github.com/frankbardon/vellum/spec"
)

// crosstab is the shape an analytical table actually has: a two-level column
// banner, a merged row-header stub, a total row and an annotated cell.
//
// Built by count so a test can ask for one that fits and one that does not
// without two fixtures that could drift apart.
func crosstab(rows int) spec.Block {
	bands := make(spec.HeaderTree, 0, rows)
	body := make([][]spec.Cell, 0, rows)
	for i := 0; i < rows; i++ {
		bands = append(bands, spec.HeaderNode{Label: "Band " + strconv.Itoa(i+1)})

		row := []spec.Cell{
			{Text: strconv.Itoa(40 + i)},
			{Text: strconv.Itoa(60 - i)},
			{Text: "100"},
		}
		if i == 0 {
			row[0].Annotations = []spec.Annotation{{Text: "a"}}
		}
		if i == rows-1 {
			for j := range row {
				row[j].Class = spec.CellTotal
			}
		}
		body = append(body, row)
	}

	return spec.Block{Kind: spec.BlockTable, Table: &spec.Table{
		ColumnHeaders: spec.HeaderTree{
			{Label: "Region", Span: 2, Children: spec.HeaderTree{
				{Label: "North"}, {Label: "South"},
			}},
			{Label: "Total"},
		},
		RowHeaders: spec.HeaderTree{{Label: "Age", Children: bands}},
		Body:       body,
		Caption:    "Percentages. Base: all adults.",
	}}
}

// resolved runs the resolve pass without lowering, so a test can break the
// resolved document in one place and watch the writer refuse it.
func resolved(t *testing.T, blocks ...spec.Block) *fragment.Doc {
	t.Helper()

	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Title:         "Test",
		Theme:         "embeddable",
		Sections:      []spec.Section{{ID: "s", Blocks: blocks}},
	}
	res, err := resolve.Resolve(context.Background(), s, resolve.Options{
		Format: artifact.FormatPDF, Themes: embeddableTheme(t), Assets: fontStore(),
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return res.Doc
}

// pageText returns every string a page draws, in drawing order.
func pageText(p pdf.Page) []string {
	var out []string
	for _, it := range p.Items {
		if it.Kind != pdf.ItemText {
			continue
		}
		var b strings.Builder
		for _, line := range it.Text.Lines {
			b.WriteString(line.Text())
		}
		out = append(out, strings.TrimSpace(b.String()))
	}
	return out
}

// draws reports whether a page draws a string exactly.
func draws(p pdf.Page, want string) bool {
	for _, got := range pageText(p) {
		if got == want {
			return true
		}
	}
	return false
}

// TestTable_ATableThatFitsIsStillReported pins the report's shape.
//
// A report that appears only when something went wrong is one nobody can
// distinguish from a missing report, so a table that fitted is one part of one.
func TestTable_ATableThatFitsIsStillReported(t *testing.T) {
	d := lower(t, crosstab(3))

	if len(d.Pages) != 1 {
		t.Fatalf("pages = %d, want 1", len(d.Pages))
	}
	if len(d.Overflow) != 1 {
		t.Fatalf("Overflow = %+v, want one part", d.Overflow)
	}
	got := d.Overflow[0]
	want := pdf.TableSplit{
		SectionID: "s", SectionIndex: 0, BlockIndex: 0,
		Page: 0, Part: 0, Parts: 1,
		FromRow: 0, Rows: 3, TotalRows: 3, HeaderRows: 2,
	}
	if got != want {
		t.Errorf("Overflow[0] = %+v, want %+v", got, want)
	}
}

// TestTable_ContinuesWithItsHeadersRepeated is the policy, observed.
//
// The banner count is half of it. A split that carried the right rows and
// stopped repeating the header would satisfy every other check here and produce
// a second page that is a grid of numbers with no column names — which is the
// failure the whole policy exists to prevent.
func TestTable_ContinuesWithItsHeadersRepeated(t *testing.T) {
	const rows = 80
	d := lower(t, crosstab(rows))

	if len(d.Overflow) < 2 {
		t.Fatalf("Overflow = %+v, want the table split across pages", d.Overflow)
	}

	// The parts tile the table exactly: no row placed twice, none dropped.
	next := 0
	for i, s := range d.Overflow {
		if s.Part != i || s.Parts != len(d.Overflow) {
			t.Errorf("part %d reports Part=%d Parts=%d", i, s.Part, s.Parts)
		}
		if s.FromRow != next {
			t.Errorf("part %d starts at row %d, want %d", i, s.FromRow, next)
		}
		if s.Rows <= 0 {
			t.Errorf("part %d carries %d rows", i, s.Rows)
		}
		next += s.Rows

		page := d.Pages[s.Page]
		for _, label := range []string{"Region", "North", "South", "Total"} {
			if !draws(page, label) {
				t.Errorf("part %d on page %d does not draw the banner label %q",
					i, s.Page, label)
			}
		}
	}
	if next != rows {
		t.Errorf("the parts cover %d rows, want %d", next, rows)
	}

	// Greedy, not balanced: the first part is full and the last carries the
	// remainder. Balancing would make every part's contents a function of the
	// total, so appending one row would move every boundary before it.
	if first, last := d.Overflow[0].Rows, d.Overflow[len(d.Overflow)-1].Rows; last > first {
		t.Errorf("the last part carries %d rows and the first %d; the fill is not greedy", last, first)
	}
}

// TestTable_NothingIsDrawnBelowTheBottomMargin is the guarantee the split
// exists to provide.
//
// Every other check here reads a number the writer computed. This one reads
// where the ink landed, which is the only evidence that the capacity it
// computed and the table it drew are the same table.
func TestTable_NothingIsDrawnBelowTheBottomMargin(t *testing.T) {
	d := lower(t, crosstab(80))

	// One inch, the default page's margin, in the thousandths of a point this
	// writer positions in.
	const bottom = object.Real(72 * object.RealScale)

	for i, p := range d.Pages {
		for _, it := range p.Items {
			switch it.Kind {
			case pdf.ItemRule:
				if it.Rule.Y < bottom {
					t.Errorf("page %d draws a rule at y=%s, below the %s margin",
						i, it.Rule.Y, bottom)
				}
			case pdf.ItemText:
				last := it.Text.Y - it.Text.Leading*object.Real(len(it.Text.Lines)-1)
				if last < bottom {
					t.Errorf("page %d sets a baseline at y=%s, below the %s margin",
						i, last, bottom)
				}
			}
		}
	}
}

// TestTable_AMergedStubRestartsOnEveryPart checks the row header a
// continuation page would otherwise lose.
//
// A stub merge computed over the whole table names a span reaching past the
// page it lands in, and the rows under it on every page but the first would
// carry no label at all. Restarting it is the same rule the column banner
// follows.
func TestTable_AMergedStubRestartsOnEveryPart(t *testing.T) {
	d := lower(t, crosstab(80))

	if len(d.Overflow) < 2 {
		t.Fatalf("Overflow = %+v, want the table split across pages", d.Overflow)
	}
	for _, s := range d.Overflow {
		if !draws(d.Pages[s.Page], "Age") {
			t.Errorf("part %d on page %d does not repeat the stub label", s.Part, s.Page)
		}
	}
}

// TestTable_DoesNotBeginOnAPageWithRoomForHeadersAlone pins the one case where
// the first page's remaining space is refused.
//
// A table beginning part way down a page is the ordinary case and is what keeps
// the heading above it from being stranded. A table beginning where only the
// banner fits is not a split, it is a page of headings, so the break happens
// first.
func TestTable_DoesNotBeginOnAPageWithRoomForHeadersAlone(t *testing.T) {
	sliver := spec.Block{Kind: spec.BlockSpacer, Spacer: &spec.Spacer{Height: spec.Points(660)}}
	d := lower(t, sliver, crosstab(40))

	if len(d.Overflow) == 0 {
		t.Fatal("the table was not placed")
	}
	if got := d.Overflow[0].Page; got == 0 {
		t.Errorf("the table began on page 0, which had room for its banner and nothing under it")
	}
	for _, it := range d.Pages[0].Items {
		if it.Kind == pdf.ItemRule {
			t.Errorf("page 0 draws part of the table: a rule at y=%s", it.Rule.Y)
		}
	}

	// The other half: a page with real room is used rather than abandoned,
	// which is what stops a heading being stranded above every long table.
	room := spec.Block{Kind: spec.BlockSpacer, Spacer: &spec.Spacer{Height: spec.Points(300)}}
	e := lower(t, room, crosstab(40))
	if got := e.Overflow[0].Page; got != 0 {
		t.Errorf("with 300pt used the table began on page %d, want 0", got)
	}
}

// TestTable_ARowThatDoesNotTileTheGridIsRefused breaks the resolved document in
// the one way this check exists to catch.
//
// A row whose cells do not cover the grid is not a document that fails to
// draw. It draws, with a hole in it, and the hole looks deliberate.
func TestTable_ARowThatDoesNotTileTheGridIsRefused(t *testing.T) {
	doc := resolved(t, crosstab(3))
	body := doc.Sections[0].Blocks[0].Table.Body
	body[1] = body[1][:len(body[1])-1]

	_, err := pdf.Lower(doc)
	if !verr.HasCode(err, verr.VELLUM_TABLE_ROW_ARITY) {
		t.Fatalf("error = %v, want VELLUM_TABLE_ROW_ARITY", err)
	}
}

// TestTable_ABannerThatDoesNotTileTheGridIsRefused is the same failure one row
// higher, and it is checked separately because it is caught separately: the
// banner is measured before the body, so a document broken in both places
// reports the banner and a test that only broke the body would never reach it.
func TestTable_ABannerThatDoesNotTileTheGridIsRefused(t *testing.T) {
	doc := resolved(t, crosstab(3))
	doc.Sections[0].Blocks[0].Table.Width++

	_, err := pdf.Lower(doc)
	if !verr.HasCode(err, verr.VELLUM_TABLE_HEADER_SPAN_MISMATCH) {
		t.Fatalf("error = %v, want VELLUM_TABLE_HEADER_SPAN_MISMATCH", err)
	}
}

// TestTable_FillsAndRulesPrecedeText pins the drawing order.
//
// PDF paints in the order the operators appear, so a cell's fill emitted after
// its neighbour's hairline covers the hairline, and text emitted before a fill
// disappears under it. The order is not an implementation detail: it is the
// only one in which every cell can be emitted independently and still land
// right.
func TestTable_FillsAndRulesPrecedeText(t *testing.T) {
	d := lower(t, crosstab(80))

	if len(d.Overflow) < 2 {
		t.Fatalf("Overflow = %+v, want a page carrying nothing but the table", d.Overflow)
	}

	// A continuation page carries the table and nothing else, so the order can
	// be read off the page directly.
	page := d.Pages[d.Overflow[1].Page]
	seenText := false
	for i, it := range page.Items {
		switch it.Kind {
		case pdf.ItemText:
			seenText = true
		case pdf.ItemRule:
			if seenText {
				t.Fatalf("item %d is a rule drawn after text; a later fill would cover the text", i)
			}
		}
	}
	if !seenText {
		t.Fatal("the continuation page drew no text at all")
	}
}
