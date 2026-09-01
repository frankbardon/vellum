package zipdet

import (
	"archive/zip"
	"bytes"
	"io"

	verr "github.com/frankbardon/vellum/errors"
)

// ReadOptions bounds a read. The zero value selects the built-in defaults.
type ReadOptions struct {
	// MaxEntryBytes bounds a single entry's uncompressed size.
	MaxEntryBytes int64

	// MaxTotalBytes bounds the sum of all entries' uncompressed sizes.
	MaxTotalBytes int64
}

// ReadEntry is one member of an archive that has been read, carrying its
// content and enough of its header to reproduce the archive faithfully.
type ReadEntry struct {
	// Name is the archive-relative entry name.
	Name string

	// Data is the entry's uncompressed content.
	Data []byte

	// Method is the compression method the source archive used. Preserved so
	// a round trip can reproduce it: re-deflating a stored entry, or storing a
	// deflated one, would change bytes Vellum promised not to touch.
	Method uint16
}

// Archive is a read archive. Entries are held in archive order — the order of
// the central directory — because that order is part of what a fill-mode round
// trip must preserve.
type Archive struct {
	entries []ReadEntry
	index   map[string]int
}

// Read parses the archive in r. Every failure is a coded error; nothing here
// panics, because this function parses untrusted input the moment a consumer
// accepts a user-supplied template.
func Read(r io.ReaderAt, size int64, opts ReadOptions) (*Archive, error) {
	maxEntry := opts.MaxEntryBytes
	if maxEntry <= 0 {
		maxEntry = defaultMaxEntryBytes
	}
	maxTotal := opts.MaxTotalBytes
	if maxTotal <= 0 {
		maxTotal = defaultMaxTotalBytes
	}

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, verr.WrapCodedError(err, verr.VELLUM_ZIP_MALFORMED,
			"the archive could not be read")
	}

	a := &Archive{
		entries: make([]ReadEntry, 0, len(zr.File)),
		index:   make(map[string]int, len(zr.File)),
	}

	var total int64
	for _, f := range zr.File {
		// Directory entries carry no content and are not parts. Skip them
		// rather than rejecting the archive: authoring tools emit them, and
		// refusing a template because Word added a folder marker would be
		// pedantry rather than safety.
		if f.FileInfo().IsDir() {
			continue
		}
		if err := validateEntryName(f.Name); err != nil {
			return nil, err
		}
		if _, dup := a.index[f.Name]; dup {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_ZIP_ENTRY_DUPLICATE,
				"two entries share a name",
				map[string]any{"entry_name": f.Name})
		}

		// Check the declared size before decompressing. This is the cheap half
		// of bomb defence: a small archive declaring an enormous entry is
		// refused without allocating for it. The read below enforces the same
		// bound against the actual bytes, because the declaration is
		// attacker-controlled and may understate the truth.
		if declared := int64(f.UncompressedSize64); declared > maxEntry {
			return nil, tooLarge(f.Name, declared, maxEntry)
		}

		data, err := readEntry(f, maxEntry)
		if err != nil {
			return nil, err
		}

		total += int64(len(data))
		if total > maxTotal {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_ZIP_TOO_LARGE,
				"the archive's total uncompressed size exceeds the configured bound",
				map[string]any{"total_bytes": total, "limit_bytes": maxTotal, "entry_name": f.Name})
		}

		a.index[f.Name] = len(a.entries)
		a.entries = append(a.entries, ReadEntry{
			Name:   f.Name,
			Data:   data,
			Method: f.Method,
		})
	}

	return a, nil
}

func readEntry(f *zip.File, maxEntry int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, verr.WrapCodedErrorWithDetails(err, verr.VELLUM_ZIP_MALFORMED,
			"opening an archive entry failed",
			map[string]any{"entry_name": f.Name})
	}
	defer rc.Close()

	var buf bytes.Buffer
	// Reading one byte past the bound distinguishes "exactly at the limit"
	// from "over it", which a plain LimitReader would silently truncate.
	n, err := io.Copy(&buf, io.LimitReader(rc, maxEntry+1))
	if err != nil {
		return nil, verr.WrapCodedErrorWithDetails(err, verr.VELLUM_ZIP_MALFORMED,
			"reading an archive entry failed",
			map[string]any{"entry_name": f.Name})
	}
	if n > maxEntry {
		return nil, tooLarge(f.Name, n, maxEntry)
	}
	return buf.Bytes(), nil
}

// Len reports the number of entries.
func (a *Archive) Len() int {
	if a == nil {
		return 0
	}
	return len(a.entries)
}

// Entries returns the entries in archive order. The slice is a copy, so a
// caller cannot reorder the archive by mutating it; the entry payloads are
// shared rather than duplicated, because copying every part would defeat the
// memory bound the read enforces.
func (a *Archive) Entries() []ReadEntry {
	if a == nil {
		return nil
	}
	out := make([]ReadEntry, len(a.entries))
	copy(out, a.entries)
	return out
}

// Get returns the named entry. The second result reports presence.
func (a *Archive) Get(name string) (ReadEntry, bool) {
	if a == nil {
		return ReadEntry{}, false
	}
	i, ok := a.index[name]
	if !ok {
		return ReadEntry{}, false
	}
	return a.entries[i], true
}

// Names returns the entry names in archive order.
func (a *Archive) Names() []string {
	if a == nil {
		return nil
	}
	out := make([]string, len(a.entries))
	for i := range a.entries {
		out[i] = a.entries[i].Name
	}
	return out
}
