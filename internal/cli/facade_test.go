package cli_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	vellumcli "github.com/frankbardon/vellum/internal/cli"
	"github.com/frankbardon/vellum/theme"
)

// smallPNG returns a tiny, valid PNG so a test exercising asset resolution
// has real bytes a decoder accepts — not just a signature the sniffer
// recognises. See TestGoldenMediaDecodes's own doc comment for why a
// hand-assembled signature-only "PNG" is not good enough.
func smallPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

// customThemeJSON derives a theme document from the built-in one under a
// different id, so a directory-backed provider has something distinct to
// serve — a test that only ever fed the provider the built-in theme's own id
// would not prove the directory was read at all.
func customThemeJSON(t *testing.T) []byte {
	t.Helper()
	src := string(theme.BuiltinJSON())
	const from = `"id": "default"`
	if !strings.Contains(src, from) {
		t.Fatalf("built-in theme JSON does not contain %q; update this test's replacement", from)
	}
	return []byte(strings.Replace(src, from, `"id": "custom"`, 1))
}

func TestNewFacade_ThemeDirWiresADirProvider(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "custom.json"), customThemeJSON(t), 0o644); err != nil {
		t.Fatalf("writing theme file: %v", err)
	}
	t.Setenv("VELLUM_THEME_DIR", dir)

	specJSON := `{"theme":"custom","sections":[{"blocks":[{"kind":"text","text":{"content":"Body."}}]}]}`
	specPath := writeTempFile(t, "spec.json", specJSON)
	outPath := filepath.Join(t.TempDir(), "out.docx")

	res := runCLI(t, "", "compose", specPath, "--format", "docx", "-o", outPath, "--json")
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.ExitCode, res.Stderr, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	if len(env.Errors) != 0 {
		t.Errorf("errors = %+v, want none", env.Errors)
	}
}

func TestNewFacade_ThemeDirUnknownIDIsThemeNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VELLUM_THEME_DIR", dir)

	specJSON := `{"theme":"nope","sections":[{"blocks":[{"kind":"text","text":{"content":"Body."}}]}]}`
	specPath := writeTempFile(t, "spec.json", specJSON)

	res := runCLI(t, "", "compose", specPath, "--format", "docx", "--json", "-o", filepath.Join(t.TempDir(), "out.docx"))
	if res.ExitCode != vellumcli.ExitFailure {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitFailure, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_THEME_NOT_FOUND)
}

func TestNewFacade_AssetDirWiresADirResolver(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pic.png"), smallPNG(t), 0o644); err != nil {
		t.Fatalf("writing asset file: %v", err)
	}
	t.Setenv("VELLUM_ASSET_DIR", dir)

	specJSON := `{"sections":[{"blocks":[{"kind":"asset","asset":{"handle":"pic.png","alt_text":"a picture"}}]}]}`
	specPath := writeTempFile(t, "spec.json", specJSON)
	outPath := filepath.Join(t.TempDir(), "out.docx")

	res := runCLI(t, "", "compose", specPath, "--format", "docx", "-o", outPath, "--json")
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.ExitCode, res.Stderr, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	if len(env.Errors) != 0 {
		t.Errorf("errors = %+v, want none", env.Errors)
	}
}

func TestNewFacade_AssetDirUnsetLeavesTheDefaultInlineResolver(t *testing.T) {
	// Purely additive: with VELLUM_ASSET_DIR unset, a handle that is not a
	// data URI must fail exactly as it always has, through asset.Inline.
	specJSON := `{"sections":[{"blocks":[{"kind":"asset","asset":{"handle":"pic.png","alt_text":"a picture"}}]}]}`
	specPath := writeTempFile(t, "spec.json", specJSON)

	res := runCLI(t, "", "compose", specPath, "--format", "docx", "--json", "-o", filepath.Join(t.TempDir(), "out.docx"))
	if res.ExitCode != vellumcli.ExitFailure {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitFailure, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_ASSET_NOT_FOUND)
}

func TestNewFacade_MalformedMaxAssetBytesFailsLoudly(t *testing.T) {
	t.Setenv("VELLUM_MAX_ASSET_BYTES", "not-a-number")

	specPath := writeTempFile(t, "spec.json", validSpecJSON)
	res := runCLI(t, "", "compose", specPath, "--format", "docx", "--json", "-o", filepath.Join(t.TempDir(), "out.docx"))
	if res.ExitCode != vellumcli.ExitFailure {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitFailure, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_USAGE)
}

func TestNewFacade_ZeroMaxAssetBytesFailsLoudly(t *testing.T) {
	t.Setenv("VELLUM_MAX_ASSET_BYTES", "0")

	specPath := writeTempFile(t, "spec.json", validSpecJSON)
	res := runCLI(t, "", "compose", specPath, "--format", "docx", "--json", "-o", filepath.Join(t.TempDir(), "out.docx"))
	if res.ExitCode != vellumcli.ExitFailure {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitFailure, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_CLI_USAGE)
}

func TestNewFacade_ValidMaxAssetBytesEnforcesTheBound(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pic.png"), smallPNG(t), 0o644); err != nil {
		t.Fatalf("writing asset file: %v", err)
	}
	t.Setenv("VELLUM_ASSET_DIR", dir)
	t.Setenv("VELLUM_MAX_ASSET_BYTES", "4")

	specJSON := `{"sections":[{"blocks":[{"kind":"asset","asset":{"handle":"pic.png","alt_text":"a picture"}}]}]}`
	specPath := writeTempFile(t, "spec.json", specJSON)

	res := runCLI(t, "", "compose", specPath, "--format", "docx", "--json", "-o", filepath.Join(t.TempDir(), "out.docx"))
	if res.ExitCode != vellumcli.ExitFailure {
		t.Fatalf("exit=%d, want %d\nstdout=%s", res.ExitCode, vellumcli.ExitFailure, res.Stdout)
	}
	env := decodeEnvelope(t, res.Stdout)
	assertOneError(t, env, verr.VELLUM_ASSET_TOO_LARGE)
}

func TestNewFacade_NeitherDirSetIsUnchangedFromBefore(t *testing.T) {
	// The exact zero-value behaviour: built-in theme only, inline assets
	// only, no VELLUM_MAX_ASSET_BYTES override. This is the case every
	// pre-existing compose test already exercises; this test names the
	// invariant explicitly so a future change to newFacade cannot silently
	// make the unset path non-additive.
	specPath := writeTempFile(t, "spec.json", validSpecJSON)
	res := runCLI(t, "", "compose", specPath, "--format", "docx", "--json", "-o", filepath.Join(t.TempDir(), "out.docx"))
	if res.ExitCode != vellumcli.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.ExitCode, res.Stderr, res.Stdout)
	}
}
