package gates

import (
	"strings"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
)

// TestNoPulseCodes holds the reverse of ingest's protocol boundary: it reads a
// Pulse envelope's JSON, never imports Pulse, and never re-emits Pulse's own
// error vocabulary as if it were Vellum's.
//
// Two ways that boundary can leak, checked separately because they fail
// differently. A stray import is caught by the same dependency-graph walk the
// other firewalls in this file use — moduleDeps already asserts it is
// non-trivial, so a mis-scoped pattern cannot pass this vacuously. A stray
// error code is not an import at all: nothing stops a future PR from adding
// `errors.Code("PULSE_SOMETHING")` by hand, wiring it into ingest without
// ever importing the module it borrowed the prefix from — so the registry
// itself is scanned as well.
func TestNoPulseCodes(t *testing.T) {
	for _, dep := range moduleDeps(t) {
		if strings.Contains(strings.ToLower(dep), "pulse") {
			t.Errorf("%s is in the dependency graph.\n\n"+
				"Vellum reads a Pulse envelope's JSON at the protocol boundary; it does not import "+
				"Pulse. Find the importer with:\n  go mod why %s", dep, dep)
		}
	}

	for _, code := range verr.AllCodes() {
		if strings.HasPrefix(string(code), "PULSE_") {
			t.Errorf("%s carries Pulse's own error prefix.\n\n"+
				"Every Vellum code is VELLUM_<AREA>_<CATEGORY>. A code that reads as Pulse's own "+
				"vocabulary, wired in by hand rather than by import, is the leak this gate exists "+
				"to catch — the dependency-graph half of this test cannot see it.", code)
		}
	}
}
