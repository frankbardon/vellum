package doc_test

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"testing"

	"github.com/frankbardon/vellum/doc"
	"github.com/frankbardon/vellum/spec"
)

// sequences declares, for the complex types this writer emits, the order
// ECMA-376 requires their children to appear in.
//
// These are xsd:sequence, not xsd:all. An element out of order is a document
// Word declares to contain unreadable content — everything is present, nothing
// is malformed, and the reader refuses it anyway. That failure mode has now
// bitten this project twice: once in a ZIP header field and once here. It is
// invisible to every check that reads the document back with a tolerant parser,
// which is every check the build can run, so it is asserted directly.
//
// Only the elements Vellum actually emits are listed. A child not named here is
// ignored rather than rejected, because this is a gate against reordering what
// we write, not a reimplementation of the schema.
var sequences = map[string][]string{
	// CT_PPr
	"w:pPr": {"w:pStyle", "w:keepNext", "w:keepLines", "w:pageBreakBefore",
		"w:numPr", "w:spacing", "w:ind", "w:jc", "w:outlineLvl", "w:rPr", "w:sectPr"},

	// CT_RPr and CT_ParaRPr share their ordering for the subset used here.
	"w:rPr": {"w:rStyle", "w:rFonts", "w:b", "w:bCs", "w:i", "w:iCs",
		"w:color", "w:sz", "w:szCs", "w:highlight", "w:u", "w:shd", "w:vertAlign"},

	// CT_SectPr
	"w:sectPr": {"w:headerReference", "w:footerReference", "w:footnotePr",
		"w:type", "w:pgSz", "w:pgMar", "w:cols", "w:titlePg"},

	// CT_TblPrBase
	"w:tblPr": {"w:tblStyle", "w:tblW", "w:jc", "w:tblBorders", "w:shd",
		"w:tblLayout", "w:tblCellMar", "w:tblLook", "w:tblCaption", "w:tblDescription"},

	// CT_TcPr
	"w:tcPr": {"w:cnfStyle", "w:tcW", "w:gridSpan", "w:hMerge", "w:vMerge",
		"w:tcBorders", "w:shd", "w:noWrap", "w:tcMar", "w:vAlign"},

	// CT_TrPr
	"w:trPr": {"w:cnfStyle", "w:gridBefore", "w:gridAfter", "w:cantSplit",
		"w:trHeight", "w:tblHeader", "w:jc"},

	// CT_Style
	"w:style": {"w:name", "w:aliases", "w:basedOn", "w:next", "w:link",
		"w:uiPriority", "w:semiHidden", "w:unhideWhenUsed", "w:qFormat",
		"w:pPr", "w:rPr", "w:tblPr", "w:trPr", "w:tcPr", "w:tblStylePr"},

	// CT_Tbl
	"w:tbl": {"w:tblPr", "w:tblGrid", "w:tr"},

	// CT_Settings, for the subset emitted.
	"w:settings": {"w:updateFields", "w:footnotePr", "w:endnotePr", "w:compat"},

	// CT_Lvl
	"w:lvl": {"w:start", "w:numFmt", "w:lvlRestart", "w:pStyle", "w:suff",
		"w:lvlText", "w:lvlJc", "w:pPr", "w:rPr"},
}

// TestWrite_ElementOrderMatchesTheSchema walks every emitted part and asserts
// that each element's children appear in the order their complex type declares.
func TestWrite_ElementOrderMatchesTheSchema(t *testing.T) {
	d := lower(t,
		heading(1, "Title"),
		text("Body.", "flagged"),
		text("Underlined and highlighted.", "muted"),
		spec.Block{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: pngURI()}},
		spec.Block{Kind: spec.BlockNotes, Notes: &spec.Notes{Content: "note"}},
		spec.Block{Kind: spec.BlockSpacer, Spacer: &spec.Spacer{Height: spec.Points(12)}},
		spec.Block{Kind: spec.BlockPageBreak, PageBreak: &spec.PageBreak{}},
		spec.Block{Kind: spec.BlockTable, Table: &spec.Table{
			ColumnHeaders: spec.HeaderTree{
				{Label: "Region", Span: 2, Children: []spec.HeaderNode{
					{Label: "N", Span: 1}, {Label: "S", Span: 1}}},
			},
			RowHeaders: spec.HeaderTree{
				{Label: "Age", Span: 2, Children: []spec.HeaderNode{
					{Label: "18-34", Span: 1}, {Label: "35+", Span: 1}}},
			},
			Body: [][]spec.Cell{
				{{Text: "1", Annotations: []spec.Annotation{{Text: "a"}}}, {Text: "2"}},
				{{Text: "3", Class: spec.CellMargin}, {Text: "4"}},
			},
			Caption: "Table 1",
		}},
	)

	// Exercise the advanced-only surface too, since the schema does not care
	// which API produced the element and these are the parts the block model
	// never reaches: a running head, a list, and a dirty TOC field.
	decorateAdvanced(d)
	raw := write(t, d)

	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	for _, f := range zr.File {
		if len(f.Name) < 4 || f.Name[len(f.Name)-4:] != ".xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		checkOrder(t, f.Name, body)
	}
}

