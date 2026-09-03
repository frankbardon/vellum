package cli

import "github.com/urfave/cli/v3"

// reportError writes err to cmd's output — the --json envelope on cmd.Writer
// when asJSON, or a human-readable line on cmd.ErrWriter otherwise — and
// returns it unchanged, so the caller can simply `return reportError(...)`
// and let [urfave/cli/v3.Command.Run] propagate it up to
// cmd/vellum/main.go, which reads the process exit code back out with
// [CodeOf].
//
// err is expected to already be one of this package's own constructors
// (usageErr, failErr, or one of the code-specific helpers), so it already
// carries the exit code; reportError only decides where to write it.
func reportError(cmd *cli.Command, asJSON bool, err error) error {
	if err == nil {
		return nil
	}
	if asJSON {
		if werr := writeEnvelopeError(cmd.Writer, err); werr != nil {
			return failErr(werr)
		}
		return err
	}
	printHumanError(cmd.ErrWriter, err)
	return err
}
