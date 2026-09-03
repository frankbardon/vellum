package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	vellumcli "github.com/frankbardon/vellum/internal/cli"
)

func TestInspect_JSONWrapsInspectReport(t *testing.T) {
	tmplPath := buildDOCXFixture(t)

	res := runCLI(t, "", "inspect", tmplPath, "--json")
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.ExitCode, res.Stderr, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
	var report struct {
		Anchors []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"anchors"`
	}
	if err := json.Unmarshal(env.Data, &report); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	found := false
	for _, a := range report.Anchors {
		if a.Name == "name" && a.Kind == "marker" {
			found = true
		}
	}
	if !found {
		t.Errorf("anchors = %+v, want a marker anchor named %q", report.Anchors, "name")
	}
}

func TestInspect_HumanModePrintsAnchorsAndFontsTables(t *testing.T) {
	tmplPath := buildDOCXFixture(t)

	res := runCLI(t, "", "inspect", tmplPath)
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Anchors:") || !strings.Contains(res.Stdout, "Fonts:") {
		t.Errorf("human output = %q, want an Anchors: and a Fonts: section", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "name") {
		t.Errorf("human output = %q, want the marker anchor named %q", res.Stdout, "name")
	}
}

func TestInspect_MissingFileIsInputNotFound(t *testing.T) {
	res := runCLI(t, "", "inspect", "/does/not/exist.docx", "--json")
	if res.ExitCode != vellumcli.ExitUsage {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitUsage, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_INPUT_NOT_FOUND)
}
