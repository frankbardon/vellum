package xmp

import (
	"strconv"
	"strings"
	"time"
)

// The namespaces the PDF/A extension-schema description itself lives in. Fixed
// by ISO 19005-2 clause 6.6.2.3.2, not chosen.
const (
	nsExtension = "http://www.aiim.org/pdfa/ns/extension/"
	nsSchema    = "http://www.aiim.org/pdfa/ns/schema#"
	nsProperty  = "http://www.aiim.org/pdfa/ns/property#"
)

// Schema is a metadata vocabulary XMP does not define.
//
// A PDF/A file may carry one, and the condition is the whole reason this type
// exists: every property outside the schemas the XMP specification defines must
// be *described*, in the packet, by a PDF/A extension schema. A property
// written without its description is a conformance failure, and it is one that
// nothing except a validator reports — the file opens, the metadata reads
// correctly in every tool, and the conformance claim it makes about itself is
// false.
//
// Description and value are therefore not two things a caller assembles and
// keeps in step. They are one declaration, written twice by [Metadata.Packet]
// from the same slice, in the same pass — the same discipline the dates get,
// and for the same reason.
type Schema struct {
	// Prefix is the namespace prefix the properties are written under.
	Prefix string

	// Namespace is the URI the prefix binds to. It identifies the vocabulary
	// and is not required to resolve to anything.
	Namespace string

	// Name is the vocabulary's human-readable name, which the description
	// carries so a reader who has never seen the namespace learns what it is.
	Name string

	// Properties are the vocabulary's properties, in the order they are
	// written. A property with no value is described and not written, which is
	// legal and is what lets a schema declare its whole vocabulary while a
	// given document uses part of it.
	Properties []Property
}

// used reports whether any of the schema's properties carry a value.
func (s Schema) used() bool {
	for _, p := range s.Properties {
		if p.Value != "" {
			return true
		}
	}
	return false
}

// Property is one property of a [Schema].
type Property struct {
	// Name is the property's local name, written under the schema's prefix.
	Name string

	// Type is the XMP value type: "Text", "Date", "Integer" or "Boolean".
	//
	// A type outside the basic set would itself need describing, in a value
	// type description beside the schema. Vellum declares none, so the set is
	// closed and [AllValueTypes] is the whole of it.
	Type ValueType

	// Category is "internal" for a value the software generated and "external"
	// for one a person supplied. A validator does not check which is right, but
	// a reader deciding whether a field is editable does.
	Category Category

	// Description says what the property means, in prose.
	Description string

	// Value is what this document carries. Empty means the property is
	// described and not written.
	Value string
}

// ValueType is an XMP basic value type.
type ValueType string

const (
	TypeText    ValueType = "Text"
	TypeDate    ValueType = "Date"
	TypeInteger ValueType = "Integer"
	TypeBoolean ValueType = "Boolean"
)

// AllValueTypes returns the value types, in declaration order.
func AllValueTypes() []ValueType {
	return []ValueType{TypeText, TypeDate, TypeInteger, TypeBoolean}
}

// Accepts reports whether a rendered value is well formed for the type.
//
// It exists because "declared Boolean, written in Go's syntax" is a real
// failure that this repository has already produced: XMP's booleans are "True"
// and "False", `strconv.FormatBool` gives "true" and "false", and the resulting
// file opens, reads correctly in every tool, and fails ISO 19005-2 clause
// 6.6.2.3.1 with a message nothing but a validator emits.
//
// The constructors below are the way to avoid it. This is how a schema someone
// else built is checked.
func (t ValueType) Accepts(value string) bool {
	switch t {
	case TypeText:
		return true
	case TypeBoolean:
		return value == "True" || value == "False"
	case TypeInteger:
		_, err := strconv.ParseInt(value, 10, 64)
		return err == nil
	case TypeDate:
		_, err := time.Parse(dateLayout, value)
		return err == nil
	}
	return false
}

// Bool renders a boolean in XMP's syntax, which is not Go's.
func Bool(v bool) string {
	if v {
		return "True"
	}
	return "False"
}

// Int renders an integer.
func Int(n int64) string { return strconv.FormatInt(n, 10) }

