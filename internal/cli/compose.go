package cli

import (
	"bytes"
	"context"
	"fmt"

	"github.com/frankbardon/vellum/spec"
	"github.com/urfave/cli/v3"
)

// newComposeCommand builds the compose verb: spec in, artifact out. See
// [runCompose] for the exact input, output and --json rules.
func newComposeCommand() *cli.Command {
	return &cli.Command{
		Name:      "compose",
		Usage:     "render a specification to an artifact",
		ArgsUsage: "[spec-file]",
		Description: "Reads a specification (JSON or YAML) from spec-file, or from stdin when\n" +
			"spec-file is omitted, and renders it to --format. The artifact is written\n" +
			"to -o/--output, or to stdout when --output is omitted and --json was not\n" +
			"requested — the two cannot share one stdout stream, so that combination\n" +
			"is a usage error (VELLUM_CLI_OUTPUT_CONFLICT) rather than corrupted output.",
		Flags:  []cli.Flag{formatFlag(true), outputFlag, jsonFlag},
		Action: runCompose,
	}
}

func runCompose(ctx context.Context, cmd *cli.Command) error {
	asJSON := cmd.Bool("json")
	outPath := cmd.String("output")

	if asJSON && outPath == "" {
		return reportError(cmd, asJSON, outputConflictErr())
	}

	format, ferr := parseFormat(cmd.String("format"))
	if ferr != nil {
		return reportError(cmd, asJSON, ferr)
	}

	data, _, rerr := readInput(cmd, 0)
	if rerr != nil {
		return reportError(cmd, asJSON, rerr)
	}

	s, derr := spec.DecodeAuto(data)
	if derr != nil {
		return reportError(cmd, asJSON, failErr(derr))
	}

	v, ferr2 := newFacade()
	if ferr2 != nil {
		return reportError(cmd, asJSON, failErr(ferr2))
	}

	// Composed into memory first, never streamed straight to the
	// destination: Compose's own contract is that no byte reaches its
	// writer until resolution and lowering both succeed, and buffering here
	// extends that same guarantee to the file or stream this command
	// eventually writes it to — a failure never leaves a truncated artifact
	// behind.
	var artifactBuf bytes.Buffer
	report, cerr := v.Compose(ctx, s, format, &artifactBuf)
	if cerr != nil {
		return reportError(cmd, asJSON, failErr(cerr))
	}

	if outPath != "" {
		f, oerr := openOutput(outPath)
		if oerr != nil {
			return reportError(cmd, asJSON, oerr)
		}
		_, werr := f.Write(artifactBuf.Bytes())
		cerr2 := f.Close()
		if werr != nil {
			return reportError(cmd, asJSON, failErr(werr))
		}
		if cerr2 != nil {
			return reportError(cmd, asJSON, failErr(cerr2))
		}
	} else {
		// Neither --json nor -o: the artifact itself is the output.
		if _, werr := cmd.Writer.Write(artifactBuf.Bytes()); werr != nil {
			return failErr(werr)
		}
	}

	if asJSON {
		if werr := writeEnvelope(cmd.Writer, report); werr != nil {
			return failErr(werr)
		}
		return nil
	}

	if outPath != "" {
		fmt.Fprintf(cmd.Writer, "wrote %s (format=%s, %d warning(s))\n", outPath, format, len(report.Warnings))
	}
	printHumanWarnings(cmd.Writer, report.Warnings)
	return nil
}
