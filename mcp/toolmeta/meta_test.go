package toolmeta_test

import (
	"strings"
	"testing"

	"github.com/frankbardon/vellum/mcp/toolmeta"
)

// wantOrder is FR-U3's own list — "compose, validate, inspect, fill,
// capabilities, boxes, schema, manifest, skills and examples" — pinned so a
// registry reorder is a deliberate edit, not an accident.
var wantOrder = []string{
	toolmeta.NameCompose,
	toolmeta.NameValidate,
	toolmeta.NameInspect,
	toolmeta.NameFill,
	toolmeta.NameCapabilities,
	toolmeta.NameBoxes,
	toolmeta.NameSchema,
	toolmeta.NameManifest,
	toolmeta.NameSkills,
	toolmeta.NameExamples,
}

func TestAllTools_CoversFRU3InOrder(t *testing.T) {
	all := toolmeta.AllTools()
	if len(all) != len(wantOrder) {
		t.Fatalf("AllTools() has %d entries, want %d", len(all), len(wantOrder))
	}
	for i, want := range wantOrder {
		if all[i].Name != want {
			t.Errorf("tool %d = %q, want %q", i, all[i].Name, want)
		}
	}
}

func TestAllTools_EveryNameCarriesThePrefix(t *testing.T) {
	for _, tool := range toolmeta.AllTools() {
		if !strings.HasPrefix(tool.Name, "vellum_") {
			t.Errorf("tool name %q does not carry the vellum_ prefix", tool.Name)
		}
	}
}

func TestAllTools_NoDuplicateNames(t *testing.T) {
	seen := make(map[string]bool)
	for _, tool := range toolmeta.AllTools() {
		if seen[tool.Name] {
			t.Errorf("duplicate tool name %q", tool.Name)
		}
		seen[tool.Name] = true
	}
}

func TestAllTools_EveryToolHasADescription(t *testing.T) {
	for _, tool := range toolmeta.AllTools() {
		if strings.TrimSpace(tool.Description) == "" {
			t.Errorf("tool %q has no description", tool.Name)
		}
	}
}

// TestAllTools_CopyReturning checks that mutating the slice AllTools hands
// back does not affect the registry a later call sees — the same guarantee
// errors.AllCodes and capability.AllFeatures already carry.
func TestAllTools_CopyReturning(t *testing.T) {
	first := toolmeta.AllTools()
	original := first[0].Name
	first[0].Name = "mutated"

	second := toolmeta.AllTools()
	if second[0].Name != original {
		t.Errorf("mutating AllTools()'s result changed a later call: got %q, want %q", second[0].Name, original)
	}
}

func TestAllTools_Deterministic(t *testing.T) {
	first := toolmeta.AllTools()
	second := toolmeta.AllTools()
	if len(first) != len(second) {
		t.Fatalf("AllTools() length changed between calls: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("AllTools()[%d] changed between calls: %+v vs %+v", i, first[i], second[i])
		}
	}
}
