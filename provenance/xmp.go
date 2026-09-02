package provenance

import (
	"github.com/frankbardon/vellum/canon"
	"github.com/frankbardon/vellum/pdf/xmp"
)

// Namespace identifies the provenance vocabulary in XMP.
//
// It follows the same convention as the payload schema's identifier: a URI
// under the project's published documentation site, so it is owned rather than
// invented and a reader who has never seen it can find out what it means. XMP
// namespaces are identifiers and are not required to resolve; this one is
// chosen so that it can.
const Namespace = "https://frankbardon.github.io/vellum/ns/provenance/1.0/"

// Prefix is the namespace prefix the properties are written under.
const Prefix = "vellum"

// SchemaName is the vocabulary's human-readable name.
const SchemaName = "Vellum provenance"

// XMPSchema returns the record as a described XMP schema, ready to embed.
//
// # What is carried, and why it is the whole record
//
// The summary properties — the version, the four hashes, the epoch — are what a
// reader indexes on. They are not what makes the artifact auditable: a hash
// answers "has this changed" and answers nothing at all about what produced it
// unless the record it hashes is stored somewhere the reader can reach.
//
// So the record travels with the file, as its canonical JSON, in the property
// named by [PropertyRecord]. Those are the same bytes [Record.Hash] digests —
// [canon.Canonical] exists for that reason — so the hash in the file describes
// the record in the file, and a reader can check one against the other without
// consulting anything else.
//
// # What is deliberately absent
//
// Nothing read from the machine that ran the render. A hostname or a user would
// make two runs that produced identical content produce different bytes, which
// is the opposite of what a provenance record is for.
func (r *Record) XMPSchema() xmp.Schema {
	s := xmp.Schema{
		Prefix:    Prefix,
		Namespace: Namespace,
		Name:      SchemaName,
		Properties: []xmp.Property{
			{Name: PropertyVersion, Type: xmp.TypeText, Category: xmp.Internal,
				Description: "The Vellum library version that produced this artifact."},
			{Name: PropertySpecHash, Type: xmp.TypeText, Category: xmp.Internal,
				Description: "Canonical hash of the specification the artifact was rendered from."},
			{Name: PropertyThemeHash, Type: xmp.TypeText, Category: xmp.Internal,
				Description: "Canonical hash of the theme document the specification was resolved against."},
			{Name: PropertyBindingHash, Type: xmp.TypeText, Category: xmp.Internal,
				Description: "Canonical hash of the binding, for a fill-mode render."},
			{Name: PropertyTemplateHash, Type: xmp.TypeText, Category: xmp.Internal,
				Description: "Canonical hash of the template, for a fill-mode render."},
			{Name: PropertyEpoch, Type: xmp.TypeDate, Category: xmp.Internal,
				Description: "The pinned timestamp every date in the artifact was stamped from."},
			{Name: PropertyGeneratedAt, Type: xmp.TypeDate, Category: xmp.Internal,
				Description: "Wall-clock time of the render. Absent from a reproducible one."},
			{Name: PropertyDeterministic, Type: xmp.TypeBoolean, Category: xmp.Internal,
				Description: "Whether the render was reproducible: false means two runs of the same specification produced different bytes."},
			{Name: PropertyRecordHash, Type: xmp.TypeText, Category: xmp.Internal,
				Description: "Canonical hash of the record carried in " + Prefix + ":" + PropertyRecord + "."},
			{Name: PropertyRecord, Type: xmp.TypeText, Category: xmp.Internal,
				Description: "The full provenance record as canonical JSON: the assets, the font manifest and the caller's upstream identifiers."},
		},
	}
	if r == nil {
		return s
	}

	set := func(name, value string) {
		for i := range s.Properties {
			if s.Properties[i].Name == name {
				s.Properties[i].Value = value
				return
			}
		}
	}

	set(PropertyVersion, r.VellumVersion)
	set(PropertySpecHash, r.SpecHash)
	set(PropertyThemeHash, r.ThemeHash)
	set(PropertyBindingHash, r.BindingHash)
	set(PropertyTemplateHash, r.TemplateHash)
	set(PropertyEpoch, xmp.Date(r.SourceDateEpoch))
	if r.GeneratedAt != nil {
		set(PropertyGeneratedAt, xmp.Date(*r.GeneratedAt))
	}
	set(PropertyDeterministic, xmp.Bool(r.Deterministic()))
	set(PropertyRecordHash, r.Hash())
	if raw, err := canon.Canonical(r); err == nil {
		set(PropertyRecord, string(raw))
	}
	return s
}

// The property names of the provenance vocabulary. Constants rather than
// literals because they are written into an artifact a consumer parses: a name
// changed here is a wire break, and a wire break should be a diff someone can
// find by searching for the name.
const (
	PropertyVersion       = "vellumVersion"
	PropertySpecHash      = "specHash"
	PropertyThemeHash     = "themeHash"
	PropertyBindingHash   = "bindingHash"
	PropertyTemplateHash  = "templateHash"
	PropertyEpoch         = "sourceDateEpoch"
	PropertyGeneratedAt   = "generatedAt"
	PropertyDeterministic = "deterministic"
	PropertyRecordHash    = "recordHash"
	PropertyRecord        = "record"
)
