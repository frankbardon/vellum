package exttool

import (
	stderrors "errors"
	"os/exec"
)

// asExitError reports whether err is an ExitError, which means the program ran
// and returned a status rather than failing to run.
func asExitError(err error, target **exec.ExitError) bool {
	return stderrors.As(err, target)
}
