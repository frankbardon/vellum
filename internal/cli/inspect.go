package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"
)

func newInspectCommand() *cli.Command {
	return &cli.Command{
		Name:      "inspect",
		Usage:     "report a template's anchor inventory and font requirements",
		ArgsUsage: "[template-file]",
		Description: "Reads an OOXML template from template-file, or from stdin when omitted,\n" +
			"and reports every fillable anchor and font family its own XML declares,\n" +
			"without modifying it and without a binding.",
		Flags:  []cli.Flag{jsonFlag},
		Action: runInspect,
	}
}

func runInspect(ctx context.Context, cmd *cli.Command) error {
	asJSON := cmd.Bool("json")

	r, size, closeFn, oerr := templateReaderAt(cmd)
	if oerr != nil {
		return reportError(cmd, asJSON, oerr)
	}
	defer closeFn()

	v, ferr := newFacade()
	if ferr != nil {
		return reportError(cmd, asJSON, failErr(ferr))
	}

	report, ierr := v.Inspect(ctx, r, size)
	if ierr != nil {
		return reportError(cmd, asJSON, failErr(ierr))
	}

	if asJSON {
		if werr := writeEnvelope(cmd.Writer, report); werr != nil {
			return failErr(werr)
		}
		return nil
	}

	fmt.Fprintln(cmd.Writer, "Anchors:")
	printTable(cmd.Writer, report.AnchorsTable())
	fmt.Fprintln(cmd.Writer)
	fmt.Fprintln(cmd.Writer, "Fonts:")
	printTable(cmd.Writer, report.FontsTable())
	return nil
}

// templateReaderAt resolves the template input every verb that opens one
// (inspect, fill, provenance) needs: the first positional argument as a file
// path, or — when none was given — the whole of stdin buffered into memory,
// since io.ReaderAt has no streaming form. The returned close function is
// always safe to defer, even when it opened nothing.
func templateReaderAt(cmd *cli.Command) (io.ReaderAt, int64, func(), error) {
	if cmd.Args().Len() > 0 {
		path := cmd.Args().Get(0)
		f, size, err := openReaderAt(path)
		if err != nil {
			return nil, 0, func() {}, err
		}
		return f, size, func() { f.Close() }, nil
	}
	data, err := io.ReadAll(cmd.Reader)
	if err != nil {
		return nil, 0, func() {}, usageErrf("reading from stdin failed",
			map[string]any{"source": stdinSource})
	}
	return bytes.NewReader(data), int64(len(data)), func() {}, nil
}
