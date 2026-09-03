package template_test

// buildNonDestructiveFixture builds the .docx-shaped package
// TestNonDestructiveCorpus fills: a realistic package structure, not a
// systematic sweep of run-fragmentation shapes (that is the fragmentation
// generator's job, in defrag_fixture_test.go). Hand-built rather than
// generated, because the point here is the realism of the package
// structure — tracked changes, a comment, a custom XML part, a footnote and
// an OLE object all present and cross-referenced the way a real Word-authored
// .docx wires them — not coverage of splice/defrag edge cases.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
)

const (
	ndNSWordprocessing = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	ndNSRelationships  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	ndNSOffice         = "urn:schemas-microsoft-com:office:office"
	ndNSVML            = "urn:schemas-microsoft-com:vml"
	ndNSCustomXML      = "http://schemas.openxmlformats.org/officeDocument/2006/customXml"

	ndCTMainDocument = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	ndCTComments     = "application/vnd.openxmlformats-officedocument.wordprocessingml.comments+xml"
	ndCTFootnotes    = "application/vnd.openxmlformats-officedocument.wordprocessingml.footnotes+xml"
	ndCTOLEObject    = "application/vnd.openxmlformats-officedocument.oleObject"
	ndCTCustomXML    = "application/xml"
	ndCTCustomProps  = "application/vnd.openxmlformats-officedocument.customXmlProperties+xml"

	ndRelOfficeDocument = ndNSRelationships + "/officeDocument"
	ndRelComments       = ndNSRelationships + "/comments"
	ndRelFootnotes      = ndNSRelationships + "/footnotes"
	ndRelOLEObject      = ndNSRelationships + "/oleObject"
	ndRelCustomXML      = ndNSRelationships + "/customXml"
	ndRelCustomXMLProps = ndNSRelationships + "/customXmlProps"
)

