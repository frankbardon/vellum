package sheet_test

import (
	"strings"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/sheet"
	"github.com/frankbardon/vellum/spec"
)

// TestLower_ProducesASheetForAnEmptyDocument pins the degenerate case.
//
// A workbook with zero sheets is one Excel refuses outright, so an empty
// specification still gets one — the same rule [pdf.Lower] follows for a page.
func TestLower_ProducesASheetForAnEmptyDocument(t *testing.T) {
	// Built directly rather than resolved: a specification with no blocks at
	// all is not one [spec.Spec.Validate] accepts, so this is the shape a
	// resolved document with genuinely nothing in it takes.
	res := &fragment.Doc{}
	wb, err := sheet.Lower(res)
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	if len(wb.Sheets) != 1 {
		t.Fatalf("Sheets = %d, want 1", len(wb.Sheets))
	}
	if wb.Sheets[0].Name != "Sheet1" {
		t.Errorf("Name = %q, want %q", wb.Sheets[0].Name, "Sheet1")
	}
}

// TestLower_HeadingIsAStyledCellAboveTheTable pins
// [capability.FeatureBlockHeading]'s declared degradation.
func TestLower_HeadingIsAStyledCellAboveTheTable(t *testing.T) {
	wb := lower(t, heading(1, "Findings"))
	if len(wb.Sheets) != 1 || len(wb.Sheets[0].Rows) != 1 {
		t.Fatalf("Sheets = %+v", wb.Sheets)
	}
	row := wb.Sheets[0].Rows[0]
	if len(row.Cells) != 1 || row.Cells[0].Column != 1 {
		t.Fatalf("row 1 = %+v, want one cell in column 1", row)
	}
	if row.Cells[0].Value.Kind != sheet.CellText || row.Cells[0].Value.Text != "Findings" {
		t.Errorf("cell = %+v, want the heading text", row.Cells[0].Value)
	}

	format := wb.Styles.Formats[row.Cells[0].StyleID]
	if format.FontIndex == 0 {
		t.Fatal("the heading cell uses the default font, so nothing distinguishes it")
	}
	font := wb.Styles.Fonts[format.FontIndex-1]
	if !font.Bold {
		t.Error("the heading cell's font is not bold")
	}
	if format.WrapText {
		t.Error("the heading cell wraps; only a text block should")
	}
}

// TestLower_TextIsAWrappedCell pins [capability.FeatureBlockText]'s declared
// degradation.
func TestLower_TextIsAWrappedCell(t *testing.T) {
	wb := lower(t, text("Some prose."))
	row := wb.Sheets[0].Rows[0]
	format := wb.Styles.Formats[row.Cells[0].StyleID]
	if !format.WrapText {
		t.Error("a text block's cell does not wrap")
	}
}

// TestLower_SpacerIsOneBlankRow pins [capability.FeatureBlockSpacer]'s
// declared degradation: "a blank row", not a height proportional to the
// spacer's own — there is no page to conserve space on.
func TestLower_SpacerIsOneBlankRow(t *testing.T) {
	wb := lower(t, text("before"), spacer, text("after"))
	rows := wb.Sheets[0].Rows
	if len(rows) != 2 {
		t.Fatalf("Rows = %+v, want two populated rows either side of one blank one", rows)
	}
	if rows[0].Index != 1 || rows[1].Index != 3 {
		t.Errorf("populated rows are %d and %d, want 1 and 3 with row 2 blank", rows[0].Index, rows[1].Index)
	}
}

// TestLower_NotesBecomeACellComment pins [capability.FeatureBlockNotes]'s
// declared degradation.
func TestLower_NotesBecomeACellComment(t *testing.T) {
	wb := lower(t, notes)
	s := wb.Sheets[0]
	if len(s.Comments) != 1 {
		t.Fatalf("Comments = %+v, want one", s.Comments)
	}
	if s.Comments[0].Text != "Base: all respondents." {
		t.Errorf("comment text = %q", s.Comments[0].Text)
	}
	if s.Comments[0].Row != 1 || s.Comments[0].Col != 1 {
		t.Errorf("comment at (%d,%d), want (1,1)", s.Comments[0].Row, s.Comments[0].Col)
	}
}

