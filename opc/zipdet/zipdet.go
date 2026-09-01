package zipdet

import (
	"compress/flate"
	"time"
)

// PinnedEpoch is the timestamp written into every entry when no
// [WriteOptions.SourceDateEpoch] is given.
//
// It is 1980-01-01T00:00:00Z rather than the Go zero time because MS-DOS
// timestamps — which is what a ZIP local header carries — cannot represent
// anything earlier: the year is stored as an offset from 1980. The Go zero
// time is therefore not merely unusual here, it is unencodable.
var PinnedEpoch = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

// deflateLevel is pinned rather than left at the default so that the default
// changing in a future Go release cannot silently move every golden.
const deflateLevel = flate.BestCompression

// zipVersion20 is the ZIP specification version stamped into every entry, in
// both the "version made by" and "version needed to extract" header fields.
//
// It has to be written explicitly. archive/zip sets these fields inside
// CreateHeader, which this package does not use — CreateRaw takes the header
// verbatim, so a field left at its zero value reaches the byte stream as zero.
// A "version needed to extract" of 0 is not a version any specification
// defines: unzip and the Go reader ignore it, but Word reads it, and reports
// the package as containing unreadable content.
//
// 2.0 is the correct value because 2.0 is the version that introduced deflate,
// which is the highest-numbered feature these archives use. The high byte of
// the "version made by" field names the host filesystem, and it is left at 0
// (MS-DOS/FAT) so an archive written on any platform is byte-identical.
//
// TestWrite_VersionFieldsArePinned reads the raw headers and fails if this
// stops being true.
const zipVersion20 = 20

// defaultMaxEntryBytes bounds a single entry's uncompressed size when reading.
// The bound exists so a decompression bomb in an untrusted template is a coded
// error rather than an out-of-memory kill. It is generous because legitimate
// decks genuinely are large — a bound that rejects real input would be worked
// around by disabling it, which is worse than no bound at all.
const defaultMaxEntryBytes = 512 << 20 // 512 MiB

// defaultMaxTotalBytes bounds the sum of all entries' uncompressed sizes.
const defaultMaxTotalBytes = 2 << 30 // 2 GiB

// Kind classifies an entry's payload so the compression method is a property
// of the content rather than a decision at the call site.
//
// Deciding per call site is how two writers come to compress the same bytes
// differently, and content sniffing is how the same bytes come to be
// compressed differently on different inputs. Neither is acceptable in a byte
// stream that has to be reproducible, so the caller declares the kind and the
// method follows from it by rule.
type Kind uint8

const (
	// KindCompressible is content that benefits from deflate: XML parts,
	// relationship files, text.
	KindCompressible Kind = iota

	// KindPrecompressed is content that is already compressed and would only
	// grow: PNG, JPEG, embedded font programs in compressed containers.
	// Stored rather than deflated.
	KindPrecompressed
)

// String implements fmt.Stringer.
func (k Kind) String() string {
	switch k {
	case KindCompressible:
		return "compressible"
	case KindPrecompressed:
		return "precompressed"
	default:
		return "unknown"
	}
}

// msdosTime encodes t as an MS-DOS date and time pair, the representation a
// ZIP local file header carries natively.
//
// Resolution is two seconds and the range starts at 1980; a t outside the
// representable range is clamped to [PinnedEpoch] rather than wrapping,
// because a wrapped date is a plausible-looking wrong answer and a clamped one
// is an obviously pinned one.
func msdosTime(t time.Time) (fdate, ftime uint16) {
	t = t.UTC()
	if t.Year() < 1980 {
		t = PinnedEpoch
	}
	if t.Year() > 2107 { // 1980 + 127, the widest the 7-bit year field reaches
		t = time.Date(2107, time.December, 31, 23, 59, 58, 0, time.UTC)
	}
	fdate = uint16(t.Day() + int(t.Month())<<5 + (t.Year()-1980)<<9)
	ftime = uint16(t.Second()/2 + t.Minute()<<5 + t.Hour()<<11)
	return fdate, ftime
}
