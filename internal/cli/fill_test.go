package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	vellumcli "github.com/frankbardon/vellum/internal/cli"
	"github.com/frankbardon/vellum/opc"
)

const nameBindingJSON = `{
  "format_version": "1.0",
  "statements": [
    {"kind": "bind", "bind": {"anchor": "name", "expr": "name"}}
  ]
}`

func TestFill_JSONReportsTouchedAndProducesAnOpenablePackage(t *testing.T) {
	tmplPath := buildDOCXFixture(t)
	bindingPath := writeTempFile(t, "binding.json", nameBindingJSON)
	outPath := filepath.Join(t.TempDir(), "filled.docx")

	res := runCLI(t, "",
		"fill", "--template", tmplPath, "--binding", bindingPath,
		"--data-json", `{"name":"World"}`, "-o", outPath, "--json")
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.ExitCode, res.Stderr, res.Stdout)
	}

	env := decodeEnvelope(t, res.Stdout)
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
	var data struct {
		Touched []string `json:"touched"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if len(data.Touched) != 1 || data.Touched[0] != "/word/document.xml" {
		t.Errorf("touched = %v, want exactly [/word/document.xml]", data.Touched)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading %s: %v", outPath, err)
	}
	pkg, err := opc.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("fill wrote a file opc.Open cannot open: %v", err)
	}
	part, ok := pkg.Get("/word/document.xml")
	if !ok {
		t.Fatal("filled package has no /word/document.xml")
	}
	docBytes, err := part.Bytes()
	if err != nil {
		t.Fatalf("reading document.xml: %v", err)
	}
	if bytes.Contains(docBytes, []byte("{{name}}")) {
		t.Error("filled document still carries the raw marker {{name}}")
	}
	if !bytes.Contains(docBytes, []byte("World")) {
		t.Error("filled document does not carry the bound value \"World\"")
	}
}

func TestFill_DataFromStdin(t *testing.T) {
	tmplPath := buildDOCXFixture(t)
	bindingPath := writeTempFile(t, "binding.json", nameBindingJSON)
	outPath := filepath.Join(t.TempDir(), "filled.docx")

	res := runCLI(t, `{"name":"Stdin Data"}`,
		"fill", "--template", tmplPath, "--binding", bindingPath, "-o", outPath)
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.ExitCode, res.Stderr, res.Stdout)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading %s: %v", outPath, err)
	}
	if !bytes.Contains(raw, []byte("word/document.xml")) {
		t.Error("output does not look like an OOXML package")
	}
}

func TestFill_JSONWithoutOutputIsUsageError(t *testing.T) {
	tmplPath := buildDOCXFixture(t)
	bindingPath := writeTempFile(t, "binding.json", nameBindingJSON)

	res := runCLI(t, "", "fill", "--template", tmplPath, "--binding", bindingPath,
		"--data-json", `{"name":"World"}`, "--json")
	if res.ExitCode != vellumcli.ExitUsage {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitUsage, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_OUTPUT_CONFLICT)
}

func TestFill_UnreconciledBindingIsOperationFailure(t *testing.T) {
	tmplPath := buildDOCXFixture(t)
	// References an anchor the template does not carry.
	badBinding := `{
	  "format_version": "1.0",
	  "statements": [{"kind": "bind", "bind": {"anchor": "not_a_real_anchor", "expr": "\"x\""}}],
	  "optional_anchors": ["name"]
	}`
	bindingPath := writeTempFile(t, "binding.json", badBinding)
	outPath := filepath.Join(t.TempDir(), "filled.docx")

	res := runCLI(t, "", "fill", "--template", tmplPath, "--binding", bindingPath,
		"--data-json", "{}", "-o", outPath, "--json")
	if res.ExitCode != vellumcli.ExitFailure {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitFailure, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	if len(env.Errors) != 1 {
		t.Fatalf("errors = %+v, want exactly one", env.Errors)
	}
}

func TestFill_MalformedDataJSONIsUsageError(t *testing.T) {
	tmplPath := buildDOCXFixture(t)
	bindingPath := writeTempFile(t, "binding.json", nameBindingJSON)
	outPath := filepath.Join(t.TempDir(), "filled.docx")

	res := runCLI(t, "", "fill", "--template", tmplPath, "--binding", bindingPath,
		"--data-json", "{not valid json", "-o", outPath, "--json")
	if res.ExitCode != vellumcli.ExitUsage {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitUsage, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_USAGE)
}

func TestFill_MissingTemplateIsInputNotFound(t *testing.T) {
	bindingPath := writeTempFile(t, "binding.json", nameBindingJSON)
	outPath := filepath.Join(t.TempDir(), "filled.docx")

	res := runCLI(t, "", "fill", "--template", "/does/not/exist.docx", "--binding", bindingPath,
		"--data-json", "{}", "-o", outPath, "--json")
	if res.ExitCode != vellumcli.ExitUsage {
		t.Fatalf("exit=%d, want %d", res.ExitCode, vellumcli.ExitUsage)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_INPUT_NOT_FOUND)
}
