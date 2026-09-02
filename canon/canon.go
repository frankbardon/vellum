// Package canon owns Vellum's canonical hashing.
//
// One implementation, used by every Hash method in the tree. Hashing is how an
// artifact's identity is established, and an identity computed two slightly
// different ways in two places is not an identity.
//
// # Why identity matters here
//
// A consumer that can only learn an artifact's name by producing it cannot use
// the name to avoid producing it. Vellum's artifact names derive from the
// specification hash and the asset hashes — both inputs — so the name is
// knowable before the render runs, and a caller can ask "does this already
// exist" and skip the work.
//
// Non-determinism in this package does not fail loudly. It silently defeats
// that check and produces a new artifact on every run: a slow storage leak in
// somebody else's product, with no error anywhere. That is why the algorithm
// is pinned by committed vectors rather than merely tested for self-consistency.
package canon

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
)

// HashLength is the number of hex characters a hash carries.
//
// Thirty-two, from the first sixteen bytes of the digest. Full SHA-256 would
// be 64, which is unwieldy in a filename and buys nothing here: these hashes
// name artifacts, they do not authenticate them, and 128 bits is far past the
// point where an accidental collision is worth designing around.
const HashLength = 32

// CanonicalHash returns a deterministic hash of v's canonical JSON form.
//
// The algorithm, stated plainly because every property of it is load-bearing:
//
//  1. Marshal v to JSON.
//  2. Walk the result, sorting object keys and normalising numeric edge cases,
//     and re-emit with no whitespace.
//  3. Digest the domain tag, a NUL separator, and the canonical bytes.
//  4. Return the first sixteen bytes, hex-encoded.
//
// The domain tag namespaces the result. Without it, two different types whose
// canonical JSON happens to coincide would produce the same hash — a
// specification and a theme that both serialise to {} would be
// indistinguishable, and a caller keying a cache on the hash alone would serve
// one for the other.
//
// Object keys are sorted, so field order cannot affect the result. Array order
// is preserved, because array order is meaning: the second section of a
// document is not interchangeable with the first.
//
// A marshalling failure returns the empty string rather than an error. Hashing
// is treated as infallible at every call site, and the inputs are structs this
// module defines and knows to be marshalable; propagating an error that cannot
// occur would put a check on several hundred call sites to no purpose.
func CanonicalHash(tag string, v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	canonical, err := canonicalize(raw)
	if err != nil {
		return ""
	}

	h := sha256.New()
	h.Write([]byte(tag))
	h.Write([]byte{0})
	h.Write(canonical)
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:HashLength/2])
}

// Canonical returns v's canonical JSON form: the exact bytes [CanonicalHash]
// digests.
//
// Exported so a record can be *carried* as well as hashed. An artifact
// embedding its own provenance and a hash of it needs the two to be the same
// bytes; produced separately, a marshalling difference between them would make
// the hash describe a record nobody has, which is worse than carrying no hash
// at all.
//
// It returns an error where [CanonicalHash] returns the empty string, because a
// caller embedding the bytes has somewhere to put a failure and a caller
// hashing at several hundred sites does not.
func Canonical(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return canonicalize(raw)
}

// canonicalize rewrites JSON into its canonical form.
func canonicalize(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	// UseNumber keeps numeric literals as text rather than turning them all
	// into float64, so an integer stays an integer and a large one does not
	// lose its low bits on the way through.
	dec.UseNumber()

	var tree any
	if err := dec.Decode(&tree); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := writeCanonical(&buf, tree); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonical(b *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if t {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case json.Number:
		b.WriteString(normalizeNumber(t))
	case string:
		enc, err := json.Marshal(t)
		if err != nil {
			return err
		}
		b.Write(enc)
	case []any:
		b.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeCanonical(b, item); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		// Bytewise, never locale-aware. Collation that depends on a locale is
		// nondeterminism with extra steps.
		sort.Strings(keys)

		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			enc, err := json.Marshal(k)
			if err != nil {
				return err
			}
			b.Write(enc)
			b.WriteByte(':')
			if err := writeCanonical(b, t[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		enc, err := json.Marshal(t)
		if err != nil {
			return err
		}
		b.Write(enc)
	}
	return nil
}

// normalizeNumber collapses numeric spellings that mean the same value.
//
// Negative zero is the case that matters in practice: it arises from ordinary
// arithmetic, compares equal to positive zero, and would otherwise give two
// specifications that are equal in every observable way two different hashes.
func normalizeNumber(n json.Number) string {
	s := n.String()

	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return strconv.FormatInt(i, 10)
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		// Not representable as an int64 or a float64 — a very large integer
		// literal, most likely. Keep the original spelling rather than
		// guessing, minus a leading plus sign that JSON does not permit anyway.
		return strings.TrimPrefix(s, "+")
	}
	if f == 0 {
		return "0"
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		// Unreachable through encoding/json, which refuses to marshal either.
		// Kept as an explicit branch so a future path that constructs a
		// json.Number by hand cannot produce a hash from a value that has no
		// canonical spelling.
		return "0"
	}
	// 'g' with -1 precision is the shortest representation that round-trips,
	// which makes the spelling a function of the value rather than of how it
	// was written.
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// BytesHash returns a deterministic hash of raw bytes, in the same shape and
// with the same domain-tagging rule as [CanonicalHash].
//
// Assets are the reason this exists. An asset is bytes, not a struct, and
// routing it through JSON to hash it would base64 the whole payload to no
// purpose. The two functions share the tag-NUL-content construction and the
// truncation so that every hash Vellum produces is the same width and the same
// kind of thing, and so that the algorithm has exactly one owner.
//
// The tag matters here for the same reason it matters there: an asset and a
// specification that happened to serialise identically must not collide, and a
// caller keying a cache on a hash alone must not be served one for the other.
func BytesHash(tag string, raw []byte) string {
	h := sha256.New()
	h.Write([]byte(tag))
	h.Write([]byte{0})
	h.Write(raw)
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:HashLength/2])
}
