//go:build soffice

package oovalidate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// EnvBinary names the environment variable holding an explicit soffice path.
//
// Set it when LibreOffice is installed somewhere [Find] does not look, or when
// several versions are present and the choice matters.
const EnvBinary = "VELLUM_SOFFICE"

// EnvRequireOptional names the environment variable that turns a missing
// external tool from a skip into a failure.
//
// It exists because an optional gate that skips silently is a gate that passes
// forever without ever running, which is worse than not having it: the build
// stays green and the coverage is imaginary. CI sets this, so a runner that
// loses its LibreOffice installation finds out by failing rather than by
// quietly stopping checking. It is deliberately shared with the other
// externally-provisioned gates rather than being per-tool, so provisioning is
// one decision instead of one per tool.
const EnvRequireOptional = "VELLUM_REQUIRE_OPTIONAL_GATES"

// DefaultTimeout bounds one conversion.
//
// Generous because LibreOffice's first run on a fresh profile builds that
// profile before it does any work, and a cold CI runner pays that once.
const DefaultTimeout = 3 * time.Minute

// Tool is a located LibreOffice installation.
type Tool struct {
	// Path is the absolute path to the soffice binary.
	Path string
}

// ErrNotFound reports that no LibreOffice installation was located. Callers
// distinguish it from a real failure so an absent tool skips and a broken one
// fails.
type ErrNotFound struct {
	// Looked lists the locations consulted, in order, for the message.
	Looked []string
}

func (e *ErrNotFound) Error() string {
	return "no LibreOffice installation found. Looked at: " + strings.Join(e.Looked, ", ") +
		". Set " + EnvBinary + " to the soffice binary, or install LibreOffice"
}

// Find locates a LibreOffice installation.
//
// The search is: the EnvBinary override, then the PATH, then the conventional
// install location for the platform. The override is first so a machine with
// several versions is not at the mercy of PATH order — the version matters,
// because tolerances differ between them.
func Find() (Tool, error) {
	var looked []string

	if p := strings.TrimSpace(os.Getenv(EnvBinary)); p != "" {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return Tool{Path: p}, nil
		}
		// An explicit override that does not resolve is a configuration
		// mistake, not an absent tool. Falling through to the PATH would run a
		// different LibreOffice than the one that was asked for, and report
		// success for it.
		return Tool{}, fmt.Errorf("%s is set to %q, which is not an executable file", EnvBinary, p)
	}

	looked = append(looked, "PATH")
	for _, name := range []string{"soffice", "libreoffice"} {
		if p, err := exec.LookPath(name); err == nil {
			return Tool{Path: p}, nil
		}
	}

	for _, p := range wellKnownPaths() {
		looked = append(looked, p)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return Tool{Path: p}, nil
		}
	}

	return Tool{}, &ErrNotFound{Looked: looked}
}

// wellKnownPaths returns the conventional install locations for the platform.
func wellKnownPaths() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"/Applications/LibreOffice.app/Contents/MacOS/soffice"}
	case "windows":
		return []string{
			`C:\Program Files\LibreOffice\program\soffice.exe`,
			`C:\Program Files (x86)\LibreOffice\program\soffice.exe`,
		}
	default:
		return []string{
			"/usr/bin/soffice",
			"/usr/bin/libreoffice",
			"/usr/lib/libreoffice/program/soffice",
			"/snap/bin/libreoffice",
		}
	}
}

// RequireOptional reports whether a missing external tool should fail rather
// than skip. See [EnvRequireOptional].
func RequireOptional() bool {
	v := strings.TrimSpace(os.Getenv(EnvRequireOptional))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

// Convert reads src, converts it with the named LibreOffice filter, and returns
// the converted bytes.
//
// filter is a LibreOffice conversion target such as "pdf" or "txt:Text". outExt
// is the extension LibreOffice will give the result, which is not always
// derivable from the filter, so it is stated rather than guessed.
//
// The conversion is run in a private user profile in a temporary directory.
// That is what makes concurrent calls safe: LibreOffice serialises on its
// profile, so two invocations sharing one either block or interfere, and a
// suite that runs cases in parallel would do both intermittently. It also keeps
// the run hermetic — nothing is read from or written to the developer's own
// LibreOffice configuration.
func (t Tool) Convert(ctx context.Context, src, filter, outExt string) ([]byte, error) {
	work, err := os.MkdirTemp("", "vellum-oovalidate-")
	if err != nil {
		return nil, fmt.Errorf("creating the conversion workspace: %w", err)
	}
	defer os.RemoveAll(work)

	profile := filepath.Join(work, "profile")
	outDir := filepath.Join(work, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating the conversion output directory: %w", err)
	}

	abs, err := filepath.Abs(src)
	if err != nil {
		return nil, fmt.Errorf("resolving the source path: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	// #nosec G204 -- test-only tooling behind a build tag; the binary is the
	// located installation and the arguments are fixtures, not user input.
	cmd := exec.CommandContext(ctx, t.Path,
		"-env:UserInstallation="+profileURL(profile),
		"--headless",
		"--invisible",
		"--norestore",
		"--nolockcheck",
		"--nodefault",
		"--nofirststartwizard",
		"--convert-to", filter,
		"--outdir", outDir,
		abs,
	)
	// LibreOffice consults HOME for state even with an explicit profile. Point
	// it at the workspace so a run cannot pick up, or disturb, anything else.
	cmd.Env = append(os.Environ(), "HOME="+work)

	combined, runErr := cmd.CombinedOutput()

	// The exit status is checked, but it is not trusted on its own: LibreOffice
	// exits 0 for several inputs it declined to convert, printing the reason
	// and writing nothing. The produced file is the real result, so its absence
	// is the failure regardless of what the status said.
	want := filepath.Join(outDir, replaceExt(filepath.Base(abs), outExt))
	out, readErr := os.ReadFile(want)
	if readErr != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("LibreOffice did not finish within %s converting to %q", DefaultTimeout, filter)
		}
		return nil, fmt.Errorf("LibreOffice produced no %q output for %s (exit: %v)\n%s",
			filter, filepath.Base(abs), runErr, indent(string(combined)))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("LibreOffice produced an empty %q output for %s\n%s",
			filter, filepath.Base(abs), indent(string(combined)))
	}
	return out, nil
}

// profileURL renders a filesystem path as the file:// URL LibreOffice expects
// for -env:UserInstallation.
func profileURL(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		// A Windows path such as C:/Users/... needs the extra slash to make a
		// well-formed file URL.
		p = "/" + p
	}
	return "file://" + p
}

// replaceExt swaps a filename's extension for ext, which carries no dot.
func replaceExt(name, ext string) string {
	return strings.TrimSuffix(name, filepath.Ext(name)) + "." + ext
}

// indent prefixes each line so LibreOffice's diagnostics are visibly its own
// and not mistaken for the test's.
func indent(s string) string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return "    (no output)"
	}
	return "    " + strings.ReplaceAll(s, "\n", "\n    ")
}
