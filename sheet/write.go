package sheet

import (
	"io"
	"strconv"
	"strings"
	"time"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
)

// Part names this writer emits, excluding per-sheet and per-comment parts,
// which are indexed and built with [worksheetPartName] and friends.
const (
	PartWorkbook      = "/xl/workbook.xml"
	PartStyles        = "/xl/styles.xml"
	PartSharedStrings = "/xl/sharedStrings.xml"
	PartCoreProps     = "/docProps/core.xml"
	PartAppProps      = "/docProps/app.xml"
	PartCustom        = "/docProps/custom.xml"
)

// WriteOptions configures a write. The zero value is the deterministic
// default.
type WriteOptions struct {
	// SourceDateEpoch stamps every date the package carries. The zero value
	// selects the pinned epoch.
	SourceDateEpoch time.Time

	// Producer names the software in the package's extended properties.
	Producer string
}

// defaultProducer is what appears in a workbook's application property when
// the caller names nothing.
const defaultProducer = "Vellum"

// writer carries the state of one package assembly.
//
// The shared string table is built here rather than by the caller, because it
// is a property of the whole workbook — deterministic first-seen order over a
// canonical walk of every sheet in [Workbook.Sheets] order, row by row, cell by
// cell — and building it anywhere else would be a second place that order
// could be decided differently.
type writer struct {
	wb       *Workbook
	epoch    time.Time
	producer string

	// strings interns text values by first-seen order and hands back their
	// index into the shared string table.
	strings *stringTable

	// sheetRels maps a sheet's index to its worksheet part's relationship ID
	// within workbook.xml.rels, and commentRels does the same for the
	// comments part a sheet with comments carries.
	sheetRels   []string
	commentRels map[int]string
	vmlRels     map[int]string
}

// stringTable interns strings in first-seen order.
//
// A slice with a lookup index rather than a map iterated at write time: the
// emitted order is part of the bytes, and first-seen order is exactly what a
// map cannot promise across two builds of the same content by different code
// paths — see CLAUDE.md's determinism rules on ordered output.
type stringTable struct {
	values []string
	index  map[string]int

	// total counts every intern call, including repeats, which is the shared
	// string part's own `count` attribute — the number of cells that reference
	// the table, as distinct from `uniqueCount`, the size of [values].
	total int
}

func newStringTable() *stringTable {
	return &stringTable{index: make(map[string]int)}
}

// intern returns s's shared-string index, assigning the next one on first
// sight, and counts this as one more use toward the table's `count`
// attribute.
//
// Called exactly once per cell, during [writer.internStrings]'s pre-scan —
// never again while the cells themselves are serialised. A second call per
// cell would count every reference twice, which is the one way this table's
// own `count` could disagree with the number of cells that actually cite it.
// [stringTable.lookup] is what the cell writer uses instead.
func (t *stringTable) intern(s string) int {
	t.total++
	if i, ok := t.index[s]; ok {
		return i
	}
	i := len(t.values)
	t.values = append(t.values, s)
	t.index[s] = i
	return i
}

// lookup returns a string's already-interned index, without counting a use.
//
// It exists because a cell is visited twice: once by the pre-scan that builds
// the table, and once when the worksheet's own XML is written. Only the first
// visit is a "use" for the table's `count` attribute; the second is reading
// back an index that visit already assigned.
func (t *stringTable) lookup(s string) int {
	// A miss here is an invariant violation, not a user error: every string a
	// cell writes was, by construction, interned during the pre-scan before
	// any part was serialised. Falling back to intern rather than panicking
	// keeps a future caller's mistake a wrong index rather than a crash, and
	// is caught the same way any other wrong index would be, in a test that
	// reads the bytes back.
	if i, ok := t.index[s]; ok {
		return i
	}
	return t.intern(s)
}

