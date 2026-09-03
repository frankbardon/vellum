package cli_test

import (
	"encoding/json"
	"testing"

	vellumcli "github.com/frankbardon/vellum/internal/cli"
)

// TestSchema_WritesRawUnwrappedJSONSchema pins CLAUDE.md's documented
// exception: schema's output is never a descriptor.Envelope, --json or not.
func TestSchema_WritesRawUnwrappedJSONSchema(t *testing.T) {
	res := runCLI(t, "", "schema")
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s", res.ExitCode, res.Stderr)
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(res.Stdout), &doc); err != nil {
		t.Fatalf("stdout is not valid JSON: %v", err)
	}
	if _, ok := doc["$schema"]; !ok {
		t.Error(`schema output has no "$schema" key`)
	}
	if _, ok := doc["$id"]; !ok {
		t.Error(`schema output has no "$id" key`)
	}
	// The envelope shape must not have leaked in.
	for _, envelopeOnlyKey := range []string{"format_version", "errors", "warnings", "data"} {
		if _, ok := doc[envelopeOnlyKey]; ok {
			t.Errorf("schema output carries envelope key %q; it must be raw, unwrapped JSON Schema", envelopeOnlyKey)
		}
	}
}
