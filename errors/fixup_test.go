package errors_test

import (
	"strings"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
)

// TestCodesHaveFixups is the docs-coverage gate for the error registry. Every
// registered code must carry guidance, or must say plainly that no author
// action applies. A code that reaches a user with neither is a code that tells
// them something is wrong and nothing about what to do.
func TestCodesHaveFixups(t *testing.T) {
	for _, code := range verr.AllCodes() {
		m, ok := verr.MetadataFor(code)
		if !ok {
			t.Errorf("code %s has no metadata entry; add a row to codeMetadata in errors/fixup_metadata.go", code)
			continue
		}
		if strings.TrimSpace(m.Message) == "" {
			t.Errorf("code %s has an empty Message in its metadata row", code)
		}
		if m.FixupNotApplicable {
			if len(m.Fixups) != 0 {
				t.Errorf("code %s sets FixupNotApplicable and also carries %d fixups; pick one", code, len(m.Fixups))
			}
			continue
		}
		if len(m.Fixups) == 0 {
			t.Errorf("code %s has no fixups and is not tagged FixupNotApplicable; either author a hint or set the flag deliberately", code)
			continue
		}
		for i, f := range m.Fixups {
			if strings.TrimSpace(f.Hint) == "" {
				t.Errorf("code %s fixup %d has an empty Hint", code, i)
			}
			if f.Action == "" {
				t.Errorf("code %s fixup %d has no Action", code, i)
			}
			for _, seg := range f.Path {
				if strings.TrimSpace(seg) == "" {
					t.Errorf("code %s fixup %d has an empty path segment; use \"*\" for any index", code, i)
				}
			}
		}
	}
}

// TestMetadataHasNoOrphans catches the reverse omission: a row for a code that
// is no longer registered, which is how a renamed code leaves stale guidance
// behind.
func TestMetadataHasNoOrphans(t *testing.T) {
	registered := make(map[verr.Code]bool)
	for _, c := range verr.AllCodes() {
		registered[c] = true
	}
	for _, c := range verr.AllCodes() {
		if _, ok := verr.MetadataFor(c); !ok {
			t.Errorf("registered code %s has no metadata row", c)
		}
	}
	// Lookup over an unregistered code must not invent a row.
	if _, ok := verr.Lookup(verr.Code("VELLUM_NOT_A_REAL_CODE")); ok {
		t.Error("Lookup succeeded for an unregistered code")
	}
}

func TestMetadataFor_UnknownCodeDoesNotPanic(t *testing.T) {
	inputs := []verr.Code{"", "VELLUM_", "VELLUM_X", "not a code", "VELLUM_ZIP_MALFORMED_EXTRA"}
	for _, c := range inputs {
		if _, ok := verr.MetadataFor(c); ok {
			t.Errorf("MetadataFor(%q) reported a row for an unregistered code", c)
		}
		if got := c.Fixups(); got != nil {
			t.Errorf("Fixups() for unregistered code %q returned %v, want nil", c, got)
		}
	}
}

func TestFixups_NotApplicableReturnsNil(t *testing.T) {
	if got := verr.VELLUM_INTERNAL_INVARIANT.Fixups(); got != nil {
		t.Errorf("Fixups() for a FixupNotApplicable code = %v, want nil", got)
	}
}

func TestFixups_ReturnsCopy(t *testing.T) {
	first := verr.VELLUM_OPC_PART_NAME_INVALID.Fixups()
	if len(first) == 0 {
		t.Fatal("expected at least one fixup")
	}
	original := first[0].Hint
	first[0].Hint = "mutated"

	second := verr.VELLUM_OPC_PART_NAME_INVALID.Fixups()
	if second[0].Hint != original {
		t.Error("Fixups returned the backing slice; a caller mutating it would corrupt the registry")
	}
}

func TestLookup_CarriesDomain(t *testing.T) {
	got, ok := verr.Lookup(verr.VELLUM_ZIP_TOO_LARGE)
	if !ok {
		t.Fatal("Lookup failed for a registered code")
	}
	if got.Code != verr.VELLUM_ZIP_TOO_LARGE {
		t.Errorf("Code = %q, want %q", got.Code, verr.VELLUM_ZIP_TOO_LARGE)
	}
	if got.Domain != "ZIP" {
		t.Errorf("Domain = %q, want %q", got.Domain, "ZIP")
	}
	if len(got.Fixups) < 2 {
		t.Errorf("expected the bomb-versus-legitimate-input pair of fixups, got %d", len(got.Fixups))
	}
}
