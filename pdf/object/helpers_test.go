package object_test

import (
	stderrors "errors"
	"fmt"
)

// errorsAs is a local alias, so the test files read as being about PDF objects
// rather than about which errors package is in scope.
func errorsAs(err error, target any) bool { return stderrors.As(err, target) }

func fmtSscan(s string, a ...any) (int, error) { return fmt.Sscan(s, a...) }