// Package assembles the OPC package for this workbook.
func (wb *Workbook) Package(opts WriteOptions) (*opc.Package, error) {
	if len(wb.Sheets) == 0 {
		return nil, verr.NewCodedError(verr.VELLUM_SHEET_INVALID,
			"a workbook must have at least one sheet")
	}
	if err := validateSheetNames(wb.Sheets); err != nil {
		return nil, err
	}

	epoch := opts.SourceDateEpoch
	if epoch.IsZero() {
		epoch = zipdet.PinnedEpoch
	}
	producer := opts.Producer
	if producer == "" {
		producer = defaultProducer
	}

	w := &writer{
		wb: wb, epoch: epoch, producer: producer,
		strings:     newStringTable(),
		commentRels: map[int]string{},
		vmlRels:     map[int]string{},
	}

	// The shared string table is built from a walk over the sheets before
	// anything is serialised, because a cell's own XML names a string by its
	// table index and the table's contents decide those indices.
	w.internStrings()

	p := opc.New()
	ct := p.ContentTypes()
	ct.SetDefault("xml", ctXML)
	ct.SetDefault("vml", ctVML)
	ct.SetDefault("rels", "application/vnd.openxmlformats-package.relationships+xml")

	if err := w.buildWorkbookRelationships(p); err != nil {
		return nil, err
	}

	if err := p.Put(&opc.Part{Name: PartWorkbook, ContentType: ctWorkbook, Data: w.workbookXML()}); err != nil {
		return nil, err
	}
	if err := p.Put(&opc.Part{Name: PartStyles, ContentType: ctStyles, Data: w.stylesXML()}); err != nil {
		return nil, err
	}
	if len(w.strings.values) > 0 {
		if err := p.Put(&opc.Part{Name: PartSharedStrings, ContentType: ctSharedStrings,
			Data: w.sharedStringsXML()}); err != nil {
			return nil, err
		}
	}

	for i := range wb.Sheets {
		s := &wb.Sheets[i]
		if err := p.Put(&opc.Part{Name: worksheetPartName(i), ContentType: ctWorksheet,
			Data: w.worksheetXML(i, s)}); err != nil {
			return nil, err
		}
		if err := w.buildWorksheetRelationships(p, i, s); err != nil {
			return nil, err
		}

		if len(s.Comments) > 0 {
			if err := p.Put(&opc.Part{Name: commentsPartName(i), ContentType: ctComments,
				Data: w.commentsXML(s)}); err != nil {
				return nil, err
			}
			// The "vml" extension default declared above already covers this
			// part's content type, so no override is needed here.
			if err := p.Put(&opc.Part{Name: vmlDrawingPartName(i),
				Data: w.vmlDrawingXML(i, s)}); err != nil {
				return nil, err
			}
		}
	}

	if err := p.Put(&opc.Part{Name: PartCoreProps, ContentType: ctCoreProperties, Data: w.corePropsXML()}); err != nil {
		return nil, err
	}
	if err := p.Put(&opc.Part{Name: PartAppProps, ContentType: ctExtendedProps, Data: w.appPropsXML()}); err != nil {
		return nil, err
	}
	if wb.Provenance != nil {
		if err := p.Put(&opc.Part{Name: PartCustom, ContentType: ctCustomProps, Data: w.customPropsXML()}); err != nil {
			return nil, err
		}
	}

	root := p.Relationships("/")
	for _, r := range []struct{ typ, target string }{
		{relExtendedProps, "docProps/app.xml"},
		{relOfficeDocument, "xl/workbook.xml"},
		{relCoreProperties, "docProps/core.xml"},
	} {
		if _, err := root.Add(r.typ, r.target, opc.TargetInternal); err != nil {
			return nil, err
		}
	}
	if wb.Provenance != nil {
		if _, err := root.Add(relCustomProps, "docProps/custom.xml", opc.TargetInternal); err != nil {
			return nil, err
		}
	}

	return p, nil
}

// internStrings walks the sheets in order and interns every text and date
// value's rendered form.
//
// Dates are not interned: a date cell is a live number, never a shared
// string, which is the entire reason [numfmt.Serial] exists. Only [CellText]
// reaches the table.
func (w *writer) internStrings() {
	for i := range w.wb.Sheets {
		s := &w.wb.Sheets[i]
		for r := range s.Rows {
			row := &s.Rows[r]
			for c := range row.Cells {
				cell := &row.Cells[c]
				if cell.Value.Kind == CellText {
					w.strings.intern(cell.Value.Text)
				}
			}
		}
	}
}

