package zipdet

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"hash/crc32"
	"io"
	"time"

	verr "github.com/frankbardon/vellum/errors"
)

// Entry is one member of an archive, in the order it will be written.
//
// Exactly one of Data and Open must be set. Open exists so a package need not
// hold every part in memory at once: entries are compressed one at a time, so
// peak memory is proportional to the largest entry rather than to the whole
// document.
type Entry struct {
	// Name is the archive-relative entry name, using forward slashes and no
	// leading slash.
	Name string

	// Kind determines the compression method by rule. See [Kind].
	Kind Kind

	// Data is the entry's uncompressed content.
	Data []byte

	// Open lazily supplies the entry's content. Called at most once.
	Open func() (io.ReadCloser, error)
}

// WriteOptions configures a write. The zero value is the deterministic
// default: pinned timestamps and rule-driven compression.
type WriteOptions struct {
	// SourceDateEpoch is the timestamp stamped into every entry. The zero
	// value selects [PinnedEpoch].
	//
	// Setting a real time is a deliberate opt-out of byte-identical output
	// across runs — the result is still deterministic for a fixed epoch, but
	// two runs at different wall-clock times will differ.
	SourceDateEpoch time.Time

	// Uncompressed stores every entry rather than deflating the compressible
	// ones. Output is larger, and byte-identical across Go toolchain versions
	// rather than only within one, because nothing passes through flate.
	Uncompressed bool

	// MaxEntryBytes bounds a single entry's uncompressed size. Zero selects
	// the built-in default.
	MaxEntryBytes int64
}

// Write emits entries to w as a deterministic ZIP archive.
//
// The same ordered entries and the same options produce byte-identical output,
// on any machine and in any process. Entry order is exactly the order given —
// no sorting is applied here, because the canonical ordering is an OPC concern
// and this layer must not second-guess it.
func Write(w io.Writer, entries []Entry, opts WriteOptions) error {
	stamp := opts.SourceDateEpoch
	if stamp.IsZero() {
		stamp = PinnedEpoch
	}
	maxEntry := opts.MaxEntryBytes
	if maxEntry <= 0 {
		maxEntry = defaultMaxEntryBytes
	}
	fdate, ftime := msdosTime(stamp)

	seen := make(map[string]bool, len(entries))
	zw := zip.NewWriter(w)

	for i := range entries {
		e := &entries[i]
		if err := validateEntryName(e.Name); err != nil {
			return err
		}
		if seen[e.Name] {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_ZIP_ENTRY_DUPLICATE,
				"two entries share a name",
				map[string]any{"entry_name": e.Name, "entry_index": i})
		}
		seen[e.Name] = true

		if (e.Data == nil) == (e.Open == nil) {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
				"entry must set exactly one of Data and Open",
				map[string]any{"entry_name": e.Name, "entry_index": i})
		}

		raw, err := entryBytes(e, maxEntry)
		if err != nil {
			return err
		}
		if err := writeEntry(zw, e, raw, fdate, ftime, opts.Uncompressed); err != nil {
			return err
		}
	}

	if err := zw.Close(); err != nil {
		return verr.WrapCodedError(err, verr.VELLUM_ZIP_MALFORMED, "closing the archive failed")
	}
	return nil
}

// entryBytes materialises one entry's uncompressed content, enforcing the
// per-entry bound as it reads rather than after, so an oversized lazy source
// is refused before it is fully buffered.
func entryBytes(e *Entry, maxEntry int64) ([]byte, error) {
	if e.Data != nil {
		if int64(len(e.Data)) > maxEntry {
			return nil, tooLarge(e.Name, int64(len(e.Data)), maxEntry)
		}
		return e.Data, nil
	}

	rc, err := e.Open()
	if err != nil {
		return nil, verr.WrapCodedErrorWithDetails(err, verr.VELLUM_ZIP_MALFORMED,
			"opening the entry source failed",
			map[string]any{"entry_name": e.Name})
	}
	defer rc.Close()

	var buf bytes.Buffer
	// Read one byte past the bound so exceeding it is detected rather than
	// silently truncated.
	n, err := io.Copy(&buf, io.LimitReader(rc, maxEntry+1))
	if err != nil {
		return nil, verr.WrapCodedErrorWithDetails(err, verr.VELLUM_ZIP_MALFORMED,
			"reading the entry source failed",
			map[string]any{"entry_name": e.Name})
	}
	if n > maxEntry {
		return nil, tooLarge(e.Name, n, maxEntry)
	}
	return buf.Bytes(), nil
}

