// Package dettest is Vellum's determinism and golden-artifact test harness.
//
// It exists before the first writer does, deliberately. Retrofitting a
// determinism suite onto an existing writer finds bugs you then have to unwind,
// and a packaging layer that is not deterministic from the start is
// unrecoverable without a rewrite. Every format epic registers cases here
// rather than growing determinism tests of its own.
//
// # What it asserts
//
// Assertions are on raw bytes, always. XML is normalised only for the failure
// display, so a mismatch reads as "three attributes differ in word/styles.xml"
// rather than as a binary diff — but the thing compared is the bytes, because
// the bytes are the guarantee.
//
// A case is emitted many times in one process, then again in freshly spawned
// processes. Both are necessary and neither is sufficient: in-process
// repetition catches map-iteration order, and only a fresh process catches
// nondeterminism that is fixed for the lifetime of one — an address-dependent
// sort, an init-order dependency, a hash seed sampled once at startup.
package dettest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"time"
)

// Case is one determinism and golden case.
//
// Adding a case is a single entry in [Cases]; the runners never change.
type Case struct {
	// Name identifies the case and names its golden directory.
	Name string

	// Ext is the artifact's file extension, without the dot.
	Ext string

	// Write emits the artifact. It is called many times, in several processes,
	// and must be a pure function of its arguments.
	Write func(w io.Writer, epoch time.Time) error
}

// Bytes runs the case and returns the emitted artifact.
func (c Case) Bytes(epoch time.Time) ([]byte, error) {
	var buf bytes.Buffer
	if err := c.Write(&buf, epoch); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Digest returns the hex SHA-256 of b.
func Digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
