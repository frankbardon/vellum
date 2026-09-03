package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	vellumcli "github.com/frankbardon/vellum/internal/cli"
)

func TestBoxes_JSONWrapsBoxSet(t *testing.T) {
	res := runCLI(t, "", "boxes", "--format", "docx", "--json")
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.ExitCode, res.Stderr, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
	var boxes []struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(env.Data, &boxes); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if len(boxes) == 0 {
		t.Error("boxes is empty, want at least the built-in theme's declared roles")
	}
}

func TestBoxes_HumanModePrintsATable(t *testing.T) {
	res := runCLI(t, "", "boxes", "--format", "docx")
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Role") || !strings.Contains(res.Stdout, "Width") {
		t.Errorf("human output = %q, want a Role/Width table", res.Stdout)
	}
}

func TestBoxes_InvalidFormatIsUsageError(t *testing.T) {
	res := runCLI(t, "", "boxes", "--format", "bogus", "--json")
	if res.ExitCode != vellumcli.ExitUsage {
		t.Fatalf("exit=%d, want %d", res.ExitCode, vellumcli.ExitUsage)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_USAGE)
}
