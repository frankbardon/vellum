package pdfa

import (
	"bytes"
	"strconv"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/object"
)

// Violation is one way an assembled document fails ISO 19005-2 level B.
//
// It carries the clause rather than only the symptom, because the useful
// question when one of these fires is not "what is wrong" but "what does the
// standard say", and a clause number is the shortest route to that.
type Violation struct {
	// Clause is the ISO 19005-2 clause the requirement comes from.
	Clause string

	// Object is the object number the violation was found in, or zero when the
	// violation is the document's rather than any single object's.
	Object int

	// Reason states what was found, in the terms of the file rather than of
	// the code that built it.
	Reason string
}

// String renders the violation for a details entry.
func (v Violation) String() string {
	s := v.Clause + ": " + v.Reason
	if v.Object > 0 {
		s += " (object " + strconv.Itoa(v.Object) + ")"
	}
	return s
}

// Preflight checks an assembled document against the requirements of
// ISO 19005-2 level B that this writer is capable of violating.
//
// It is a self-gate, not a validator. veraPDF implements the profile; this
// implements the handful of clauses where Vellum's own code is what decides the
// answer, so that a regression is caught by `make test` rather than only by an
// externally provisioned tool that a developer may not have installed and that
// runs behind a build tag. The two are complementary and neither replaces the
// other: the oracle covers clauses this does not, and this covers the case
// where the oracle is absent.
//
// Deliberately not checked here: encryption, LZW in a place Vellum never writes
// one, object streams, JavaScript, embedded files, annotations without
// appearance streams. Every one of those is unrepresentable in this writer, and
// a check for a condition that cannot arise is a check that passes forever
// while proving nothing — the failure mode this project has already hit twice
// with gates that were not looking at anything.
//
// The metadata agreement check reads the XMP packet's own bytes rather than the
// struct they were generated from. That is the point of it: the structural
// guarantee is that one value produces both forms, and this is the independent
// confirmation that the guarantee still holds in the file.
func Preflight(doc *object.Document) error {
	var r report
	catalogue := r.catalogue(doc)
	r.outputIntent(doc, catalogue)
	packet := r.metadata(doc, catalogue)
	r.info(doc, packet)
	r.identifier(doc)
	r.objects(doc)

	if len(r.found) == 0 {
		return nil
	}

	list := make([]any, len(r.found))
	for i, v := range r.found {
		list[i] = v.String()
	}
	return verr.NewCodedErrorWithDetails(verr.VELLUM_PDFA_NONCONFORMANT,
		"the assembled document would violate ISO 19005-2 level B",
		map[string]any{"violations": list, "violation_count": len(r.found)})
}

// report accumulates violations.
//
// Every check runs even after one has failed, because these arrive in groups —
// a font written through a broken path fails the same way on every page — and
// reporting them one build at a time is several runs of the same debugging.
type report struct {
	found []Violation
}

func (r *report) add(clause string, num int, reason string) {
	r.found = append(r.found, Violation{Clause: clause, Object: num, Reason: reason})
}

// catalogue resolves the document catalogue and checks it is one.
func (r *report) catalogue(doc *object.Document) object.Dict {
	d, ok := dictAt(doc, doc.Root)
	if !ok {
		r.add("6.1.2", doc.Root.Number, "the trailer's /Root does not resolve to a dictionary")
		return object.Dict{}
	}
	if name, _ := nameOf(d, "Type"); name != "Catalog" {
		r.add("6.1.2", doc.Root.Number, "the catalogue's /Type is not /Catalog")
	}
	if _, has := d.Get("Pages"); !has {
		r.add("6.1.2", doc.Root.Number, "the catalogue has no /Pages")
	}
	return d
}

// outputIntent checks the one intent PDF/A requires.
//
// Exactly one, not at least one: a file with two output intents claiming
// different output conditions makes two incompatible statements about how its
// colour is to be interpreted, which is why the standard bounds it from both
// sides.
func (r *report) outputIntent(doc *object.Document, catalogue object.Dict) {
	value, has := catalogue.Get("OutputIntents")
	if !has {
		r.add("6.2.2", doc.Root.Number, "the catalogue has no /OutputIntents, so the file states no output condition")
		return
	}
	arr, ok := value.(object.Array)
	if !ok || len(arr) != 1 {
		r.add("6.2.2", doc.Root.Number, "/OutputIntents does not hold exactly one intent")
		return
	}

	intent, ok := resolve(doc, arr[0]).(object.Dict)
	if !ok {
		r.add("6.2.2", doc.Root.Number, "the output intent is not a dictionary")
		return
	}
	if s, _ := nameOf(intent, "S"); s != OutputIntentSubtype {
		r.add("6.2.2", doc.Root.Number, "the output intent's /S is not /"+OutputIntentSubtype)
	}

	profile, has := intent.Get("DestOutputProfile")
	if !has {
		r.add("6.2.2", doc.Root.Number, "the output intent names no /DestOutputProfile, so its profile is referenced rather than embedded")
		return
	}
	ref, ok := profile.(object.Ref)
	if !ok {
		r.add("6.2.2", doc.Root.Number, "/DestOutputProfile is not an indirect reference to a stream")
		return
	}
	if _, ok := streamAt(doc, ref); !ok {
		r.add("6.2.2", ref.Number, "/DestOutputProfile does not resolve to a stream")
	}
}

