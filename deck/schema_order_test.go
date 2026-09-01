package deck_test

import (
	"bytes"
	"encoding/xml"
	"github.com/frankbardon/vellum/deck"
	"io"
	"strings"
	"testing"
)

// sequences declares, for the complex types this writer emits, the order
// ECMA-376 requires their children to appear in.
//
// These are xsd:sequence, not xsd:all. An element out of order is a file a
// reader declares to contain unreadable content — everything present, nothing
// malformed, and the reader refuses it anyway. That failure mode has bitten
// this project twice already, once in a ZIP header field and once in
// WordprocessingML, and PresentationML gives no better diagnostic than Word
// did.
//
// Only the elements Vellum actually emits are listed. A child not named here is
// ignored rather than rejected, because this is a gate against reordering what
// we write, not a reimplementation of the schema.
var sequences = map[string][]string{
	// CT_Presentation
	"p:presentation": {"p:sldMasterIdLst", "p:notesMasterIdLst", "p:handoutMasterIdLst",
		"p:sldIdLst", "p:sldSz", "p:notesSz", "p:defaultTextStyle"},

	// CT_SlideMaster
	"p:sldMaster": {"p:cSld", "p:clrMap", "p:sldLayoutIdLst", "p:transition",
		"p:timing", "p:hf", "p:txStyles"},

	// CT_SlideLayout
	"p:sldLayout": {"p:cSld", "p:clrMapOvr", "p:transition", "p:timing", "p:hf"},

	// CT_Slide and CT_NotesSlide share their ordering for the subset emitted.
	"p:sld":   {"p:cSld", "p:clrMapOvr", "p:transition", "p:timing"},
	"p:notes": {"p:cSld", "p:clrMapOvr", "p:transition", "p:timing"},

	// CT_NotesMaster
	"p:notesMaster": {"p:cSld", "p:clrMap", "p:hf", "p:notesStyle"},

	// CT_CommonSlideData
	"p:cSld": {"p:bg", "p:spTree", "p:custDataLst", "p:controls"},

	// CT_GroupShape. The shapes themselves follow the two property elements and
	// may appear in any order among themselves.
	"p:spTree": {"p:nvGrpSpPr", "p:grpSpPr"},

	// CT_ShapeNonVisual and its picture counterpart.
	"p:nvSpPr":  {"p:cNvPr", "p:cNvSpPr", "p:nvPr"},
	"p:nvPicPr": {"p:cNvPr", "p:cNvPicPr", "p:nvPr"},

	// CT_Shape and CT_Picture.
	"p:sp":  {"p:nvSpPr", "p:spPr", "p:style", "p:txBody"},
	"p:pic": {"p:nvPicPr", "p:blipFill", "p:spPr", "p:style"},

	// CT_ShapeProperties
	"p:spPr": {"a:xfrm", "a:custGeom", "a:prstGeom", "a:noFill", "a:solidFill",
		"a:gradFill", "a:blipFill", "a:pattFill", "a:grpFill", "a:ln",
		"a:effectLst", "a:scene3d", "a:sp3d", "a:extLst"},

	// CT_BlipFillProperties
	"p:blipFill": {"a:blip", "a:srcRect", "a:tile", "a:stretch"},

	// CT_TextBody
	"p:txBody": {"a:bodyPr", "a:lstStyle"},

	// CT_Transform2D
	"a:xfrm": {"a:off", "a:ext", "a:chOff", "a:chExt"},

	// CT_TextParagraph. The runs follow the properties.
	"a:p": {"a:pPr"},

	// CT_TextParagraphProperties, which every lvlNpPr shares.
	"a:pPr":     textParagraphProperties,
	"a:lvl1pPr": textParagraphProperties,
	"a:lvl2pPr": textParagraphProperties,
	"a:lvl3pPr": textParagraphProperties,
	"a:lvl4pPr": textParagraphProperties,
	"a:lvl5pPr": textParagraphProperties,
	"a:lvl6pPr": textParagraphProperties,
	"a:lvl7pPr": textParagraphProperties,
	"a:lvl8pPr": textParagraphProperties,
	"a:lvl9pPr": textParagraphProperties,
	"a:defPPr":  textParagraphProperties,

	// CT_TextCharacterProperties, shared by the three places run properties
	// appear.
	"a:rPr":        textCharacterProperties,
	"a:defRPr":     textCharacterProperties,
	"a:endParaRPr": textCharacterProperties,

	// CT_TextRun
	"a:r": {"a:rPr", "a:t"},

	// CT_Theme
	"a:theme": {"a:themeElements", "a:objectDefaults", "a:extraClrSchemeLst"},

	// CT_BaseStyles
	"a:themeElements": {"a:clrScheme", "a:fontScheme", "a:fmtScheme"},

	// CT_ColorScheme. Twelve slots, in this order and no other.
	"a:clrScheme": {"a:dk1", "a:lt1", "a:dk2", "a:lt2",
		"a:accent1", "a:accent2", "a:accent3", "a:accent4", "a:accent5", "a:accent6",
		"a:hlink", "a:folHlink"},

	// CT_FontScheme and CT_FontCollection.
	"a:fontScheme": {"a:majorFont", "a:minorFont"},
	"a:majorFont":  {"a:latin", "a:ea", "a:cs"},
	"a:minorFont":  {"a:latin", "a:ea", "a:cs"},

	// CT_StyleMatrix
	"a:fmtScheme": {"a:fillStyleLst", "a:lnStyleLst", "a:effectStyleLst", "a:bgFillStyleLst"},

	// CT_LineProperties
	"a:ln": {"a:noFill", "a:solidFill", "a:gradFill", "a:pattFill",
		"a:prstDash", "a:custDash", "a:round", "a:bevel", "a:miter",
		"a:headEnd", "a:tailEnd"},

	// CT_TextListStyle, which the master's three text styles are.
	"p:titleStyle": listStyle,
	"p:bodyStyle":  listStyle,
	"p:otherStyle": listStyle,
	"p:notesStyle": listStyle,
	"a:lstStyle":   listStyle,

	// CT_Blip
	"a:blip": {"a:alphaBiLevel", "a:alphaCeiling", "a:alphaFloor", "a:extLst"},

	// CT_BackgroundProperties
	"p:bgPr": {"a:noFill", "a:solidFill", "a:gradFill", "a:blipFill",
		"a:pattFill", "a:grpFill", "a:effectLst", "a:effectDag"},
}

