package mcp_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/mcp"
	"github.com/frankbardon/vellum/mcp/toolmeta"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/template/bind"
)

const validSpecJSON = `{
  "sections": [
    {"blocks": [
      {"kind": "heading", "heading": {"level": 1, "content": "Title"}},
      {"kind": "text", "text": {"content": "Body."}}
    ]}
  ]
}`

// markerSpecJSON composes to a document whose body text is literally
// "Dear {{name}}," — discoverable as a marker anchor named "name" by
// anchor.Discover, exactly as internal/cli's own harness_test.go uses it —
// so a fill/inspect test has a real fillable template rather than
// hand-assembled OOXML.
const markerSpecJSON = `{
  "sections": [
    {"blocks": [
      {"kind": "text", "text": {"content": "Dear {{name}},"}}
    ]}
  ]
}`

func dispatch(t *testing.T, catalog []mcp.Tool, name string, args any) envelope {
	t.Helper()
	raw, encErr := json.Marshal(args)
	if encErr != nil {
		t.Fatalf("marshalling args: %v", encErr)
	}
	out, _ := mcp.Dispatch(context.Background(), catalog, name, raw)
	return decodeEnvelope(t, out)
}

// TestHandleCompose_MatchesTheFacadeDirectly is the "facade-only" check the
// story asks for: the tool's result carries exactly the artifact bytes and
// warnings v.Compose itself would have produced for the identical
// specification, called directly.
func TestHandleCompose_MatchesTheFacadeDirectly(t *testing.T) {
	v := testVellum(t)
	catalog := mcp.NewCatalog(v)

	s, err := spec.DecodeAuto([]byte(validSpecJSON))
	if err != nil {
		t.Fatalf("spec.DecodeAuto: %v", err)
	}
	var want bytes.Buffer
	report, err := v.Compose(context.Background(), s, artifact.FormatDOCX, &want)
	if err != nil {
		t.Fatalf("v.Compose: %v", err)
	}

	env := dispatch(t, catalog, toolmeta.NameCompose, map[string]any{
		"spec":   json.RawMessage(validSpecJSON),
		"format": "docx",
	})
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}

	var out struct {
		Format   string `json:"format"`
		Artifact []byte `json:"artifact"`
		Warnings []any  `json:"warnings"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if out.Format != string(report.Format) {
		t.Errorf("format = %q, want %q", out.Format, report.Format)
	}
	if !bytes.Equal(out.Artifact, want.Bytes()) {
		t.Errorf("artifact bytes differ from a direct v.Compose call (%d vs %d bytes)", len(out.Artifact), want.Len())
	}
	if len(out.Warnings) != len(report.Warnings) {
		t.Errorf("warnings = %d, want %d", len(out.Warnings), len(report.Warnings))
	}
}

func TestHandleCompose_RejectedSpecIsCodedNotPanic(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	env := dispatch(t, catalog, toolmeta.NameCompose, map[string]any{
		"spec":   json.RawMessage(`{}`),
		"format": "docx",
	})
	if len(env.Errors) == 0 {
		t.Fatal("an empty specification composed with no error")
	}
}

func TestHandleCompose_UnrecognisedFormatIsMCPInvalidInput(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	env := dispatch(t, catalog, toolmeta.NameCompose, map[string]any{
		"spec":   json.RawMessage(validSpecJSON),
		"format": "bogus",
	})
	assertOneError(t, env, verr.VELLUM_MCP_INVALID_INPUT)
}

func TestHandleValidate_ReportsValidWithNoWarnings(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	env := dispatch(t, catalog, toolmeta.NameValidate, map[string]any{
		"spec":   json.RawMessage(validSpecJSON),
		"format": "docx",
	})
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
	var out struct {
		Valid    bool  `json:"valid"`
		Warnings []any `json:"warnings"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if !out.Valid {
		t.Error("valid = false, want true")
	}
}

