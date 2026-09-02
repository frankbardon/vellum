// Package xmp builds the metadata a PDF/A file carries, in both the forms it
// has to carry it in.
//
// PDF states its metadata twice: once in the trailer's information dictionary,
// in PDF's own date syntax, and once as an XMP packet in RDF/XML, in ISO 8601.
// PDF/A requires the two to agree, and a validator compares them field by
// field.
//
// They are therefore generated from one value here rather than assembled
// separately and checked. Divergence between the information dictionary and the
// XMP dates is the most common way a nearly correct PDF/A file fails
// validation, and the only reliable fix is to make disagreement impossible to
// express.
package xmp

import (
	"strings"
	"time"

	"github.com/frankbardon/vellum/pdf/object"
)

// PacketID is the fixed identifier every XMP packet carries.
//
// It is not a generated identifier despite looking like one: the XMP
// specification prescribes this exact string so that a tool scanning a file of
// any format can locate a packet without parsing the container.
const PacketID = "W5M0MpCehiHzreSzNTczkc9d"

// Metadata is a document's descriptive metadata.
type Metadata struct {
	// Title is the document title. PDF/A-2b requires it to be present in the
	// XMP when the catalogue marks the document as having one, and a titleless
	// document is unhelpful regardless.
	Title string

	// Author is the document's author, which may be empty.
	Author string

	// Subject is a one-line description, which may be empty.
	Subject string

	// Keywords is a comma-separated list, which may be empty.
	Keywords string

	// Creator names the application the content originated in — the consumer's
	// application, not Vellum.
	Creator string

	// Producer names the software that wrote the file, which is Vellum.
	Producer string

	// Date is both the creation and the modification time.
	//
	// One field rather than two because Vellum writes files and never updates
	// them, so a modification time that differed from the creation time would
	// be describing an event that did not happen. It comes from
	// SourceDateEpoch, never from the clock.
	Date time.Time

	// Extensions are vocabularies XMP does not define, each carrying its own
	// description. See [Schema]: a property written without one is a
	// conformance failure nothing but a validator reports.
	//
	// An ordered slice rather than a map, as everywhere on this path. It is
	// read while bytes are being written, and a map ranged there is a
	// nondeterminism source sitting directly upstream of the output.
	Extensions []Schema
}

// InfoEntries returns the information dictionary.
//
// Empty fields are omitted rather than written empty, because a validator
// treats a present-but-empty entry as a claim that the document has no title,
// which is different from making no claim.
func (m Metadata) InfoEntries() object.Dict {
	var d object.Dict
	setIfSet(&d, "Title", m.Title)
	setIfSet(&d, "Author", m.Author)
	setIfSet(&d, "Subject", m.Subject)
	setIfSet(&d, "Keywords", m.Keywords)
	setIfSet(&d, "Creator", m.Creator)
	setIfSet(&d, "Producer", m.Producer)

	stamp := object.String(pdfDate(m.Date))
	d.Set("CreationDate", stamp)
	d.Set("ModDate", stamp)
	return d
}

