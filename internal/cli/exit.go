// Package cli is the CLI shell over the Vellum library facade: it parses
// flags and calls [vellum.Vellum]'s methods, and contains no business logic
// of its own — every decision about what a specification renders to, what a
// template declares, or what a binding does belongs to the packages this one
// wraps.
//
// One file per verb group, per CLAUDE.md's package map: compose.go,
// validate.go, fill.go, inspect.go, boxes.go, capabilities.go, schema.go,
// provenance.go, and the mcp/doctor stubs. [New] assembles all of them into
// one *cli.Command tree; cmd/vellum/main.go is wiring only.
//
// # Exit codes
//
// CLAUDE.md and the PRD do not pin an exact convention, so this package
// states one, deliberately, since it is effectively public contract once a
// script depends on it:
//
//	0  success — the command ran and produced no errors.
//	1  the command ran, was itself well-formed, and the operation it asked
//	   the facade to perform failed: a rejected specification, a binding
//	   that would not reconcile, a template that would not open. The
//	   coded error is whatever the facade returned, unwrapped.
//	2  usage error — the command itself was malformed: an unrecognised
//	   --format value, a required flag or argument left unset, a file path
//	   that does not exist, or a flag combination this CLI refuses (see
//	   VELLUM_CLI_OUTPUT_CONFLICT). Nothing was attempted.
//
// This mirrors the conventional Unix shape (getopt-style tools use 2 for a
// usage error) and gives a caller scripting against this CLI a way to tell
// "my input was bad" from "my invocation was bad" without parsing stderr.
package cli

import (
	"errors"

	verr "github.com/frankbardon/vellum/errors"
)

const (
	// ExitOK is the process exit code for a command that ran without error.
	ExitOK = 0

	// ExitFailure is the process exit code for a command that ran, was
	// itself well-formed, and reports that the operation it asked for
	// failed — a rejected specification, a template that would not fill.
	ExitFailure = 1

	// ExitUsage is the process exit code for a command whose own invocation
	// was malformed: a bad flag, a missing argument, a file that does not
	// exist. Nothing was attempted.
	ExitUsage = 2
)

// ExitError pairs a process exit code with the error that produced it.
//
// urfave/cli/v3's own os.Exit-on-ExitCoder machinery is deliberately not
// used: it would exit the process before this package had a chance to write
// a --json error envelope to stdout, which the Output Format Contract
// requires happen even on failure. Every Action in this package returns a
// plain error (already reported to the right stream by the Action itself)
// wrapped in one of these, and cmd/vellum/main.go reads the code back out
// with [CodeOf] after [urfave/cli/v3.Command.Run] returns.
type ExitError struct {
	Code int
	Err  error
}

// Error implements error.
func (e *ExitError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

// Unwrap exposes the underlying error to errors.Is and errors.As.
func (e *ExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// usageErr wraps err as a usage failure (exit code 2). When err is not
// already a [*verr.CodedError] it is coded VELLUM_CLI_USAGE, so every usage
// failure this package raises is reportable through the same envelope path
// as a facade error.
func usageErr(err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: ExitUsage, Err: err}
}

// usageErrf constructs a VELLUM_CLI_USAGE error and wraps it as a usage
// failure in one step — the common case, where the CLI itself detected the
// problem rather than propagating one the facade returned.
func usageErrf(message string, details map[string]any) error {
	var e error
	if details == nil {
		e = verr.NewCodedError(verr.VELLUM_CLI_USAGE, message)
	} else {
		e = verr.NewCodedErrorWithDetails(verr.VELLUM_CLI_USAGE, message, details)
	}
	return usageErr(e)
}

// inputNotFoundErr constructs a VELLUM_CLI_INPUT_NOT_FOUND usage failure for
// a named path this command could not open.
func inputNotFoundErr(path string, cause error) error {
	return usageErr(verr.WrapCodedErrorWithDetails(cause, verr.VELLUM_CLI_INPUT_NOT_FOUND,
		"the named input could not be opened", map[string]any{"path": path}))
}

// outputConflictErr constructs the VELLUM_CLI_OUTPUT_CONFLICT usage failure
// for a --json invocation with a binary artifact and no -o/--output: the two
// cannot share one stdout stream.
func outputConflictErr() error {
	return usageErr(verr.NewCodedErrorWithDetails(verr.VELLUM_CLI_OUTPUT_CONFLICT,
		"cannot write a binary artifact and a --json envelope to the same stdout stream",
		map[string]any{"hint": "pass -o/--output to name a file for the artifact"}))
}

// notImplementedErr constructs the VELLUM_CLI_NOT_IMPLEMENTED failure a stub
// verb (mcp, doctor) returns when actually invoked. Exit code 1 rather than
// 2: the invocation itself was fine — the verb exists and was spelled
// correctly — it is running it that does not yet work.
func notImplementedErr(verb, landsIn string) error {
	return failErr(verr.NewCodedErrorWithDetails(verr.VELLUM_CLI_NOT_IMPLEMENTED,
		"this verb is registered but does not run yet",
		map[string]any{"verb": verb, "lands_in": landsIn}))
}

// failErr wraps err as an operation failure (exit code 1): the command was
// well-formed and the facade call it made returned this error.
func failErr(err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: ExitFailure, Err: err}
}

// CodeOf returns the process exit code err implies. A nil error is
// [ExitOK]; any error that is not an [*ExitError] is treated as
// [ExitFailure], which is the conservative default for a failure this
// package did not classify.
func CodeOf(err error) int {
	if err == nil {
		return ExitOK
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return ExitFailure
}
