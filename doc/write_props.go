package doc

import (
	"strconv"
	"strings"
	"time"
)

// corePropsXML emits docProps/core.xml.
func (w *writer) corePropsXML() []byte {
	stamp := w.epoch.UTC().Format("2006-01-02T15:04:05Z")

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<cp:coreProperties xmlns:cp="` + nsCoreProps +
		`" xmlns:dc="` + nsDublinCore +
		`" xmlns:dcterms="` + nsDublinTerms +
		`" xmlns:xsi="` + nsXSI + `">`)
	if w.doc.Title != "" {
		b.WriteString(`<dc:title>` + escapeText(w.doc.Title) + `</dc:title>`)
	}
	b.WriteString(`<dcterms:created xsi:type="dcterms:W3CDTF">` + escapeAttr(stamp) + `</dcterms:created>`)
	b.WriteString(`<dcterms:modified xsi:type="dcterms:W3CDTF">` + escapeAttr(stamp) + `</dcterms:modified>`)
	b.WriteString(`</cp:coreProperties>`)
	return []byte(b.String())
}

// appPropsXML emits docProps/app.xml.
func (w *writer) appPropsXML() []byte {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<Properties xmlns="` + nsExtendedProps + `">`)
	b.WriteString(`<Application>` + escapeText(w.producer) + `</Application>`)
	b.WriteString(`</Properties>`)
	return []byte(b.String())
}

// customProperty is one entry in docProps/custom.xml.
type customProperty struct {
	Name  string
	Value string
}

// customPropsXML embeds the provenance record in the package's custom document
// properties.
//
// Custom properties rather than a part of Vellum's own invention, because these
// are the ones a reader can actually see: Word shows them in File > Info >
// Properties > Advanced, and every OOXML tool can read them without knowing
// anything about Vellum. A record nobody can read is a record that does not do
// its job.
//
// The property names carry a Vellum prefix so they cannot collide with a
// consumer's own, and the set is sorted by name — these bytes are part of the
// package and therefore part of the determinism guarantee, which is also why the
// record itself carries no machine identity.
func (w *writer) customPropsXML() []byte {
	props := sortedProvenanceKeys(provenanceProperties(w))

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<Properties xmlns="` + nsCustomProps + `" xmlns:vt="` + nsDocPropsVT + `">`)
	for i, p := range props {
		// pid starts at 2. Values 0 and 1 are reserved by the specification,
		// and a document numbering from 0 is one Word refuses to open.
		b.WriteString(`<property fmtid="{D5CDD505-2E9C-101B-9397-08002B2CF9AE}" pid="` +
			strconv.Itoa(i+2) + `" name="` + escapeAttr(p.Name) + `">`)
		b.WriteString(`<vt:lpwstr>` + escapeText(p.Value) + `</vt:lpwstr></property>`)
	}
	b.WriteString(`</Properties>`)
	return []byte(b.String())
}

// provenanceProperties flattens the record into name/value pairs.
//
// Flattened rather than embedded as JSON, because a custom property is a
// scalar and a reader looking at Word's properties dialogue should see fields
// rather than a wall of braces. The asset and font manifests, which are lists,
// are summarised: a count and a joined digest list, so the record stays
// auditable without becoming the document's largest part.
func provenanceProperties(w *writer) []customProperty {
	r := w.doc.Provenance
	if r == nil {
		return nil
	}

	props := []customProperty{
		{"VellumVersion", r.VellumVersion},
		{"VellumSourceDateEpoch", r.SourceDateEpoch.UTC().Format(time.RFC3339)},
	}
	add := func(name, value string) {
		if value != "" {
			props = append(props, customProperty{name, value})
		}
	}
	add("VellumSpecHash", r.SpecHash)
	add("VellumThemeHash", r.ThemeHash)
	add("VellumBindingHash", r.BindingHash)
	add("VellumTemplateHash", r.TemplateHash)
	add("VellumProvenanceHash", r.Hash())

	if r.GeneratedAt != nil {
		// Present only when a caller deliberately opted out of deterministic
		// output. Its absence is therefore itself information: a document with
		// no VellumGeneratedAt is one whose bytes are reproducible.
		add("VellumGeneratedAt", r.GeneratedAt.UTC().Format(time.RFC3339))
	}

	if len(r.Assets) > 0 {
		hashes := make([]string, 0, len(r.Assets))
		for _, a := range r.Assets {
			hashes = append(hashes, a.Hash)
		}
		add("VellumAssetCount", strconv.Itoa(len(r.Assets)))
		add("VellumAssetHashes", strings.Join(hashes, " "))
	}
	if len(r.Fonts) > 0 {
		faces := make([]string, 0, len(r.Fonts))
		for _, f := range r.Fonts {
			// The substitution is the part worth auditing: a face that was
			// silently replaced is the thing that makes two renders differ, so
			// the record says which face actually appeared.
			entry := f.Family
			if f.SubstitutedWith != "" {
				entry += "->" + f.SubstitutedWith
			}
			if f.Embedded {
				entry += "(embedded)"
			}
			faces = append(faces, entry)
		}
		add("VellumFonts", strings.Join(faces, "; "))
	}
	for _, src := range r.Sources {
		add("VellumSource_"+src.Kind, src.ID)
	}
	return props
}
