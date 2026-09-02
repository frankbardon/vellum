package sheet

import (
	"sort"
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
	if w.wb.Title != "" {
		b.WriteString(`<dc:title>` + escapeText(w.wb.Title) + `</dc:title>`)
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
// Mirrors [doc]'s writer of the same name and for the same reasons: these are
// properties a reader can actually see — Excel shows them in File > Info >
// Properties > Advanced — without knowing anything about Vellum, and the
// names carry a Vellum prefix so they cannot collide with a consumer's own.
func (w *writer) customPropsXML() []byte {
	props := sortedProvenanceKeys(provenanceProperties(w))

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<Properties xmlns="` + nsCustomProps + `" xmlns:vt="` + nsDocPropsVT + `">`)
	for i, p := range props {
		// pid starts at 2. Values 0 and 1 are reserved by the specification,
		// and a document numbering from 0 is one Excel refuses to open.
		b.WriteString(`<property fmtid="{D5CDD505-2E9C-101B-9397-08002B2CF9AE}" pid="` +
			strconv.Itoa(i+2) + `" name="` + escapeAttr(p.Name) + `">`)
		b.WriteString(`<vt:lpwstr>` + escapeText(p.Value) + `</vt:lpwstr></property>`)
	}
	b.WriteString(`</Properties>`)
	return []byte(b.String())
}

// sortedProvenanceKeys sorts custom properties by name, so the emitted bytes
// are a function of the record's content rather than of the order this
// function happened to build them in.
func sortedProvenanceKeys(props []customProperty) []customProperty {
	out := append([]customProperty(nil), props...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// provenanceProperties flattens the record into name/value pairs. Mirrors
// [doc]'s helper of the same name.
func provenanceProperties(w *writer) []customProperty {
	r := w.wb.Provenance
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
