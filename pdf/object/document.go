package object

import (
	"io"
	"strconv"

	verr "github.com/frankbardon/vellum/errors"
)

// Version is the PDF version written in the header.
//
// 1.7 rather than 2.0: PDF/A-2b is defined against 1.7, and the features 2.0
// adds are ones this writer deliberately does not use.
const Version = "1.7"

// binaryMarker is the four high bytes of the second header line.
//
// The specification recommends a comment containing bytes above 127 so that
// tools which sniff a file as text or binary classify a PDF as binary. veraPDF
// checks for it, and a transport that "helpfully" converts line endings in what
// it believes to be a text file destroys every stream in the document.
var binaryMarker = []byte{'%', 0xE2, 0xE3, 0xCF, 0xD3, '\n'}

// Document accumulates numbered objects and writes the file.
//
// Object numbers are assigned in the order objects are added, and the order
// objects are added is fixed by the code that builds the document. Nothing here
// sorts or renumbers, so two runs that build the same document produce the same
// numbering — which is a precondition for byte-identity, since object numbers
// appear inside every reference in the file.
type Document struct {
	// Trailer holds entries added to the trailer dictionary beyond the ones
	// this package computes: /Root and /Info are set through the fields below,
	// and /Size, /ID and the xref offset are computed on write.
	Trailer Dict

	// Root is the document catalogue. Required.
	Root Ref

	// Info is the document information dictionary. Optional in general and
	// required by PDF/A, which also requires its dates to agree with the XMP
	// metadata.
	Info Ref

	// ID is the file identifier written to the trailer, as two byte strings.
	//
	// PDF/A requires it. Both halves are derived from the document's content
	// rather than generated, because a random identifier is the single easiest
	// way to lose byte-identity and the hardest to notice: the file differs
	// between runs in sixteen bytes buried in the trailer, and everything else
	// matches.
	ID [2][]byte

	// Uncompressed stores every stream verbatim rather than deflating it.
	//
	// The output is larger, and byte-identical across Go toolchain versions
	// rather than only within one, because nothing passes through flate. The
	// same escape hatch the OPC writer offers, for the same reason: a caller
	// whose attestation spans toolchain upgrades needs it, and everybody else
	// wants the smaller file.
	Uncompressed bool

	objects []Object
}

// Add appends an object and returns its reference.
func (d *Document) Add(o Object) Ref {
	d.objects = append(d.objects, o)
	return Ref{Number: len(d.objects)}
}

// Reserve allocates an object number without supplying the object.
//
// Needed because the page tree is cyclic: a page names its parent and the
// parent names its kids, so one of the two references must exist before the
// object it points at. Reserving is the honest way to express that; the
// alternative is patching a reference after the fact, which puts a mutable
// object graph where a written file should be.
//
// Every reserved number must be filled before the document is written, and
// [Document.WriteTo] fails naming the ones that were not.
func (d *Document) Reserve() Ref {
	d.objects = append(d.objects, nil)
	return Ref{Number: len(d.objects)}
}

// Fill supplies the object for a reference obtained from [Document.Reserve].
func (d *Document) Fill(ref Ref, o Object) error {
	if ref.Number < 1 || ref.Number > len(d.objects) {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_PDF_OBJECT_UNRESOLVED,
			"the reference does not name an object in this document",
			map[string]any{"object_number": ref.Number, "object_count": len(d.objects)})
	}
	d.objects[ref.Number-1] = o
	return nil
}

// AddStream adds raw as an indirect stream object, compressed unless
// [Document.Uncompressed] is set.
func (d *Document) AddStream(dict Dict, raw []byte) (Ref, error) {
	if d.Uncompressed {
		return d.AddRawStream(dict, raw), nil
	}
	s, err := Deflate(dict, raw)
	if err != nil {
		return Ref{}, err
	}
	return d.Add(s), nil
}

// AddRawStream adds a stream whose data is stored uncompressed.
//
// Used where compression is pointless or forbidden: an ICC profile a validator
// reads, and the XMP metadata packet, which PDF/A requires to be readable by a
// consumer that does not parse PDF at all.
func (d *Document) AddRawStream(dict Dict, data []byte) Ref {
	return d.Add(Stream{Dict: dict, Data: data})
}

