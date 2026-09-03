package cli

import (
	"context"

	"github.com/urfave/cli/v3"
)

// newMCPCommand registers the mcp verb's shape — so it appears in --help and
// in shell completion — without any protocol handling. E12-S2 replaces
// [runNotImplemented] here with the actual MCP server wiring; this story's
// job is only to leave a working command framework for that story to extend.
func newMCPCommand() *cli.Command {
	return &cli.Command{
		Name:   "mcp",
		Usage:  "run the MCP server (not yet implemented — lands in E12-S2)",
		Flags:  []cli.Flag{jsonFlag},
		Action: notImplementedAction("mcp", "E12-S2"),
	}
}

// newDoctorCommand is doctor's counterpart, landing in E12-S4.
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
