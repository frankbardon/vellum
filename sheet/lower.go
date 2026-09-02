package sheet

import (
	"sort"
	"strconv"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/theme"
)

// Lower converts a resolved document into a SpreadsheetML workbook.
//
// It takes a [fragment.Doc] rather than a specification, which is the whole
// reason the resolve pass exists: theme application, font selection, number
// formatting and asset resolution have already happened, once, in a place all
// four writers share. A block kind this writer cannot render is a hard error
// naming the kind and its position, for the reason [doc.Lower] and
// [deck.Lower]'s are: silently dropping content is the failure mode this
// library exists to prevent.
//
// # What is different here
//
// Nothing paginates. [capability.FeatureOverflowContinue] degrades to "one
// continuous sheet" rather than to a split, because a sheet has no page: a
// table of any length is written as consecutive rows in the sheet it started
// on. The one thing that does start a new sheet is an explicit
// [spec.BlockPageBreak], which is what a caller reaches for to give two large
// tables independent frozen panes and filter ranges — a sheet carries at most
// one of each, so a document putting two tables on one sheet gets freezing at
// the first and ordinary scrolling at the rest.
func Lower(d *fragment.Doc) (*Workbook, error) {
	if d == nil {
		return nil, verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT,
			"the resolved document is nil")
	}

	l := &lowering{
		doc:    d,
		styles: newStyleBuilder(defaultFontFrom(d)),
		theme:  stylesFrom(d),
	}

	for i := range d.Sections {
		if err := l.section(i, &d.Sections[i]); err != nil {
			return nil, err
		}
	}
	l.closeSheet()

	if len(l.sheets) == 0 {
		// A workbook with no content is still a workbook: SpreadsheetML has
		// nowhere else to put the styles part, and a workbook with zero
		// sheets is one Excel refuses outright rather than opening empty.
		l.sheets = []Sheet{{Name: "Sheet1"}}
	}

	return &Workbook{
		Title:  d.Title,
		Sheets: l.sheets,
		Styles: l.styles.sheet(),
	}, nil
}

// defaultFontFrom is the workbook's font index 0: the body face at its own
// size, which is what an ordinary cell with no other styling takes.
func defaultFontFrom(d *fragment.Doc) Font {
	th := stylesFrom(d)
	return Font{Name: th.BodyFont, SizeEMU: th.BodySize, Color: th.TextColor}
}

// lowering carries the state of one conversion.
type lowering struct {
	doc    *fragment.Doc
	styles *styleBuilder
	theme  themeStyles

	// sheets are the sheets already closed, and current the one being filled.
	sheets  []Sheet
	current *Sheet

	// row is the next row index to write in the current sheet, one-based.
	row int

	// colWidth tracks the widest content seen in each column of the current
	// sheet, in runes, for [lowering.closeSheet]'s width heuristic. Wrapped
	// cells are deliberately excluded — see [lowering.measure] — because a
	// wrapped cell's entire point is that its column need not grow to hold it.
	colWidth map[int]int
}

// section lowers one resolved section.
//
// A section does not start a sheet. A specification's sections are logical
// divisions, not sheet breaks — breaking between every heading and its prose
// would put a heading alone on a sheet, which is not what the author asked
// for. An explicit page_break block is what starts a sheet, mirroring what a
// page_break means in every other format this library writes.
func (l *lowering) section(index int, s *fragment.Section) error {
	for i := range s.Blocks {
		if err := l.block(index, s.ID, i, &s.Blocks[i]); err != nil {
			return err
		}
	}
	return nil
}

func (l *lowering) block(sectionIndex int, sectionID string, blockIndex int, b *fragment.Block) error {
	switch b.Kind {
	case spec.BlockHeading:
		l.heading(b.Paragraph)
		return nil

	case spec.BlockText:
		l.paragraph(b.Paragraph)
		return nil

	case spec.BlockSpacer:
		// A blank row, singular — there is no page to conserve space on, so
		// the space a spacer asked for and the space between two ordinary
		// rows collapse to the same thing: one row with nothing in it.
		l.ensureSheet()
		l.row++
		return nil

	case spec.BlockPageBreak:
		l.closeSheet()
		return nil

	case spec.BlockNotes:
		l.note(b.Note)
		return nil

	case spec.BlockTable:
		return l.table(b.Table, sectionIndex, sectionID, blockIndex)
	}

	// A kind the matrix declares and this writer does not draw is an
	// invariant rather than a capability rejection, and deliberately so: the
	// matrix says it renders — every kind but asset does, and asset is
	// refused before resolution ever produces this document — so the gap
	// would be in this package rather than in what Vellum promises.
	return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
		"the XLSX writer does not lower this block kind",
		map[string]any{
			"kind":          string(b.Kind),
			"section_index": sectionIndex,
			"section_id":    sectionID,
			"block_index":   blockIndex,
		})
}

