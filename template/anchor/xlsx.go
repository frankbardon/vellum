package anchor

// discoverXLSX is E11-S1's xlsx discoverer: it finds two anchor mechanisms —
// a workbook-scoped defined name resolving to a single absolute cell
// (KindDefinedName), and an Excel Table's columns (KindTableColumn) — neither
// of which has anything in common with DOCX's w:sdt or {{marker}}, so this
// file shares no code with docx.go beyond the xmlcopy.Walk-only discipline
// every discoverer in this package follows.
//
// Both mechanisms are read starting from mainPart (xl/workbook.xml): the
// <sheets> list gives every worksheet's own name and relationship id, and
// <definedNames> gives every defined name's own name and formula text.
// Resolving a sheet name or a table's own relationship id to a real OPC part
// name goes through the package's relationship graph — never a hardcoded
// "xl/worksheets/sheet1.xml" guess — mirroring how template.Open itself
// resolves the officeDocument relationship rather than assuming a part name.

import (
	"sort"
	"strconv"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/xmlcopy"
)

const (
	// nsSpreadsheet is the SpreadsheetML main namespace, matched by resolved
	// URI rather than by a literal "" default-namespace guess, mirroring
	// template/anchor's own nsWordprocessing constant and the same reasoning.
	nsSpreadsheet = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"

	// nsRelationships is the officeDocument relationships namespace: every
	// r:id attribute this file reads (a sheet's own r:id, a tablePart's own
	// r:id) resolves to this URI once xmlcopy.Walk has seen the owning
	// element's xmlns:r declaration.
	nsRelationships = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

	// relTable is the relationship type a worksheet's own <tableParts> entry
	// carries, naming the xl/tables/tableN.xml part that defines the Excel
	// Table.
	relTable = nsRelationships + "/table"
)

// sheetRef is one <sheet> entry read from the workbook part: its own name and
// the relationship id that resolves to its worksheet part.
type sheetRef struct {
	name string
	rID  string
}

// definedNameRef is one <definedName> entry read from the workbook part: its
// own name and formula text, unparsed.
type definedNameRef struct {
	name    string
	formula string
}

// discoverXLSX walks the workbook part once for sheets and defined names,
// then walks every worksheet part once for its own <tableParts> reference and
// each referenced table's own definition, producing one KindDefinedName
// anchor per defined name and one KindTableColumn anchor per (table, column)
// pair.
func discoverXLSX(pkg *opc.Package, mainPart string) (*Inventory, error) {
	wbSrc, err := partBytes(pkg, mainPart)
	if err != nil {
		return nil, err
	}

	sheets, definedNames, err := scanWorkbook(wbSrc)
	if err != nil {
		return nil, err
	}

	var anchors []Anchor
	seen := make(map[string]xlsxSeenEntry, len(definedNames)) // lookup only, never ranged

	for _, dn := range definedNames {
		a, err := discoverDefinedName(pkg, mainPart, sheets, dn)
		if err != nil {
			return nil, err
		}
		if prior, dup := seen[a.Name]; dup {
			return nil, duplicateXLSXAnchorErr(a.Name, prior, xlsxSeenEntry{kind: a.Kind, part: a.Part, span: a.Span})
		}
		seen[a.Name] = xlsxSeenEntry{kind: a.Kind, part: a.Part, span: a.Span}
		anchors = append(anchors, a)
	}

	for _, sh := range sheets {
		sheetPart, ok := resolveRel(pkg, mainPart, sh.rID)
		if !ok {
			continue // a <sheet> whose r:id does not resolve is an xlsx structural
			// problem outside a defined name or a table; nothing this story's own
			// scope needs to reject.
		}
		sheetSrc, err := partBytes(pkg, sheetPart)
		if err != nil {
			return nil, err
		}

		tableRIDs, err := scanTableParts(sheetSrc)
		if err != nil {
			return nil, err
		}
		for _, rid := range tableRIDs {
			tablePart, ok := resolveRelFrom(pkg, sheetPart, rid, relTable)
			if !ok {
				continue
			}
			tableSrc, err := partBytes(pkg, tablePart)
			if err != nil {
				return nil, err
			}
			cols, err := discoverTableColumns(sheetPart, sheetSrc, tablePart, tableSrc)
			if err != nil {
				return nil, err
			}
			for _, a := range cols {
				if prior, dup := seen[a.Name]; dup {
					return nil, duplicateXLSXAnchorErr(a.Name, prior, xlsxSeenEntry{kind: a.Kind, part: a.Part, span: a.Span})
				}
				seen[a.Name] = xlsxSeenEntry{kind: a.Kind, part: a.Part, span: a.Span}
				anchors = append(anchors, a)
			}
		}
	}

	// Deterministic order across possibly-several worksheet parts: document
	// order within a part, parts in bytewise name order — the natural
	// generalisation of discoverDOCX's own single-part Span.Start sort, which
	// alone is not enough once anchors can come from more than one part.
	sort.SliceStable(anchors, func(i, j int) bool {
		if anchors[i].Part != anchors[j].Part {
			return anchors[i].Part < anchors[j].Part
		}
		return anchors[i].Span.Start < anchors[j].Span.Start
	})

	return &Inventory{Anchors: anchors}, nil
}

