package pdfa

import (
	"errors"
	"strings"
	"testing"
	"time"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/object"
	"github.com/frankbardon/vellum/pdf/xmp"
)

// conformingMeta is the metadata the fixture carries.
//
// Every field the agreement check compares is set, so a case that removes one
// from either side is testing the check rather than testing an absence that was
// already there.
var conformingMeta = xmp.Metadata{
	Title:    "A Title",
	Author:   "An Author",
	Subject:  "A subject line",
	Keywords: "one, two",
	Creator:  "The Consumer",
	Producer: "Vellum",
	Date:     time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
}

// refs names the objects a case reaches for.
type refs struct {
	catalogue  object.Ref
	info       object.Ref
	metadata   object.Ref
	page       object.Ref
	pagesRoot  object.Ref
	descendant object.Ref
	descriptor object.Ref
	image      object.Ref
}

// fixture builds a document that passes.
//
// Assembled the way the writer assembles one rather than reduced to the
// minimum, because a fixture that omits the parts the checks look at would pass
// every check by having nothing to check.
func fixture(t *testing.T) (*object.Document, refs) {
	t.Helper()

	var doc object.Document
	doc.Uncompressed = true

	program := doc.AddRawStream(object.NewDict("Length1", object.Int(4)), []byte("FONT"))
	descriptor := doc.Add(object.NewDict(
		"Type", object.Name("FontDescriptor"),
		"FontName", object.Name("ABCDEF+Test"),
		"FontFile2", program,
	))
	descendant := doc.Add(object.NewDict(
		"Type", object.Name("Font"),
		"Subtype", object.Name("CIDFontType2"),
		"BaseFont", object.Name("ABCDEF+Test"),
		"FontDescriptor", descriptor,
	))
	doc.Add(object.NewDict(
		"Type", object.Name("Font"),
		"Subtype", object.Name("Type0"),
		"BaseFont", object.Name("ABCDEF+Test"),
		"DescendantFonts", object.Array{descendant},
	))

	image := doc.AddRawStream(object.NewDict(
		"Type", object.Name("XObject"),
		"Subtype", object.Name("Image"),
		"Width", object.Int(1),
		"Height", object.Int(1),
		"ColorSpace", object.Name("DeviceRGB"),
		"BitsPerComponent", object.Int(8),
	), []byte{0, 0, 0})

	contents, err := doc.AddStream(object.Dict{}, []byte("BT ET\n"))
	if err != nil {
		t.Fatalf("adding the content stream: %v", err)
	}

	page := object.NewDict(
		"MediaBox", object.Array{object.Int(0), object.Int(0), object.Int(612), object.Int(792)},
		"Resources", object.NewDict("ProcSet", object.Array{object.Name("PDF")}),
		"Contents", contents,
	)
	pagesRoot, err := object.BuildPageTree(&doc, []object.Dict{page}, object.PageTreeOptions{})
	if err != nil {
		t.Fatalf("building the page tree: %v", err)
	}

	metadata := doc.AddRawStream(object.NewDict(
		"Type", object.Name("Metadata"),
		"Subtype", object.Name("XML"),
	), conformingMeta.Packet())

	intent := AddSRGBOutputIntent(&doc)

	doc.Root = doc.Add(object.NewDict(
		"Type", object.Name("Catalog"),
		"Pages", pagesRoot,
		"Metadata", metadata,
		"OutputIntents", object.Array{intent},
	))
	doc.Info = doc.Add(conformingMeta.InfoEntries())
	doc.ID = [2][]byte{{0xde, 0xad}, {0xde, 0xad}}

	return &doc, refs{
		catalogue:  doc.Root,
		info:       doc.Info,
		metadata:   metadata,
		page:       findType(t, &doc, "Page"),
		pagesRoot:  pagesRoot,
		descendant: descendant,
		descriptor: descriptor,
		image:      image,
	}
}

func TestPreflight_TheFixtureConforms(t *testing.T) {
	doc, _ := fixture(t)
	if err := Preflight(doc); err != nil {
		t.Fatalf("the fixture must pass, or every case below proves nothing: %v", err)
	}
}

