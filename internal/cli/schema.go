package cli

import (
	"context"

	"github.com/frankbardon/vellum/descriptor"
	"github.com/urfave/cli/v3"
)

// newSchemaCommand builds the schema verb.
//
// This is the one verb CLAUDE.md's Output Format Contract carves out
// explicitly: "vellum schema writes the raw JSON Schema unwrapped, because
// the artifact is self-contained and carries its own $schema and $id." It
// therefore accepts no --json flag at all — there is no wrapped and unwrapped
// mode to choose between, only the one documented output.
func newSchemaCommand() *cli.Command {
	return &cli.Command{
		Name:   "schema",
		Usage:  "write the published JSON Schema for a specification",
		Action: runSchema,
	}
}

func runSchema(ctx context.Context, cmd *cli.Command) error {
	raw := descriptor.BuildPayloadSchema()
	_, err := cmd.Writer.Write(raw)
	if err != nil {
		return failErr(err)
	}
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		_, err = cmd.Writer.Write([]byte("\n"))
	}
	if err != nil {
		return failErr(err)
	}
	return nil
}
