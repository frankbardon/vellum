package cli

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

func newCapabilitiesCommand() *cli.Command {
	return &cli.Command{
		Name:  "capabilities",
		Usage: "report the declared (feature, format) outcome matrix",
		Description: "Every feature the capability matrix knows about --format, and what happens\n" +
			"to it: renders natively, degrades to a stated alternative, or is rejected\n" +
			"at validate time. Answerable before any specification exists, so a\n" +
			"consumer scheduling an unattended job can learn the answer before it runs.",
		Flags:  []cli.Flag{formatFlag(true), jsonFlag},
		Action: runCapabilities,
	}
}

func runCapabilities(ctx context.Context, cmd *cli.Command) error {
	asJSON := cmd.Bool("json")

	format, ferr := parseFormat(cmd.String("format"))
	if ferr != nil {
		return reportError(cmd, asJSON, ferr)
	}

	v, ferr2 := newFacade()
	if ferr2 != nil {
		return reportError(cmd, asJSON, failErr(ferr2))
	}

	matrix := v.Capabilities(format)

	if asJSON {
		if werr := writeEnvelope(cmd.Writer, matrix); werr != nil {
			return failErr(werr)
		}
		return nil
	}

	if len(matrix) == 0 {
		fmt.Fprintln(cmd.Writer, "no capability rows declared for this format")
		return nil
	}
	rows := [][]string{{"Feature", "Outcome", "Degrades To / Code", "Note"}}
	for _, e := range matrix {
		second := string(e.Code)
		if e.Degrade != "" {
			second = e.Degrade
			if e.Code != "" {
				second += " (" + string(e.Code) + ")"
			}
		}
		rows = append(rows, []string{string(e.Feature), string(e.Outcome), second, e.Note})
	}
	printTable(cmd.Writer, rows)
	return nil
}
