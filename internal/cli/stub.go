package cli

import (
	"context"

	"github.com/urfave/cli/v3"
)

// newDoctorCommand is the mcp verb's remaining sibling stub: mcp itself
// moved to mcp.go once E12-S2 wired it for real, the same move every other
// verb already made out of this file when it stopped being a stub.
func newDoctorCommand() *cli.Command {
	return &cli.Command{
		Name:   "doctor",
		Usage:  "check the local environment for optional tooling (not yet implemented — lands in E12-S4)",
		Flags:  []cli.Flag{jsonFlag},
		Action: notImplementedAction("doctor", "E12-S4"),
	}
}

// notImplementedAction builds an Action that reports VELLUM_CLI_NOT_IMPLEMENTED
// through the same --json/human split every other verb uses, naming which
// story lands the real behaviour.
func notImplementedAction(verb, landsIn string) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		return reportError(cmd, cmd.Bool("json"), notImplementedErr(verb, landsIn))
	}
}
