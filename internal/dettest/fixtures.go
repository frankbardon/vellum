package dettest

import (
	"context"
	"encoding/base64"
	"io"
	"strconv"
	"time"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/doc"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/pdf"
	"github.com/frankbardon/vellum/pdf/content"
	pdffont "github.com/frankbardon/vellum/pdf/font"
	"github.com/frankbardon/vellum/pdf/object"
	"github.com/frankbardon/vellum/pdf/xmp"
	"github.com/frankbardon/vellum/provenance"
	"github.com/frankbardon/vellum/resolve"
	"github.com/frankbardon/vellum/spec"
	"golang.org/x/image/font/gofont/goregular"
)

// Cases returns every registered determinism and golden case.
//
// Adding a case is one entry here. The runners in the test files never change,
// which is the point: a format epic proves its determinism by registering,
// not by writing a suite of its own that may quietly assert something weaker.
func Cases() []Case {
	return []Case{
		substrateCase(),
		docxSkeletonCase(),
		docxProfileCase(),
		pdfSpikeCase(),
		pdfPagesCase(),
	}
}

// pdfPagesCase exercises the page tree at a size where its shape matters.
//
// Twenty pages against a branching factor of eight, so the tree is three levels
// deep and neither level is full — which is the arrangement that catches an
// off-by-one in the fold, where a full tree would not. Every page shares its
// media box and resources, so the case also proves the inheritable attributes
// are lifted onto the root rather than repeated twenty times.
func pdfPagesCase() Case {
	return Case{
		Name:  "pdf-pages",
		Ext:   "pdf",
		Write: writePDFPages,
	}
}

// pageCount is the length of the multi-page fixture.
//
// Twenty rather than a round power of the branching factor, deliberately: a
// count that divides evenly hides a remainder bug in the last group of every
// level, and the last group is the one the fold gets wrong.
const pageCount = 20

func writePDFPages(w io.Writer, epoch time.Time) error {
	face, err := pdffont.New(pdffont.Options{
		Resource: "F1",
		BaseName: "GoRegular",
		Program:  goregular.TTF,
		Plan:     pdffont.PlanSubset,
	})
	if err != nil {
		return err
	}

	pages := make([]pdf.Page, pageCount)
	for i := range pages {
		label := "Page " + strconv.Itoa(i+1) + " of " + strconv.Itoa(pageCount)
		gids, err := face.Encode(label)
		if err != nil {
			return err
		}

		var c content.Builder
		c.BeginText().
			SetFont("F1", object.Points(18)).
			MoveText(object.Points(72), object.Points(700)).
			ShowGlyphs(gids).
			EndText()

		pages[i] = pdf.Page{
			Width:   object.Points(612),
			Height:  object.Points(792),
			Content: c.Bytes(),
			Fonts:   []*pdffont.Face{face},
		}
	}

	d := pdf.Document{
		Metadata: xmp.Metadata{
			Title:    "Twenty Pages",
			Creator:  "Vellum determinism fixture",
			Producer: "Vellum 0.0.0-golden",
			Date:     epoch,
		},
		Pages: pages,
	}
	return d.WriteTo(w, pdf.WriteOptions{SourceDateEpoch: epoch})
}

// pdfSpikeCase is the PDF substrate proved end to end on one page.
//
// One page, one line, one subsetted face, an sRGB output intent and XMP that
// agrees with the information dictionary. Small, and it touches every hard part
// of the PDF work at once — the object writer, the cross-reference table, the
// subsetter, the composite font encoding and the conformance metadata — so a
// wrong assumption in any of them surfaces here rather than after the rest is
// built on top of it.
//
// The face is Go Regular. It is BSD-3, it arrives as a Go package rather than
// as a binary in testdata, and it carries composite glyphs, which is the part
// of subsetting a naive implementation gets wrong.
func pdfSpikeCase() Case {
	return Case{
		Name:  "pdf-spike",
		Ext:   "pdf",
		Write: writePDFSpike,
	}
}

