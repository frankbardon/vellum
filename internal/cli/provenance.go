package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/provenance"
	"github.com/urfave/cli/v3"
)

// pdfMagic is the byte prefix every PDF file starts with — the sniff
// [runProvenance] uses to decide between [provenance.ExtractPDF] and
// [opc.Open] + [provenance.Extract], without shelling out to any tool that
// might actually parse the file, per FR-P3.
var pdfMagic = []byte("%PDF")

// provenanceReport is the --json data shape for the provenance verb.
// Present is carried explicitly, rather than leaving Record nil and letting
// "data": null speak for it, so a --json consumer can branch on one boolean
// field without special-casing a null payload.
type provenanceReport struct {
	Present bool               `json:"present"`
	Record  *provenance.Record `json:"record,omitempty"`
}

func newProvenanceCommand() *cli.Command {
	return &cli.Command{
		Name:      "provenance",
		Usage:     "report an artifact's own embedded provenance record",
		ArgsUsage: "<artifact-file>",
		Description: "Reads a produced DOCX, XLSX, PPTX or PDF and reports its embedded\n" +
			"provenance record, if it carries one, without opening it in any external\n" +
			"tool. Most artifacts carry none today — Compose does not yet populate it\n" +
			"automatically — and that is reported honestly rather than as an error.",
		Flags:  []cli.Flag{jsonFlag},
		Action: runProvenance,
	}
}

func runProvenance(ctx context.Context, cmd *cli.Command) error {
	asJSON := cmd.Bool("json")

	if cmd.Args().Len() == 0 {
		return reportError(cmd, asJSON, usageErrf("provenance requires an artifact file path", nil))
	}
	path := cmd.Args().Get(0)
	data, rerr := requireFile(path)
	if rerr != nil {
		return reportError(cmd, asJSON, rerr)
	}

	record, perr := extractProvenance(path, data)
	if perr != nil {
		return reportError(cmd, asJSON, perr)
	}

	result := provenanceReport{Present: record != nil, Record: record}
	if asJSON {
		if werr := writeEnvelope(cmd.Writer, result); werr != nil {
			return failErr(werr)
		}
		return nil
	}

	if record == nil {
		fmt.Fprintln(cmd.Writer, "no provenance embedded")
		return nil
	}
	printProvenanceHuman(cmd.Writer, record)
	return nil
}

// extractProvenance sniffs data's own container format and calls the
// matching extractor — [provenance.ExtractPDF] for a PDF, [provenance.Extract]
// for an OPC package. A file that is neither is a usage error naming what was
// found, rather than a silent "no provenance": a caller pointing this at the
// wrong file should learn that, not conclude their artifact carries none.
func extractProvenance(path string, data []byte) (*provenance.Record, error) {
	if bytes.HasPrefix(data, pdfMagic) {
		r, err := provenance.ExtractPDF(data)
		if err != nil {
			return nil, failErr(err)
		}
		return r, nil
	}

	pkg, err := opc.Open(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, usageErrf("the file is neither a PDF nor a recognisable OOXML package",
			map[string]any{"path": path})
	}
	r, err := provenance.Extract(pkg)
	if err != nil {
		return nil, failErr(err)
	}
	return r, nil
}

func printProvenanceHuman(w io.Writer, r *provenance.Record) {
	fmt.Fprintf(w, "vellum_version: %s\n", r.VellumVersion)
	fmt.Fprintf(w, "source_date_epoch: %s\n", r.SourceDateEpoch.UTC().Format("2006-01-02T15:04:05Z"))
	if r.GeneratedAt != nil {
		fmt.Fprintf(w, "generated_at: %s\n", r.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z"))
	}
	fmt.Fprintf(w, "deterministic: %v\n", r.Deterministic())
	if r.SpecHash != "" {
		fmt.Fprintf(w, "spec_hash: %s\n", r.SpecHash)
	}
	if r.ThemeHash != "" {
		fmt.Fprintf(w, "theme_hash: %s\n", r.ThemeHash)
	}
	if r.BindingHash != "" {
		fmt.Fprintf(w, "binding_hash: %s\n", r.BindingHash)
	}
	if r.TemplateHash != "" {
		fmt.Fprintf(w, "template_hash: %s\n", r.TemplateHash)
	}
	for _, a := range r.Assets {
		fmt.Fprintf(w, "asset: handle=%q media=%q hash=%s\n", a.Handle, a.Media, a.Hash)
	}
	for _, f := range r.Fonts {
		fmt.Fprintf(w, "font: family=%q embedded=%v substituted_with=%q\n", f.Family, f.Embedded, f.SubstitutedWith)
	}
	for _, s := range r.Sources {
		fmt.Fprintf(w, "source: kind=%q id=%q\n", s.Kind, s.ID)
	}
}
