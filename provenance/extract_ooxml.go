package provenance

import (
	"encoding/xml"
	"strings"
	"time"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
)

// partCustomProperties is the OOXML custom document properties part name:
// the same convention doc, sheet and deck's own writers use for it (see each
// package's own PartCustom constant). provenance cannot import any of the
// three — they import provenance, for Document.Provenance and its
// siblings — so the convention is restated here as the one constant this
// function needs rather than imported from where it is authoritative.
const partCustomProperties = "/docProps/custom.xml"

// vellumPropertyPrefix is the prefix every property [Extract] recognises is
// written under — see provenanceProperties in doc/write_props.go and its
// sheet and deck counterparts. A property without this prefix belongs to
// some other tool or author and is left alone.
const vellumPropertyPrefix = "Vellum"

// vellumSourcePrefix names a per-source property: "VellumSource_" + the
// source's own Kind, holding its ID as the value.
const vellumSourcePrefix = vellumPropertyPrefix + "Source_"

// Extract reads pkg's docProps/custom.xml, if it carries one, and
// reconstructs the [Record] [provenanceProperties] (doc/write_props.go, and
// its sheet and deck counterparts) flattened into it — the inverse of that
// flattening, for a fill-mode template, a compose-mode artifact a caller set
// Document.Provenance on directly, or any other DOCX, XLSX or PPTX package
// carrying the same custom-properties convention.
//
// # The reconstruction is lossy in exactly the places the flattening is
//
// A custom property is a scalar. [Record.Assets] and [Record.Fonts] are both
// lists of structured data, and provenanceProperties summarised each into one
// joined string on the way in — a count and a space-joined hash list for
// assets, a "; "-joined "family[->substitute][(embedded)]" list for fonts —
// so only that summary comes back out here. An [AssetRef] Extract recovers
// carries its Hash and nothing else: Handle and Media are not in the
// summary. A [FontRef] recovers its Family, SubstitutedWith and Embedded, but
// never its SubsetProfile, for the same reason. [Record.Sources] round-trips
// exactly, because provenanceProperties already gave each one its own scalar
// property rather than folding it into a joined list.
//
// # Absence is reported honestly, not as an error
//
// A package carrying no docProps/custom.xml at all, or carrying it with no
// property under the Vellum prefix, is the ordinary case today — Compose does
// not yet populate Document.Provenance on its own path, so most artifacts
// Extract is handed will have neither. Extract returns a nil *Record and a
// nil error for both: "no provenance embedded" is a fact worth reporting
// plainly, not a failure.
//
// A part that *does* carry a Vellum-prefixed property, but one Extract cannot
// parse — a malformed timestamp — is VELLUM_PROVENANCE_MALFORMED, because
// that is a real defect: the artifact is claiming to carry a record and the
// claim does not hold up.
func Extract(pkg *opc.Package) (*Record, error) {
	if pkg == nil {
		return nil, nil
	}
	part, ok := pkg.Get(partCustomProperties)
	if !ok {
		return nil, nil
	}
	data, err := part.Bytes()
	if err != nil {
		return nil, err
	}
	return extractFromCustomProperties(data)
}

// customPropertiesXML is the minimal shape of docProps/custom.xml this
// package reads. encoding/xml matches an untagged element name against any
// namespace, so <vt:lpwstr> is matched by the bare "lpwstr" tag without this
// type needing to know the docPropsVTypes namespace URI at all.
type customPropertiesXML struct {
	XMLName    xml.Name            `xml:"Properties"`
	Properties []customPropertyXML `xml:"property"`
}

type customPropertyXML struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"lpwstr"`
}

func extractFromCustomProperties(data []byte) (*Record, error) {
	var doc customPropertiesXML
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, verr.WrapCodedErrorWithDetails(err, verr.VELLUM_PROVENANCE_MALFORMED,
			"docProps/custom.xml does not parse as well-formed XML",
			map[string]any{"part": partCustomProperties})
	}

	values := make(map[string]string, len(doc.Properties))
	var sources []Source
	for _, p := range doc.Properties {
		if kind, ok := strings.CutPrefix(p.Name, vellumSourcePrefix); ok {
			sources = append(sources, Source{Kind: kind, ID: p.Value})
			continue
		}
		if !strings.HasPrefix(p.Name, vellumPropertyPrefix) {
			continue
		}
		values[p.Name] = p.Value
	}
	if len(values) == 0 && len(sources) == 0 {
		return nil, nil
	}

	r := &Record{
		VellumVersion: values["VellumVersion"],
		SpecHash:      values["VellumSpecHash"],
		ThemeHash:     values["VellumThemeHash"],
		BindingHash:   values["VellumBindingHash"],
		TemplateHash:  values["VellumTemplateHash"],
		Sources:       sources,
	}

	if raw, ok := values["VellumSourceDateEpoch"]; ok {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, malformedTimestamp("VellumSourceDateEpoch", raw, err)
		}
		r.SourceDateEpoch = t
	}
	if raw, ok := values["VellumGeneratedAt"]; ok {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, malformedTimestamp("VellumGeneratedAt", raw, err)
		}
		r.GeneratedAt = &t
	}
	if raw, ok := values["VellumAssetHashes"]; ok {
		for _, h := range strings.Fields(raw) {
			r.Assets = append(r.Assets, AssetRef{Hash: h})
		}
	}
	if raw := values["VellumFonts"]; raw != "" {
		for _, entry := range strings.Split(raw, "; ") {
			r.Fonts = append(r.Fonts, parseFontEntry(entry))
		}
	}

	return r, nil
}

// parseFontEntry inverts the "family[->substitute][(embedded)]" shape
// provenanceProperties writes one [FontRef] as.
func parseFontEntry(entry string) FontRef {
	var f FontRef
	if trimmed, ok := strings.CutSuffix(entry, "(embedded)"); ok {
		f.Embedded = true
		entry = trimmed
	}
	if family, sub, ok := strings.Cut(entry, "->"); ok {
		f.Family = family
		f.SubstitutedWith = sub
		return f
	}
	f.Family = entry
	return f
}

func malformedTimestamp(property, raw string, cause error) error {
	return verr.WrapCodedErrorWithDetails(cause, verr.VELLUM_PROVENANCE_MALFORMED,
		"a provenance property does not carry a valid RFC 3339 timestamp",
		map[string]any{"property": property, "value": raw})
}