// buildNonDestructiveFixture returns the serialised bytes of a .docx-shaped
// package. word/document.xml's body, in order:
//
//  1. a tracked insertion (w:ins)                    -- untouched
//  2. a tracked deletion (w:del / w:delText)          -- untouched
//  3. a comment range + reference                     -- untouched
//  4. a footnote reference                             -- untouched
//  5. an embedded OLE object (w:object/v:shape)        -- untouched
//  6. a {{customer_name}} marker anchor                -- spliced
//  7. a native "body" content control (w:sdt)          -- spliced
//  8. a closing paragraph                              -- untouched
//
// Items 1-5 all precede both anchors and item 8 follows both, which is what
// lets TestNonDestructiveCorpus assert non-destructiveness directly against
// xmlcopy.Apply's own contract (everything before the first replacement and
// after the last is copied through verbatim) instead of re-deriving offsets
// by hand for each structural element.
func buildNonDestructiveFixture(t *testing.T) []byte {
	t.Helper()

	body := "" +
		// 1. Tracked insertion.
		`<w:p><w:ins w:id="100" w:author="Reviewer" w:date="2020-01-01T00:00:00Z">` +
		`<w:r><w:t>inserted text</w:t></w:r></w:ins></w:p>` +
		// 2. Tracked deletion.
		`<w:p><w:del w:id="101" w:author="Reviewer" w:date="2020-01-01T00:00:00Z">` +
		`<w:r><w:delText>deleted text</w:delText></w:r></w:del></w:p>` +
		// 3. Comment range + reference.
		`<w:p><w:commentRangeStart w:id="0"/>` +
		`<w:r><w:t>Commented text</w:t></w:r>` +
		`<w:commentRangeEnd w:id="0"/>` +
		`<w:r><w:commentReference w:id="0"/></w:r></w:p>` +
		// 4. Footnote reference.
		`<w:p><w:r><w:t>See note</w:t></w:r>` +
		`<w:r><w:footnoteReference w:id="1"/></w:r></w:p>` +
		// 5. Embedded OLE object.
		`<w:p><w:r><w:object w:dxaOrig="1440" w:dyaOrig="1440">` +
		`<v:shape id="_x0000_i1025" type="#_x0000_t75" style="width:72pt;height:72pt"></v:shape>` +
		`<o:OLEObject Type="Embed" ProgID="Package" ShapeID="_x0000_i1025" DrawAspect="Content" r:id="__OLE_RID__" ObjectID="_1234567890"/>` +
		`</w:object></w:r></w:p>` +
		// 6. Marker anchor.
		`<w:p><w:r><w:t>Dear {{customer_name}}, thanks for your business.</w:t></w:r></w:p>` +
		// 7. Native anchor.
		`<w:sdt><w:sdtPr><w:tag w:val="body"/><w:alias w:val="Body"/></w:sdtPr>` +
		`<w:sdtContent><w:p><w:r><w:t>placeholder</w:t></w:r></w:p></w:sdtContent></w:sdt>` +
		// 8. Closing paragraph.
		`<w:p><w:r><w:t>closing paragraph</w:t></w:r></w:p>`

	pkg := opc.New()

	// word/document.xml, with its own relationships assigned and frozen
	// before the body text embedding its r:id is finalised, mirroring the
	// Freeze()-then-IDFor pattern template/splice's own asset embedding
	// uses for exactly the same reason: the text references an id the
	// relationships part must agree with once both are serialised.
	docRels := pkg.Relationships("/word/document.xml")
	if _, err := docRels.Add(ndRelComments, "comments.xml", opc.TargetInternal); err != nil {
		t.Fatalf("adding comments relationship: %v", err)
	}
	if _, err := docRels.Add(ndRelFootnotes, "footnotes.xml", opc.TargetInternal); err != nil {
		t.Fatalf("adding footnotes relationship: %v", err)
	}
	if _, err := docRels.Add(ndRelOLEObject, "embeddings/oleObject1.bin", opc.TargetInternal); err != nil {
		t.Fatalf("adding OLE object relationship: %v", err)
	}
	docRels.Freeze()
	oleRID, ok := docRels.IDFor(ndRelOLEObject, "embeddings/oleObject1.bin")
	if !ok {
		t.Fatal("OLE object relationship not found after Freeze")
	}
	body = strings.ReplaceAll(body, "__OLE_RID__", oleRID)

	documentXML := ndXMLDecl +
		`<w:document xmlns:w="` + ndNSWordprocessing + `" xmlns:r="` + ndNSRelationships +
		`" xmlns:o="` + ndNSOffice + `" xmlns:v="` + ndNSVML + `">` +
		`<w:body>` + body + `</w:body></w:document>`

	if err := pkg.Put(&opc.Part{
		Name:        "/word/document.xml",
		ContentType: ndCTMainDocument,
		Data:        []byte(documentXML),
	}); err != nil {
		t.Fatalf("Put document.xml: %v", err)
	}

	// word/comments.xml
	commentsXML := ndXMLDecl +
		`<w:comments xmlns:w="` + ndNSWordprocessing + `">` +
		`<w:comment w:id="0" w:author="Reviewer" w:date="2020-01-01T00:00:00Z" w:initials="R">` +
		`<w:p><w:r><w:t>This needs review.</w:t></w:r></w:p></w:comment>` +
		`</w:comments>`
	if err := pkg.Put(&opc.Part{
		Name:        "/word/comments.xml",
		ContentType: ndCTComments,
		Data:        []byte(commentsXML),
	}); err != nil {
		t.Fatalf("Put comments.xml: %v", err)
	}

	// word/footnotes.xml, including the separator/continuation-separator
	// pair every real docx's footnotes part carries alongside its content,
	// mirroring doc/write_parts.go's own footnotesXML.
	footnotesXML := ndXMLDecl +
		`<w:footnotes xmlns:w="` + ndNSWordprocessing + `">` +
		`<w:footnote w:type="separator" w:id="-1"><w:p><w:r><w:separator/></w:r></w:p></w:footnote>` +
		`<w:footnote w:type="continuationSeparator" w:id="0"><w:p><w:r><w:continuationSeparator/></w:r></w:p></w:footnote>` +
		`<w:footnote w:id="1"><w:p><w:r><w:t>This is a footnote.</w:t></w:r></w:p></w:footnote>` +
		`</w:footnotes>`
	if err := pkg.Put(&opc.Part{
		Name:        "/word/footnotes.xml",
		ContentType: ndCTFootnotes,
		Data:        []byte(footnotesXML),
	}); err != nil {
		t.Fatalf("Put footnotes.xml: %v", err)
	}

	// word/embeddings/oleObject1.bin -- arbitrary synthetic bytes. This is
	// deliberately not a valid OLE compound file: the guarantee under test is
	// that the part's bytes survive, never that Vellum understands them.
	if err := pkg.Put(&opc.Part{
		Name:        "/word/embeddings/oleObject1.bin",
		ContentType: ndCTOLEObject,
		Data:        []byte("OLE-SYNTHETIC-PAYLOAD-NOT-A-REAL-COMPOUND-FILE"),
	}); err != nil {
		t.Fatalf("Put oleObject1.bin: %v", err)
	}

	// customXml/item1.xml + its own rels -- the kind of part a consumer's
	// own tooling stashes structured data in. Vellum never looks inside it.
	itemXML := ndXMLDecl + `<consumerData xmlns="http://example.com/consumer-schema"><field>value</field></consumerData>`
	if err := pkg.Put(&opc.Part{
		Name:        "/customXml/item1.xml",
		ContentType: ndCTCustomXML,
		Data:        []byte(itemXML),
	}); err != nil {
		t.Fatalf("Put customXml/item1.xml: %v", err)
	}
	itemPropsXML := ndXMLDecl +
		`<ds:datastoreItem ds:itemID="{12345678-1234-1234-1234-123456789012}" xmlns:ds="` + ndNSCustomXML + `">` +
		`<ds:schemaRefs><ds:schemaRef ds:uri="http://example.com/consumer-schema"/></ds:schemaRefs>` +
		`</ds:datastoreItem>`
	if err := pkg.Put(&opc.Part{
		Name:        "/customXml/itemProps1.xml",
		ContentType: ndCTCustomProps,
		Data:        []byte(itemPropsXML),
	}); err != nil {
		t.Fatalf("Put customXml/itemProps1.xml: %v", err)
	}
	itemRels := pkg.Relationships("/customXml/item1.xml")
	if _, err := itemRels.Add(ndRelCustomXMLProps, "itemProps1.xml", opc.TargetInternal); err != nil {
		t.Fatalf("adding customXml item rels: %v", err)
	}

	// Root relationships: officeDocument (required for template.Open to
	// identify the format) and the customXml relationship real Word wires
	// at the package root, not from document.xml.
	rootRels := pkg.Relationships("/")
	if _, err := rootRels.Add(ndRelOfficeDocument, "word/document.xml", opc.TargetInternal); err != nil {
		t.Fatalf("adding officeDocument relationship: %v", err)
	}
	if _, err := rootRels.Add(ndRelCustomXML, "customXml/item1.xml", opc.TargetInternal); err != nil {
		t.Fatalf("adding customXml relationship: %v", err)
	}

	var buf bytes.Buffer
	if err := pkg.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

const ndXMLDecl = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"
