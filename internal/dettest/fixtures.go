package dettest

import (
	"io"
	"time"

	"github.com/frankbardon/vellum/doc"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/spec"
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
	}
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
		Write: func(w io.Writer, epoch time.Time) error {
			d, err := doc.Lower(&spec.Spec{
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
			})
			if err != nil {
				return err
			}
			return d.WriteTo(w, doc.WriteOptions{SourceDateEpoch: epoch})
		},
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
				Data:        []byte("\x89PNG\r\n\x1a\nsubstrate fixture, not a real image"),
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