// Packet returns the XMP packet.
//
// Written by hand rather than marshalled. XMP is RDF/XML with a prescribed
// shape, and the packet's leading and trailing processing instructions, its
// byte-order mark and its padding are all positional requirements that a
// general XML encoder would reorder or normalise away.
func (m Metadata) Packet() []byte {
	var b strings.Builder

	// The begin attribute carries a UTF-8 byte-order mark, which is how a
	// scanner determines the packet's encoding without parsing it.
	b.WriteString("<?xpacket begin=\"\uFEFF\" id=\"" + PacketID + "\"?>\n")
	b.WriteString(`<x:xmpmeta xmlns:x="adobe:ns:meta/" x:xmptk="Vellum">` + "\n")
	b.WriteString(`  <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` + "\n")

	// The conformance claim. A file asserting this and failing validation is
	// worse than one making no claim, which is why the veraPDF gate is not
	// optional in CI.
	b.WriteString(`    <rdf:Description rdf:about="" xmlns:pdfaid="http://www.aiim.org/pdfa/ns/id/">` + "\n")
	b.WriteString("      <pdfaid:part>2</pdfaid:part>\n")
	b.WriteString("      <pdfaid:conformance>B</pdfaid:conformance>\n")
	b.WriteString("    </rdf:Description>\n")

	b.WriteString(`    <rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/">` + "\n")
	b.WriteString("      <dc:format>application/pdf</dc:format>\n")
	if m.Title != "" {
		b.WriteString("      <dc:title>\n        <rdf:Alt>\n")
		b.WriteString(`          <rdf:li xml:lang="x-default">` + escapeXML(m.Title) + "</rdf:li>\n")
		b.WriteString("        </rdf:Alt>\n      </dc:title>\n")
	}
	if m.Author != "" {
		b.WriteString("      <dc:creator>\n        <rdf:Seq>\n")
		b.WriteString("          <rdf:li>" + escapeXML(m.Author) + "</rdf:li>\n")
		b.WriteString("        </rdf:Seq>\n      </dc:creator>\n")
	}
	if m.Subject != "" {
		b.WriteString("      <dc:description>\n        <rdf:Alt>\n")
		b.WriteString(`          <rdf:li xml:lang="x-default">` + escapeXML(m.Subject) + "</rdf:li>\n")
		b.WriteString("        </rdf:Alt>\n      </dc:description>\n")
	}
	b.WriteString("    </rdf:Description>\n")

	stamp := iso8601(m.Date)
	b.WriteString(`    <rdf:Description rdf:about="" xmlns:xmp="http://ns.adobe.com/xap/1.0/">` + "\n")
	b.WriteString("      <xmp:CreateDate>" + stamp + "</xmp:CreateDate>\n")
	b.WriteString("      <xmp:ModifyDate>" + stamp + "</xmp:ModifyDate>\n")
	b.WriteString("      <xmp:MetadataDate>" + stamp + "</xmp:MetadataDate>\n")
	if m.Creator != "" {
		b.WriteString("      <xmp:CreatorTool>" + escapeXML(m.Creator) + "</xmp:CreatorTool>\n")
	}
	b.WriteString("    </rdf:Description>\n")

	if m.Producer != "" || m.Keywords != "" {
		b.WriteString(`    <rdf:Description rdf:about="" xmlns:pdf="http://ns.adobe.com/pdf/1.3/">` + "\n")
		if m.Producer != "" {
			b.WriteString("      <pdf:Producer>" + escapeXML(m.Producer) + "</pdf:Producer>\n")
		}
		if m.Keywords != "" {
			b.WriteString("      <pdf:Keywords>" + escapeXML(m.Keywords) + "</pdf:Keywords>\n")
		}
		b.WriteString("    </rdf:Description>\n")
	}

	writeExtensions(&b, m.Extensions)

	b.WriteString("  </rdf:RDF>\n")
	b.WriteString("</x:xmpmeta>\n")

	// The trailing "w" declares the packet read-only: there is no padding for
	// an in-place editor to grow into. Vellum rewrites a document rather than
	// patching one, so reserving space for an editor would be reserving it for
	// nobody.
	b.WriteString(`<?xpacket end="w"?>`)
	return []byte(b.String())
}

// pdfDate renders the information dictionary's date syntax.
//
// Always in UTC with an explicit zero offset. A local offset would make the
// same instant render differently depending on where the file was written,
// which is a difference in the bytes for a reason that is not in the document.
func pdfDate(t time.Time) string {
	return "D:" + t.UTC().Format("20060102150405") + "+00'00'"
}

// iso8601 renders the XMP date syntax for the same instant.
//
// The same formatter an extension schema's dates go through, so a packet cannot
// carry two date syntaxes. See [Date].
func iso8601(t time.Time) string { return Date(t) }

// setIfSet adds a string entry when it is non-empty.
func setIfSet(d *object.Dict, key object.Name, value string) {
	if value != "" {
		d.Set(key, object.String(value))
	}
}

// escapeXML escapes the five characters that cannot appear as text.
//
// Written here rather than taken from encoding/xml, which escapes a different
// set — it turns tabs and newlines into character references, which is legal
// and makes the packet unreadable in a hex dump for no benefit.
func escapeXML(s string) string {
	if !strings.ContainsAny(s, `<>&'"`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 16)
	for _, r := range s {
		switch r {
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '&':
			b.WriteString("&amp;")
		case '\'':
			b.WriteString("&apos;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
