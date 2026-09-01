package doc

import (
	"io"
	"sort"
	"strconv"
	"time"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
)

// Part names this writer emits.
const (
	PartDocument  = "/word/document.xml"
	PartStyles    = "/word/styles.xml"
	PartNumbering = "/word/numbering.xml"
	PartFootnotes = "/word/footnotes.xml"
	PartSettings  = "/word/settings.xml"
	PartCoreProps = "/docProps/core.xml"
	PartAppProps  = "/docProps/app.xml"
	PartCustom    = "/docProps/custom.xml"
)

// WriteOptions configures a write. The zero value is the deterministic default.
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

// defaultProducer is what appears in a document's application property when the
// caller names nothing.
const defaultProducer = "Vellum"

// writer carries the state of one package assembly.
//
// Relationship IDs are assigned here, once, before any part is serialised —
// because document.xml references an image by relationship ID and the
// relationships part is written afterwards, so the two must agree and only one
// of them can be authoritative.
type writer struct {
	doc      *Document
	epoch    time.Time
	producer string

	// imageRels, headerRels and footerRels map an index to its relationship ID.
	//
	// Filled in from the frozen relationship set rather than from a counter of
	// this writer's own. opc derives identifiers from a sorted walk of the
	// relationships' own content — a byte-layout invariant — so a writer that
	// numbered them itself would be a second authority on the same value, and
	// the first disagreement would leave document.xml pointing at the wrong
	// part.
	imageRels  []string
	headerRels []string
	footerRels []string
}

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

	w := &writer{doc: d, epoch: epoch, producer: producer}

	p := opc.New()
	ct := p.ContentTypes()
	ct.SetDefault("xml", ctXML)
	ct.SetDefault("rels", "application/vnd.openxmlformats-package.relationships+xml")

	// Relationships first. document.xml references media and running heads by
	// identifier, so the identifiers must be settled before it is serialised.
	if err := w.buildDocumentRelationships(p); err != nil {
		return nil, err
	}

	if err := p.Put(&opc.Part{Name: PartDocument, ContentType: ctMainDocument, Data: w.documentXML()}); err != nil {
		return nil, err
	}
	if err := p.Put(&opc.Part{Name: PartStyles, ContentType: ctStyles, Data: w.stylesXML()}); err != nil {
		return nil, err
	}
	if err := p.Put(&opc.Part{Name: PartSettings, ContentType: ctSettings, Data: w.settingsXML()}); err != nil {
		return nil, err
	}
	if !d.Numbering.IsEmpty() {
		if err := p.Put(&opc.Part{Name: PartNumbering, ContentType: ctNumbering, Data: w.numberingXML()}); err != nil {
			return nil, err
		}
	}
	if len(d.Footnotes) > 0 {
		if err := p.Put(&opc.Part{Name: PartFootnotes, ContentType: ctFootnotes, Data: w.footnotesXML()}); err != nil {
			return nil, err
		}
	}

	for i := range d.Headers {
		if err := p.Put(&opc.Part{Name: headerPartName(i), ContentType: ctHeader,
			Data: w.headerFooterXML("hdr", &d.Headers[i])}); err != nil {
			return nil, err
		}
	}
	for i := range d.Footers {
		if err := p.Put(&opc.Part{Name: footerPartName(i), ContentType: ctFooter,
			Data: w.headerFooterXML("ftr", &d.Footers[i])}); err != nil {
			return nil, err
		}
	}

	for i := range d.Media {
		m := &d.Media[i]
		name := mediaPartName(i, m.MediaType)
		ct.SetOverride(name, m.MediaType)
		if err := p.Put(&opc.Part{Name: name, ContentType: m.MediaType,
			Data: m.Bytes}); err != nil {
			return nil, err
		}
	}

	if err := p.Put(&opc.Part{Name: PartCoreProps, ContentType: ctCoreProperties, Data: w.corePropsXML()}); err != nil {
		return nil, err
	}
	if err := p.Put(&opc.Part{Name: PartAppProps, ContentType: ctExtendedProps, Data: w.appPropsXML()}); err != nil {
		return nil, err
	}
	if d.Provenance != nil {
		if err := p.Put(&opc.Part{Name: PartCustom, ContentType: ctCustomProps, Data: w.customPropsXML()}); err != nil {
			return nil, err
		}
	}

	root := p.Relationships("/")
	for _, r := range []struct{ typ, target string }{
		{relExtendedProps, "docProps/app.xml"},
		{relOfficeDocument, "word/document.xml"},
		{relCoreProperties, "docProps/core.xml"},
	} {
		if _, err := root.Add(r.typ, r.target, opc.TargetInternal); err != nil {
			return nil, err
		}
	}
	if d.Provenance != nil {
		if _, err := root.Add(relCustomProps, "docProps/custom.xml", opc.TargetInternal); err != nil {
			return nil, err
		}
	}

	return p, nil
}

