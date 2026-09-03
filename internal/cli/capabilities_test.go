package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	vellumcli "github.com/frankbardon/vellum/internal/cli"
)

func TestCapabilities_JSONWrapsMatrix(t *testing.T) {
	res := runCLI(t, "", "capabilities", "--format", "pdf", "--json")
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.ExitCode, res.Stderr, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
	var matrix []struct {
		Feature string `json:"feature"`
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(env.Data, &matrix); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if len(matrix) == 0 {
		t.Error("matrix is empty, want the declared PDF rows")
	}
}

func TestCapabilities_HumanModePrintsATable(t *testing.T) {
	res := runCLI(t, "", "capabilities", "--format", "docx")
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Feature") || !strings.Contains(res.Stdout, "Outcome") {
		t.Errorf("human output = %q, want a Feature/Outcome table", res.Stdout)
	}
}

func TestCapabilities_InvalidFormatIsUsageError(t *testing.T) {
	res := runCLI(t, "", "capabilities", "--format", "bogus", "--json")
	if res.ExitCode != vellumcli.ExitUsage {
		t.Fatalf("exit=%d, want %d", res.ExitCode, vellumcli.ExitUsage)
	}
}
