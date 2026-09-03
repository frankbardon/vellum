package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/template/bind"
	"github.com/urfave/cli/v3"
)

// fillResult is the --json data shape for the fill verb: [template.Result]'s
// non-destructiveness receipt. Result.Package itself carries no JSON shape
// worth wrapping — a filled OPC package is bytes, not data — so only Touched
// travels through the envelope, per the story's own instruction.
type fillResult struct {
	Touched []string `json:"touched"`
}

func newFillCommand() *cli.Command {
	return &cli.Command{
		Name:  "fill",
		Usage: "bind data into an OOXML template",
		Description: "Binds --data (or --data-json, or stdin when neither is given) into\n" +
			"--template according to --binding, and writes the filled package to\n" +
			"-o/--output, or to stdout when --output is omitted and --json was not\n" +
			"requested. Every part outside the anchors the binding actually touched\n" +
			"is byte-identical to the source template.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "template", Required: true, Usage: "path to the OOXML template"},
			&cli.StringFlag{Name: "binding", Required: true, Usage: "path to a binding document (JSON or YAML)"},
			&cli.StringFlag{Name: "data", Usage: "path to the JSON data file; defaults to stdin"},
			&cli.StringFlag{Name: "data-json", Usage: "inline JSON data, instead of --data or stdin"},
			outputFlag,
			jsonFlag,
		},
		Action: runFill,
	}
}

func runFill(ctx context.Context, cmd *cli.Command) error {
	asJSON := cmd.Bool("json")
	outPath := cmd.String("output")

	if asJSON && outPath == "" {
		return reportError(cmd, asJSON, outputConflictErr())
	}

	bindingData, berr := requireFile(cmd.String("binding"))
	if berr != nil {
		return reportError(cmd, asJSON, berr)
	}
	binding, decErr := bind.DecodeAuto(bindingData)
	if decErr != nil {
		return reportError(cmd, asJSON, failErr(decErr))
	}

	scope, serr := fillScope(cmd)
	if serr != nil {
		return reportError(cmd, asJSON, serr)
	}

	f, size, oerr := openReaderAt(cmd.String("template"))
	if oerr != nil {
		return reportError(cmd, asJSON, oerr)
	}
	defer f.Close()

	v, ferr := newFacade()
	if ferr != nil {
		return reportError(cmd, asJSON, failErr(ferr))
	}

	result, fillErr := v.Fill(ctx, f, size, binding, scope)
	if fillErr != nil {
		return reportError(cmd, asJSON, failErr(fillErr))
	}

	var out bytes.Buffer
	if werr := result.Package.WriteTo(&out, zipdet.WriteOptions{}); werr != nil {
		return reportError(cmd, asJSON, failErr(werr))
	}

	if outPath != "" {
		of, cerr := openOutput(outPath)
		if cerr != nil {
			return reportError(cmd, asJSON, cerr)
		}
		_, werr := of.Write(out.Bytes())
		cerr2 := of.Close()
		if werr != nil {
			return reportError(cmd, asJSON, failErr(werr))
		}
		if cerr2 != nil {
			return reportError(cmd, asJSON, failErr(cerr2))
		}
	} else {
		if _, werr := cmd.Writer.Write(out.Bytes()); werr != nil {
			return failErr(werr)
		}
	}

	if asJSON {
		if werr := writeEnvelope(cmd.Writer, fillResult{Touched: result.Touched}); werr != nil {
			return failErr(werr)
		}
		return nil
	}

	if outPath != "" {
		fmt.Fprintf(cmd.Writer, "wrote %s (%d part(s) touched)\n", outPath, len(result.Touched))
	}
	for _, name := range result.Touched {
		fmt.Fprintf(cmd.Writer, "  touched: %s\n", name)
	}
	return nil
}

// fillScope resolves the binding data: --data-json, --data, or stdin, in
// that order — the first one actually set wins, since exactly one input
// source is meaningful and letting two disagree silently would be worse than
// picking one.
func fillScope(cmd *cli.Command) (bind.Scope, error) {
	var raw []byte
	switch {
	case cmd.String("data-json") != "":
		raw = []byte(cmd.String("data-json"))
	case cmd.String("data") != "":
		data, err := requireFile(cmd.String("data"))
		if err != nil {
			return nil, err
		}
		raw = data
	default:
		data, err := io.ReadAll(cmd.Reader)
		if err != nil {
			return nil, usageErrf("reading from stdin failed", map[string]any{"source": stdinSource})
		}
		raw = data
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte("{}")
	}

	var scope bind.Scope
	if err := json.Unmarshal(raw, &scope); err != nil {
		return nil, usageErrf("the data is not valid JSON", map[string]any{"error": err.Error()})
	}
	return scope, nil
}