// textParagraphProperties is CT_TextParagraphProperties' sequence.
//
// The bullet elements are a choice group rather than a sequence among
// themselves, but each choice occupies one position in the parent sequence, so
// listing them at the position they share is correct and catches a bullet
// emitted after the default run properties.
var textParagraphProperties = []string{
	"a:lnSpc", "a:spcBef", "a:spcAft",
	"a:buClrTx", "a:buClr", "a:buSzTx", "a:buSzPct", "a:buSzPts",
	"a:buFontTx", "a:buFont",
	"a:buNone", "a:buAutoNum", "a:buChar",
	"a:tabLst", "a:defRPr", "a:extLst",
}

// textCharacterProperties is CT_TextCharacterProperties' sequence.
var textCharacterProperties = []string{
	"a:ln",
	"a:noFill", "a:solidFill", "a:gradFill", "a:blipFill", "a:pattFill", "a:grpFill",
	"a:effectLst", "a:effectDag",
	"a:highlight", "a:uLnTx", "a:uLn", "a:uFillTx", "a:uFill",
	"a:latin", "a:ea", "a:cs", "a:sym",
	"a:hlinkClick", "a:hlinkMouseOver", "a:rtl", "a:extLst",
}

// listStyle is CT_TextListStyle's sequence.
var listStyle = []string{
	"a:defPPr",
	"a:lvl1pPr", "a:lvl2pPr", "a:lvl3pPr", "a:lvl4pPr", "a:lvl5pPr",
	"a:lvl6pPr", "a:lvl7pPr", "a:lvl8pPr", "a:lvl9pPr",
	"a:extLst",
}

// TestWrite_ElementOrderMatchesTheSchema walks every emitted part and asserts
// that each element's children appear in the order their complex type declares.
func TestWrite_ElementOrderMatchesTheSchema(t *testing.T) {
	d := sample(t)

	// A free-standing text box and a bulleted, spaced paragraph, so the
	// elements the sample's placeholders do not exercise are still walked. The
	// schema does not care which API produced an element.
	d.Slides = append(d.Slides, deck.Slide{
		LayoutID: deck.LayoutIDBlank,
		Shapes: []deck.Shape{{
			Name:  "Free Text",
			Frame: deck.Frame{X: 100, Y: 200, Width: 3000000, Height: 1000000},
			Text: &deck.TextBody{
				Anchor: deck.AnchorCenter,
				NoWrap: true,
				Paragraphs: []deck.Paragraph{
					{
						Align:       deck.AlignCenter,
						SpaceBefore: 6 * pt,
						Bullet:      deck.Bullet{Kind: deck.BulletChar, Char: "•", Font: "Arial"},
						Runs: []deck.Run{
							{Text: "bold ", Style: deck.RunStyle{Bold: true}},
							{Text: "and coloured", Style: deck.RunStyle{
								Color: deck.SchemeColor(deck.SchemeAccent1), Font: deck.FontMinor}},
						},
						EndStyle: deck.RunStyle{SizeEMU: 18 * pt},
					},
					{
						Level:  1,
						Bullet: deck.Bullet{Kind: deck.BulletNumber, Format: "arabicPeriod"},
						Runs:   []deck.Run{{Text: "numbered"}},
					},
				},
			},
		}},
	})

	p := unzip(t, write(t, d))
	for _, name := range p.sorted() {
		if !strings.HasSuffix(name, ".xml") {
			continue
		}
		checkOrder(t, name, p.parts[name])
	}
}

// checkOrder walks one part's elements and reports any child that appears
// earlier than a sibling it must follow.
func checkOrder(t *testing.T, part string, body []byte) {
	t.Helper()

	dec := xml.NewDecoder(bytes.NewReader(body))
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
								"CT sequences are ordered; a reader reports an out-of-order child as unreadable content.",
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
	case "http://schemas.openxmlformats.org/presentationml/2006/main":
		return "p:" + n.Local
	case "http://schemas.openxmlformats.org/drawingml/2006/main":
		return "a:" + n.Local
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