// xlsxSeenEntry records where an xlsx anchor name was first claimed, across
// however many worksheet parts a duplicate might span — deliberately its own
// type rather than docx.go's seenEntry, because a cross-part collision needs
// both anchors' own part names in the error, which seenEntry's single-part
// assumption (and duplicateAnchorErr's single "part" argument) does not
// carry. DOCX's discoverer and its own tests are untouched by this split.
type xlsxSeenEntry struct {
	kind Kind
	part string
	span xmlcopy.Span
}

func duplicateXLSXAnchorErr(name string, first, second xlsxSeenEntry) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_ANCHOR_DUPLICATE,
		"two anchors share one name",
		map[string]any{
			"name":              name,
			"first_kind":        string(first.kind),
			"first_part":        first.part,
			"first_span_start":  first.span.Start,
			"first_span_end":    first.span.End,
			"second_kind":       string(second.kind),
			"second_part":       second.part,
			"second_span_start": second.span.Start,
			"second_span_end":   second.span.End,
		})
}

// discoverDefinedName resolves one defined name to a KindDefinedName anchor.
func discoverDefinedName(pkg *opc.Package, mainPart string, sheets []sheetRef, dn definedNameRef) (Anchor, error) {
	sheetName, colLetters, row, ok := parseDefinedNameFormula(dn.formula)
	if !ok {
		return Anchor{}, definedNameUnsupportedErr(dn.name, dn.formula,
			"the formula is not a single-sheet, absolute, single-cell reference of the form SheetName!$COL$ROW")
	}

	var rID string
	found := false
	for _, sh := range sheets {
		if sh.name == sheetName {
			rID = sh.rID
			found = true
			break
		}
	}
	if !found {
		return Anchor{}, definedNameUnsupportedErr(dn.name, dn.formula,
			"the formula names a sheet the workbook's own <sheets> list does not contain")
	}

	sheetPart, ok := resolveRel(pkg, mainPart, rID)
	if !ok {
		return Anchor{}, definedNameUnsupportedErr(dn.name, dn.formula,
			"the sheet's own relationship does not resolve to a part in the package")
	}

	sheetSrc, err := partBytes(pkg, sheetPart)
	if err != nil {
		return Anchor{}, err
	}

	ref := colLetters + strconv.Itoa(row)
	cell, found, err := findCell(sheetSrc, ref)
	if err != nil {
		return Anchor{}, err
	}
	if !found {
		return Anchor{}, definedNameUnsupportedErr(dn.name, dn.formula,
			"the target cell "+ref+" is not present in the worksheet's own XML")
	}

	return Anchor{
		Name: dn.name,
		Kind: KindDefinedName,
		Part: sheetPart,
		Span: cell.Span,
	}, nil
}

