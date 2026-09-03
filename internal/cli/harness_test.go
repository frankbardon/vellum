package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	vellumcli "github.com/frankbardon/vellum/internal/cli"
)

// cliResult is what one in-process invocation of the CLI produced.
type cliResult struct {
	Stdout   string
	Stderr   string
	Err      error
	ExitCode int
}

// runCLI builds a fresh root command per call — urfave/cli/v3 commands carry
// run-once state, so a fresh tree per invocation is what makes this safe to
// call repeatedly across subtests — feeds stdin, and runs args exactly as
// cmd/vellum/main.go would, positional arguments included.
func runCLI(t *testing.T, stdin string, args ...string) cliResult {
	t.Helper()
	app := vellumcli.New("test")
	var stdout, stderr bytes.Buffer
	app.Reader = strings.NewReader(stdin)
	app.Writer = &stdout
	app.ErrWriter = &stderr

	full := append([]string{"vellum"}, args...)
	err := app.Run(context.Background(), full)

	return cliResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Err:      err,
		ExitCode: vellumcli.CodeOf(err),
	}
}

// envelope mirrors descriptor.Envelope's wire shape loosely enough for a
// test to assert against without importing descriptor's own type, which
// would make the test at least as coupled as importing it directly — this
// is deliberately the same fields, decoded independently, so a test that
// parses a --json response is really checking the wire shape rather than
// trusting the producer's own struct tags.
type envelope struct {
	FormatVersion string          `json:"format_version"`
	Data          json.RawMessage `json:"data"`
	Errors        []envelopeEntry `json:"errors"`
	Warnings      []envelopeEntry `json:"warnings"`
}

type envelopeEntry struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

func decodeEnvelope(t *testing.T, raw string) envelope {
	t.Helper()
	var env envelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("stdout is not a valid envelope: %v\nstdout:\n%s", err, raw)
	}
	if env.Errors == nil || env.Warnings == nil {
		t.Errorf("envelope has a nil errors or warnings array: %+v", env)
	}
	return env
}

// assertOneError checks that env carries exactly one error, under code.
// code is a typed [verr.Code] rather than a bare string literal so this
// comparison cannot be mistaken by TestClaudeMdMentionsAllEnvVars for a
// VELLUM_-prefixed environment variable name — that gate walks the AST for
// string literals matching that shape, and an error code compared as a
// literal string in a test file outside errors/ looks identical to one.
func assertOneError(t *testing.T, env envelope, code verr.Code) {
	t.Helper()
	if len(env.Errors) != 1 {
		t.Fatalf("errors = %+v, want exactly one", env.Errors)
	}
	if env.Errors[0].Code != string(code) {
		t.Errorf("error code = %q, want %q", env.Errors[0].Code, code)
	}
}

// writeTempFile writes contents to name under t.TempDir() and returns the
// full path.
func writeTempFile(t *testing.T, name, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

const validSpecJSON = `{
  "sections": [
    {"blocks": [
      {"kind": "heading", "heading": {"level": 1, "content": "Title"}},
      {"kind": "text", "text": {"content": "Body."}}
    ]}
  ]
}`

// markerSpecJSON composes to a document whose body text is literally
// "Dear {{name}}," — discoverable as a marker anchor named "name" by
// anchor.Discover, which is what makes a docx built from this a usable fill
// target without needing to hand-assemble OOXML byte for byte.
const markerSpecJSON = `{
  "sections": [
    {"blocks": [
      {"kind": "text", "text": {"content": "Dear {{name}},"}}
    ]}
  ]
}`

// buildDOCXFixture composes markerSpecJSON to a DOCX file under t.TempDir()
// and returns its path, for fill and inspect tests that need a real
// fillable template rather than hand-assembled OOXML.
func buildDOCXFixture(t *testing.T) string {
	t.Helper()
	specPath := writeTempFile(t, "marker-spec.json", markerSpecJSON)
	outPath := filepath.Join(t.TempDir(), "template.docx")
	res := runCLI(t, "", "compose", specPath, "--format", "docx", "-o", outPath)
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("building the DOCX fixture failed: exit=%d stderr=%s", res.ExitCode, res.Stderr)
	}
	return outPath
}