// metadata checks the XMP packet and returns its bytes.
func (r *report) metadata(doc *object.Document, catalogue object.Dict) []byte {
	value, has := catalogue.Get("Metadata")
	if !has {
		r.add("6.6.2.1", doc.Root.Number, "the catalogue has no /Metadata, so the file carries no XMP")
		return nil
	}
	ref, ok := value.(object.Ref)
	if !ok {
		r.add("6.6.2.1", doc.Root.Number, "/Metadata is not an indirect reference")
		return nil
	}
	s, ok := streamAt(doc, ref)
	if !ok {
		r.add("6.6.2.1", ref.Number, "/Metadata does not resolve to a stream")
		return nil
	}

	if sub, _ := nameOf(s.Dict, "Subtype"); sub != "XML" {
		r.add("6.6.2.1", ref.Number, "the metadata stream's /Subtype is not /XML")
	}
	// The packet has to be readable by a consumer that does not parse PDF at
	// all — that is what makes a metadata packet findable in a file whose
	// structure is damaged — so a filter on it defeats its purpose.
	if _, filtered := s.Dict.Get("Filter"); filtered {
		r.add("6.6.2.1", ref.Number, "the metadata stream is filtered, so the packet cannot be read without decoding it")
	}

	if part, ok := xmpField(s.Data, "pdfaid:part"); !ok || part != "2" {
		r.add("6.6.4", ref.Number, "the packet does not declare pdfaid:part 2")
	}
	if conf, ok := xmpField(s.Data, "pdfaid:conformance"); !ok || conf != "B" {
		r.add("6.6.4", ref.Number, "the packet does not declare pdfaid:conformance B")
	}
	return s.Data
}

// infoFields pairs an information dictionary key with the XMP property that
// states the same thing.
//
// In the order the information dictionary writes them, so a document failing
// several reads top to bottom.
var infoFields = []struct {
	key  object.Name
	elem string
	date bool
}{
	{"Title", "dc:title", false},
	{"Author", "dc:creator", false},
	{"Subject", "dc:description", false},
	{"Keywords", "pdf:Keywords", false},
	{"Creator", "xmp:CreatorTool", false},
	{"Producer", "pdf:Producer", false},
	{"CreationDate", "xmp:CreateDate", true},
	{"ModDate", "xmp:ModifyDate", true},
}

// info checks that the information dictionary and the XMP packet agree.
//
// This is the clause the whole metadata design exists to satisfy, and the
// reason both forms are generated from one value. Checking it here is not
// redundant with that: the structural guarantee lives in one function and this
// reads the bytes, so a second write path — or a change to either renderer —
// cannot make them diverge without failing a test.
func (r *report) info(doc *object.Document, packet []byte) {
	if doc.Info.IsZero() {
		r.add("6.6.2.3", 0, "the trailer names no /Info, which PDF/A requires alongside the XMP")
		return
	}
	d, ok := dictAt(doc, doc.Info)
	if !ok {
		r.add("6.6.2.3", doc.Info.Number, "/Info does not resolve to a dictionary")
		return
	}
	if packet == nil {
		return
	}

	for _, f := range infoFields {
		value, inInfo := d.Get(f.key)
		text, inXMP := xmpField(packet, f.elem)

		switch {
		case !inInfo && !inXMP:
			continue
		case inInfo && !inXMP:
			r.add("6.6.2.3", doc.Info.Number,
				"/"+string(f.key)+" is in the information dictionary and "+f.elem+" is absent from the XMP")
			continue
		case !inInfo && inXMP:
			r.add("6.6.2.3", doc.Info.Number,
				f.elem+" is in the XMP and /"+string(f.key)+" is absent from the information dictionary")
			continue
		}

		if f.date {
			if digitsOf(pdfText(value)) != digitsOf(text) {
				r.add("6.6.2.3", doc.Info.Number,
					"/"+string(f.key)+" and "+f.elem+" name different instants")
			}
			continue
		}

		// Compared in the information dictionary's own syntax rather than by
		// decoding it: the XMP value is re-encoded as a PDF string and the
		// bytes are compared. Equality then means the two forms describe the
		// same characters, without this check owning a string decoder whose
		// bugs would look like divergence.
		want := object.String(unescapeXML(text)).AppendPDF(nil)
		if !bytes.Equal(want, value.AppendPDF(nil)) {
			r.add("6.6.2.3", doc.Info.Number,
				"/"+string(f.key)+" and "+f.elem+" carry different values")
		}
	}
}

