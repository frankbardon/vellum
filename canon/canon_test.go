package canon_test

import (
	"encoding/json"
	"testing"

	"github.com/frankbardon/vellum/canon"
)

func TestCanonicalHash_FieldOrderInvariant(t *testing.T) {
	a := map[string]any{"alpha": 1, "beta": 2, "gamma": 3}
	b := map[string]any{"gamma": 3, "beta": 2, "alpha": 1}

	if canon.CanonicalHash("t", a) != canon.CanonicalHash("t", b) {
		t.Error("two maps with the same content hashed differently; object keys must be sorted")
	}
}

// TestCanonicalHash_ArrayOrderMatters is the counterpart. Array order is
// meaning: the second section of a document is not interchangeable with the
// first.
func TestCanonicalHash_ArrayOrderMatters(t *testing.T) {
	a := []any{1, 2, 3}
	b := []any{3, 2, 1}

	if canon.CanonicalHash("t", a) == canon.CanonicalHash("t", b) {
		t.Error("reordering an array did not change the hash; array order is content")
	}
}

// TestCanonicalHash_DomainTagNamespaces covers the collision the tag exists to
// prevent: two different types whose canonical JSON coincides.
func TestCanonicalHash_DomainTagNamespaces(t *testing.T) {
	empty := map[string]any{}

	spec := canon.CanonicalHash("spec", empty)
	theme := canon.CanonicalHash("theme", empty)
	if spec == theme {
		t.Error("two domains produced the same hash for identical content; a caller keying a cache on the hash alone would serve one for the other")
	}
}

// TestCanonicalHash_TagCannotBeForgedBySeparator checks that the NUL separator
// actually separates. Without it, tag "ab" with content "c" and tag "a" with
// content "bc" could collide.
func TestCanonicalHash_TagCannotBeForgedBySeparator(t *testing.T) {
	if canon.CanonicalHash("ab", "c") == canon.CanonicalHash("a", "bc") {
		t.Error("the domain tag ran into the content; the separator is not doing its job")
	}
}

func TestCanonicalHash_NegativeZero(t *testing.T) {
	pos := canon.CanonicalHash("t", map[string]any{"v": 0.0})
	neg := canon.CanonicalHash("t", map[string]any{"v": negZero()})

	if pos != neg {
		t.Error("negative zero hashed differently from positive zero; they compare equal and arise from ordinary arithmetic")
	}
}

func negZero() float64 {
	z := 0.0
	return -z
}

// TestCanonicalHash_NumberSpellings pins that a value's hash depends on the
// value, not on how it was written.
func TestCanonicalHash_NumberSpellings(t *testing.T) {
	tests := [][2]string{
		{`{"v":1}`, `{"v":1.0}`},
		{`{"v":1}`, `{"v":1e0}`},
		{`{"v":100}`, `{"v":1e2}`},
		{`{"v":0}`, `{"v":-0}`},
		{`{"v":0.5}`, `{"v":5e-1}`},
		{`{"v":-0.0}`, `{"v":0}`},
	}
	for _, tt := range tests {
		a := json.RawMessage(tt[0])
		b := json.RawMessage(tt[1])
		if canon.CanonicalHash("t", a) != canon.CanonicalHash("t", b) {
			t.Errorf("%s and %s hashed differently; they are the same value", tt[0], tt[1])
		}
	}
}

// TestCanonicalHash_LargeIntegersKeepTheirLowBits covers values beyond
// float64's exact integer range.
//
// The fixtures are RawMessage rather than decoded maps on purpose: decoding
// into an any turns every number into a float64, which would destroy the
// precision in the test setup and prove nothing about the implementation. Raw
// JSON reaches the canonicaliser exactly as an encoder would have produced it.
func TestCanonicalHash_LargeIntegersKeepTheirLowBits(t *testing.T) {
	a := json.RawMessage(`{"v":9007199254740993}`)
	b := json.RawMessage(`{"v":9007199254740992}`)

	if canon.CanonicalHash("t", a) == canon.CanonicalHash("t", b) {
		t.Error("two adjacent large integers collided; UseNumber is not preserving precision")
	}

	// And the same value written two ways still agrees.
	c := json.RawMessage(`{ "v" : 9007199254740993 }`)
	if canon.CanonicalHash("t", a) != canon.CanonicalHash("t", c) {
		t.Error("whitespace changed the hash of a large integer")
	}
}

func TestCanonicalHash_WhitespaceInvariant(t *testing.T) {
	compact := json.RawMessage(`{"a":[1,2],"b":"x"}`)
	spaced := json.RawMessage("{\n  \"a\" : [ 1 , 2 ],\n  \"b\" : \"x\"\n}")
	if canon.CanonicalHash("t", compact) != canon.CanonicalHash("t", spaced) {
		t.Error("whitespace affected the hash")
	}
}

func TestCanonicalHash_Length(t *testing.T) {
	got := canon.CanonicalHash("t", map[string]any{"a": 1})
	if len(got) != canon.HashLength {
		t.Errorf("hash length = %d, want %d", len(got), canon.HashLength)
	}
	for _, r := range got {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Fatalf("hash contains a non-hex character %q: %s", r, got)
		}
	}
}

func TestCanonicalHash_UnmarshalableReturnsEmpty(t *testing.T) {
	// Hashing is infallible at the call site; an input that cannot be
	// marshalled yields the empty string rather than a panic.
	if got := canon.CanonicalHash("t", make(chan int)); got != "" {
		t.Errorf("hash of an unmarshalable value = %q, want the empty string", got)
	}
}

func TestCanonicalHash_NilAndEmptyAreDistinct(t *testing.T) {
	if canon.CanonicalHash("t", nil) == canon.CanonicalHash("t", map[string]any{}) {
		t.Error("nil and an empty object hashed alike; they are different documents")
	}
}

// TestCanonicalHash_PinnedVectors is the gate that makes the algorithm a
// contract rather than an implementation detail.
//
// These values were computed once. Changing one is changing the identity of
// every artifact every consumer has ever cached, so it requires a
// format_version bump in the same change — never a quiet update to match new
// output.
func TestCanonicalHash_PinnedVectors(t *testing.T) {
	tests := []struct {
		name  string
		tag   string
		value any
		want  string
	}{
		{"empty object", "spec", map[string]any{}, "ed9cdeb03935e80c964c397aafb020c5"},
		{"nil", "spec", nil, "d653240a8f5ce5b7377b0fff2a11371c"},
		{"simple object", "spec", map[string]any{"a": 1, "b": "x"}, "abed08e5ec4d34b59df6d2344154ab19"},
		{"array", "spec", []any{1, 2, 3}, "e4c8fb738ccc2a8bd5c1f33fd571cadf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canon.CanonicalHash(tt.tag, tt.value)
			if got != tt.want {
				t.Errorf("CanonicalHash(%q, %v) = %q, want %q\n"+
					"If this change is intended, it changes the identity of every artifact every consumer has cached. "+
					"Bump format_version in the same change.", tt.tag, tt.value, got, tt.want)
			}
		})
	}
}