// TestPreflight_Violations drives every check by breaking the document in the
// one way that check exists to catch.
//
// A check nobody has watched fail is a check that may not be looking at
// anything, which this project has now been bitten by twice. Each case names
// the clause it expects and a fragment of the reason, so a case that starts
// passing for a different reason than it was written for fails.
func TestPreflight_Violations(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, doc *object.Document, r refs)
		clause string
		reason string
	}{
		{
			name: "a catalogue that is not one",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				setName(t, doc, r.catalogue, "Type", "Pages")
			},
			clause: "6.1.2",
			reason: "/Type is not /Catalog",
		},
		{
			name: "no output intent",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				deleteKey(t, doc, r.catalogue, "OutputIntents")
			},
			clause: "6.2.2",
			reason: "no /OutputIntents",
		},
		{
			name: "two output intents",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				d := dictOf(t, doc, r.catalogue)
				value, _ := d.Get("OutputIntents")
				arr := value.(object.Array)
				d.Set("OutputIntents", append(object.Array{}, arr[0], arr[0]))
				fill(t, doc, r.catalogue, d)
			},
			clause: "6.2.2",
			reason: "exactly one intent",
		},
		{
			name: "an output intent whose profile is only named",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				d := dictOf(t, doc, r.catalogue)
				value, _ := d.Get("OutputIntents")
				intent := value.(object.Array)[0].(object.Dict).Clone()
				intent.Delete("DestOutputProfile")
				d.Set("OutputIntents", object.Array{intent})
				fill(t, doc, r.catalogue, d)
			},
			clause: "6.2.2",
			reason: "referenced rather than embedded",
		},
		{
			name: "no XMP at all",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				deleteKey(t, doc, r.catalogue, "Metadata")
			},
			clause: "6.6.2.1",
			reason: "no /Metadata",
		},
		{
			name: "a compressed metadata packet",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				s := streamOf(t, doc, r.metadata)
				s.Dict = s.Dict.Clone()
				s.Dict.Set("Filter", object.Name("FlateDecode"))
				fill(t, doc, r.metadata, s)
			},
			clause: "6.6.2.1",
			reason: "is filtered",
		},
		{
			name: "a packet claiming PDF/A-1",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				s := streamOf(t, doc, r.metadata)
				s.Data = []byte(strings.Replace(string(s.Data),
					"<pdfaid:part>2</pdfaid:part>", "<pdfaid:part>1</pdfaid:part>", 1))
				fill(t, doc, r.metadata, s)
			},
			clause: "6.6.4",
			reason: "pdfaid:part 2",
		},
		{
			name: "a packet claiming level A",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				s := streamOf(t, doc, r.metadata)
				s.Data = []byte(strings.Replace(string(s.Data),
					"<pdfaid:conformance>B<", "<pdfaid:conformance>A<", 1))
				fill(t, doc, r.metadata, s)
			},
			clause: "6.6.4",
			reason: "pdfaid:conformance B",
		},
		{
			name: "dates that disagree",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				d := dictOf(t, doc, r.info)
				d.Set("CreationDate", object.String("D:20250101120000+00'00'"))
				fill(t, doc, r.info, d)
			},
			clause: "6.6.2.3",
			reason: "different instants",
		},
		{
			name: "a title that disagrees",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				d := dictOf(t, doc, r.info)
				d.Set("Title", object.String("Another Title"))
				fill(t, doc, r.info, d)
			},
			clause: "6.6.2.3",
			reason: "different values",
		},
		{
			name: "a title the XMP does not carry",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				s := streamOf(t, doc, r.metadata)
				s.Data = []byte(cut(string(s.Data), "<dc:title>", "</dc:title>"))
				fill(t, doc, r.metadata, s)
			},
			clause: "6.6.2.3",
			reason: "absent from the XMP",
		},
		{
			name: "a producer only the XMP carries",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				deleteKey(t, doc, r.info, "Producer")
			},
			clause: "6.6.2.3",
			reason: "absent from the information dictionary",
		},
		{
			name: "no file identifier",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				doc.ID = [2][]byte{}
			},
			clause: "6.1.3",
			reason: "no /ID",
		},
		{
			name: "an encrypted document",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				doc.Trailer.Set("Encrypt", object.NewDict("Filter", object.Name("Standard")))
			},
			clause: "6.1.3",
			reason: "/Encrypt",
		},
		{
			name: "a font that is not embedded",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				deleteKey(t, doc, r.descriptor, "FontFile2")
			},
			clause: "6.2.11.4.1",
			reason: "embeds no program",
		},
		{
			name: "a font with no descriptor",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				deleteKey(t, doc, r.descendant, "FontDescriptor")
			},
			clause: "6.2.11.4.1",
			reason: "no /FontDescriptor",
		},
		{
			name: "resources hoisted onto the page tree root",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				page := dictOf(t, doc, r.page)
				resources, _ := page.Get("Resources")
				page.Delete("Resources")
				fill(t, doc, r.page, page)

				root := dictOf(t, doc, r.pagesRoot)
				root.Set("Resources", resources)
				fill(t, doc, r.pagesRoot, root)
			},
			clause: "6.2.2",
			reason: "depend on inheritance",
		},
		{
			name: "a page with no resources at all",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				deleteKey(t, doc, r.page, "Resources")
			},
			clause: "6.2.2",
			reason: "no /Resources of its own",
		},
		{
			name: "an interpolated image",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				s := streamOf(t, doc, r.image)
				s.Dict = s.Dict.Clone()
				s.Dict.Set("Interpolate", object.Bool(true))
				fill(t, doc, r.image, s)
			},
			clause: "6.2.8.3",
			reason: "/Interpolate true",
		},
		{
			name: "an image with no colour space",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				s := streamOf(t, doc, r.image)
				s.Dict = s.Dict.Clone()
				s.Dict.Delete("ColorSpace")
				fill(t, doc, r.image, s)
			},
			clause: "6.2.4.1",
			reason: "no /ColorSpace",
		},
		{
			name: "an LZW compressed image",
			mutate: func(t *testing.T, doc *object.Document, r refs) {
				s := streamOf(t, doc, r.image)
				s.Dict = s.Dict.Clone()
				s.Dict.Set("Filter", object.Array{object.Name("LZWDecode")})
				fill(t, doc, r.image, s)
			},
			clause: "6.1.10",
			reason: "/LZWDecode",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, r := fixture(t)
			tc.mutate(t, doc, r)

			found := violations(t, doc)
			for _, v := range found {
				if strings.HasPrefix(v, tc.clause+":") && strings.Contains(v, tc.reason) {
					return
				}
			}
			t.Fatalf("expected a %s violation mentioning %q; got:\n  %s",
				tc.clause, tc.reason, strings.Join(found, "\n  "))
		})
	}
}

