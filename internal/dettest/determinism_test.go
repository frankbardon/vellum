package dettest_test

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/frankbardon/vellum/internal/dettest"
	"github.com/frankbardon/vellum/opc/zipdet"
)

var update = flag.Bool("update", false, "regenerate golden artifacts and the manifest")

// caseEnv names the case a re-executed child process should emit. Its presence
// is also what distinguishes a child run from a normal one.
const caseEnv = "VELLUM_DETTEST_CASE"

// repeatCount is how many times a case is emitted in one process.
//
// A thousand is not superstition: map-iteration order in Go is randomised per
// range statement, so a package with a handful of parts can easily produce the
// canonical order by luck a hundred times running. The count buys confidence
// that "it passed" means "there is no map on the write path" rather than "we
// were lucky".
const repeatCount = 1000

func repeats() int {
	if testing.Short() {
		return 25
	}
	return repeatCount
}

// TestDeterminismRepeat emits every case many times in one process and
// requires exactly one digest.
func TestDeterminismRepeat(t *testing.T) {
	for _, c := range dettest.Cases() {
		t.Run(c.Name, func(t *testing.T) {
			seen := make(map[string]int)
			var first []byte
			n := repeats()
			for i := range n {
				got, err := c.Bytes(zipdet.PinnedEpoch)
				if err != nil {
					t.Fatalf("emit %d: %v", i, err)
				}
				if first == nil {
					first = got
				}
				seen[dettest.Digest(got)]++
			}
			if len(seen) != 1 {
				t.Fatalf("%d emissions produced %d distinct digests, want 1: %v", n, len(seen), seen)
			}
		})
	}
}

// TestDeterminismGOMAXPROCS re-runs the cases at one and at eight procs.
//
// This is where a map on the write path shows itself: iteration order varies
// with scheduling, and a single-threaded run can be accidentally stable.
func TestDeterminismGOMAXPROCS(t *testing.T) {
	original := runtime.GOMAXPROCS(0)
	t.Cleanup(func() { runtime.GOMAXPROCS(original) })

	for _, procs := range []int{1, 8} {
		t.Run(fmt.Sprintf("procs=%d", procs), func(t *testing.T) {
			runtime.GOMAXPROCS(procs)
			for _, c := range dettest.Cases() {
				seen := make(map[string]bool)
				for range 200 {
					got, err := c.Bytes(zipdet.PinnedEpoch)
					if err != nil {
						t.Fatalf("%s: %v", c.Name, err)
					}
					seen[dettest.Digest(got)] = true
				}
				if len(seen) != 1 {
					t.Errorf("%s produced %d distinct digests at GOMAXPROCS=%d", c.Name, len(seen), procs)
				}
			}
		})
	}
}

// TestDeterminismCrossProcess re-executes this test binary in fresh processes
// and compares digests.
//
// In-process repetition cannot catch nondeterminism that is fixed for the
// lifetime of a process — an address-dependent sort, an init-order
// dependency, a hash seed sampled once at startup. Only a new process can, so
// the children are spawned for real rather than simulated.
func TestDeterminismCrossProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-process determinism spawns subprocesses; skipped under -short")
	}
	if os.Getenv(caseEnv) != "" {
		t.Skip("child process")
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locating the test binary: %v", err)
	}

	for _, c := range dettest.Cases() {
		t.Run(c.Name, func(t *testing.T) {
			inProcess, err := c.Bytes(zipdet.PinnedEpoch)
			if err != nil {
				t.Fatalf("emit: %v", err)
			}
			want := dettest.Digest(inProcess)

			for i := range 10 {
				cmd := exec.Command(exe, "-test.run=TestChildEmitsDigest", "-test.v=false")
				cmd.Env = append(os.Environ(), caseEnv+"="+c.Name)
				var out, errOut bytes.Buffer
				cmd.Stdout = &out
				cmd.Stderr = &errOut
				if err := cmd.Run(); err != nil {
					t.Fatalf("child %d failed: %v\nstdout: %s\nstderr: %s", i, err, out.String(), errOut.String())
				}
				got := extractDigest(t, out.String())
				if got != want {
					t.Fatalf("child %d digest %s, parent digest %s; the emitter depends on something that varies per process", i, got, want)
				}
			}
		})
	}
}

// TestChildEmitsDigest is the child-process entry point for
// TestDeterminismCrossProcess. It does nothing in an ordinary run.
//
// Named outside the TestDeterminism prefix on purpose: that prefix is reserved
// for CI gates that CLAUDE.md must list, and this is a helper, not a gate.
func TestChildEmitsDigest(t *testing.T) {
	name := os.Getenv(caseEnv)
	if name == "" {
		t.Skip("not a child process")
	}
	for _, c := range dettest.Cases() {
		if c.Name != name {
			continue
		}
		got, err := c.Bytes(zipdet.PinnedEpoch)
		if err != nil {
			t.Fatalf("emit: %v", err)
		}
		fmt.Printf("DIGEST %s\n", dettest.Digest(got))
		return
	}
	t.Fatalf("unknown case %q", name)
}

func extractDigest(t *testing.T, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "DIGEST "); ok {
			return rest
		}
	}
	t.Fatalf("child produced no digest line; output was:\n%s", out)
	return ""
}

// TestDeterminismEpochInvariance proves that wall-clock time does not reach
// the output, and that an explicit epoch is honoured — stable for a given
// value, and different from the pinned default.
func TestDeterminismEpochInvariance(t *testing.T) {
	explicit := time.Date(2021, time.March, 4, 5, 6, 8, 0, time.UTC)

	for _, c := range dettest.Cases() {
		t.Run(c.Name, func(t *testing.T) {
			pinnedA, err := c.Bytes(zipdet.PinnedEpoch)
			if err != nil {
				t.Fatalf("emit: %v", err)
			}
			time.Sleep(2 * time.Millisecond)
			pinnedB, err := c.Bytes(zipdet.PinnedEpoch)
			if err != nil {
				t.Fatalf("emit: %v", err)
			}
			if !bytes.Equal(pinnedA, pinnedB) {
				t.Fatal("two emissions at different wall-clock times differed; something is reading the clock")
			}

			explicitA, err := c.Bytes(explicit)
			if err != nil {
				t.Fatalf("emit: %v", err)
			}
			explicitB, err := c.Bytes(explicit)
			if err != nil {
				t.Fatalf("emit: %v", err)
			}
			if !bytes.Equal(explicitA, explicitB) {
				t.Error("the same explicit epoch produced different bytes")
			}
			if bytes.Equal(explicitA, pinnedA) {
				t.Error("an explicit epoch produced the same bytes as the pinned default; the option has no effect")
			}
		})
	}
}
