package shape_test

import stderrors "errors"

func errorsAs(err error, target any) bool { return stderrors.As(err, target) }
