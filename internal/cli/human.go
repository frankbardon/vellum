package cli

import (
	"fmt"
	"io"
	"sort"

	verr "github.com/frankbardon/vellum/errors"
)

// printHumanError writes err to w in the default (non-JSON) human-readable
// form: the code, the message, and any structured details, one per line and
// sorted by key so the output is deterministic across runs of the same
// error.
func printHumanError(w io.Writer, err error) {
	var ce *verr.CodedError
	if !asCoded(err, &ce) {
		fmt.Fprintf(w, "error: %v\n", err)
		return
	}
	fmt.Fprintf(w, "error: %s: %s\n", ce.Code, ce.Message)
	if len(ce.Details) == 0 {
		return
	}
	keys := make([]string, 0, len(ce.Details))
	for k := range ce.Details {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "  %s: %v\n", k, ce.Details[k])
	}
}

// printHumanWarnings writes every warning in warnings to w, sorted by code —
// the same order [artifact.Report.Warnings] is already documented to carry,
// restated here defensively so this helper's output is deterministic even if
// a caller hands it an unsorted slice.
func printHumanWarnings(w io.Writer, warnings []*verr.CodedError) {
	sorted := make([]*verr.CodedError, len(warnings))
	copy(sorted, warnings)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Code < sorted[j].Code })
	for _, wrn := range sorted {
		fmt.Fprintf(w, "warning: %s: %s\n", wrn.Code, wrn.Message)
	}
}

// asCoded finds the first *errors.CodedError in err's chain.
func asCoded(err error, target **verr.CodedError) bool {
	for err != nil {
		if ce, ok := err.(*verr.CodedError); ok {
			*target = ce
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
