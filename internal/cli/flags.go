package cli

import (
	"strings"

	"github.com/frankbardon/vellum/artifact"
	"github.com/urfave/cli/v3"
)

// formatUsage lists the accepted --format values, for a flag's Usage text and
// for a usage error's fixup-friendly detail.
func formatUsage() string {
	all := artifact.AllFormats()
	names := make([]string, len(all))
	for i, f := range all {
		names[i] = string(f)
	}
	return strings.Join(names, ", ")
}

// formatFlag is the --format flag shared by compose, validate, boxes and
// capabilities.
func formatFlag(required bool) *cli.StringFlag {
	return &cli.StringFlag{
		Name:     "format",
		Aliases:  []string{"f"},
		Usage:    "output format: " + formatUsage(),
		Required: required,
	}
}

// jsonFlag is the --json flag every verb but schema (which is always raw
// JSON, per CLAUDE.md's documented exception) accepts.
var jsonFlag = &cli.BoolFlag{
	Name:  "json",
	Usage: "wrap the result in a descriptor.Envelope and write it to stdout",
}

// outputFlag is the -o/--output flag compose and fill accept for the
// artifact they write.
var outputFlag = &cli.StringFlag{
	Name:    "output",
	Aliases: []string{"o"},
	Usage:   "file to write the artifact to; stdout when omitted",
}

// parseFormat resolves cmd's --format flag, or a usage error naming the
// accepted set.
func parseFormat(raw string) (artifact.Format, error) {
	f, ok := artifact.ParseFormat(raw)
	if !ok {
		return "", usageErrf("unrecognised --format value", map[string]any{
			"format": raw,
			"accepted": func() []string {
				all := artifact.AllFormats()
				out := make([]string, len(all))
				for i, v := range all {
					out[i] = string(v)
				}
				return out
			}(),
		})
	}
	return f, nil
}
