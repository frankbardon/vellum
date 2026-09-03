package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	vellumcli "github.com/frankbardon/vellum/internal/cli"
	"github.com/frankbardon/vellum/opc"
)

func TestCompose_JSONWithOutputWritesEnvelopeAndArtifact(t *testing.T) {
	specPath := writeTempFile(t, "spec.json", validSpecJSON)
	outPath := filepath.Join(t.TempDir(), "out.docx")

	res := runCLI(t, "", "compose", specPath, "--format", "docx", "-o", outPath, "--json")
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.ExitCode, res.Stderr, res.Stdout)
	}

	env := decodeEnvelope(t, res.Stdout)
	if len(env.Errors) != 0 {
		t.Errorf("errors = %+v, want none", env.Errors)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading %s: %v", outPath, err)
	}
	if _, err := opc.Open(bytes.NewReader(raw), int64(len(raw))); err != nil {
		t.Errorf("compose wrote a file opc.Open cannot open: %v", err)
	}
}

func TestCompose_NoFlagsWritesArtifactBytesToStdout(t *testing.T) {
	specPath := writeTempFile(t, "spec.json", validSpecJSON)

	res := runCLI(t, "", "compose", specPath, "--format", "docx")
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s", res.ExitCode, res.Stderr)
	}

	raw := []byte(res.Stdout)
	if _, err := opc.Open(bytes.NewReader(raw), int64(len(raw))); err != nil {
		n := len(raw)
		if n > 16 {
			n = 16
		}
		t.Errorf("stdout is not an openable package: %v\nfirst bytes: %q", err, raw[:n])
	}
}

func TestCompose_ReadsSpecFromStdin(t *testing.T) {
	res := runCLI(t, validSpecJSON, "compose", "--format", "docx")
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s", res.ExitCode, res.Stderr)
	}
	raw := []byte(res.Stdout)
	if _, err := opc.Open(bytes.NewReader(raw), int64(len(raw))); err != nil {
		t.Errorf("stdout (from stdin input) is not an openable package: %v", err)
	}
}

func TestCompose_JSONWithoutOutputIsUsageError(t *testing.T) {
	specPath := writeTempFile(t, "spec.json", validSpecJSON)

	res := runCLI(t, "", "compose", specPath, "--format", "docx", "--json")
	if res.ExitCode != vellumcli.ExitUsage {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitUsage, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_OUTPUT_CONFLICT)
}

func TestCompose_InvalidFormatIsUsageError(t *testing.T) {
	specPath := writeTempFile(t, "spec.json", validSpecJSON)
	outPath := filepath.Join(t.TempDir(), "out.docx")

	res := runCLI(t, "", "compose", specPath, "--format", "bogus", "-o", outPath, "--json")
	if res.ExitCode != vellumcli.ExitUsage {
		t.Fatalf("exit=%d, want %d", res.ExitCode, vellumcli.ExitUsage)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_USAGE)
}

func TestCompose_MissingSpecFileIsInputNotFound(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "out.docx")
	res := runCLI(t, "", "compose", "/does/not/exist.json", "--format", "docx", "-o", outPath, "--json")
	if res.ExitCode != vellumcli.ExitUsage {
		t.Fatalf("exit=%d, want %d", res.ExitCode, vellumcli.ExitUsage)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_INPUT_NOT_FOUND)
}

func TestCompose_InvalidSpecIsOperationFailure(t *testing.T) {
	specPath := writeTempFile(t, "spec.json", `{"not": "a valid spec"}`)
	outPath := filepath.Join(t.TempDir(), "out.docx")

	res := runCLI(t, "", "compose", specPath, "--format", "docx", "-o", outPath, "--json")
	if res.ExitCode != vellumcli.ExitFailure {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitFailure, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_SPEC_INVALID)
}

func TestCompose_HumanModeReportsWhereItWrote(t *testing.T) {
	specPath := writeTempFile(t, "spec.json", validSpecJSON)
	outPath := filepath.Join(t.TempDir(), "out.docx")

	res := runCLI(t, "", "compose", specPath, "--format", "docx", "-o", outPath)
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, outPath) {
		t.Errorf("human output = %q, want it to mention %q", res.Stdout, outPath)
	}
}
