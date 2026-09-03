package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/frankbardon/vellum/doc"
	verr "github.com/frankbardon/vellum/errors"
	vellumcli "github.com/frankbardon/vellum/internal/cli"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/provenance"
)

// buildDOCXWithProvenance writes a minimal DOCX carrying rec as its
// provenance record directly through [doc.Document], the same construction
// provenance's own extract_ooxml_test.go uses — this is deliberately not
// routed through the compose verb, because Compose does not populate
// Document.Provenance on its own path yet (a documented, separate gap).
func buildDOCXWithProvenance(t *testing.T, rec *provenance.Record) string {
	t.Helper()
	d := &doc.Document{
		Sections:   []doc.Section{{Page: doc.A4Portrait()}},
		Provenance: rec,
	}
	var buf bytes.Buffer
	if err := d.WriteTo(&buf, doc.WriteOptions{SourceDateEpoch: zipdet.PinnedEpoch}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	path := filepath.Join(t.TempDir(), "with-provenance.docx")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func TestProvenance_JSONReportsAnEmbeddedRecord(t *testing.T) {
	path := buildDOCXWithProvenance(t, &provenance.Record{
		VellumVersion: "1.2.3",
		SpecHash:      "abc123",
	})

	res := runCLI(t, "", "provenance", path, "--json")
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.ExitCode, res.Stderr, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
	var data struct {
		Present bool `json:"present"`
		Record  struct {
			VellumVersion string `json:"vellum_version"`
		} `json:"record"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if !data.Present {
		t.Fatal("present = false, want true for a document that carries provenance")
	}
	if data.Record.VellumVersion != "1.2.3" {
		t.Errorf("VellumVersion = %q, want %q", data.Record.VellumVersion, "1.2.3")
	}
}

func TestProvenance_JSONReportsAbsenceHonestly(t *testing.T) {
	path := buildDOCXWithProvenance(t, nil)

	res := runCLI(t, "", "provenance", path, "--json")
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s", res.ExitCode, res.Stderr)
	}
	env := decodeEnvelope(t, res.Stdout)
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none — absence is not an error", env.Errors)
	}
	var data struct {
		Present bool `json:"present"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if data.Present {
		t.Error("present = true, want false for a document with no embedded provenance")
	}
}

func TestProvenance_HumanModeReportsAbsence(t *testing.T) {
	path := buildDOCXWithProvenance(t, nil)

	res := runCLI(t, "", "provenance", path)
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s", res.ExitCode, res.Stderr)
	}
	if res.Stdout != "no provenance embedded\n" {
		t.Errorf("human output = %q, want %q", res.Stdout, "no provenance embedded\n")
	}
}

func TestProvenance_MissingFileIsInputNotFound(t *testing.T) {
	res := runCLI(t, "", "provenance", "/does/not/exist.docx", "--json")
	if res.ExitCode != vellumcli.ExitUsage {
		t.Fatalf("exit=%d, want %d", res.ExitCode, vellumcli.ExitUsage)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_INPUT_NOT_FOUND)
}

func TestProvenance_MissingArgumentIsUsageError(t *testing.T) {
	res := runCLI(t, "", "provenance", "--json")
	if res.ExitCode != vellumcli.ExitUsage {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitUsage, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_USAGE)
}

func TestProvenance_NotAnArtifactIsUsageError(t *testing.T) {
	path := writeTempFile(t, "not-an-artifact.txt", "just some text")
	res := runCLI(t, "", "provenance", path, "--json")
	if res.ExitCode != vellumcli.ExitUsage {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitUsage, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_USAGE)
}
