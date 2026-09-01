package descriptor_test

import (
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/capability"
	"github.com/frankbardon/vellum/descriptor"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/spec"
)

var update = flag.Bool("update", false, "regenerate the committed goldens")

func TestEnvelope_ArrayInvariant(t *testing.T) {
	raw, err := json.Marshal(descriptor.NewEnvelope(map[string]any{"x": 1}))
	if err != nil {
		t.Fatal(err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"format_version", "data", "errors", "warnings"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("the envelope omits %q", key)
		}
	}
	if string(decoded["errors"]) != "[]" || string(decoded["warnings"]) != "[]" {
		t.Errorf("errors = %s, warnings = %s; both must be empty arrays, never null",
			decoded["errors"], decoded["warnings"])
	}
	if _, ok := decoded["request"]; ok {
		t.Error("the envelope emitted a request key when none was attached; echoing a large specification by default would double every response")
	}
}

// TestEnvelope_BareStructStillHoldsTheInvariant covers an envelope built as a
// struct literal rather than through the constructor, which is how a nil slice
// would otherwise reach the wire as null.
func TestEnvelope_BareStructStillHoldsTheInvariant(t *testing.T) {
	raw, err := json.Marshal(&descriptor.Envelope{Data: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"errors":[]`) || !strings.Contains(string(raw), `"warnings":[]`) {
		t.Errorf("a bare envelope serialised null arrays: %s", raw)
	}
	if !strings.Contains(string(raw), `"format_version":"`+descriptor.FormatVersion+`"`) {
		t.Errorf("a bare envelope did not gain a format version: %s", raw)
	}
}

// TestEnvelope_AddCodedErrorPreservesTheCode covers the tempting shortcut:
// flattening an error to a string destroys the thing that makes it useful,
// because a consumer branching on a code cannot branch on prose.
func TestEnvelope_AddCodedErrorPreservesTheCode(t *testing.T) {
	e := descriptor.NewEnvelope(nil)
	e.AddCodedError(verr.NewCodedErrorWithDetails(verr.VELLUM_CAPABILITY_REJECTED,
		"the target format does not render this feature",
		map[string]any{"feature": "block.asset"}))

	if e.OK() {
		t.Error("an envelope with an error reported OK")
	}
	if len(e.Errors) != 1 {
		t.Fatalf("got %d errors, want 1", len(e.Errors))
	}
	got := e.Errors[0]
	if got.Code != string(verr.VELLUM_CAPABILITY_REJECTED) {
		t.Errorf("code = %q", got.Code)
	}
	if got.Details["feature"] != "block.asset" {
		t.Errorf("details did not survive: %v", got.Details)
	}
}

func TestEnvelope_NilReceiverIsSafe(t *testing.T) {
	var e *descriptor.Envelope
	e.AddError("X", "y", nil)
	e.AddWarning("X", "y", nil)
	e.AddCodedError(verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT, "x"))
	if e.OK() {
		t.Error("a nil envelope reported OK")
	}
	if e.WithRequest(1) != nil {
		t.Error("WithRequest on nil returned non-nil")
	}
	raw, err := json.Marshal(e)
	if err != nil || string(raw) != "null" {
		t.Errorf("marshalling a nil envelope = %s, %v", raw, err)
	}
}

// TestManifestBlocksComplete and its siblings keep the manifest from drifting
// from the registries it claims to describe.
func TestManifestBlocksComplete(t *testing.T) {
	m := descriptor.BuildManifest()
	if len(m.Blocks) != len(spec.AllBlockKinds()) {
		t.Fatalf("manifest lists %d blocks, the registry has %d", len(m.Blocks), len(spec.AllBlockKinds()))
	}
	for i, k := range spec.AllBlockKinds() {
		if m.Blocks[i].Kind != k {
			t.Errorf("block %d = %q, want %q", i, m.Blocks[i].Kind, k)
		}
		if strings.TrimSpace(m.Blocks[i].Purpose) == "" {
			t.Errorf("block %q has no purpose; the manifest is the first thing an agent reads", k)
		}
	}
}

func TestManifestFormatsComplete(t *testing.T) {
	m := descriptor.BuildManifest()
	if len(m.Formats) != len(artifact.AllFormats()) {
		t.Fatalf("manifest lists %d formats, the registry has %d", len(m.Formats), len(artifact.AllFormats()))
	}
	for i, f := range artifact.AllFormats() {
		if m.Formats[i].Name != f {
			t.Errorf("format %d = %q, want %q", i, m.Formats[i].Name, f)
		}
		if len(m.Formats[i].Renders) == 0 {
			t.Errorf("format %q renders nothing, which cannot be right", f)
		}
	}
}

func TestManifestErrorCodesComplete(t *testing.T) {
	m := descriptor.BuildManifest()
	if m.ErrorCodesCount != len(verr.AllCodes()) {
		t.Errorf("manifest counts %d codes, the registry has %d", m.ErrorCodesCount, len(verr.AllCodes()))
	}
	if len(m.ErrorDomains) != len(verr.AllDomains()) {
		t.Errorf("manifest lists %d domains, the registry has %d", len(m.ErrorDomains), len(verr.AllDomains()))
	}
	// Names only: the prose for a code is fetched on demand so the manifest an
	// agent loads at session start stays small.
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"fixups", "FixupNotApplicable"} {
		if strings.Contains(string(raw), needle) {
			t.Errorf("the manifest carries %q; per-code prose belongs behind a lookup, not in the bootstrap payload", needle)
		}
	}
}

func TestManifestCapabilitiesComplete(t *testing.T) {
	m := descriptor.BuildManifest()
	want := len(capability.AllFeatures()) * len(artifact.AllFormats())
	if len(m.Capabilities) != want {
		t.Errorf("manifest carries %d capability entries, want %d", len(m.Capabilities), want)
	}
}

func TestPayloadSchemaVersionMatchesEnvelope(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(descriptor.BuildPayloadSchema(), &doc); err != nil {
		t.Fatal(err)
	}
	id, _ := doc["$id"].(string)
	if !strings.Contains(id, descriptor.FormatVersion) {
		t.Errorf("payload schema $id %q does not carry the envelope's format version %q", id, descriptor.FormatVersion)
	}
	if !strings.Contains(spec.SchemaID, descriptor.FormatVersion) {
		t.Errorf("the specification schema id %q and the envelope version %q have drifted apart", spec.SchemaID, descriptor.FormatVersion)
	}
}

// TestPayloadSchemaEmbedsSpecDefinitions checks that embedding the
// specification schema left its internal references resolvable, rather than
// producing a document with dangling $refs.
func TestPayloadSchemaEmbedsSpecDefinitions(t *testing.T) {
	var doc struct {
		Defs map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(descriptor.BuildPayloadSchema(), &doc); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"spec", "envelope", "entry", "block", "table", "cell", "annotation", "headerNode", "length"} {
		if _, ok := doc.Defs[name]; !ok {
			t.Errorf("$defs is missing %q; a $ref to it would dangle", name)
		}
	}
}

// TestDescriptorNoExecutionImports is the no-execute firewall. This package
// describes; it never produces. That is what makes "what can Vellum do" a
// cheap question, answerable with no writer, theme provider or asset resolver
// anywhere in sight.
func TestDescriptorNoExecutionImports(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/frankbardon/vellum/descriptor").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	forbidden := []string{
		"github.com/frankbardon/vellum/doc",
		"github.com/frankbardon/vellum/sheet",
		"github.com/frankbardon/vellum/deck",
		"github.com/frankbardon/vellum/pdf",
		"github.com/frankbardon/vellum/template",
		"github.com/frankbardon/vellum/resolve",
		"github.com/frankbardon/vellum/opc",
	}
	for _, line := range strings.Split(string(out), "\n") {
		for _, f := range forbidden {
			if strings.TrimSpace(line) == f {
				t.Errorf("descriptor depends on %s; it must describe without being able to produce", f)
			}
		}
	}
}

const goldenDir = "testdata"

func TestGoldensNotHandEdited(t *testing.T) {
	goldens := []struct {
		file  string
		build func() any
	}{
		{"manifest.json", func() any { return descriptor.BuildManifest() }},
		{"payload-schema.json", func() any { return descriptor.BuildPayloadSchema() }},
	}

	for _, g := range goldens {
		t.Run(g.file, func(t *testing.T) {
			path := filepath.Join(goldenDir, g.file)
			rendered, err := descriptor.RenderGolden(g.build())
			if err != nil {
				t.Fatalf("rendering: %v", err)
			}

			if *update {
				if err := os.MkdirAll(goldenDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, rendered, 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("wrote %s (%d bytes)", path, len(rendered))
				return
			}

			committed, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v\nRegenerate with: go test ./descriptor -update", err)
			}
			body, err := descriptor.SplitGolden(committed)
			if err != nil {
				t.Fatalf("%v", err)
			}
			wantBody, err := descriptor.SplitGolden(rendered)
			if err != nil {
				t.Fatalf("rendering produced an invalid golden: %v", err)
			}
			if string(body) != string(wantBody) {
				t.Errorf("%s is out of date.\nRegenerate with: go test ./descriptor -update\nfirst difference at byte %d",
					g.file, firstDiff(body, wantBody))
			}
		})
	}
}

func firstDiff(a, b []byte) int {
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}