// discoverTableColumns resolves one Excel Table part to its KindTableColumn
// anchors, one per declared column, all sharing the table's one sample data
// row's own Span.
func discoverTableColumns(sheetPart string, sheetSrc []byte, tablePart string, tableSrc []byte) ([]Anchor, error) {
	tbl, err := parseTable(tableSrc)
	if err != nil {
		return nil, err
	}

	dataRows := tbl.toRow - tbl.fromRow
	if dataRows != 1 {
		return nil, tableUnsupportedErr(tbl.displayName, tablePart,
			"the table's ref must cover exactly one data row below its header for a table_row repeat to have a sample row to clone",
			map[string]any{"data_row_count": dataRows, "ref": tbl.ref})
	}
	dataRowNum := tbl.fromRow + 1

	row, found, err := findRow(sheetSrc, dataRowNum)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, tableUnsupportedErr(tbl.displayName, tablePart,
			"the table's own data row is not present in the worksheet's own XML",
			map[string]any{"row": dataRowNum})
	}

	anchors := make([]Anchor, 0, len(tbl.columns))
	for i, col := range tbl.columns {
		absCol := tbl.fromCol + i
		present, err := rowHasCellForColumn(sheetSrc, row, absCol)
		if err != nil {
			return nil, err
		}
		if !present {
			return nil, tableUnsupportedErr(tbl.displayName, tablePart,
				"the table's data row has no placeholder cell for one of its declared columns",
				map[string]any{"column_name": col, "column": ColumnLetters(absCol) + strconv.Itoa(dataRowNum)})
		}

		anchors = append(anchors, Anchor{
			Name: tbl.displayName + "." + col,
			Kind: KindTableColumn,
			Part: sheetPart,
			Span: row.Span,
			Table: &TableColumn{
				DisplayName: tbl.displayName,
				Column:      absCol,
				TablePart:   tablePart,
				HeaderRow:   tbl.fromRow,
				FromColumn:  tbl.fromCol,
				ToColumn:    tbl.toCol,
			},
		})
	}
	return anchors, nil
}

// parsedTable is table1.xml's own declared shape, read once.
type parsedTable struct {
	displayName string
	ref         string
	fromCol     int
	fromRow     int
	toCol       int
	toRow       int
	columns     []string // tableColumn "name" attributes, in document order
}

func parseTable(src []byte) (parsedTable, error) {
	var tbl parsedTable
	rootFound := false

	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		switch {
		case e.Name.Space == nsSpreadsheet && e.Name.Local == "table" && !rootFound:
			rootFound = true
			tbl.displayName, _ = attrVal2(e, "", "displayName")
			tbl.ref, _ = attrVal2(e, "", "ref")

		case e.Name.Space == nsSpreadsheet && e.Name.Local == "tableColumn":
			if name, ok := attrVal2(e, "", "name"); ok {
				tbl.columns = append(tbl.columns, name)
			}
		}
		return nil
	})
	if err != nil {
		return parsedTable{}, err
	}
	if !rootFound || tbl.displayName == "" {
		return parsedTable{}, verr.NewCodedErrorWithDetails(verr.VELLUM_ANCHOR_TABLE_UNSUPPORTED,
			"the table part carries no <table displayName=...> root element",
			map[string]any{})
	}

	parts := strings.SplitN(tbl.ref, ":", 2)
	if len(parts) != 2 {
		return parsedTable{}, tableUnsupportedErr(tbl.displayName, "",
			"the table's ref attribute is not a two-corner range", map[string]any{"ref": tbl.ref})
	}
	fromCol, fromRow, ok1 := ParseSimpleCellRef(parts[0])
	toCol, toRow, ok2 := ParseSimpleCellRef(parts[1])
	if !ok1 || !ok2 {
		return parsedTable{}, tableUnsupportedErr(tbl.displayName, "",
			"the table's ref attribute does not parse as two cell references", map[string]any{"ref": tbl.ref})
	}
	tbl.fromCol = ColumnNumber(fromCol)
	tbl.fromRow = fromRow
	tbl.toCol = ColumnNumber(toCol)
	tbl.toRow = toRow

	return tbl, nil
}