// heading writes a paragraph's text as one bold, larger cell — [FeatureBlockHeading]'s
// declared degradation: "a styled cell above the table".
//
// Not wrapped. A heading is a title, and a title that overruns its column
// simply overlaps the empty cells beside it, which is how a short label reads
// in any spreadsheet — wrapping it would shrink it into several short lines
// for no reason a heading has.
func (l *lowering) heading(p *fragment.Paragraph) {
	l.ensureSheet()
	text := p.Text()

	size := l.doc.Scale.HeadingSize(p.OutlineLevel)
	format := l.styles.format(CellFormat{
		FontIndex: l.styles.font(Font{
			Name: l.theme.HeadingFont, SizeEMU: size, Color: l.theme.headingColor(l.doc), Bold: true,
		}),
	})
	l.place(1, Text(text), format)
	l.measure(1, text, false)
	l.row++
}

// paragraph writes a body-text block as one wrapped cell — [FeatureBlockText]'s
// declared degradation: "a wrapped cell".
func (l *lowering) paragraph(p *fragment.Paragraph) {
	l.ensureSheet()
	text := p.Text()

	format := l.styles.format(CellFormat{WrapText: true})
	l.place(1, Text(text), format)
	l.row++
}

// note attaches a comment to a cell of its own — [FeatureBlockNotes]'s declared
// degradation: "a cell comment". Unlike a table cell's own annotation, a
// standalone notes block has no existing cell to attach to, so it gets an
// empty one at column 1 of its own row.
func (l *lowering) note(n *fragment.Note) {
	l.ensureSheet()
	l.current.Comments = append(l.current.Comments, Comment{
		Row: l.row, Col: 1, Text: n.Body.Text(),
	})
	l.row++
}

// ensureSheet opens a sheet if none is open.
func (l *lowering) ensureSheet() {
	if l.current == nil {
		l.current = &Sheet{}
		l.row = 1
		l.colWidth = map[int]int{}
	}
}

// closeSheet finishes the current sheet, names it, applies the accumulated
// column-width heuristic, and appends it. A closeSheet with nothing written
// is a no-op — an explicit page_break before any content, or two in a row,
// does not produce an empty leading sheet nobody asked for.
func (l *lowering) closeSheet() {
	if l.current == nil || l.row == 1 {
		l.current = nil
		return
	}

	s := *l.current
	s.Name = sheetName(len(l.sheets))
	s.Columns = widthsFrom(l.colWidth)
	l.sheets = append(l.sheets, s)
	l.current = nil
}

// sheetName is the default tab name a sheet with no name of its own asked for.
// Excel's own convention, so a Vellum workbook's tabs read like an authored
// one's until a consumer has a reason to name them.
func sheetName(index int) string {
	return "Sheet" + strconv.Itoa(index+1)
}

// place writes one cell at (row, col) in the current sheet, at the row the
// cursor is on.
func (l *lowering) place(col int, v CellValue, styleID int) {
	l.current.Rows = append(l.current.Rows, Row{
		Index: l.row,
		Cells: []Cell{{Column: col, Value: v, StyleID: styleID}},
	})
}

// measure feeds one cell's content length into the current sheet's
// column-width heuristic.
//
// Wrapped cells are excluded on purpose: a wrapped cell's whole reason for
// being is that the column does not have to grow to hold it, and feeding a
// paragraph's length in here would make the flow column as wide as its
// longest line — exactly the growth wrapping exists to avoid.
func (l *lowering) measure(col int, text string, wrapped bool) {
	if wrapped {
		return
	}
	if n := len([]rune(text)); n > l.colWidth[col] {
		l.colWidth[col] = n
	}
}

// headingColor resolves the heading colour role, falling back to the body
// text colour when the theme declares none — the same fallback [doc]'s
// heading style takes.
func (t themeStyles) headingColor(d *fragment.Doc) string {
	if v, ok := d.Palette.Lookup(theme.ColorHeading); ok {
		return v
	}
	return t.TextColor
}

// minColumnWidth, maxColumnWidth and columnPadding bound and pad the
// content-length heuristic. Cosmetic only: xlsx column width is not a layout
// constraint the way a page's content width is, so there is no capacity this
// number has to be exactly right for — a column a little too narrow or a
// little too wide is legible either way, which is what makes a simple
// heuristic the right amount of investment here.
const (
	minColumnWidth = 8.0
	maxColumnWidth = 40.0
	columnPadding  = 2.0
)

// widthsFrom converts the accumulated per-column content lengths into
// [ColumnWidth] overrides, sorted by column so the emitted `<cols>` collection
// is a function of the sheet's own content rather than of map iteration order.
func widthsFrom(seen map[int]int) []ColumnWidth {
	if len(seen) == 0 {
		return nil
	}
	cols := make([]int, 0, len(seen))
	for c := range seen {
		cols = append(cols, c)
	}
	sort.Ints(cols)

	out := make([]ColumnWidth, 0, len(cols))
	for _, c := range cols {
		width := float64(seen[c]) + columnPadding
		if width < minColumnWidth {
			width = minColumnWidth
		}
		if width > maxColumnWidth {
			width = maxColumnWidth
		}
		out = append(out, ColumnWidth{Column: c, Width: width})
	}
	return out
}