// buildDocumentRelationships declares the document part's relationships and
// resolves the identifiers the document markup will reference.
//
// Declared, then frozen, then read back. The freeze is what makes the read
// meaningful: before it, identifiers are in insertion order and will change
// when the set is serialised.
func (w *writer) buildDocumentRelationships(p *opc.Package) error {
	rels := p.Relationships(PartDocument)
	rels.AlwaysEmit()

	targets := []struct{ typ, target string }{
		{relStyles, "styles.xml"},
		{relSettings, "settings.xml"},
	}
	if !w.doc.Numbering.IsEmpty() {
		targets = append(targets, struct{ typ, target string }{relNumbering, "numbering.xml"})
	}
	if len(w.doc.Footnotes) > 0 {
		targets = append(targets, struct{ typ, target string }{relFootnotes, "footnotes.xml"})
	}
	for i := range w.doc.Media {
		targets = append(targets, struct{ typ, target string }{
			relImage, "media/" + mediaFileName(i, w.doc.Media[i].MediaType)})
	}
	for i := range w.doc.Headers {
		targets = append(targets, struct{ typ, target string }{
			relHeader, "header" + strconv.Itoa(i+1) + ".xml"})
	}
	for i := range w.doc.Footers {
		targets = append(targets, struct{ typ, target string }{
			relFooter, "footer" + strconv.Itoa(i+1) + ".xml"})
	}

	for _, t := range targets {
		if _, err := rels.Add(t.typ, t.target, opc.TargetInternal); err != nil {
			return err
		}
	}
	rels.Freeze()

	resolve := func(typ, target string) (string, error) {
		id, ok := rels.IDFor(typ, target)
		if !ok {
			return "", verr.NewCodedErrorWithDetails(verr.VELLUM_OPC_RELATIONSHIP_INVALID,
				"a relationship the document markup references was not declared",
				map[string]any{"type": typ, "target": target})
		}
		return id, nil
	}

	w.imageRels = make([]string, len(w.doc.Media))
	for i := range w.doc.Media {
		id, err := resolve(relImage, "media/"+mediaFileName(i, w.doc.Media[i].MediaType))
		if err != nil {
			return err
		}
		w.imageRels[i] = id
	}
	w.headerRels = make([]string, len(w.doc.Headers))
	for i := range w.doc.Headers {
		id, err := resolve(relHeader, "header"+strconv.Itoa(i+1)+".xml")
		if err != nil {
			return err
		}
		w.headerRels[i] = id
	}
	w.footerRels = make([]string, len(w.doc.Footers))
	for i := range w.doc.Footers {
		id, err := resolve(relFooter, "footer"+strconv.Itoa(i+1)+".xml")
		if err != nil {
			return err
		}
		w.footerRels[i] = id
	}
	return nil
}

func (w *writer) imageRel(index int) (string, bool) {
	if index < 0 || index >= len(w.imageRels) {
		return "", false
	}
	return w.imageRels[index], true
}

func (w *writer) headerRel(id string) (string, bool) {
	for i := range w.doc.Headers {
		if w.doc.Headers[i].ID == id && id != "" {
			return w.headerRels[i], true
		}
	}
	return "", false
}

func (w *writer) footerRel(id string) (string, bool) {
	for i := range w.doc.Footers {
		if w.doc.Footers[i].ID == id && id != "" {
			return w.footerRels[i], true
		}
	}
	return "", false
}

// hasDirtyField reports whether any field needs recalculation on open.
func (w *writer) hasDirtyField() bool {
	for _, list := range [][]Section{w.doc.Sections} {
		for i := range list {
			for j := range list[i].Content {
				if contentHasDirtyField(&list[i].Content[j]) {
					return true
				}
			}
		}
	}
	for i := range w.doc.Headers {
		for j := range w.doc.Headers[i].Content {
			if contentHasDirtyField(&w.doc.Headers[i].Content[j]) {
				return true
			}
		}
	}
	for i := range w.doc.Footers {
		for j := range w.doc.Footers[i].Content {
			if contentHasDirtyField(&w.doc.Footers[i].Content[j]) {
				return true
			}
		}
	}
	return false
}

func contentHasDirtyField(c *Content) bool {
	if c.Paragraph != nil {
		for i := range c.Paragraph.Runs {
			if f := c.Paragraph.Runs[i].Field; f != nil && f.Dirty {
				return true
			}
		}
	}
	if c.Table != nil {
		for i := range c.Table.Rows {
			for j := range c.Table.Rows[i].Cells {
				for k := range c.Table.Rows[i].Cells[j].Content {
					if contentHasDirtyField(&c.Table.Rows[i].Cells[j].Content[k]) {
						return true
					}
				}
			}
		}
	}
	return false
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

// mediaPartName returns the OPC part name for an embedded image.
//
// Indexed by position in a slice that is itself ordered by content hash, so a
// document's media part names are a function of what is in them rather than of
// the order the content mentioned them.
func mediaPartName(index int, mediaType string) string {
	return "/word/media/" + mediaFileName(index, mediaType)
}

func mediaFileName(index int, mediaType string) string {
	return "image" + strconv.Itoa(index+1) + "." + mediaExtension(mediaType)
}

func headerPartName(index int) string { return "/word/header" + strconv.Itoa(index+1) + ".xml" }
func footerPartName(index int) string { return "/word/footer" + strconv.Itoa(index+1) + ".xml" }

// sortedProvenanceKeys is used by the custom-properties writer. Declared here
// so the property order is one decision in one place.
func sortedProvenanceKeys(props []customProperty) []customProperty {
	out := append([]customProperty(nil), props...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
