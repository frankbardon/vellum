package cli

import "github.com/urfave/cli/v3"

// New builds the root *cli.Command: every verb FR-U2 names, wired against
// the library facade. cmd/vellum/main.go's whole job is to call this and run
// the result — no flag parsing, no facade calls, no business logic live
// there, per CLAUDE.md's "The library is the deliverable; the CLI is an
// adapter."
func New(version string) *cli.Command {
	return &cli.Command{
		Name:    "vellum",
		Usage:   "spec in, document out",
		Version: version,
		Description: "Vellum composes DOCX, XLSX, PPTX and PDF/A-2b from a declarative block\n" +
			"specification, and fills existing OOXML templates with bound data without\n" +
			"disturbing the parts it does not understand. Every command accepts --json\n" +
			"to write a descriptor.Envelope to stdout instead of the human-readable\n" +
			"default, except schema, which always writes raw JSON Schema.",
		Commands: []*cli.Command{
			newComposeCommand(),
			newFillCommand(),
			newInspectCommand(),
			newValidateCommand(),
			newBoxesCommand(),
			newCapabilitiesCommand(),
			newSchemaCommand(),
			newProvenanceCommand(),
			newMCPCommand(),
			newDoctorCommand(),
		},
	}
}
