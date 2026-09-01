package object

import (
	"bytes"
	"compress/zlib"

	verr "github.com/frankbardon/vellum/errors"
)

// deflateLevel is the compression level for every stream Vellum deflates.
//
// Pinned, and pinned to the same value zipdet uses, because the level is an
// input to the compressed bytes: a stream written at the default level on one
// machine and at best-compression on another differ while carrying identical
// content.
const deflateLevel = zlib.BestCompression

// Stream is a PDF stream: a dictionary followed by raw data.
//
// A stream must be an indirect object — the specification requires it, because
// /Length may itself be an indirect reference — so this is added through
// [Document.AddStream] rather than nested inside another object.
type Stream struct {
	// Dict is the stream dictionary. /Length is set on write and must not be
	// set by the caller; anything else, including /Filter, is the caller's.
	Dict Dict

	// Data is the stream's bytes, already filtered if Filter says they are.
	Data []byte
}

// AppendPDF implements [Object].
//
// /Length is written from the data rather than trusted from the dictionary, so
// a caller cannot produce a stream whose declared length disagrees with its
// content — which is the single most common way a hand-built PDF becomes
// unreadable, and one that no reader reports usefully.
func (s Stream) AppendPDF(dst []byte) []byte {
	d := s.Dict
	d.Set("Length", Int(len(s.Data)))

	dst = d.AppendPDF(dst)
	dst = append(dst, "\nstream\n"...)
	dst = append(dst, s.Data...)
	// The specification requires an EOL before endstream, and requires it not
	// to be counted in /Length.
	dst = append(dst, "\nendstream"...)
	return dst
}

// Deflate returns a stream whose data is zlib-compressed, with /Filter set.
//
// PDF's FlateDecode is zlib, not raw deflate: the two-byte header and the
// trailing Adler-32 are part of what a reader expects. Writing raw deflate here
// produces a file that most readers reject and a few silently render blank,
// which is worse.
//
// The compressed bytes are stable for a fixed Go toolchain minor and not
// guaranteed across them, exactly as with the OPC writer. That is the stated
// limit of byte-identity, and it does not touch artifact identity, which comes
// from the spec hash rather than from the file.
func Deflate(dict Dict, raw []byte) (Stream, error) {
	var buf bytes.Buffer
	zw, err := zlib.NewWriterLevel(&buf, deflateLevel)
	if err != nil {
		// Unreachable: deflateLevel is a compile-time constant in range. Kept
		// as a hard failure so a future change to it cannot silently produce an
		// uncompressed stream that claims to be compressed.
		return Stream{}, verr.WrapCodedError(err, verr.VELLUM_INTERNAL_INVARIANT,
			"constructing the deflate writer failed")
	}
	if _, err := zw.Write(raw); err != nil {
		return Stream{}, verr.WrapCodedError(err, verr.VELLUM_PDF_STREAM_INVALID,
			"compressing the stream failed")
	}
	if err := zw.Close(); err != nil {
		return Stream{}, verr.WrapCodedError(err, verr.VELLUM_PDF_STREAM_INVALID,
			"finishing the stream compression failed")
	}

	d := dict
	d.Set("Filter", Name("FlateDecode"))
	return Stream{Dict: d, Data: buf.Bytes()}, nil
}
