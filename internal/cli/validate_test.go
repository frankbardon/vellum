package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	vellumcli "github.com/frankbardon/vellum/internal/cli"
)

func TestValidate_ValidSpecReportsValid(t *testing.T) {
	specPath := writeTempFile(t, "spec.json", validSpecJSON)

	res := runCLI(t, "", "validate", specPath, "--format", "docx", "--json")
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.ExitCode, res.Stderr, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	if len(env.Errors) != 0 {
		t.Errorf("errors = %+v, want none", env.Errors)
	}
	var data struct {
		Valid bool `json:"valid"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if !data.Valid {
		t.Error("valid = false, want true for a well-formed specification")
	}
}

// TestValidate_RejectedFeatureIsOperationFailure exercises a real matrix
// rejection: an asset block against xlsx, which capability/matrix.go
// declares Rejects.
func TestValidate_RejectedFeatureIsOperationFailure(t *testing.T) {
	specJSON := `{
		"sections": [
			{"blocks": [
				{"kind": "asset", "asset": {"handle": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="}}
			]}
		]
	}`
	specPath := writeTempFile(t, "spec.json", specJSON)

	res := runCLI(t, "", "validate", specPath, "--format", "xlsx", "--json")
	if res.ExitCode != vellumcli.ExitFailure {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitFailure, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CAPABILITY_REJECTED)
}

func TestValidate_HumanModeReportsDegradationCount(t *testing.T) {
	// A heading against xlsx degrades (capability/matrix.go), which lets this
	// test see the "N degradation(s)" line without needing a rejection.
	specJSON := `{"sections": [{"blocks": [{"kind": "heading", "heading": {"level": 1, "content": "Title"}}]}]}`
	specPath := writeTempFile(t, "spec.json", specJSON)

	res := runCLI(t, "", "validate", specPath, "--format", "xlsx")
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "degradation") {
		t.Errorf("human output = %q, want it to mention a degradation", res.Stdout)
	}
}

func TestValidate_InvalidFormatIsUsageError(t *testing.T) {
	specPath := writeTempFile(t, "spec.json", validSpecJSON)
	res := runCLI(t, "", "validate", specPath, "--format", "bogus", "--json")
	if res.ExitCode != vellumcli.ExitUsage {
		t.Fatalf("exit=%d, want %d", res.ExitCode, vellumcli.ExitUsage)
	}
}