// scanWorkbook walks the workbook part once for its <sheets> list and its
// <definedNames> list.
func scanWorkbook(src []byte) ([]sheetRef, []definedNameRef, error) {
	var sheets []sheetRef
	var names []definedNameRef

	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		switch {
		case e.Name.Space == nsSpreadsheet && e.Name.Local == "sheet":
			name, _ := attrVal2(e, "", "name")
			rid, _ := attrVal2(e, nsRelationships, "id")
			if name != "" && rid != "" {
				sheets = append(sheets, sheetRef{name: name, rID: rid})
			}

		case e.Name.Space == nsSpreadsheet && e.Name.Local == "definedName":
			name, _ := attrVal2(e, "", "name")
			formula := string(src[e.Content.Start:e.Content.End])
			if name != "" {
				names = append(names, definedNameRef{name: name, formula: formula})
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return sheets, names, nil
}

// scanTableParts walks a worksheet part once for its own <tableParts>
// <tablePart r:id="..."/> entries.
func scanTableParts(src []byte) ([]string, error) {
	var rids []string
	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if e.Name.Space == nsSpreadsheet && e.Name.Local == "tablePart" {
			if rid, ok := attrVal2(e, nsRelationships, "id"); ok {
				rids = append(rids, rid)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rids, nil
}

// findCell walks a worksheet part once looking for the <c> element whose r
// attribute exactly matches ref.
func findCell(src []byte, ref string) (xmlcopy.Element, bool, error) {
	var result xmlcopy.Element
	found := false
	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if found || e.Name.Space != nsSpreadsheet || e.Name.Local != "c" {
			return nil
		}
		if v, ok := attrVal2(e, "", "r"); ok && v == ref {
			result = e
			found = true
		}
		return nil
	})
	if err != nil {
		return xmlcopy.Element{}, false, err
	}
	return result, found, nil
}

// findRow walks a worksheet part once looking for the <row> element whose r
// attribute exactly matches rowNum.
func findRow(src []byte, rowNum int) (xmlcopy.Element, bool, error) {
	want := strconv.Itoa(rowNum)
	var result xmlcopy.Element
	found := false
	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if found || e.Name.Space != nsSpreadsheet || e.Name.Local != "row" {
			return nil
		}
		if v, ok := attrVal2(e, "", "r"); ok && v == want {
			result = e
			found = true
		}
		return nil
	})
	if err != nil {
		return xmlcopy.Element{}, false, err
	}
	return result, found, nil
}

