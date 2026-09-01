package object_test

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/object"
)

func TestReal_Formatting(t *testing.T) {
	cases := []struct {
		name string
		in   object.Real
		want string
	}{
		{"zero", 0, "0"},
		{"whole", object.Points(12), "12"},
		{"negative whole", object.Points(-12), "-12"},
		{"one thousandth", object.Thousandths(1), "0.001"},
		{"trailing zeros trimmed", object.Thousandths(1500), "1.5"},
		{"below one keeps the leading zero", object.Thousandths(500), "0.5"},
		{"negative below one keeps the sign", object.Thousandths(-500), "-0.5"},
		{"negative fraction", object.Thousandths(-1500), "-1.5"},
		{"three places", object.Thousandths(1234), "1.234"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.in.String(); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestReal_NegativeBelowOneKeepsItsSign pins the arm most easily lost.
//
// The sign of a fixed-point value below one lives only in the fractional part,
// because the integer part is a signed zero that formats as "0". Dropping it
// produces a coordinate mirrored about the origin, which is a defect that looks
// like a layout bug rather than a formatting one.
func TestReal_NegativeBelowOneKeepsItsSign(t *testing.T) {
	for m := int64(-999); m < 0; m++ {
		got := object.Thousandths(m).String()
		if !strings.HasPrefix(got, "-0.") {
			t.Fatalf("Thousandths(%d) = %q, which has lost the sign", m, got)
		}
	}
}

func TestRatio_RoundsHalfAwayFromZero(t *testing.T) {
	cases := []struct {
		num, den int64
		want     string
	}{
		{1, 2, "0.5"},
		{-1, 2, "-0.5"},
		{1, 3, "0.333"},
		{2, 3, "0.667"},
		{-2, 3, "-0.667"},
		{1, 0, "0"},
	}
	for _, c := range cases {
		if got := object.Ratio(c.num, c.den).String(); got != c.want {
			t.Errorf("Ratio(%d, %d) = %q, want %q", c.num, c.den, got, c.want)
		}
	}
}

func TestName_EscapesDelimiters(t *testing.T) {
	cases := []struct {
		in   object.Name
		want string
	}{
		{"Type", "/Type"},
		{"ABCDEF+Go-Regular", "/ABCDEF+Go-Regular"},
		{"a b", "/a#20b"},
		{"a/b", "/a#2Fb"},
		{"a#b", "/a#23b"},
		{"a(b)", "/a#28b#29"},
	}
	for _, c := range cases {
		if got := string(c.in.AppendPDF(nil)); got != c.want {
			t.Errorf("Name(%q) = %q, want %q", string(c.in), got, c.want)
		}
	}
}

func TestString_Escaping(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"plain", "(plain)"},
		{"a(b", `(a\(b)`},
		{"a)b", `(a\)b)`},
		{`a\b`, `(a\\b)`},
		{"a\nb", `(a\nb)`},
		{"\x01", `(\001)`},
		{"caf\xc3\xa9", `(caf\303\251)`},
	}
	for _, c := range cases {
		if got := string(object.String(c.in).AppendPDF(nil)); got != c.want {
			t.Errorf("String(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHexString(t *testing.T) {
	got := string(object.HexString([]byte{0x00, 0x9f, 0xff}).AppendPDF(nil))
	if want := "<009FFF>"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestDict_KeepsInsertionOrder is the determinism property the type exists for.
func TestDict_KeepsInsertionOrder(t *testing.T) {
	d := object.NewDict(
		"Type", object.Name("Page"),
		"Parent", object.Ref{Number: 3},
		"MediaBox", object.Array{object.Int(0), object.Int(0), object.Int(612), object.Int(792)},
	)
	got := string(d.AppendPDF(nil))
	want := "<</Type /Page/Parent 3 0 R/MediaBox [0 0 612 792]>>"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// TestDict_SetKeepsPosition pins that adjusting a value does not reorder.
//
// Without it, a dictionary built by one path and then corrected by another
// produces different bytes from one built correctly in the first place, and the
// difference appears only in whichever documents happen to take the second
// path.
func TestDict_SetKeepsPosition(t *testing.T) {
	a := object.NewDict("A", object.Int(1), "B", object.Int(2), "C", object.Int(3))
	a.Set("B", object.Int(9))

	b := object.NewDict("A", object.Int(1), "B", object.Int(9), "C", object.Int(3))

	if string(a.AppendPDF(nil)) != string(b.AppendPDF(nil)) {
		t.Errorf("replacing a value reordered the dictionary:\n got %s\nwant %s",
			a.AppendPDF(nil), b.AppendPDF(nil))
	}
}

func TestDict_SetIf(t *testing.T) {
	d := object.NewDict("A", object.Int(1))
	d.SetIf(false, "B", object.Int(2))
	d.SetIf(true, "C", object.Int(3))

	if _, ok := d.Get("B"); ok {
		t.Error("SetIf(false) stored the entry")
	}
	if _, ok := d.Get("C"); !ok {
		t.Error("SetIf(true) did not store the entry")
	}
}

// TestStream_LengthComesFromTheData pins that a caller cannot declare a length
// the stream does not have.
func TestStream_LengthComesFromTheData(t *testing.T) {
	s := object.Stream{
		Dict: object.NewDict("Length", object.Int(9999)),
		Data: []byte("hello"),
	}
	got := string(s.AppendPDF(nil))
	if !strings.Contains(got, "/Length 5") {
		t.Errorf("the declared length was trusted over the data:\n%s", got)
	}
	if strings.Contains(got, "9999") {
		t.Errorf("the caller's wrong length survived:\n%s", got)
	}
}

func TestDeflate_RoundTrips(t *testing.T) {
	raw := bytes.Repeat([]byte("BT /F1 12 Tf (hello) Tj ET\n"), 40)
	s, err := object.Deflate(object.NewDict("Type", object.Name("XObject")), raw)
	if err != nil {
		t.Fatalf("Deflate: %v", err)
	}
	if len(s.Data) >= len(raw) {
		t.Errorf("compressed size %d is not smaller than %d", len(s.Data), len(raw))
	}
	f, ok := s.Dict.Get("Filter")
	if !ok || string(f.AppendPDF(nil)) != "/FlateDecode" {
		t.Errorf("Filter is %v, want /FlateDecode", f)
	}
	// zlib, not raw deflate: the two-byte header is what readers expect.
	if len(s.Data) < 2 || s.Data[0] != 0x78 {
		t.Errorf("the stream does not begin with a zlib header: % x", s.Data[:min(4, len(s.Data))])
	}
}

func TestDocument_RequiresACatalogue(t *testing.T) {
	var d object.Document
	d.Add(object.NewDict("Type", object.Name("Pages")))

	err := d.Write(&bytes.Buffer{})
	if !verr.HasCode(err, verr.VELLUM_PDF_OBJECT_UNRESOLVED) {
		t.Fatalf("got %v, want VELLUM_PDF_OBJECT_UNRESOLVED", err)
	}
}

// TestDocument_ReservedObjectsMustBeFilled is the check that catches a page
// tree assembled in the wrong order.
func TestDocument_ReservedObjectsMustBeFilled(t *testing.T) {
	var d object.Document
	d.Root = d.Add(object.NewDict("Type", object.Name("Catalog")))
	d.Reserve()
	d.Reserve()

	err := d.Write(&bytes.Buffer{})
	if !verr.HasCode(err, verr.VELLUM_PDF_OBJECT_UNRESOLVED) {
		t.Fatalf("got %v, want VELLUM_PDF_OBJECT_UNRESOLVED", err)
	}
	var ce *verr.CodedError
	if !errorsAs(err, &ce) {
		t.Fatalf("the error carries no details")
	}
	nums, ok := ce.Detail("object_numbers")
	if !ok {
		t.Fatal("the error does not name which objects were unfilled")
	}
	if got := len(nums.([]any)); got != 2 {
		t.Errorf("named %d unfilled objects, want 2", got)
	}
}

func TestDocument_FillRejectsAnUnknownReference(t *testing.T) {
	var d object.Document
	err := d.Fill(object.Ref{Number: 7}, object.Null{})
	if !verr.HasCode(err, verr.VELLUM_PDF_OBJECT_UNRESOLVED) {
		t.Fatalf("got %v, want VELLUM_PDF_OBJECT_UNRESOLVED", err)
	}
}

var objHeaderRe = regexp.MustCompile(`^(\d+) 0 obj`)

// TestDocument_XrefOffsetsPointAtTheirObjects is the structural check on the
// whole file.
//
// The cross-reference table is the one part of a PDF nothing else validates:
// every tolerant reader rebuilds it by scanning when it does not match, so an
// off-by-one survives every casual check and fails in a strict reader. Reading
// each recorded offset back and confirming the object it lands on is the object
// it claims is the assertion that actually establishes the file is well formed.
func TestDocument_XrefOffsetsPointAtTheirObjects(t *testing.T) {
	raw := buildSampleDocument(t)

	xrefAt := bytes.LastIndex(raw, []byte("\nstartxref\n"))
	if xrefAt < 0 {
		t.Fatal("no startxref")
	}
	tail := string(raw[xrefAt+len("\nstartxref\n"):])
	offStr, _, _ := strings.Cut(tail, "\n")
	xrefOffset, err := strconv.Atoi(strings.TrimSpace(offStr))
	if err != nil {
		t.Fatalf("startxref is not a number: %q", offStr)
	}
	if xrefOffset <= 0 || xrefOffset >= len(raw) {
		t.Fatalf("startxref %d is outside the file of %d bytes", xrefOffset, len(raw))
	}
	if !bytes.HasPrefix(raw[xrefOffset:], []byte("xref\n")) {
		t.Fatalf("startxref does not point at the table: %q", raw[xrefOffset:min(xrefOffset+16, len(raw))])
	}

	// "xref\n0 N\n" then twenty bytes per entry.
	body := raw[xrefOffset+len("xref\n"):]
	countLine, rest, _ := bytes.Cut(body, []byte("\n"))
	var first, count int
	if _, err := fmtSscan(string(countLine), &first, &count); err != nil {
		t.Fatalf("the subsection header %q does not parse: %v", countLine, err)
	}
	if first != 0 {
		t.Fatalf("the subsection starts at %d, want 0", first)
	}

	for i := range count {
		entry := rest[i*20 : (i+1)*20]
		if len(entry) != 20 {
			t.Fatalf("entry %d is %d bytes, want exactly 20", i, len(entry))
		}
		if i == 0 {
			if string(entry) != "0000000000 65535 f\r\n" {
				t.Errorf("the free-list head is %q", entry)
			}
			continue
		}
		off, err := strconv.Atoi(string(entry[:10]))
		if err != nil {
			t.Fatalf("entry %d offset %q does not parse: %v", i, entry[:10], err)
		}
		if off <= 0 || off >= len(raw) {
			t.Fatalf("entry %d offset %d is outside the file", i, off)
		}
		m := objHeaderRe.FindSubmatch(raw[off:])
		if m == nil {
			t.Fatalf("entry %d offset %d does not land on an object header: %q",
				i, off, raw[off:min(off+24, len(raw))])
		}
		if got := string(m[1]); got != strconv.Itoa(i) {
			t.Errorf("entry %d points at object %s", i, got)
		}
	}
}

func TestDocument_HeaderAndTrailer(t *testing.T) {
	raw := buildSampleDocument(t)

	if !bytes.HasPrefix(raw, []byte("%PDF-1.7\n%")) {
		t.Errorf("the header is wrong: %q", raw[:min(16, len(raw))])
	}
	// The binary marker must contain bytes above 127, or a transport may treat
	// the file as text and rewrite its line endings.
	marker := raw[9:14]
	high := 0
	for _, c := range marker[1:] {
		if c > 127 {
			high++
		}
	}
	if high < 4 {
		t.Errorf("the binary marker has %d high bytes, want 4: % x", high, marker)
	}
	if !bytes.HasSuffix(raw, []byte("\n%%EOF\n")) {
		t.Errorf("the file does not end with %%%%EOF")
	}
	if !bytes.Contains(raw, []byte("/ID [<")) {
		t.Errorf("the trailer carries no /ID")
	}
}

// TestDocument_IsDeterministic emits the same document repeatedly.
func TestDocument_IsDeterministic(t *testing.T) {
	first := buildSampleDocument(t)
	for range 50 {
		if got := buildSampleDocument(t); !bytes.Equal(first, got) {
			t.Fatal("two identical builds produced different bytes")
		}
	}
}

// buildSampleDocument assembles a minimal page tree, exercising the forward
// reference that makes Reserve necessary.
func buildSampleDocument(t *testing.T) []byte {
	t.Helper()

	var d object.Document
	pages := d.Reserve()

	content, err := d.AddStream(object.Dict{}, []byte("BT /F1 12 Tf 72 720 Td (hello) Tj ET\n"))
	if err != nil {
		t.Fatalf("AddStream: %v", err)
	}

	page := d.Add(object.NewDict(
		"Type", object.Name("Page"),
		"Parent", pages,
		"MediaBox", object.Array{object.Int(0), object.Int(0), object.Int(612), object.Int(792)},
		"Contents", content,
	))
	if err := d.Fill(pages, object.NewDict(
		"Type", object.Name("Pages"),
		"Kids", object.Array{page},
		"Count", object.Int(1),
	)); err != nil {
		t.Fatalf("Fill: %v", err)
	}

	d.Root = d.Add(object.NewDict("Type", object.Name("Catalog"), "Pages", pages))
	d.Info = d.Add(object.NewDict("Producer", object.String("Vellum")))
	d.ID = [2][]byte{
		{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
	}

	var buf bytes.Buffer
	if err := d.Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return buf.Bytes()
}
