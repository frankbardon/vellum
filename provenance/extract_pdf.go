package provenance

import (
	"bytes"
	"encoding/json"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
)

// xpacketBegin and xpacketEnd bracket the XMP packet [xmp.Metadata.Packet]
// writes into every PDF Vellum produces. It is always stored via
// object.Document.AddRawStream rather than object.Document.AddStream — see
// pdf/pdf.go's own WriteTo — so unlike every other stream in the file it is
// never deflate-compressed, whatever [WriteOptions.Uncompressed] said, and
// its bytes appear in the file exactly as [xmp.Metadata.Packet] built them.
// That is what makes finding it a byte-string search rather than a PDF
// parse: Vellum has no PDF reader, and building one only to locate one
// stream would be a great deal of machinery for a file format whose author
// already made this one part of itself grep-able by design — see the
// packet's own doc comment on why it is written by hand rather than
// marshalled.
var (
	xpacketBegin = []byte("<?xpacket begin=")
	xpacketEnd   = []byte("<?xpacket end=")
)

// recordOpenTag and recordCloseTag bracket the one property [Record.XMPSchema]
// carries the whole record's canonical JSON in — see PropertyRecord and
// [Prefix] in provenance/xmp.go. Every other property that schema writes is a
// derived summary; this one is the record itself, so — unlike [Extract]'s
// OOXML custom-properties reconstruction — recovering it from a PDF is
// lossless.
var (
	recordOpenTag  = []byte("<" + Prefix + ":" + PropertyRecord + ">")
	recordCloseTag = []byte("</" + Prefix + ":" + PropertyRecord + ">")
)

// ExtractPDF reads a PDF file's own XMP packet, if it carries one, and looks
// for the vellum:record property [Record.XMPSchema] writes, which holds the
// whole record as canonical JSON.
//
// Unlike [Extract]'s OOXML reconstruction, this is a full recovery rather
// than a lossy one: the packet carries the record's own JSON, byte for byte,
// rather than a set of summarised scalar properties. It is nonetheless the
// PDF-side counterpart of the same operation, because the mechanism is
// entirely different — an XMP packet inside a raw metadata stream, not a
// custom document property — which is why this is a separate function
// rather than a second case inside Extract; the two file formats have
// nothing about provenance embedding to share beyond the [Record] type
// itself.
//
// A PDF Vellum wrote always carries an XMP packet — [pdf.Document.WriteTo]
// writes one unconditionally — but the vellum:record property inside it is
// only present when a caller populated [xmp.Metadata.Extensions] with
// [Record.XMPSchema] directly, which Compose's own path does not yet do.
// ExtractPDF therefore returns a nil *Record and a nil error both when data
// carries no XMP packet at all — it is not even a PDF Vellum wrote, or not a
// PDF at all — and when it carries one with no vellum:record property: in
// both cases, "no provenance embedded" is the honest, non-error answer.
//
// A packet that does carry the property, with a value that is not valid
// JSON or does not decode as a [Record], is VELLUM_PROVENANCE_MALFORMED: the
// file is claiming to carry a record and the claim does not hold up.
func ExtractPDF(data []byte) (*Record, error) {
	packet, ok := findXMPPacket(data)
	if !ok {
		return nil, nil
	}

	start := bytes.Index(packet, recordOpenTag)
	if start < 0 {
		return nil, nil
	}
	start += len(recordOpenTag)
	end := bytes.Index(packet[start:], recordCloseTag)
	if end < 0 {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_PROVENANCE_MALFORMED,
			"the XMP packet's vellum:record property has no closing tag",
			map[string]any{"property": PropertyRecord})
	}

	raw := unescapeXML(string(packet[start : start+end]))

	var r Record
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return nil, verr.WrapCodedErrorWithDetails(err, verr.VELLUM_PROVENANCE_MALFORMED,
			"the XMP packet's vellum:record property is not valid JSON",
			map[string]any{"property": PropertyRecord})
	}
	return &r, nil
}

// findXMPPacket locates the first XMP packet in data, if any, and returns its
// full bytes including both processing instructions.
func findXMPPacket(data []byte) ([]byte, bool) {
	start := bytes.Index(data, xpacketBegin)
	if start < 0 {
		return nil, false
	}
	relEnd := bytes.Index(data[start:], xpacketEnd)
	if relEnd < 0 {
		return nil, false
	}
	// Extend to the end of the closing processing instruction itself ("?>"),
	// so the packet's own trailing bytes are included rather than truncated
	// at the start of "<?xpacket end=".
	tail := data[start+relEnd:]
	closeAt := bytes.Index(tail, []byte("?>"))
	if closeAt < 0 {
		return nil, false
	}
	end := start + relEnd + closeAt + len("?>")
	return data[start:end], true
}

// unescapeXML inverts [escapeXML] in pdf/xmp/xmp.go: the five characters XMP
// text content cannot carry literally. strings.Replacer performs one
// left-to-right pass with no rescanning, so this cannot double-unescape an
// "&amp;lt;" into "<" the way a naive repeated-substitution approach would.
func unescapeXML(s string) string {
	if !strings.ContainsRune(s, '&') {
		return s
	}
	return xmlUnescaper.Replace(s)
}

var xmlUnescaper = strings.NewReplacer(
	"&lt;", "<",
	"&gt;", ">",
	"&amp;", "&",
	"&apos;", "'",
	"&quot;", `"`,
)