// rowHasCellForColumn reports whether row's own content carries a <c> child
// whose reference's column matches absCol, regardless of what row number that
// cell's own reference states (the row it was found under is what matters).
func rowHasCellForColumn(src []byte, row xmlcopy.Element, absCol int) (bool, error) {
	found := false
	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if found || e.Name.Space != nsSpreadsheet || e.Name.Local != "c" {
			return nil
		}
		if e.Span.Start < row.Content.Start || e.Span.End > row.Content.End {
			return nil
		}
		v, ok := attrVal2(e, "", "r")
		if !ok {
			return nil
		}
		colLetters, _, ok := ParseSimpleCellRef(v)
		if ok && ColumnNumber(colLetters) == absCol {
			found = true
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return found, nil
}

// attrVal2 returns the value of the attribute named local in namespace space
// ("" for an attribute with no namespace, such as every plain SpreadsheetML
// attribute this file reads), and whether it was present at all — unlike
// docx.go's attrVal, which returns only the value and treats "absent" and
// "present but empty" alike, xlsx discovery needs to distinguish "no name
// attribute at all" (a structurally broken sheet or definedName) from "name
// is the empty string" in a couple of call sites, so this carries the ok
// return docx.go's own helper does not need.
func attrVal2(e xmlcopy.Element, space, local string) (string, bool) {
	for _, a := range e.Attr {
		if a.Name.Space == space && a.Name.Local == local {
			return a.Value, true
		}
	}
	return "", false
}

// parseDefinedNameFormula parses the restricted formula shape this version
// supports: an optional single-quoted sheet name (for a name containing a
// space or another character an unquoted sheet name may not), or a bare one,
// followed by "!", then an absolute single-cell reference — "$" + uppercase
// column letters + "$" + digits — and nothing else. See CLAUDE.md's own
// "Step 1" for why this is a direct match rather than a formula parser:
// nothing else a defined name's formula could legally contain (a range, a
// relative reference, a reference to another name) is in this version's
// scope, and the shape it does support is regular enough not to need one.
func parseDefinedNameFormula(formula string) (sheetName, colLetters string, row int, ok bool) {
	f := strings.TrimSpace(formula)

	var rest string
	if strings.HasPrefix(f, "'") {
		end := strings.IndexByte(f[1:], '\'')
		if end < 0 {
			return "", "", 0, false
		}
		end++ // index within f of the closing quote
		sheetName = f[1:end]
		if sheetName == "" || len(f) < end+2 || f[end+1] != '!' {
			return "", "", 0, false
		}
		rest = f[end+2:]
	} else {
		bang := strings.IndexByte(f, '!')
		if bang <= 0 {
			return "", "", 0, false
		}
		sheetName = f[:bang]
		if strings.ContainsAny(sheetName, "'!") {
			return "", "", 0, false
		}
		rest = f[bang+1:]
	}

	if len(rest) < 2 || rest[0] != '$' {
		return "", "", 0, false
	}
	body := rest[1:]

	i := 0
	for i < len(body) && body[i] >= 'A' && body[i] <= 'Z' {
		i++
	}
	if i == 0 {
		return "", "", 0, false
	}
	colLetters = body[:i]

	remainder := body[i:]
	if len(remainder) < 2 || remainder[0] != '$' {
		return "", "", 0, false
	}
	digits := remainder[1:]
	if digits == "" {
		return "", "", 0, false
	}
	n := 0
	for j := 0; j < len(digits); j++ {
		c := digits[j]
		if c < '0' || c > '9' {
			return "", "", 0, false
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 {
		return "", "", 0, false
	}
	return sheetName, colLetters, n, true
}

// partBytes fetches a part's own bytes, or an internal-invariant error when
// the package does not carry the named part — reachable only by a caller
// resolving a relationship to a part the package's own graph is internally
// inconsistent about, not by anything an untrusted template's own bytes can
// trigger through this file's own discovery path (every part name reaching
// here was itself resolved from the package's relationship graph a moment
// earlier).
func partBytes(pkg *opc.Package, name string) ([]byte, error) {
	part, ok := pkg.Get(name)
	if !ok {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"anchor discovery resolved a relationship to a part the package does not contain",
			map[string]any{"part": name})
	}
	return part.Bytes()
}

// resolveRel resolves a relationship id owned by owner (typically mainPart,
// the workbook part) to its target part name, following the same "" ->
// unresolved convention [opc.Relationships.Resolve] uses.
func resolveRel(pkg *opc.Package, owner, rID string) (string, bool) {
	rels, ok := pkg.RelationshipsFor(owner)
	if !ok {
		return "", false
	}
	rel, ok := rels.Resolve(rID)
	if !ok {
		return "", false
	}
	return resolveRelTarget(owner, rel.Target), true
}

// resolveRelFrom is [resolveRel] additionally checking the resolved
// relationship's own Type — used for a worksheet's <tablePart>, where the
// r:id could in principle collide with some other relationship type on the
// same part (it cannot in a well-formed package, but checking costs nothing
// and turns a theoretical mismatch into a clean "not found" rather than a
// wrong part silently accepted).
func resolveRelFrom(pkg *opc.Package, owner, rID, wantType string) (string, bool) {
	rels, ok := pkg.RelationshipsFor(owner)
	if !ok {
		return "", false
	}
	rel, ok := rels.Resolve(rID)
	if !ok || rel.Type != wantType {
		return "", false
	}
	return resolveRelTarget(owner, rel.Target), true
}

// resolveRelTarget resolves a relationship target against owner's directory,
// mirroring [template.resolveTarget] exactly (this package cannot import
// template — template imports anchor — so it carries its own copy, per this
// codebase's own "own your own constants" convention for anything a writer
// package cannot reach across an import boundary).
func resolveRelTarget(owner, target string) string {
	if strings.HasPrefix(target, "/") {
		return target
	}

	base := "/"
	if owner != "/" && owner != "" {
		if i := strings.LastIndexByte(owner, '/'); i >= 0 {
			base = owner[:i+1]
		}
	}

	segments := strings.Split(strings.TrimPrefix(base, "/"), "/")
	if len(segments) > 0 && segments[len(segments)-1] == "" {
		segments = segments[:len(segments)-1]
	}
	for _, seg := range strings.Split(target, "/") {
		switch seg {
		case "", ".":
			// doubled slash or current-directory segment: nothing to do.
		case "..":
			if len(segments) > 0 {
				segments = segments[:len(segments)-1]
			}
		default:
			segments = append(segments, seg)
		}
	}
	return "/" + strings.Join(segments, "/")
}

func definedNameUnsupportedErr(name, formula, reason string) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_ANCHOR_DEFINED_NAME_UNSUPPORTED,
		"a defined name does not resolve to a supported anchor",
		map[string]any{"name": name, "formula": formula, "reason": reason})
}

func tableUnsupportedErr(displayName, tablePart, reason string, details map[string]any) error {
	out := map[string]any{"table": displayName, "table_part": tablePart, "reason": reason}
	for k, v := range details {
		out[k] = v
	}
	return verr.NewCodedErrorWithDetails(verr.VELLUM_ANCHOR_TABLE_UNSUPPORTED,
		"an Excel Table's shape does not support row-insert repetition", out)
}