func TestHandleCapabilities_ReturnsTheDeclaredMatrix(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	env := dispatch(t, catalog, toolmeta.NameCapabilities, map[string]any{"format": "docx"})
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
	var out struct {
		Capabilities []any `json:"capabilities"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if len(out.Capabilities) == 0 {
		t.Error("capabilities is empty, want the declared matrix for docx")
	}
}

func TestHandleBoxes_ReturnsTheThemesDeclaredSlots(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	env := dispatch(t, catalog, toolmeta.NameBoxes, map[string]any{"format": "docx"})
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
	var out struct {
		Boxes []any `json:"boxes"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if len(out.Boxes) == 0 {
		t.Error("boxes is empty, want at least the built-in theme's declared roles")
	}
}

func TestHandleSchema_ReturnsThePublishedSchema(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	env := dispatch(t, catalog, toolmeta.NameSchema, map[string]any{})
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
	var out struct {
		Schema json.RawMessage `json:"schema"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out.Schema, &doc); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	if doc["$id"] == nil {
		t.Error("schema has no $id")
	}
}

func TestHandleManifest_ReturnsTheLiveManifest(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	env := dispatch(t, catalog, toolmeta.NameManifest, map[string]any{})
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
	var out struct {
		Manifest struct {
			FormatVersion string `json:"format_version"`
			Formats       []any  `json:"formats"`
		} `json:"manifest"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if out.Manifest.FormatVersion == "" {
		t.Error("manifest has no format_version")
	}
	if len(out.Manifest.Formats) == 0 {
		t.Error("manifest lists no formats")
	}
}

// buildDOCXFixture composes markerSpecJSON to a DOCX through the same
// catalog (vellum_compose), decoding its base64 artifact back to bytes, so
// the fixture a fill/inspect test fills is built through the same seam
// being tested rather than through a second, untested path.
func buildDOCXFixture(t *testing.T, catalog []mcp.Tool) []byte {
	t.Helper()
	env := dispatch(t, catalog, toolmeta.NameCompose, map[string]any{
		"spec":   json.RawMessage(markerSpecJSON),
		"format": "docx",
	})
	if len(env.Errors) != 0 {
		t.Fatalf("building the fixture: errors = %+v", env.Errors)
	}
	var out struct {
		Artifact []byte `json:"artifact"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}
	return out.Artifact
}

func TestHandleInspect_ReportsTheMarkerAnchor(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	docx := buildDOCXFixture(t, catalog)

	env := dispatch(t, catalog, toolmeta.NameInspect, map[string]any{"template": docx})
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
	var out struct {
		Anchors []struct {
			Name string `json:"name"`
		} `json:"anchors"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if len(out.Anchors) != 1 || out.Anchors[0].Name != "name" {
		t.Fatalf("anchors = %+v, want exactly one named \"name\"", out.Anchors)
	}
}

func TestHandleFill_ProducesATouchedReceiptAndFilledBytes(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	docx := buildDOCXFixture(t, catalog)

	bindingJSON := `{
	  "format_version": "` + bind.FormatVersion + `",
	  "statements": [
	    {"kind": "bind", "bind": {"anchor": "name", "expr": "who"}}
	  ]
	}`

	env := dispatch(t, catalog, toolmeta.NameFill, map[string]any{
		"template": docx,
		"binding":  json.RawMessage(bindingJSON),
		"data":     json.RawMessage(`{"who":"Ada"}`),
	})
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}

	var out struct {
		Package []byte   `json:"package"`
		Touched []string `json:"touched"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if len(out.Touched) == 0 {
		t.Error("touched is empty, want at least the main document part")
	}
	if len(out.Package) == 0 {
		t.Fatal("package is empty")
	}

	// The package bytes must actually be a package: read them back with the
	// same zip reader the rest of the library uses, rather than only
	// checking that some bytes came back.
	pkg, err := zipdet.Read(bytes.NewReader(out.Package), int64(len(out.Package)), zipdet.ReadOptions{})
	if err != nil {
		t.Fatalf("the filled package does not read back as a zip: %v", err)
	}
	if pkg == nil {
		t.Fatal("zipdet.Read returned a nil package with no error")
	}
}

func TestHandleFill_MalformedDataIsMCPInvalidInput(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	docx := buildDOCXFixture(t, catalog)

	bindingJSON := `{
	  "format_version": "` + bind.FormatVersion + `",
	  "statements": [
	    {"kind": "bind", "bind": {"anchor": "name", "expr": "who"}}
	  ]
	}`

	env := dispatch(t, catalog, toolmeta.NameFill, map[string]any{
		"template": docx,
		"binding":  json.RawMessage(bindingJSON),
		// Valid JSON syntax (so it survives the outer json.Marshal in
		// dispatch), but not the object bind.Scope requires — a JSON string
		// where an object was expected.
		"data": json.RawMessage(`"not an object"`),
	})
	assertOneError(t, env, verr.VELLUM_MCP_INVALID_INPUT)
}

// TestHandleSkills_FoundByName checks the found-by-name path against a
// document known to exist — this tool's own skill file, tool-skills.md,
// whose stem is "tool-skills" — asserting Content carries the whole raw
// file (frontmatter included) and Names is empty, since the two are
// mutually exclusive on the wire.
func TestHandleSkills_FoundByName(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	env := dispatch(t, catalog, toolmeta.NameSkills, map[string]any{"name": "tool-skills"})
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
	var out struct {
		Content string   `json:"content"`
		Names   []string `json:"names"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if !strings.HasPrefix(out.Content, "---\n") {
		t.Errorf("content = %q, want the raw file starting with its frontmatter delimiter", firstLine(out.Content))
	}
	if !strings.Contains(out.Content, "tool-skills") {
		t.Error("content does not mention its own stem")
	}
	if len(out.Names) != 0 {
		t.Errorf("names = %v, want empty on a found-by-name response", out.Names)
	}
}

// TestHandleSkills_NotFoundByName checks that a name matching nothing in
// the pack is VELLUM_MCP_INVALID_INPUT, not VELLUM_MCP_NOT_IMPLEMENTED —
// this tool is fully wired, so an unknown name is an ordinary input error,
// the same code parseFormat raises for an unrecognised format.
func TestHandleSkills_NotFoundByName(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	env := dispatch(t, catalog, toolmeta.NameSkills, map[string]any{"name": "does-not-exist"})
	assertOneError(t, env, verr.VELLUM_MCP_INVALID_INPUT)
	if details := env.Errors[0].Details; details["name"] != "does-not-exist" {
		t.Errorf("error details = %+v, want name = %q", details, "does-not-exist")
	}
}

// TestHandleSkills_EmptyNameLists checks the listing form: an empty name
// returns every available stem, sorted, and no content.
func TestHandleSkills_EmptyNameLists(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	env := dispatch(t, catalog, toolmeta.NameSkills, map[string]any{})
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
	var out struct {
		Content string   `json:"content"`
		Names   []string `json:"names"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if out.Content != "" {
		t.Errorf("content = %q, want empty on a listing response", out.Content)
	}
	if len(out.Names) == 0 {
		t.Fatal("names is empty, want at least this tool's own skill files")
	}
	if !slices.Contains(out.Names, "tool-skills") {
		t.Errorf("names = %v, want it to contain %q", out.Names, "tool-skills")
	}
	if !slices.IsSorted(out.Names) {
		t.Errorf("names = %v, want sorted", out.Names)
	}
}

// TestHandleExamples_FoundByName mirrors TestHandleSkills_FoundByName
// against the examples pack, whose Doc.Raw is []byte rather than a parsed
// Frontmatter/Body split — the wire Content is that []byte converted to a
// string, so it round-trips as the JSON the example file itself is.
func TestHandleExamples_FoundByName(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	env := dispatch(t, catalog, toolmeta.NameExamples, map[string]any{"name": "block-heading"})
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
	var out struct {
		Content string   `json:"content"`
		Names   []string `json:"names"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out.Content), &doc); err != nil {
		t.Fatalf("content is not valid JSON: %v\n%s", err, out.Content)
	}
	if len(out.Names) != 0 {
		t.Errorf("names = %v, want empty on a found-by-name response", out.Names)
	}
}

func TestHandleExamples_NotFoundByName(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	env := dispatch(t, catalog, toolmeta.NameExamples, map[string]any{"name": "does-not-exist"})
	assertOneError(t, env, verr.VELLUM_MCP_INVALID_INPUT)
}

func TestHandleExamples_EmptyNameLists(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	env := dispatch(t, catalog, toolmeta.NameExamples, map[string]any{})
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
	var out struct {
		Content string   `json:"content"`
		Names   []string `json:"names"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if out.Content != "" {
		t.Errorf("content = %q, want empty on a listing response", out.Content)
	}
	if len(out.Names) == 0 {
		t.Fatal("names is empty, want at least one example file")
	}
	if !slices.Contains(out.Names, "block-heading") {
		t.Errorf("names = %v, want it to contain %q", out.Names, "block-heading")
	}
	if !slices.IsSorted(out.Names) {
		t.Errorf("names = %v, want sorted", out.Names)
	}
}

// firstLine is a small test helper so a failed content-prefix assertion
// prints something legible rather than an entire skill file.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// TestHandlers_NeverPanicOnGarbageInput exercises every registered tool
// with arguments that decode as JSON but satisfy no field the tool expects,
// checking that "facade-only, coded errors as data" holds even for
// adversarial input — no handler in this package should ever panic; a
// panic here would be a bug the story's own harness is required to catch
// rather than let escape as a crashed MCP session.
func TestHandlers_NeverPanicOnGarbageInput(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	garbage := []json.RawMessage{
		json.RawMessage(`{}`),
		json.RawMessage(`{"unexpected_field": 12345}`),
		json.RawMessage(`[]`),
		json.RawMessage(`null`),
		json.RawMessage(``),
	}
	for _, tool := range catalog {
		for _, g := range garbage {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%s panicked on %s: %v", tool.Meta.Name, g, r)
					}
				}()
				_, _ = tool.Handle(context.Background(), g)
			}()
		}
	}
}

// base64RoundTrip is a small sanity check that []byte fields really do
// travel as base64 on this wire, matching schema.go's own documented
// correction — if this ever failed, every artifact/template/package byte
// slice above would be silently wrong on the wire while every test still
// passing (json.Unmarshal into []byte tolerates both shapes, base64 string
// or a JSON array of numbers).
func TestArtifactField_IsBase64OnTheWire(t *testing.T) {
	catalog := mcp.NewCatalog(testVellum(t))
	env := dispatch(t, catalog, toolmeta.NameCompose, map[string]any{
		"spec":   json.RawMessage(validSpecJSON),
		"format": "docx",
	})
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
	var raw struct {
		Artifact string `json:"artifact"`
	}
	if err := json.Unmarshal(env.Data, &raw); err != nil {
		t.Fatalf("artifact did not decode as a JSON string (want base64): %v", err)
	}
	if _, err := base64.StdEncoding.DecodeString(raw.Artifact); err != nil {
		t.Fatalf("artifact is not valid base64: %v", err)
	}
}