// writePDFSpike composes the spike document.
func writePDFSpike(w io.Writer, epoch time.Time) error {
	const (
		heading = "Hello, PDF/A"
		body    = "Composed by Vellum. Every glyph here is embedded as a subset."
	)

	face, err := pdffont.New(pdffont.Options{
		Resource: "F1",
		BaseName: "GoRegular",
		Program:  goregular.TTF,
		Plan:     pdffont.PlanSubset,
	})
	if err != nil {
		return err
	}
	headingGlyphs, err := face.Encode(heading)
	if err != nil {
		return err
	}
	bodyGlyphs, err := face.Encode(body)
	if err != nil {
		return err
	}

	var c content.Builder
	// A filled rule above the heading, which puts a DeviceRGB colour in the
	// file. Without one the output intent is present but unexercised, and the
	// spike would not have proved the part of conformance most likely to be
	// wrong.
	c.Save().
		SetFillRGB(object.Ratio(37, 255), object.Ratio(99, 235), object.Ratio(235, 255)).
		Rect(object.Points(72), object.Points(724), object.Points(180), object.Points(4)).
		Fill().
		Restore()

	c.BeginText().
		SetFont("F1", object.Points(24)).
		MoveText(object.Points(72), object.Points(684)).
		ShowGlyphs(headingGlyphs).
		EndText()

	c.BeginText().
		SetFont("F1", object.Points(11)).
		MoveText(object.Points(72), object.Points(652)).
		ShowGlyphs(bodyGlyphs).
		EndText()

	d := pdf.Document{
		Metadata: xmp.Metadata{
			Title:    heading,
			Creator:  "Vellum determinism fixture",
			Producer: "Vellum 0.0.0-golden",
			Date:     epoch,
		},
		Pages: []pdf.Page{{
			Width:   object.Points(612),
			Height:  object.Points(792),
			Content: c.Bytes(),
			Fonts:   []*pdffont.Face{face},
		}},
	}
	return d.WriteTo(w, pdf.WriteOptions{SourceDateEpoch: epoch})
}

// docxSkeletonCase is the first real artifact: a heading and a paragraph
// rendered to a .docx.
//
// It is registered rather than tested privately in the doc package, because a
// format epic should prove its determinism by joining this suite rather than
// by writing one of its own that may quietly assert something weaker.
func docxSkeletonCase() Case {
	return Case{
		Name: "docx-skeleton",
		Ext:  "docx",
		Write: writeDOCX(&spec.Spec{
			FormatVersion: spec.FormatVersion,
			Title:         "Walking Skeleton",
			Sections: []spec.Section{{
				ID: "intro",
				Blocks: []spec.Block{
					{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 1, Content: "Walking Skeleton"}},
					{Kind: spec.BlockText, Text: &spec.Text{Content: "The substrate carries a real artifact end to end."}},
					{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 2, Content: "Why this exists"}},
					{Kind: spec.BlockText, Text: &spec.Text{Content: "Breadth is not the goal; structural correctness and byte-identical output are."}},
				},
			}},
		}, nil),
	}
}

// docxProfileCase exercises the DOCX conformance profile at its full breadth in
// one document.
//
// One broad case rather than one per feature, deliberately. The features
// interact — a table inside a section whose geometry differs, an annotation
// inside a merged cell, a footnote anchored after a picture — and a suite of
// narrow fixtures proves each in isolation while proving nothing about the
// document a consumer actually produces.
func docxProfileCase() Case {
	png := "data:image/png;base64," + base64.StdEncoding.EncodeToString(fixturePNG())

	return Case{
		Name: "docx-profile",
		Ext:  "docx",
		Write: writeDOCX(&spec.Spec{
			FormatVersion: spec.FormatVersion,
			Title:         "Conformance Profile",
			Sections: []spec.Section{
				{
					ID: "prose",
					Blocks: []spec.Block{
						{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 1, Content: "Findings"}},
						{Kind: spec.BlockText, Text: &spec.Text{Content: "Unmarked prose, which takes its style and carries no direct formatting."}},
						{Kind: spec.BlockText, Marks: []string{"flagged"},
							Text: &spec.Text{Content: "Marked prose, whose mark the theme styles."}},
						{Kind: spec.BlockText, Marks: []string{"unstyled-mark"},
							Text: &spec.Text{Content: "A mark the theme does not style, which warns and renders plain."}},
						{Kind: spec.BlockSpacer, Spacer: &spec.Spacer{Height: spec.Points(18)}},
					},
				},
				{
					ID: "table",
					Blocks: []spec.Block{
						{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 2, Content: "Crosstab"}},
						{Kind: spec.BlockTable, Table: profileTable()},
						{Kind: spec.BlockNotes, Notes: &spec.Notes{Content: "Base: all respondents."}},
					},
				},
				{
					ID: "figure",
					Blocks: []spec.Block{
						{Kind: spec.BlockPageBreak, PageBreak: &spec.PageBreak{}},
						{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 2, Content: "Figure"}},
						{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: png, Role: "asset.full", AltText: "a four-by-three placeholder"}},
					},
				},
			},
		}, &provenance.Record{
			VellumVersion:   "0.0.0-golden",
			SourceDateEpoch: zipdet.PinnedEpoch,
			SpecHash:        "00000000000000000000000000000000",
			ThemeHash:       "11111111111111111111111111111111",
			Fonts: []provenance.FontRef{
				{Family: "Georgia", SubstitutedWith: "Times New Roman"},
			},
			Sources: []provenance.Source{{Kind: "fixture", ID: "docx-profile"}},
		}),
	}
}