// writeEntry compresses raw and appends it through CreateRaw.
//
// CreateRaw rather than Create is the whole point: it takes a header whose
// sizes and CRC are already known, so no data descriptor is emitted and the
// general-purpose bit flag stays clear. The streaming Create path cannot do
// that, because it does not know the sizes until after the payload is written.
func writeEntry(zw *zip.Writer, e *Entry, raw []byte, fdate, ftime uint16, forceStore bool) error {
	method := uint16(zip.Deflate)
	if forceStore || e.Kind == KindPrecompressed {
		method = zip.Store
	}

	payload := raw
	if method == zip.Deflate {
		var cbuf bytes.Buffer
		fw, err := flate.NewWriter(&cbuf, deflateLevel)
		if err != nil {
			// Unreachable: deflateLevel is a compile-time constant in range.
			// Kept as a hard failure so a future change to the constant cannot
			// silently produce an unwritten entry.
			return verr.WrapCodedErrorWithDetails(err, verr.VELLUM_INTERNAL_INVARIANT,
				"constructing the deflate writer failed",
				map[string]any{"entry_name": e.Name, "level": deflateLevel})
		}
		if _, err := fw.Write(raw); err != nil {
			return verr.WrapCodedErrorWithDetails(err, verr.VELLUM_ZIP_MALFORMED,
				"compressing the entry failed", map[string]any{"entry_name": e.Name})
		}
		if err := fw.Close(); err != nil {
			return verr.WrapCodedErrorWithDetails(err, verr.VELLUM_ZIP_MALFORMED,
				"finishing the entry compression failed", map[string]any{"entry_name": e.Name})
		}
		payload = cbuf.Bytes()
	}

	fh := &zip.FileHeader{
		Name:   e.Name,
		Method: method,

		// Pinned rather than inherited from the platform, so an archive
		// written on Windows and on Linux is byte-identical.
		CreatorVersion:     0,
		ExternalAttrs:      0,
		CRC32:              crc32.ChecksumIEEE(raw),
		CompressedSize64:   uint64(len(payload)),
		UncompressedSize64: uint64(len(raw)),
	}
	setMSDOSTime(fh, fdate, ftime)

	w, err := zw.CreateRaw(fh)
	if err != nil {
		return verr.WrapCodedErrorWithDetails(err, verr.VELLUM_ZIP_MALFORMED,
			"creating the archive entry failed", map[string]any{"entry_name": e.Name})
	}
	if _, err := w.Write(payload); err != nil {
		return verr.WrapCodedErrorWithDetails(err, verr.VELLUM_ZIP_MALFORMED,
			"writing the archive entry failed", map[string]any{"entry_name": e.Name})
	}
	return nil
}

func tooLarge(name string, size, limit int64) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_ZIP_TOO_LARGE,
		"entry exceeds the configured uncompressed size bound",
		map[string]any{"entry_name": name, "size_bytes": size, "limit_bytes": limit})
}

// setMSDOSTime writes the timestamp into the legacy MS-DOS header fields,
// leaving FileHeader.Modified at its zero value.
//
// Those two fields are deprecated, and the deprecation notice says to use
// Modified instead. Doing so is exactly what this package must not do: a
// non-zero Modified makes archive/zip append an Info-ZIP extended timestamp
// extra field, which puts a second copy of the time into the header bytes and
// varies the header length. The deprecated fields are the only way to write a
// timestamp without that extra field, so the deprecation is suppressed here
// deliberately rather than worked around.
//
// TestWrite_NoExtraFields reads the raw local headers and fails if this ever
// stops being true.
func setMSDOSTime(fh *zip.FileHeader, fdate, ftime uint16) {
	//lint:ignore SA1019 Modified would emit an extended-timestamp extra field; see the comment above.
	fh.ModifiedDate = fdate
	//lint:ignore SA1019 Modified would emit an extended-timestamp extra field; see the comment above.
	fh.ModifiedTime = ftime
}