// Date renders an instant in XMP's date syntax.
//
// Always UTC with an explicit zero offset, which is what the packet's own dates
// use — one formatter, so a schema's dates and the document's cannot end up in
// two syntaxes. A local offset would make the same instant render differently
// depending on where the file was written, which is a difference in the bytes
// for a reason that is not in the document.
//
// The zero time renders as the empty string, which is how a property says it
// has no value rather than saying its value is the beginning of year one.
func Date(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(dateLayout)
}

// dateLayout is XMP's date syntax as Go spells it.
const dateLayout = "2006-01-02T15:04:05-07:00"

// Category is whether a property's value was generated or supplied.
type Category string

const (
	// Internal is a value the software produced.
	Internal Category = "internal"

	// External is a value a person supplied.
	External Category = "external"
)

// AllCategories returns the categories, in declaration order.
func AllCategories() []Category { return []Category{Internal, External} }

// writeExtensions appends the values and then the description of every schema
// that carries one.
//
// Both from the same slice, in one call, so a document cannot write a property
// it did not describe. Splitting this into two exported steps would give a
// caller a way to do exactly that, and the resulting file passes every check
// but the validator's.
func writeExtensions(b *strings.Builder, schemas []Schema) {
	used := make([]Schema, 0, len(schemas))
	for _, s := range schemas {
		if s.Prefix != "" && s.Namespace != "" && s.used() {
			used = append(used, s)
		}
	}
	if len(used) == 0 {
		return
	}

	for _, s := range used {
		b.WriteString(`    <rdf:Description rdf:about="" xmlns:` + s.Prefix +
			`="` + escapeXML(s.Namespace) + `">` + "\n")
		for _, p := range s.Properties {
			if p.Value == "" {
				continue
			}
			b.WriteString("      <" + s.Prefix + ":" + p.Name + ">" +
				escapeXML(p.Value) + "</" + s.Prefix + ":" + p.Name + ">\n")
		}
		b.WriteString("    </rdf:Description>\n")
	}

	b.WriteString(`    <rdf:Description rdf:about=""` + "\n")
	b.WriteString(`        xmlns:pdfaExtension="` + nsExtension + `"` + "\n")
	b.WriteString(`        xmlns:pdfaSchema="` + nsSchema + `"` + "\n")
	b.WriteString(`        xmlns:pdfaProperty="` + nsProperty + `">` + "\n")
	b.WriteString("      <pdfaExtension:schemas>\n        <rdf:Bag>\n")

	for _, s := range used {
		b.WriteString(`          <rdf:li rdf:parseType="Resource">` + "\n")
		b.WriteString("            <pdfaSchema:schema>" + escapeXML(s.Name) + "</pdfaSchema:schema>\n")
		b.WriteString("            <pdfaSchema:namespaceURI>" + escapeXML(s.Namespace) + "</pdfaSchema:namespaceURI>\n")
		b.WriteString("            <pdfaSchema:prefix>" + escapeXML(s.Prefix) + "</pdfaSchema:prefix>\n")
		b.WriteString("            <pdfaSchema:property>\n              <rdf:Seq>\n")
		for _, p := range s.Properties {
			b.WriteString(`                <rdf:li rdf:parseType="Resource">` + "\n")
			b.WriteString("                  <pdfaProperty:name>" + escapeXML(p.Name) + "</pdfaProperty:name>\n")
			b.WriteString("                  <pdfaProperty:valueType>" + escapeXML(string(p.Type)) + "</pdfaProperty:valueType>\n")
			b.WriteString("                  <pdfaProperty:category>" + escapeXML(string(p.Category)) + "</pdfaProperty:category>\n")
			b.WriteString("                  <pdfaProperty:description>" + escapeXML(p.Description) + "</pdfaProperty:description>\n")
			b.WriteString("                </rdf:li>\n")
		}
		b.WriteString("              </rdf:Seq>\n            </pdfaSchema:property>\n")
		b.WriteString("          </rdf:li>\n")
	}

	b.WriteString("        </rdf:Bag>\n      </pdfaExtension:schemas>\n")
	b.WriteString("    </rdf:Description>\n")
}
