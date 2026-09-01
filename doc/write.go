package doc

import (
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
)

// Part names this writer emits.
const (
	PartDocument  = "/word/document.xml"
	PartCoreProps = "/docProps/core.xml"
	PartAppProps  = "/docProps/app.xml"
)

// WriteOptions configures a write. The zero value is the deterministic
// default.
type WriteOptions struct {
	// SourceDateEpoch stamps every date the package carries — the zip entry
	// timestamps and the core properties alike. The zero value selects the
	// pinned epoch.
	//
	// It reaches the document metadata as well as the archive because a
	// creation date read from the clock would make the same specification
	// produce different bytes on every run, which is the guarantee this whole
	// layer exists to keep.
	SourceDateEpoch time.Time

	// Producer names the software in the package's extended properties.
	Producer string
}

// defaultProducer is what appears in a document's application property when
// the caller names nothing.
const defaultProducer = "Vellum"

// Package assembles the OPC package for this document.
func (d *Document) Package(opts WriteOptions) (*opc.Package, error) {
	epoch := opts.SourceDateEpoch
	if epoch.IsZero() {
		epoch = zipdet.PinnedEpoch
	}
	producer := opts.Producer
	if producer == "" {
		producer = defaultProducer
	}

	p := opc.New()
	p.ContentTypes().SetDefault(defaultExtensionXML, ctXML)

	if err := p.Put(&opc.Part{Name: PartDocument, ContentType: ctMainDocument, Data: d.documentXML()}); err != nil {
		return nil, err
	}
	if err := p.Put(&opc.Part{Name: PartCoreProps, ContentType: ctCoreProperties, Data: d.corePropsXML(epoch)}); err != nil {
		return nil, err
	}
	if err := p.Put(&opc.Part{Name: PartAppProps, ContentType: ctExtendedProps, Data: appPropsXML(producer)}); err != nil {
		return nil, err
	}

	root := p.Relationships("/")
	if _, err := root.Add(relOfficeDocument, "word/document.xml", opc.TargetInternal); err != nil {
		return nil, err
	}
	if _, err := root.Add(relCoreProperties, "docProps/core.xml", opc.TargetInternal); err != nil {
		return nil, err
	}
	if _, err := root.Add(relExtendedProps, "docProps/app.xml", opc.TargetInternal); err != nil {
		return nil, err
	}

	// The main document part references nothing yet, but Word writes an empty
	// relationships part for it regardless. Matching that shape keeps the
	// package structurally indistinguishable from an authored one.
	p.Relationships(PartDocument).AlwaysEmit()

	return p, nil
}

// WriteTo emits the document as a .docx.
func (d *Document) WriteTo(w io.Writer, opts WriteOptions) error {
	p, err := d.Package(opts)
	if err != nil {
		return err
	}
	epoch := opts.SourceDateEpoch
	if epoch.IsZero() {
		epoch = zipdet.PinnedEpoch
	}
	return p.WriteTo(w, zipdet.WriteOptions{SourceDateEpoch: epoch})
}

func (d *Document) documentXML() []byte {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<w:document xmlns:w="`)
	b.WriteString(nsWordprocessing)
	b.WriteString(`"><w:body>`)

	for i := range d.Body {
		writeParagraph(&b, &d.Body[i])
	}

	writeSectPr(&b, d.Page)
	b.WriteString(`</w:body></w:document>`)
	return []byte(b.String())
}

func writeParagraph(b *strings.Builder, p *Paragraph) {
	b.WriteString(`<w:p>`)
	if p.OutlineLevel > 0 {
		// The outline level is what makes a heading navigable and what a
		// table-of-contents field later collects. It is emitted even in the
		// skeleton because it is structural rather than cosmetic.
		b.WriteString(`<w:pPr><w:outlineLvl w:val="`)
		b.WriteString(strconv.Itoa(p.OutlineLevel - 1))
		b.WriteString(`"/></w:pPr>`)
	}
	for i := range p.Runs {
		writeRun(b, &p.Runs[i])
	}
	b.WriteString(`</w:p>`)
}

func writeRun(b *strings.Builder, r *Run) {
	b.WriteString(`<w:r>`)
	if r.Bold || r.SizeHalfPoints > 0 {
		b.WriteString(`<w:rPr>`)
		if r.Bold {
			b.WriteString(`<w:b/>`)
		}
		if r.SizeHalfPoints > 0 {
			size := strconv.Itoa(r.SizeHalfPoints)
			// szCs sets the complex-script size. Omitting it lets a
			// complex-script run fall back to a different size than its Latin
			// neighbour, which is a subtle and unpleasant defect.
			b.WriteString(`<w:sz w:val="` + size + `"/><w:szCs w:val="` + size + `"/>`)
		}
		b.WriteString(`</w:rPr>`)
	}
	// xml:space="preserve" on every text node, unconditionally. Leading and
	// trailing whitespace is content, and deciding case by case whether to
	// preserve it is how a writer comes to eat a space it was given.
	b.WriteString(`<w:t xml:space="preserve">`)
	b.WriteString(escapeText(r.Text))
	b.WriteString(`</w:t></w:r>`)
}

func writeSectPr(b *strings.Builder, page PageSetup) {
	b.WriteString(`<w:sectPr><w:pgSz w:w="`)
	b.WriteString(strconv.Itoa(page.WidthTwips))
	b.WriteString(`" w:h="`)
	b.WriteString(strconv.Itoa(page.HeightTwips))
	b.WriteString(`"/><w:pgMar w:top="`)
	b.WriteString(strconv.Itoa(page.MarginTop))
	b.WriteString(`" w:right="`)
	b.WriteString(strconv.Itoa(page.MarginRight))
	b.WriteString(`" w:bottom="`)
	b.WriteString(strconv.Itoa(page.MarginBottom))
	b.WriteString(`" w:left="`)
	b.WriteString(strconv.Itoa(page.MarginLeft))
	b.WriteString(`" w:header="`)
	b.WriteString(strconv.Itoa(page.MarginHeader))
	b.WriteString(`" w:footer="`)
	b.WriteString(strconv.Itoa(page.MarginFooter))
	b.WriteString(`" w:gutter="0"/></w:sectPr>`)
}

func (d *Document) corePropsXML(epoch time.Time) []byte {
	stamp := epoch.UTC().Format("2006-01-02T15:04:05Z")

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<cp:coreProperties xmlns:cp="` + nsCoreProps +
		`" xmlns:dc="` + nsDublinCore +
		`" xmlns:dcterms="` + nsDublinTerms +
		`" xmlns:xsi="` + nsXSI + `">`)
	if d.Title != "" {
		b.WriteString(`<dc:title>` + escapeText(d.Title) + `</dc:title>`)
	}
	b.WriteString(`<dcterms:created xsi:type="dcterms:W3CDTF">` + escapeAttr(stamp) + `</dcterms:created>`)
	b.WriteString(`<dcterms:modified xsi:type="dcterms:W3CDTF">` + escapeAttr(stamp) + `</dcterms:modified>`)
	b.WriteString(`</cp:coreProperties>`)
	return []byte(b.String())
}

func appPropsXML(producer string) []byte {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<Properties xmlns="` + nsExtendedProps + `">`)
	b.WriteString(`<Application>` + escapeText(producer) + `</Application>`)
	b.WriteString(`</Properties>`)
	return []byte(b.String())
}