// profileTable is a crosstab exercising every table feature the profile
// declares: a two-level column banner, a row-header stub that merges
// vertically, cell annotations, a margin row, and a per-cell mark.
func profileTable() *spec.Table {
	return &spec.Table{
		ColumnHeaders: spec.HeaderTree{
			{Label: "Region", Span: 2, Children: []spec.HeaderNode{
				{Label: "North", Span: 1},
				{Label: "South", Span: 1},
			}},
			{Label: "Total", Span: 1},
		},
		RowHeaders: spec.HeaderTree{
			{Label: "Age", Span: 2, Children: []spec.HeaderNode{
				{Label: "18-34", Span: 1},
				{Label: "35+", Span: 1},
			}},
			{Label: "All", Span: 1},
		},
		Body: [][]spec.Cell{
			{
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 0.412}, Format: "0.0%",
					Annotations: []spec.Annotation{{Text: "a"}}},
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 0.388}, Format: "0.0%"},
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 0.400}, Format: "0.0%"},
			},
			{
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 0.213}, Format: "0.0%",
					Marks: []string{"muted"}},
				{Text: "*"},
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 0.207}, Format: "0.0%"},
			},
			{
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 1200}, Format: "#,##0", Class: spec.CellMargin},
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 1100}, Format: "#,##0", Class: spec.CellMargin},
				{Value: &spec.Value{Kind: spec.ValueNumber, Number: 2300}, Format: "#,##0", Class: spec.CellTotal},
			},
		},
		Caption: "Table 1. Awareness by region and age.",
	}
}

// writeDOCX returns a Case writer that resolves and lowers a specification.
//
// Resolution happens inside the writer rather than once outside it, so every
// repetition of a determinism run exercises the whole pipeline. A fixture that
// resolved once and lowered many times would prove the writer deterministic
// while saying nothing about the resolver.
func writeDOCX(s *spec.Spec, rec *provenance.Record) func(io.Writer, time.Time) error {
	return func(w io.Writer, epoch time.Time) error {
		res, err := resolve.Resolve(context.Background(), s, resolve.Options{Format: artifact.FormatDOCX})
		if err != nil {
			return err
		}
		d, err := doc.Lower(res.Doc)
		if err != nil {
			return err
		}
		d.Provenance = rec
		return d.WriteTo(w, doc.WriteOptions{SourceDateEpoch: epoch})
	}
}

const (
	ctXML     = "application/xml"
	ctPNG     = "image/png"
	relCustom = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/customXml"
	relImage  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"
)

// substrateCase exercises the packaging layer with no writer above it.
//
// It exists so the harness is proven against a real artifact before any format
// package exists — which is the whole reason this story is sequenced before
// the first writer rather than after it. It deliberately includes a stored
// media part and two relationships, because those are the two places
// nondeterminism enters a package: compression choice and identifier
// assignment.
func substrateCase() Case {
	return Case{
		Name: "substrate-package",
		Ext:  "zip",
		Write: func(w io.Writer, epoch time.Time) error {
			p := opc.New()

			if err := p.Put(&opc.Part{
				Name:        "/content/document.xml",
				ContentType: ctXML,
				Data:        []byte(`<?xml version="1.0"?><document><body>substrate</body></document>`),
			}); err != nil {
				return err
			}
			if err := p.Put(&opc.Part{
				Name:        "/content/notes.xml",
				ContentType: ctXML,
				Data:        []byte(`<?xml version="1.0"?><notes/>`),
			}); err != nil {
				return err
			}
			if err := p.Put(&opc.Part{
				Name:        "/media/image1.png",
				ContentType: ctPNG,
				Data:        fixturePNG(),
			}); err != nil {
				return err
			}

			if _, err := p.Relationships("/").Add(relCustom, "content/document.xml", opc.TargetInternal); err != nil {
				return err
			}
			if _, err := p.Relationships("/content/document.xml").Add(relImage, "../media/image1.png", opc.TargetInternal); err != nil {
				return err
			}
			if _, err := p.Relationships("/content/document.xml").Add(relCustom, "notes.xml", opc.TargetInternal); err != nil {
				return err
			}

			return p.WriteTo(w, zipdet.WriteOptions{SourceDateEpoch: epoch})
		},
	}
}

// fixturePNG is a real 4x3 PNG: two flat bands, produced by image/png and
// round-tripped through its decoder before being encoded here.
//
// It is real because the first version was not. That one was hand-assembled
// from a signature, an IHDR and an IEND — no IDAT, no chunk CRCs. Vellum's
// sniffer recognised the signature, its probe read 4x3 out of the IHDR, the
// package assembled correctly, and the determinism harness was entirely
// satisfied, because every stage was comparing our bytes against our bytes.
// Word drew "the picture can't be displayed".
//
// A fixture no reader accepts proves the packaging and proves nothing about the
// embedding, which is most of what a golden containing an image is for.
// TestGoldenMediaDecodes now fails the build for it.
func fixturePNG() []byte {
	const encoded = "iVBORw0KGgoAAAANSUhEUgAAAAQAAAADCAIAAAA7ljmRAAAAHElEQVR42mKR96tk" +
		"gAEmBiTAeOHCBTgHEAAA//88uANeGlr4UwAAAABJRU5ErkJggg=="
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		panic("dettest: the fixture PNG does not decode: " + err.Error())
	}
	return raw
}