// Len returns the number of objects allocated so far.
func (d *Document) Len() int { return len(d.objects) }

// Write serialises the document.
//
// Named Write rather than WriteTo because it is not an io.WriterTo: that
// interface reports the byte count, and reporting it here would invite a caller
// to believe the count means something about the document rather than about the
// destination.
//
// The whole body is built in memory before anything is written, because the
// cross-reference table records the byte offset of every object and those are
// not known until the body exists. This is not a limitation worth engineering
// away: a document large enough for it to matter is one whose assets dominate
// the total anyway.
func (d *Document) Write(w io.Writer) error {
	if d.Root.IsZero() {
		return verr.NewCodedError(verr.VELLUM_PDF_OBJECT_UNRESOLVED,
			"the document has no catalogue; set Root before writing")
	}
	if err := d.checkFilled(); err != nil {
		return err
	}

	out := make([]byte, 0, 4096)
	out = append(out, "%PDF-"...)
	out = append(out, Version...)
	out = append(out, '\n')
	out = append(out, binaryMarker...)

	// Offsets are 1-based by object number; index 0 is the free head.
	offsets := make([]int, len(d.objects)+1)
	for i, o := range d.objects {
		offsets[i+1] = len(out)
		out = strconv.AppendInt(out, int64(i+1), 10)
		out = append(out, " 0 obj\n"...)
		out = o.AppendPDF(out)
		out = append(out, "\nendobj\n"...)
	}

	xrefOffset := len(out)
	out = d.appendXref(out, offsets)
	out = d.appendTrailer(out, xrefOffset)

	if _, err := w.Write(out); err != nil {
		return verr.WrapCodedError(err, verr.VELLUM_PDF_WRITE_FAILED, "writing the document failed")
	}
	return nil
}

// checkFilled reports any reserved object number that was never supplied.
//
// All of them are named rather than the first, because they are usually one
// mistake — a page tree assembled in the wrong order — and fixing them one
// failure at a time is several runs of the same debugging.
func (d *Document) checkFilled() error {
	var missing []any
	for i, o := range d.objects {
		if o == nil {
			missing = append(missing, i+1)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return verr.NewCodedErrorWithDetails(verr.VELLUM_PDF_OBJECT_UNRESOLVED,
		"object numbers were reserved and never filled",
		map[string]any{"object_numbers": missing})
}

// appendXref writes the classic cross-reference table.
//
// Every entry is exactly twenty bytes, which the specification requires so a
// reader can seek to an entry arithmetically. Getting the width wrong produces
// a file that opens in tolerant readers, because they rebuild the table by
// scanning, and fails in strict ones — the same asymmetry that has already
// bitten this project three times in OOXML.
func (d *Document) appendXref(out []byte, offsets []int) []byte {
	out = append(out, "xref\n0 "...)
	out = strconv.AppendInt(out, int64(len(offsets)), 10)
	out = append(out, '\n')

	// The head of the free list: object 0, generation 65535, always.
	out = append(out, "0000000000 65535 f\r\n"...)

	for _, off := range offsets[1:] {
		out = appendPadded(out, off, 10)
		out = append(out, ' ')
		out = appendPadded(out, 0, 5)
		out = append(out, " n\r\n"...)
	}
	return out
}

// appendTrailer writes the trailer dictionary, the xref offset and the marker.
func (d *Document) appendTrailer(out []byte, xrefOffset int) []byte {
	t := d.Trailer
	t.Set("Size", Int(len(d.objects)+1))
	t.Set("Root", d.Root)
	if !d.Info.IsZero() {
		t.Set("Info", d.Info)
	}
	if len(d.ID[0]) > 0 || len(d.ID[1]) > 0 {
		t.Set("ID", Array{HexString(d.ID[0]), HexString(d.ID[1])})
	}

	out = append(out, "trailer\n"...)
	out = t.AppendPDF(out)
	out = append(out, "\nstartxref\n"...)
	out = strconv.AppendInt(out, int64(xrefOffset), 10)
	out = append(out, "\n%%EOF\n"...)
	return out
}

// appendPadded writes n as a zero-padded decimal of exactly width digits.
func appendPadded(out []byte, n, width int) []byte {
	s := strconv.Itoa(n)
	for i := len(s); i < width; i++ {
		out = append(out, '0')
	}
	return append(out, s...)
}