// TestLower_PageBreakStartsANewSheet pins [capability.FeatureBlockPageBreak]'s
// declared degradation: "a new sheet".
func TestLower_PageBreakStartsANewSheet(t *testing.T) {
	wb := lower(t, heading(1, "First"), pageBreak, heading(1, "Second"))
	if len(wb.Sheets) != 2 {
		t.Fatalf("Sheets = %d, want 2", len(wb.Sheets))
	}
	if got := wb.Sheets[0].Rows[0].Cells[0].Value.Text; got != "First" {
		t.Errorf("sheet 1 carries %q", got)
	}
	if got := wb.Sheets[1].Rows[0].Cells[0].Value.Text; got != "Second" {
		t.Errorf("sheet 2 carries %q", got)
	}
	if wb.Sheets[0].Name == wb.Sheets[1].Name {
		t.Errorf("both sheets are named %q", wb.Sheets[0].Name)
	}
}

// TestLower_APageBreakOnAnEmptySheetProducesNoLeadingSheet checks the edge a
// naive close-and-open would get wrong: a page_break before any content, or
// two adjacent ones, must not leave an empty tab nobody asked for.
func TestLower_APageBreakOnAnEmptySheetProducesNoLeadingSheet(t *testing.T) {
	wb := lower(t, pageBreak, pageBreak, heading(1, "Only"))
	if len(wb.Sheets) != 1 {
		t.Fatalf("Sheets = %d, want 1 (no empty leading tabs)", len(wb.Sheets))
	}
}

// TestLower_SectionsDoNotStartASheet checks that a specification's logical
// divisions do not, by themselves, break the sheet — only an explicit
// page_break does, mirroring what a section means in every other format this
// library writes.
func TestLower_SectionsDoNotStartASheet(t *testing.T) {
	wb := lowerSpec(t, &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Sections: []spec.Section{
			{ID: "a", Blocks: []spec.Block{heading(1, "A")}},
			{ID: "b", Blocks: []spec.Block{heading(1, "B")}},
		},
	})
	if len(wb.Sheets) != 1 {
		t.Fatalf("Sheets = %d, want 1", len(wb.Sheets))
	}
	if len(wb.Sheets[0].Rows) != 2 {
		t.Fatalf("Rows = %+v, want both headings on the one sheet", wb.Sheets[0].Rows)
	}
}

// TestLower_RefusesANilDocument pins the invariant every writer's Lower
// checks.
func TestLower_RefusesANilDocument(t *testing.T) {
	_, err := sheet.Lower(nil)
	if !verr.HasCode(err, verr.VELLUM_INTERNAL_INVARIANT) {
		t.Fatalf("error = %v, want VELLUM_INTERNAL_INVARIANT", err)
	}
}

// TestLower_AssetNeverReachesTheWriter checks that FeatureBlockAsset's
// rejection is enforced at resolve time, before any fragment.Doc carrying an
// asset block for XLSX can exist — so this writer needs no branch for it at
// all.
func TestLower_AssetNeverReachesTheWriter(t *testing.T) {
	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Sections: []spec.Section{{ID: "s", Blocks: []spec.Block{
			{Kind: spec.BlockAsset, Asset: &spec.Asset{
				Handle: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
			}},
		}}},
	}
	_, err := resolveErr(t, s)
	if !verr.HasCode(err, verr.VELLUM_CAPABILITY_REJECTED) {
		t.Fatalf("error = %v, want VELLUM_CAPABILITY_REJECTED", err)
	}
}

// TestSheet_ContentReachesThePackage checks that lowered content actually
// lands in the written bytes, not only in the model — the shared string table
// and the cell placement together.
func TestSheet_ContentReachesThePackage(t *testing.T) {
	wb := lower(t, heading(1, "Composed To XLSX"), text("Some prose that should wrap."))
	raw := write(t, wb)

	xml := sheetPart(t, raw, 0)
	strs := part(t, raw, "xl/sharedStrings.xml")
	if !strings.Contains(strs, "Composed To XLSX") {
		t.Errorf("shared strings do not carry the heading:\n%s", strs)
	}
	if !strings.Contains(xml, `r="A1"`) {
		t.Errorf("sheet1 does not place a cell at A1:\n%s", xml)
	}
}
