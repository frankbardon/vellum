package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	vellumcli "github.com/frankbardon/vellum/internal/cli"
)

// doctorData mirrors DoctorReport's wire shape, decoded independently from
// the producer's own struct tags — the same reasoning envelope (in
// harness_test.go) is decoded independently.
type doctorData struct {
	OK     bool `json:"ok"`
	Checks []struct {
		Name   string `json:"name"`
		OK     bool   `json:"ok"`
		Detail string `json:"detail"`
	} `json:"checks"`
}

// cleanDoctorEnv unsets every environment variable doctor reads, in every
// subtest that runs it, so the result does not depend on whatever the
// developer's or CI's own shell happens to export.
func cleanDoctorEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"VELLUM_THEME_DIR",
		"VELLUM_ASSET_DIR",
		"VELLUM_MAX_ASSET_BYTES",
		"VELLUM_SOURCE_DATE_EPOCH",
	} {
		t.Setenv(name, "")
	}
}

func TestDoctor_JSONHealthyEnvironmentSucceeds(t *testing.T) {
	cleanDoctorEnv(t)
	dir := t.TempDir()

	res := runCLI(t, "", "doctor", "--json", "--dir", dir)
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.ExitCode, res.Stderr, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
	var data doctorData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if !data.OK {
		t.Errorf("data.ok = false, want true: %+v", data.Checks)
	}
	if len(data.Checks) == 0 {
		t.Error("checks is empty, want at least the built-in theme and ICC profile checks")
	}
	for _, c := range data.Checks {
		if !c.OK {
			t.Errorf("check %q failed unexpectedly: %s", c.Name, c.Detail)
		}
	}
}

func TestDoctor_HumanModePrintsATable(t *testing.T) {
	cleanDoctorEnv(t)
	dir := t.TempDir()

	res := runCLI(t, "", "doctor", "--dir", dir)
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.ExitCode, res.Stderr, res.Stdout)
	}
	if !strings.Contains(res.Stdout, "Check") || !strings.Contains(res.Stdout, "Status") {
		t.Errorf("human output = %q, want a Check/Status table", res.Stdout)
	}
}

func TestDoctor_UnreadableThemeDirIsReported(t *testing.T) {
	cleanDoctorEnv(t)
	dir := t.TempDir()
	t.Setenv("VELLUM_THEME_DIR", dir+"/does-not-exist")

	res := runCLI(t, "", "doctor", "--json", "--dir", dir)
	if res.ExitCode != vellumcli.ExitFailure {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitFailure, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_DOCTOR_FAILED)

	var data doctorData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if data.OK {
		t.Error("data.ok = true, want false")
	}
	found := false
	for _, c := range data.Checks {
		if c.Name == "env.vellum_theme_dir" {
			found = true
			if c.OK {
				t.Errorf("env.vellum_theme_dir check passed, want it to fail for a non-existent directory")
			}
		}
	}
	if !found {
		t.Errorf("no env.vellum_theme_dir check in %+v", data.Checks)
	}
}

func TestDoctor_MalformedMaxAssetBytesIsReported(t *testing.T) {
	cleanDoctorEnv(t)
	dir := t.TempDir()
	t.Setenv("VELLUM_MAX_ASSET_BYTES", "not-a-number")

	res := runCLI(t, "", "doctor", "--json", "--dir", dir)
	if res.ExitCode != vellumcli.ExitFailure {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitFailure, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_DOCTOR_FAILED)

	var data doctorData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	for _, c := range data.Checks {
		if c.Name == "env.vellum_max_asset_bytes" && c.OK {
			t.Errorf("env.vellum_max_asset_bytes check passed, want it to fail for %q", "not-a-number")
		}
	}
}

func TestDoctor_MalformedSourceDateEpochIsReported(t *testing.T) {
	cleanDoctorEnv(t)
	dir := t.TempDir()
	t.Setenv("VELLUM_SOURCE_DATE_EPOCH", "not-a-timestamp")

	res := runCLI(t, "", "doctor", "--json", "--dir", dir)
	if res.ExitCode != vellumcli.ExitFailure {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitFailure, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_DOCTOR_FAILED)
}

func TestDoctor_WellFormedSourceDateEpochPasses(t *testing.T) {
	cleanDoctorEnv(t)
	dir := t.TempDir()
	t.Setenv("VELLUM_SOURCE_DATE_EPOCH", "2020-01-01T00:00:00Z")

	res := runCLI(t, "", "doctor", "--json", "--dir", dir)
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.ExitCode, res.Stderr, res.Stdout)
	}
}

func TestDoctor_UnwritableDirIsReported(t *testing.T) {
	cleanDoctorEnv(t)

	res := runCLI(t, "", "doctor", "--json", "--dir", t.TempDir()+"/does-not-exist")
	if res.ExitCode != vellumcli.ExitFailure {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitFailure, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_DOCTOR_FAILED)

	var data doctorData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	for _, c := range data.Checks {
		if c.Name == "write.permissions" && c.OK {
			t.Error("write.permissions check passed, want it to fail against a non-existent directory")
		}
	}
}

func TestDoctor_EveryCheckStillRunsAfterAnEarlyFailure(t *testing.T) {
	cleanDoctorEnv(t)
	t.Setenv("VELLUM_THEME_DIR", "/definitely/does/not/exist")

	res := runCLI(t, "", "doctor", "--json", "--dir", t.TempDir())
	env := decodeEnvelope(t, res.Stdout)
	var data doctorData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	// The ICC profile and write-permission checks are unrelated to
	// VELLUM_THEME_DIR and must still be present and passing: one failing
	// check must never short-circuit the rest.
	seen := map[string]bool{}
	for _, c := range data.Checks {
		seen[c.Name] = true
		if c.Name == "icc.srgb_profile" && !c.OK {
			t.Errorf("icc.srgb_profile failed: %s", c.Detail)
		}
		if c.Name == "write.permissions" && !c.OK {
			t.Errorf("write.permissions failed: %s", c.Detail)
		}
	}
	if !seen["icc.srgb_profile"] || !seen["write.permissions"] || !seen["theme.builtin"] {
		t.Errorf("not every check ran: %+v", data.Checks)
	}
}
