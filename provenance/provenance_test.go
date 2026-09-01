package provenance_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/frankbardon/vellum/provenance"
)

func record() *provenance.Record {
	return &provenance.Record{
		VellumVersion:   "0.1.0",
		SourceDateEpoch: time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC),
		SpecHash:        "4a9722728134cc516a8b844b81c7accb",
		ThemeHash:       "0123456789abcdef0123456789abcdef",
		Assets: []provenance.AssetRef{
			{Handle: "chart-1", Media: "image/png", Hash: "aaaa"},
		},
		Fonts: []provenance.FontRef{
			{Family: "BERA Sans", Embedded: true, SubsetProfile: "glyf"},
			{Family: "Helvetica Neue", Embedded: false, SubstitutedWith: "Arial"},
		},
		Sources: []provenance.Source{{Kind: "envelope", ID: "env-42"}},
	}
}

func TestRecord_HashIsStable(t *testing.T) {
	first := record().Hash()
	for range 50 {
		if got := record().Hash(); got != first {
			t.Fatalf("hash is not stable: %q then %q", first, got)
		}
	}
}

func TestRecord_HashMovesWithContent(t *testing.T) {
	base := record().Hash()

	tests := []struct {
		name   string
		mutate func(*provenance.Record)
	}{
		{"version", func(r *provenance.Record) { r.VellumVersion = "0.2.0" }},
		{"spec hash", func(r *provenance.Record) { r.SpecHash = "different" }},
		{"theme hash", func(r *provenance.Record) { r.ThemeHash = "different" }},
		{"asset hash", func(r *provenance.Record) { r.Assets[0].Hash = "bbbb" }},
		{"substitution", func(r *provenance.Record) { r.Fonts[1].SubstitutedWith = "Liberation Sans" }},
		{"subset profile", func(r *provenance.Record) { r.Fonts[0].SubsetProfile = "cff" }},
		{"source", func(r *provenance.Record) { r.Sources[0].ID = "env-43" }},
		{"epoch", func(r *provenance.Record) { r.SourceDateEpoch = time.Unix(0, 0).UTC() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := record()
			tt.mutate(r)
			if r.Hash() == base {
				t.Error("a content change did not move the hash; the field is not participating")
			}
		})
	}
}

// TestRecord_Deterministic covers why GeneratedAt is a pointer. A record with a
// wall-clock time describes a render that will not reproduce, and a consumer
// comparing digests needs to know that before concluding a document changed.
func TestRecord_Deterministic(t *testing.T) {
	r := record()
	if !r.Deterministic() {
		t.Error("a record with no generation time should report as deterministic")
	}

	now := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	r.GeneratedAt = &now
	if r.Deterministic() {
		t.Error("a record carrying a wall-clock time should not report as deterministic")
	}

	var nilRecord *provenance.Record
	if nilRecord.Deterministic() {
		t.Error("a nil record should not report as deterministic")
	}
}

// TestRecord_OmitsGeneratedAtByDefault checks the wire shape: in deterministic
// mode the key is absent rather than null, so a reader can tell "reproducible"
// from "the field exists and is empty".
func TestRecord_OmitsGeneratedAtByDefault(t *testing.T) {
	raw, err := json.Marshal(record())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "generated_at") {
		t.Errorf("a deterministic record serialised a generated_at key:\n%s", raw)
	}
}

// TestRecord_CarriesNoMachineIdentity guards the rule that keeps the record
// useful: nothing read from the machine that ran the render, because that
// would make two runs producing identical bytes carry different provenance.
func TestRecord_CarriesNoMachineIdentity(t *testing.T) {
	raw, err := json.Marshal(record())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"hostname", "host", "user", "username", "cwd", "working_dir", "pid"} {
		if strings.Contains(string(raw), `"`+forbidden+`"`) {
			t.Errorf("the record carries %q; machine identity makes two identical renders differ", forbidden)
		}
	}
}

func TestRecord_NilHash(t *testing.T) {
	var r *provenance.Record
	if r.Hash() == "" {
		t.Error("hashing a nil record produced the empty string")
	}
	if r.Hash() == record().Hash() {
		t.Error("a nil record hashed the same as a real one")
	}
}
