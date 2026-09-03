package cli

import (
	"context"
	"fmt"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/spec"
	"github.com/urfave/cli/v3"
)

// validateResult is the --json data shape for the validate verb: a nil
// Errors is always what a successful Validate call returns, but the shape is
// carried explicitly rather than "data": null so a consumer can branch on
// Valid without inspecting the envelope's own top-level Errors array, which
// also fills on this path.
type validateResult struct {
	Valid    bool               `json:"valid"`
	Warnings []*verr.CodedError `json:"warnings"`
}

func newValidateCommand() *cli.Command {
	return &cli.Command{
		Name:      "validate",
		Usage:     "check a specification against a format's capability matrix",
		ArgsUsage: "[spec-file]",
		Description: "Reads a specification (JSON or YAML) from spec-file, or from stdin when\n" +
			"spec-file is omitted, and reports every degradation and rejection the\n" +
			"capability matrix declares for --format, without writing an artifact.",
		Flags:  []cli.Flag{formatFlag(true), jsonFlag},
		Action: runValidate,
	}
}

func runValidate(ctx context.Context, cmd *cli.Command) error {
	asJSON := cmd.Bool("json")

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

	warnings, verr2 := v.Validate(ctx, s, format)
	if verr2 != nil {
		return reportError(cmd, asJSON, failErr(verr2))
	}

	result := validateResult{Valid: true, Warnings: warnings}
	if asJSON {
		if werr := writeEnvelope(cmd.Writer, result); werr != nil {
			return failErr(werr)
		}
		return nil
	}

	if len(warnings) == 0 {
		fmt.Fprintln(cmd.Writer, "valid: no degradations")
		return nil
	}
	fmt.Fprintf(cmd.Writer, "valid: %d degradation(s)\n", len(warnings))
	printHumanWarnings(cmd.Writer, warnings)
	return nil
}
