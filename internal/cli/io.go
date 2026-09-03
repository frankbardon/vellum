package cli

import (
	"io"
	"os"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/urfave/cli/v3"
)

// stdinSource is the source name reported in an error's details when input
// came from stdin rather than a named file.
const stdinSource = "<stdin>"

// readInput reads the argIndex'th positional argument as a file path, or —
// when no such argument was given — every byte of cmd's own Reader (stdin in
// the ordinary case, whatever a test substituted otherwise). It returns the
// bytes and the source name an error should name.
func readInput(cmd *cli.Command, argIndex int) ([]byte, string, error) {
	if cmd.Args().Len() > argIndex {
		path := cmd.Args().Get(argIndex)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, path, inputNotFoundErr(path, err)
		}
		return data, path, nil
	}
	data, err := io.ReadAll(cmd.Reader)
	if err != nil {
		return nil, stdinSource, usageErr(verr.WrapCodedErrorWithDetails(err, verr.VELLUM_CLI_USAGE,
			"reading from stdin failed", map[string]any{"source": stdinSource}))
	}
	return data, stdinSource, nil
}

// requireFile reads path as a required, explicitly-named file — used where a
// command takes more than one file input, so positional-argument-or-stdin
// (see [readInput]) is ambiguous about which input is meant.
func requireFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, inputNotFoundErr(path, err)
	}
	return data, nil
}

// openReaderAt opens path for reading and reports its size, for a facade
// method that wants an io.ReaderAt rather than a byte slice — [Vellum.Fill]
// and [Vellum.Inspect] both read a template that way rather than requiring
// the whole file resident in memory before the call.
//
// The caller is responsible for closing the returned file.
func openReaderAt(path string) (*os.File, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, inputNotFoundErr(path, err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, inputNotFoundErr(path, err)
	}
	return f, info.Size(), nil
}

// openOutput opens path for writing the artifact bytes a compose or fill
// command produces, truncating any existing content.
func openOutput(path string) (*os.File, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, usageErr(verr.WrapCodedErrorWithDetails(err, verr.VELLUM_CLI_USAGE,
			"the output path could not be opened for writing", map[string]any{"path": path}))
	}
	return f, nil
}
