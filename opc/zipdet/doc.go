// Package zipdet is a deterministic ZIP reader and writer.
//
// Go's archive/zip is not deterministic enough for Vellum's purposes, in four
// separate ways, each of which this package pins:
//
//   - It stamps modification times, and — worse — emitting a non-zero
//     [archive/zip.FileHeader.Modified] makes it append an Info-ZIP extended
//     timestamp extra field, so the header bytes differ even when the visible
//     timestamp does not. This package leaves Modified zero and writes the
//     legacy MS-DOS date and time fields directly.
//   - Its streaming writer does not know an entry's size in advance, so it
//     sets the data-descriptor flag and writes sizes after the payload. This
//     package compresses each entry into memory first and writes sizes and
//     CRC up front.
//   - Compression is chosen per call site rather than by rule. Here it is a
//     pure function of the entry's declared [Kind].
//   - Entry order is whatever the caller happened to do. Here the caller
//     supplies an ordered slice, and the API accepts no map anywhere, so
//     order-dependent nondeterminism is unrepresentable rather than merely
//     tested against.
//
// # The limit worth knowing
//
// Go's compress/flate output is stable for a pinned level within a toolchain
// but is not guaranteed stable across Go minor versions. Byte-identical output
// is therefore guaranteed for a fixed Go toolchain minor version, and a
// toolchain bump is a deliberate golden rebaseline — the same discipline that
// would have applied to a converter version. Callers who need byte-identity
// across toolchains can set [WriteOptions.Uncompressed].
//
// This does not weaken artifact identity, which derives from input hashes
// rather than from output bytes.
package zipdet