// TestPreflight_ReportsEveryViolationAtOnce checks the report does not stop at
// the first failure.
//
// These arrive in groups — one broken write path fails the same way on every
// page — and a preflight that named one per run would be several builds of the
// same debugging.
func TestPreflight_ReportsEveryViolationAtOnce(t *testing.T) {
	doc, r := fixture(t)
	deleteKey(t, doc, r.descriptor, "FontFile2")
	deleteKey(t, doc, r.page, "Resources")
	doc.ID = [2][]byte{}

	if got := len(violations(t, doc)); got != 3 {
		t.Fatalf("three independent faults must produce three violations, got %d:\n  %s",
			got, strings.Join(violations(t, doc), "\n  "))
	}
}

// TestPreflight_TheErrorIsCoded pins the code, because a caller distinguishing
// a conformance failure from a write failure does it by code.
func TestPreflight_TheErrorIsCoded(t *testing.T) {
	doc, r := fixture(t)
	deleteKey(t, doc, r.catalogue, "OutputIntents")

	err := Preflight(doc)
	if !verr.HasCode(err, verr.VELLUM_PDFA_NONCONFORMANT) {
		t.Fatalf("want VELLUM_PDFA_NONCONFORMANT, got %v", err)
	}
}

// TestPreflight_XMPFieldReadsThroughEscapes checks the packet scanner against
// the escaping the packet writer applies.
//
// The pair has to agree exactly: a value the writer escapes and the reader does
// not unescape looks like divergence, which would make the agreement check fire
// on a document that is correct.
func TestPreflight_XMPFieldReadsThroughEscapes(t *testing.T) {
	meta := xmp.Metadata{
		Title:    `Sales & Marketing <"2026">`,
		Producer: "Vellum",
		Date:     conformingMeta.Date,
	}

	got, ok := xmpField(meta.Packet(), "dc:title")
	if !ok {
		t.Fatal("dc:title was not found in the packet")
	}
	if unescapeXML(got) != meta.Title {
		t.Fatalf("round trip through the packet changed the title:\n  want %q\n  got  %q",
			meta.Title, unescapeXML(got))
	}
}

