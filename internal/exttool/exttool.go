// Package exttool locates and runs the external programs the test suite uses as
// oracles.
//
// # Why external oracles exist at all
//
// Vellum's own suite compares its bytes against its bytes. That proves
// determinism and proves nothing about whether an artifact opens: a file can be
// byte-identical across a thousand runs and still be one no reader accepts.
// Three defects reached a human that way — zip version fields left at zero,
// ECMA-376 children out of schema order, and a golden "PNG" with no IDAT chunk.
// Every Go-side reader accepted all three.
//
// The tools here are readers Vellum did not write. They read artifacts it did
// and report whether they could.
//
// # What they are not
//
// None of them produces anything Vellum ships, and no assertion compares their
// output for equality — only for presence and content. A conversion's bytes
// vary with the tool's version and the fonts installed beside it, which is
// exactly the nondeterminism Vellum refuses to depend on. Borrowing a reader is
// not the same as taking a dependency on a renderer.
//
// The library runs no subprocess. TestNoExternalToolingOnTheLibraryPath proves
// no shipped package reaches this one.
package exttool

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// EnvRequireOptional names the environment variable that turns a missing tool
// from a skip into a failure.
//
// An optional gate that skips silently is a gate that passes forever without
// ever running, which is worse than not having it: the build stays green and
// the coverage is imaginary. CI sets this, so a runner that loses an
// installation finds out by failing rather than by quietly stopping checking.
//
// One variable for every tool rather than one each, so provisioning is a single
// decision.
const EnvRequireOptional = "VELLUM_REQUIRE_OPTIONAL_GATES"

// RequireOptional reports whether a missing tool should fail rather than skip.
func RequireOptional() bool {
	v := strings.TrimSpace(os.Getenv(EnvRequireOptional))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

// Spec describes how to find one program.
type Spec struct {
	// Name is how the tool is referred to in messages.
	Name string

	// Env is the environment variable holding an explicit path, if the tool has
	// one.
	Env string

	// Commands are the names to look for on the PATH, in preference order.
	Commands []string

	// Paths are conventional install locations, consulted after the PATH. Keys
	// are GOOS values; the empty key applies to every other platform.
	Paths map[string][]string

	// Install is the one-line hint printed when the tool is absent.
	Install string
}

// Tool is a located program.
type Tool struct {
	// Spec is what was looked for.
	Spec Spec

	// Path is the absolute path to the executable.
	Path string
}

// NotFoundError reports that a tool could not be located.
//
// Distinguished from a real failure so an absent tool skips and a broken one
// fails, which are different situations that must not be reported the same way.
type NotFoundError struct {
	// Spec is what was looked for.
	Spec Spec

	// Looked lists the locations consulted, in order.
	Looked []string
}

func (e *NotFoundError) Error() string {
	msg := e.Spec.Name + " was not found. Looked at: " + strings.Join(e.Looked, ", ")
	if e.Spec.Env != "" {
		msg += ". Set " + e.Spec.Env + " to its path"
	}
	if e.Spec.Install != "" {
		msg += ", or install it: " + e.Spec.Install
	}
	return msg
}

// Find locates the program described by spec.
//
// The environment override is consulted first, so a machine with several
// versions is not at the mercy of PATH order — the version matters, because
// tolerances differ between them. An override that does not resolve is a
// configuration mistake rather than an absent tool, and is reported as one:
// falling through to the PATH would silently run a different program than the
// one that was asked for and report success for it.
func Find(spec Spec) (Tool, error) {
	var looked []string

	if spec.Env != "" {
		if p := strings.TrimSpace(os.Getenv(spec.Env)); p != "" {
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				return Tool{Spec: spec, Path: p}, nil
			}
			return Tool{}, fmt.Errorf("%s is set to %q, which is not an executable file", spec.Env, p)
		}
		looked = append(looked, "$"+spec.Env)
	}

	looked = append(looked, "PATH")
	for _, name := range spec.Commands {
		if p, err := exec.LookPath(name); err == nil {
			return Tool{Spec: spec, Path: p}, nil
		}
	}

	candidates := spec.Paths[runtime.GOOS]
	if candidates == nil {
		candidates = spec.Paths[""]
	}
	for _, p := range candidates {
		looked = append(looked, p)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return Tool{Spec: spec, Path: p}, nil
		}
	}

	return Tool{}, &NotFoundError{Spec: spec, Looked: looked}
}

// DefaultTimeout bounds one invocation.
//
// Generous, because a tool starting for the first time on a cold runner may
// build a profile or unpack a runtime before doing any work.
const DefaultTimeout = 3 * time.Minute

// Result is one invocation's outcome.
type Result struct {
	// Stdout and Stderr are what the program wrote.
	Stdout, Stderr []byte

	// ExitCode is the program's status, or -1 if it did not run to completion.
	ExitCode int
}

// Combined returns the program's output, for a failure message.
func (r Result) Combined() string {
	var b bytes.Buffer
	b.Write(r.Stdout)
	if len(r.Stderr) > 0 {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.Write(r.Stderr)
	}
	return b.String()
}

// Run invokes the tool.
//
// A non-zero exit status is returned in the result rather than as an error,
// because several of these programs use the status to report a verdict about
// the file rather than a failure of their own. The error result is reserved for
// the tool not running at all.
//
// env supplies additional environment entries, which is how a run is made
// hermetic: a tool that keeps state under HOME is pointed at a temporary one.
func (t Tool) Run(ctx context.Context, env []string, args ...string) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	// #nosec G204 -- test-only tooling. The executable is the located
	// installation and the arguments are fixtures, not user input.
	cmd := exec.CommandContext(ctx, t.Path, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), ExitCode: cmd.ProcessState.ExitCode()}

	if ctx.Err() != nil {
		return res, fmt.Errorf("%s did not finish within %s", t.Spec.Name, DefaultTimeout)
	}
	var exitErr *exec.ExitError
	if err != nil && !asExitError(err, &exitErr) {
		return res, fmt.Errorf("running %s: %w", t.Spec.Name, err)
	}
	return res, nil
}

// TempHome returns environment entries pointing a tool's state at dir.
//
// Several of these programs read and write configuration under HOME even when
// told to use an explicit profile. Redirecting it keeps a run from picking up,
// or disturbing, the developer's own settings — and keeps two concurrent runs
// from sharing a lock.
func TempHome(dir string) []string {
	return []string{"HOME=" + dir, "XDG_CONFIG_HOME=" + filepath.Join(dir, "config")}
}