// identifier checks the trailer's file identifier.
func (r *report) identifier(doc *object.Document) {
	if len(doc.ID[0]) == 0 || len(doc.ID[1]) == 0 {
		r.add("6.1.3", 0, "the trailer has no /ID, or one half of it is empty")
	}
	if _, encrypted := doc.Trailer.Get("Encrypt"); encrypted {
		r.add("6.1.3", 0, "the trailer names an /Encrypt dictionary")
	}
}

// objects walks every object in the document and checks the ones whose type
// carries a requirement.
//
// Every object rather than every reachable object. The stricter reading is also
// the cheaper one, and an unreferenced nonconformant object still occupies the
// file a validator reads.
func (r *report) objects(doc *object.Document) {
	for i := 1; i <= doc.Len(); i++ {
		ref := object.Ref{Number: i}
		o, ok := doc.Object(ref)
		if !ok {
			continue
		}

		d, isDict := o.(object.Dict)
		if s, isStream := o.(object.Stream); isStream {
			d, isDict = s.Dict, true
		}
		if !isDict {
			continue
		}

		kind, _ := nameOf(d, "Type")
		switch kind {
		case "Font":
			r.font(doc, i, d)
		case "Page":
			r.page(i, d)
		case "Pages":
			r.pagesNode(i, d)
		case "XObject":
			if sub, _ := nameOf(d, "Subtype"); sub == "Image" {
				r.image(i, d)
			}
		}
	}
}

// font checks that a descendant font's program is embedded.
//
// Only the descendant, because that is where the descriptor lives: a Type0
// font names its program through /DescendantFonts and carries no descriptor of
// its own, so checking it would report a missing key that is correctly missing.
func (r *report) font(doc *object.Document, num int, d object.Dict) {
	sub, _ := nameOf(d, "Subtype")
	if sub != "CIDFontType0" && sub != "CIDFontType2" {
		return
	}

	value, has := d.Get("FontDescriptor")
	if !has {
		r.add("6.2.11.4.1", num, "the font has no /FontDescriptor, so nothing states whether its program is embedded")
		return
	}
	ref, ok := value.(object.Ref)
	if !ok {
		r.add("6.2.11.4.1", num, "/FontDescriptor is not an indirect reference")
		return
	}
	desc, ok := dictAt(doc, ref)
	if !ok {
		r.add("6.2.11.4.1", ref.Number, "/FontDescriptor does not resolve to a dictionary")
		return
	}

	for _, key := range []object.Name{"FontFile", "FontFile2", "FontFile3"} {
		program, has := desc.Get(key)
		if !has {
			continue
		}
		file, ok := program.(object.Ref)
		if !ok {
			r.add("6.2.11.4.1", ref.Number, "/"+string(key)+" is not an indirect reference to a stream")
			return
		}
		if _, ok := streamAt(doc, file); !ok {
			r.add("6.2.11.4.1", file.Number, "/"+string(key)+" does not resolve to a stream")
		}
		return
	}
	r.add("6.2.11.4.1", ref.Number,
		"the font descriptor embeds no program, so the document depends on a font installed on the reader's machine")
}

// page checks a leaf of the page tree.
func (r *report) page(num int, d object.Dict) {
	if _, has := d.Get("Resources"); !has {
		r.add("6.2.2", num,
			"the page has no /Resources of its own; PDF/A requires the association to be explicit rather than inherited")
	}
	if _, has := d.Get("MediaBox"); !has {
		// Legal to inherit under ISO 32000, and the page tree does hoist it, so
		// this fires only when the hoisted copy is missing too — which is why
		// the check is on the pair rather than on the page alone.
		if _, hoisted := d.Get("Parent"); !hoisted {
			r.add("6.1.2", num, "the page has neither a /MediaBox nor a parent to inherit one from")
		}
	}
}

// pagesNode checks an interior node of the page tree.
//
// The rule it enforces is the one that is easiest to break by making the file
// smaller: /Resources is inheritable under ISO 32000-1, and lifting one copy
// onto the tree's root removes a dictionary from every page. ISO 19005-2 clause
// 6.2.2 requires the association to be explicit, and veraPDF rejects the
// inherited form.
func (r *report) pagesNode(num int, d object.Dict) {
	if _, has := d.Get("Resources"); has {
		r.add("6.2.2", num,
			"a page tree node carries /Resources, which makes the pages below it depend on inheritance")
	}
}

