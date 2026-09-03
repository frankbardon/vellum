package cli_test

import (
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	vellumcli "github.com/frankbardon/vellum/internal/cli"
)

func TestDoctor_IsRegisteredButNotImplemented(t *testing.T) {
	res := runCLI(t, "", "doctor", "--json")
	if res.ExitCode != vellumcli.ExitFailure {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitFailure, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_NOT_IMPLEMENTED)
}

