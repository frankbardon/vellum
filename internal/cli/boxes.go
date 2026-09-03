package cli

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

func newBoxesCommand() *cli.Command {
	return &cli.Command{
		Name:  "boxes",
		Usage: "report the asset slots a theme offers a format",
		Description: "Answerable before any specification exists: the asset slots (role, width,\n" +
			"height) a theme's own master layouts declare for --format, so a host can\n" +
			"pre-render, cache and warm against a theme rather than against a document.",
		Flags: []cli.Flag{
			formatFlag(true),
			jsonFlag,
			&cli.StringFlag{Name: "theme", Usage: "theme id; empty selects the built-in theme"},
		},
		Action: runBoxes,
	}
}

func runBoxes(ctx context.Context, cmd *cli.Command) error {
	asJSON := cmd.Bool("json")

	format, ferr := parseFormat(cmd.String("format"))
	if ferr != nil {
		return reportError(cmd, asJSON, ferr)
	}

	v, ferr2 := newFacade()
	if ferr2 != nil {
		return reportError(cmd, asJSON, failErr(ferr2))
	}

	boxes, berr := v.Boxes(ctx, cmd.String("theme"), format)
	if berr != nil {
		return reportError(cmd, asJSON, failErr(berr))
	}

	if asJSON {
		if werr := writeEnvelope(cmd.Writer, boxes); werr != nil {
			return failErr(werr)
		}
		return nil
	}

	if len(boxes) == 0 {
		fmt.Fprintln(cmd.Writer, "no boxes declared for this (theme, format) pair")
		return nil
	}
	rows := [][]string{{"Role", "Width", "Height"}}
	for _, b := range boxes {
		height := "intrinsic"
		if !b.IntrinsicHeight() {
			height = fmt.Sprintf("%g%s", b.Height.Value, b.Height.Unit)
		}
		rows = append(rows, []string{string(b.Role), fmt.Sprintf("%g%s", b.Width.Value, b.Width.Unit), height})
	}
	printTable(cmd.Writer, rows)
	return nil
}