// image checks an image XObject.
func (r *report) image(num int, d object.Dict) {
	if b, ok := d.Get("Interpolate"); ok {
		if flag, isBool := b.(object.Bool); isBool && bool(flag) {
			r.add("6.2.8.3", num, "the image sets /Interpolate true, which PDF/A forbids")
		}
	}

	_, isMask := d.Get("ImageMask")
	if _, has := d.Get("ColorSpace"); !has && !isMask {
		r.add("6.2.4.1", num, "the image names no /ColorSpace, so its samples have no defined interpretation")
	}

	for _, f := range filtersOf(d) {
		if f == "LZWDecode" {
			r.add("6.1.10", num, "the image uses /LZWDecode, which PDF/A forbids")
		}
	}
}

// filtersOf returns a stream dictionary's filters, which may be one name or an
// array of them.
func filtersOf(d object.Dict) []object.Name {
	value, has := d.Get("Filter")
	if !has {
		return nil
	}
	switch f := value.(type) {
	case object.Name:
		return []object.Name{f}
	case object.Array:
		out := make([]object.Name, 0, len(f))
		for _, e := range f {
			if n, ok := e.(object.Name); ok {
				out = append(out, n)
			}
		}
		return out
	}
	return nil
}

// resolve follows an indirect reference once. A direct object is returned as it
// is.
func resolve(doc *object.Document, o object.Object) object.Object {
	ref, ok := o.(object.Ref)
	if !ok {
		return o
	}
	target, ok := doc.Object(ref)
	if !ok {
		return nil
	}
	return target
}

// dictAt resolves a reference to a dictionary.
func dictAt(doc *object.Document, ref object.Ref) (object.Dict, bool) {
	o, ok := doc.Object(ref)
	if !ok {
		return object.Dict{}, false
	}
	if s, isStream := o.(object.Stream); isStream {
		return s.Dict, true
	}
	d, isDict := o.(object.Dict)
	return d, isDict
}

// streamAt resolves a reference to a stream.
func streamAt(doc *object.Document, ref object.Ref) (object.Stream, bool) {
	o, ok := doc.Object(ref)
	if !ok {
		return object.Stream{}, false
	}
	s, isStream := o.(object.Stream)
	return s, isStream
}

// nameOf reads a dictionary entry that should be a name.
func nameOf(d object.Dict, key object.Name) (object.Name, bool) {
	value, has := d.Get(key)
	if !has {
		return "", false
	}
	n, ok := value.(object.Name)
	return n, ok
}

// pdfText returns the characters of a PDF string object, or the empty string
// for anything else.
//
// Used only for dates, whose syntax is ASCII with no escapes, so no decoding is
// involved. A text field goes the other way — the XMP value is encoded and the
// bytes compared — because that direction needs no decoder at all.
func pdfText(o object.Object) string {
	switch s := o.(type) {
	case object.String:
		return string(s)
	case object.HexString:
		return string(s)
	}
	return ""
}

// digitsOf keeps only the decimal digits of a string.
//
// The two date syntaxes differ in every separator and agree in every digit —
// D:20260101120000+00'00' against 2026-01-01T12:00:00+00:00 — so comparing the
// digits compares the instant without a parser for either form. It would also
// equate two dates that differ only in punctuation, which is a difference
// neither syntax can express.
func digitsOf(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// xmpField returns the text of an XMP property.
//
// A scanner rather than an XML parser, because what it is checking is a packet
// this project wrote in a prescribed shape, and a parser would normalise away
// the very differences worth catching. It descends through one rdf:li, which is
// how the Dublin Core properties wrap their value.
func xmpField(packet []byte, elem string) (string, bool) {
	inner, ok := elementText(packet, elem)
	if !ok {
		return "", false
	}
	if li, ok := elementText(inner, "rdf:li"); ok {
		inner = li
	}
	return strings.TrimSpace(string(inner)), true
}

// elementText returns the bytes between an element's tags.
func elementText(packet []byte, elem string) ([]byte, bool) {
	start := bytes.Index(packet, []byte("<"+elem))
	if start < 0 {
		return nil, false
	}
	// Past the attributes, if the tag carries any.
	open := bytes.IndexByte(packet[start:], '>')
	if open < 0 {
		return nil, false
	}
	from := start + open + 1

	end := bytes.Index(packet[from:], []byte("</"+elem+">"))
	if end < 0 {
		return nil, false
	}
	return packet[from : from+end], true
}

// unescapeXML reverses the five escapes the packet writer applies.
func unescapeXML(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	r := strings.NewReplacer(
		"&lt;", "<",
		"&gt;", ">",
		"&apos;", "'",
		"&quot;", `"`,
		"&amp;", "&",
	)
	// A Replacer walks the input once and never rescans what it wrote, so an
	// escaped ampersand in the source text survives as one: "&amp;lt;" becomes
	// "&lt;" and stops there. That is the property this needs and it is a
	// property of the Replacer rather than of the order below.
	return r.Replace(s)
}
