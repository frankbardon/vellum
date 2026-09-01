package font_test

import stderrors "errors"

// errorsAs is a local alias, so these files read as being about fonts rather
// than about which errors package is in scope.
func errorsAs(err error, target any) bool { return stderrors.As(err, target) }
