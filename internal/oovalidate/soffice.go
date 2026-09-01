//go:build soffice

package oovalidate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/frankbardon/vellum/internal/exttool"
)

// EnvBinary names the environment variable holding an explicit soffice path.
const EnvBinary = "VELLUM_SOFFICE"

// spec describes how to find LibreOffice.
var spec = exttool.Spec{
	Name:     "LibreOffice",
	Env:      EnvBinary,
	Commands: []string{"soffice", "libreoffice"},
	Paths: map[string][]string{
		"darwin": {"/Applications/LibreOffice.app/Contents/MacOS/soffice"},
		"windows": {
			`C:\Program Files\LibreOffice\program\soffice.exe`,
			`C:\Program Files (x86)\LibreOffice\program\soffice.exe`,
		},
		"": {
			"/usr/bin/soffice",
			"/usr/bin/libreoffice",
			"/usr/lib/libreoffice/program/soffice",
			"/snap/bin/libreoffice",
		},
	},
	Install: "brew install --cask libreoffice, or apt install libreoffice",
}

// Tool is a located LibreOffice installation.
type Tool struct {
	inner exttool.Tool
}

// Path is the located soffice binary.
func (t Tool) Path() string { return t.inner.Path }

// Find locates a LibreOffice installation.
func Find() (Tool, error) {
	inner, err := exttool.Find(spec)
	if err != nil {
		return Tool{}, err
	}
	return Tool{inner: inner}, nil
}

// Convert reads src, converts it with the named LibreOffice filter, and returns
// the converted bytes.
//
// filter is a conversion target such as "pdf" or "txt:Text". outExt is the
// extension LibreOffice gives the result, which is not always derivable from
// the filter, so it is stated rather than guessed.
//
// The conversion runs in a private user profile in a temporary directory. That
// is what makes concurrent calls safe: LibreOffice serialises on its profile,
// so two invocations sharing one either block or interfere, and a suite running
// cases in parallel would do both intermittently.
func (t Tool) Convert(ctx context.Context, src, filter, outExt string) ([]byte, error) {
	work, err := os.MkdirTemp("", "vellum-oovalidate-")
	if err != nil {
		return nil, fmt.Errorf("creating the conversion workspace: %w", err)
	}
	defer os.RemoveAll(work)

	outDir := filepath.Join(work, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating the conversion output directory: %w", err)
	}
	abs, err := filepath.Abs(src)
	if err != nil {
		return nil, fmt.Errorf("resolving the source path: %w", err)
	}

	res, err := t.inner.Run(ctx, exttool.TempHome(work),
		"-env:UserInstallation="+profileURL(filepath.Join(work, "profile")),
		"--headless", "--invisible", "--norestore",
		"--nolockcheck", "--nodefault", "--nofirststartwizard",
		"--convert-to", filter,
		"--outdir", outDir,
		abs,
	)
	if err != nil {
		return nil, err
	}

	// The exit status is not trusted on its own: LibreOffice exits zero for
	// several inputs it declined to convert, printing the reason and writing
	// nothing. The produced file is the real result, so its absence is the
	// failure regardless of what the status said.
	want := filepath.Join(outDir, replaceExt(filepath.Base(abs), outExt))
	out, readErr := os.ReadFile(want)
	if readErr != nil {
		return nil, fmt.Errorf("LibreOffice produced no %q output for %s (exit %d)\n%s",
			filter, filepath.Base(abs), res.ExitCode, indent(res.Combined()))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("LibreOffice produced an empty %q output for %s\n%s",
			filter, filepath.Base(abs), indent(res.Combined()))
	}
	return out, nil
}

// profileURL renders a filesystem path as the file:// URL that
// -env:UserInstallation expects.
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

// indent prefixes each line so the tool's diagnostics are visibly its own.
func indent(s string) string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return "    (no output)"
	}
	return "    " + strings.ReplaceAll(s, "\n", "\n    ")
}