// TestPreflight_EscapedMetadataAgrees is the same property stated where it
// matters: a title full of markup characters must not be reported as a
// divergence between the two forms.
func TestPreflight_EscapedMetadataAgrees(t *testing.T) {
	doc, r := fixture(t)

	meta := conformingMeta
	meta.Title = `Sales & Marketing <"2026">`

	s := streamOf(t, doc, r.metadata)
	s.Data = meta.Packet()
	fill(t, doc, r.metadata, s)
	fill(t, doc, r.info, meta.InfoEntries())

	if err := Preflight(doc); err != nil {
		t.Fatalf("an escaped title is not a divergence: %v", err)
	}
}

// violations runs the preflight and returns the rendered violations.
func violations(t *testing.T, doc *object.Document) []string {
	t.Helper()

	err := Preflight(doc)
	if err == nil {
		return nil
	}

	var coded *verr.CodedError
	if !errors.As(err, &coded) {
		t.Fatalf("the preflight must return a coded error, got %T", err)
	}
	value, ok := coded.Detail("violations")
	if !ok {
		t.Fatal("the error carries no violations detail")
	}
	list, ok := value.([]any)
	if !ok {
		t.Fatalf("violations must be a list, got %T", value)
	}

	out := make([]string, len(list))
	for i, v := range list {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("violation %d is not a string, got %T", i, v)
		}
		out[i] = s
	}
	return out
}

// findType returns the first object whose /Type is the given name.
func findType(t *testing.T, doc *object.Document, want object.Name) object.Ref {
	t.Helper()

	for i := 1; i <= doc.Len(); i++ {
		ref := object.Ref{Number: i}
		o, ok := doc.Object(ref)
		if !ok {
			continue
		}
		d, isDict := o.(object.Dict)
		if !isDict {
			continue
		}
		if got, _ := nameOf(d, "Type"); got == want {
			return ref
		}
	}
	t.Fatalf("the fixture holds no object of /Type /%s", want)
	return object.Ref{}
}

// dictOf returns an independent copy of an object's dictionary.
func dictOf(t *testing.T, doc *object.Document, ref object.Ref) object.Dict {
	t.Helper()

	d, ok := dictAt(doc, ref)
	if !ok {
		t.Fatalf("object %d is not a dictionary", ref.Number)
	}
	// Cloned, because a Dict shares its entries with the one it was copied
	// from: mutating the value the document holds would change the document
	// before Fill was called, and a case that removed a key would appear to
	// work while testing nothing.
	return d.Clone()
}

// streamOf returns a stream with an independent dictionary.
func streamOf(t *testing.T, doc *object.Document, ref object.Ref) object.Stream {
	t.Helper()

	s, ok := streamAt(doc, ref)
	if !ok {
		t.Fatalf("object %d is not a stream", ref.Number)
	}
	return s
}

// fill replaces an object.
func fill(t *testing.T, doc *object.Document, ref object.Ref, o object.Object) {
	t.Helper()

	if err := doc.Fill(ref, o); err != nil {
		t.Fatalf("replacing object %d: %v", ref.Number, err)
	}
}

// setName replaces a name-valued entry.
func setName(t *testing.T, doc *object.Document, ref object.Ref, key, value object.Name) {
	t.Helper()

	d := dictOf(t, doc, ref)
	d.Set(key, value)
	fill(t, doc, ref, d)
}

// deleteKey removes an entry, from a dictionary or from a stream's dictionary.
func deleteKey(t *testing.T, doc *object.Document, ref object.Ref, key object.Name) {
	t.Helper()

	o, ok := doc.Object(ref)
	if !ok {
		t.Fatalf("object %d does not exist", ref.Number)
	}
	if s, isStream := o.(object.Stream); isStream {
		s.Dict = s.Dict.Clone()
		s.Dict.Delete(key)
		fill(t, doc, ref, s)
		return
	}

	d := dictOf(t, doc, ref)
	d.Delete(key)
	fill(t, doc, ref, d)
}

// cut removes a region of the packet, delimiters included.
func cut(s, open, close string) string {
	from := strings.Index(s, open)
	if from < 0 {
		return s
	}
	to := strings.Index(s[from:], close)
	if to < 0 {
		return s
	}
	return s[:from] + s[from+to+len(close):]
}
