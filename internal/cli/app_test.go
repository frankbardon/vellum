package cli_test

import (
	"testing"

	vellumcli "github.com/frankbardon/vellum/internal/cli"
)

// TestNew_RegistersEveryFRU2Verb pins the command index PRD FR-U2 names, so
// a verb accidentally dropped from the tree fails here rather than being
// noticed only by a human reading --help.
func TestNew_RegistersEveryFRU2Verb(t *testing.T) {
	want := []string{
		"compose", "fill", "inspect", "validate", "boxes",
		"capabilities", "schema", "provenance", "mcp", "doctor",
	}
	app := vellumcli.New("test")

	got := make(map[string]bool, len(app.Commands))
	for _, c := range app.Commands {
		got[c.Name] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("verb %q is not registered", name)
		}
	}
}