// checkOrder walks one part's elements and reports any child that appears
// earlier than a sibling it must follow.
func checkOrder(t *testing.T, part string, body []byte) {
	t.Helper()

	dec := xml.NewDecoder(bytes.NewReader(body))
	// stack holds, per open element, its qualified name and the highest
	// sequence position any child has reached so far.
	type frame struct {
		name     string
		furthest int
		prev     string
	}
	var stack []frame

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("%s: %v", part, err)
		}

		switch tok := tok.(type) {
		case xml.StartElement:
			name := qualify(tok.Name)
			if n := len(stack); n > 0 {
				parent := &stack[n-1]
				if order, ok := sequences[parent.name]; ok {
					if pos := indexOf(order, name); pos >= 0 {
						if pos < parent.furthest {
							t.Errorf("%s: in <%s>, <%s> appears after <%s>, but the schema requires it before.\n"+
								"CT sequences are ordered; Word reports an out-of-order child as unreadable content.",
								part, parent.name, name, parent.prev)
						} else {
							parent.furthest = pos
							parent.prev = name
						}
					}
				}
			}
			stack = append(stack, frame{name: name, furthest: -1})

		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
}

func qualify(n xml.Name) string {
	switch n.Space {
	case "http://schemas.openxmlformats.org/wordprocessingml/2006/main":
		return "w:" + n.Local
	}
	return n.Local
}

func indexOf(order []string, name string) int {
	for i, s := range order {
		if s == name {
			return i
		}
	}
	return -1
}

// decorateAdvanced adds the model features no block kind reaches, so the schema
// gate covers the exported API as well as the lowered one.
func decorateAdvanced(d *doc.Document) {
	d.Headers = []doc.HeaderFooter{{
		ID: "h1",
		Content: []doc.Content{{Paragraph: &doc.Paragraph{
			StyleID: doc.StyleHeader,
			Runs: []doc.Run{
				{Text: "Report"},
				{Tab: true},
				{Field: &doc.Field{Instruction: "PAGE", Result: "1"}},
			},
		}}},
	}}
	d.Footers = []doc.HeaderFooter{{
		ID: "f1",
		Content: []doc.Content{{Paragraph: &doc.Paragraph{
			StyleID: doc.StyleFooter,
			Runs:    []doc.Run{{Text: "Confidential"}},
		}}},
	}}
	d.Sections[0].HeaderID = "h1"
	d.Sections[0].FooterID = "f1"
	d.Sections[0].TitlePage = true

	d.Styles.Character = append(d.Styles.Character, doc.CharacterStyle{
		ID:   "Everything",
		Name: "everything",
		Run: doc.RunProperties{
			Font: "Georgia", SizeEMU: 139700, Color: "1A1A1A", Highlight: "FFFF00",
			Bold: true, Italic: true, Underline: true, Subscript: true,
		},
	})

	d.Numbering = doc.Numbering{
		Abstract: []doc.AbstractNumbering{{Levels: []doc.NumberingLevel{{
			Format:     "bullet",
			Text:       "\u2022",
			IndentEMU:  457200,
			HangingEMU: 228600,
			Font:       "Symbol",
		}}}},
		Instances: []doc.NumberingInstance{{ID: 1, AbstractIndex: 0}},
	}

	// One run carrying every character property at once. Without it the rPr
	// sequence is only ever exercised a few elements at a time, and the gate
	// passes because the violation it is looking for never gets emitted —
	// which is exactly what happened the first time this test was written.
	d.Sections[0].Content = append(d.Sections[0].Content,
		doc.Content{Paragraph: &doc.Paragraph{
			StyleID: doc.StyleNormal,
			Runs: []doc.Run{{
				StyleID: doc.StyleFootnoteRef,
				Text:    "every property at once",
				Properties: doc.RunProperties{
					Font:        "Georgia",
					SizeEMU:     139700,
					Color:       "1A1A1A",
					Highlight:   "FFFF00",
					Bold:        true,
					Italic:      true,
					Underline:   true,
					Superscript: true,
				},
			}},
		}},
	)

	d.Sections[0].Content = append(d.Sections[0].Content,
		doc.Content{Paragraph: &doc.Paragraph{
			StyleID:     doc.StyleListParagraph,
			NumberingID: 1,
			Runs:        []doc.Run{{Text: "A bulleted item."}},
		}},
		doc.Content{Paragraph: &doc.Paragraph{
			StyleID: doc.StyleNormal,
			Runs: []doc.Run{{Field: &doc.Field{
				Instruction: tocInstruction,
				Result:      "Right-click to update this table of contents.",
				Dirty:       true,
			}}},
		}},
	)
}

// tocInstruction is the field code Word writes for a table of contents over
// heading levels one to three.
const tocInstruction = `TOC \o "1-3" \h \z \u`