// buildWorkbookRelationships declares workbook.xml's relationships and
// resolves the identifiers the workbook markup will reference.
func (w *writer) buildWorkbookRelationships(p *opc.Package) error {
	rels := p.Relationships(PartWorkbook)
	rels.AlwaysEmit()

	targets := []struct{ typ, target string }{
		{relStyles, "styles.xml"},
	}
	if len(w.strings.values) > 0 {
		targets = append(targets, struct{ typ, target string }{relSharedStrings, "sharedStrings.xml"})
	}
	for i := range w.wb.Sheets {
		targets = append(targets, struct{ typ, target string }{
			relWorksheet, "worksheets/sheet" + strconv.Itoa(i+1) + ".xml"})
	}

	for _, t := range targets {
		if _, err := rels.Add(t.typ, t.target, opc.TargetInternal); err != nil {
			return err
		}
	}
	rels.Freeze()

	w.sheetRels = make([]string, len(w.wb.Sheets))
	for i := range w.wb.Sheets {
		id, ok := rels.IDFor(relWorksheet, "worksheets/sheet"+strconv.Itoa(i+1)+".xml")
		if !ok {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_OPC_RELATIONSHIP_INVALID,
				"a worksheet relationship the workbook markup references was not declared",
				map[string]any{"sheet_index": i})
		}
		w.sheetRels[i] = id
	}
	return nil
}

// buildWorksheetRelationships declares one worksheet's own relationships — to
// its comments and legacy-drawing parts, when it has any — and resolves the
// identifier the sheet markup's `legacyDrawing` reference needs.
func (w *writer) buildWorksheetRelationships(p *opc.Package, index int, s *Sheet) error {
	if len(s.Comments) == 0 {
		return nil
	}

	rels := p.Relationships(worksheetPartName(index))
	rels.AlwaysEmit()

	if _, err := rels.Add(relComments, "../comments"+strconv.Itoa(index+1)+".xml", opc.TargetInternal); err != nil {
		return err
	}
	if _, err := rels.Add(relVMLDrawing, "../drawings/vmlDrawing"+strconv.Itoa(index+1)+".vml", opc.TargetInternal); err != nil {
		return err
	}
	rels.Freeze()

	id, ok := rels.IDFor(relVMLDrawing, "../drawings/vmlDrawing"+strconv.Itoa(index+1)+".vml")
	if !ok {
		return verr.NewCodedError(verr.VELLUM_OPC_RELATIONSHIP_INVALID,
			"the legacy drawing relationship was not declared")
	}
	w.vmlRels[index] = id
	return nil
}

// WriteTo emits the workbook as an .xlsx.
func (wb *Workbook) WriteTo(w io.Writer, opts WriteOptions) error {
	p, err := wb.Package(opts)
	if err != nil {
		return err
	}
	epoch := opts.SourceDateEpoch
	if epoch.IsZero() {
		epoch = zipdet.PinnedEpoch
	}
	return p.WriteTo(w, zipdet.WriteOptions{SourceDateEpoch: epoch})
}

func worksheetPartName(index int) string {
	return "/xl/worksheets/sheet" + strconv.Itoa(index+1) + ".xml"
}
func commentsPartName(index int) string {
	return "/xl/comments" + strconv.Itoa(index+1) + ".xml"
}
func vmlDrawingPartName(index int) string {
	return "/xl/drawings/vmlDrawing" + strconv.Itoa(index+1) + ".vml"
}

// validateSheetNames checks Excel's own constraints: non-empty, at most 31
// characters, none of the seven characters the schema forbids, and no two
// sheets sharing a name.
//
// [VELLUM_SHEET_INVALID] rather than a silent truncation or rename. A workbook
// whose second "Findings" sheet was quietly renamed "Findings (2)" is a
// workbook whose tab names no longer match whatever the caller — or a fill
// binding referencing a sheet by name — expected to find.
func validateSheetNames(sheets []Sheet) error {
	seen := make(map[string]bool, len(sheets))
	for i := range sheets {
		name := sheets[i].Name
		if name == "" {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_SHEET_INVALID,
				"a sheet has no name", map[string]any{"sheet_index": i})
		}
		if len(name) > 31 {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_SHEET_INVALID,
				"a sheet name is longer than Excel's 31-character limit",
				map[string]any{"sheet_index": i, "name": name})
		}
		if strings.ContainsAny(name, `:\/?*[]`) {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_SHEET_INVALID,
				"a sheet name carries a character Excel forbids in a tab name",
				map[string]any{"sheet_index": i, "name": name, "forbidden": `: \ / ? * [ ]`})
		}
		if seen[name] {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_SHEET_INVALID,
				"two sheets share a name", map[string]any{"sheet_index": i, "name": name})
		}
		seen[name] = true
	}
	return nil
}
