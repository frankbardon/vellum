package mcp

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/frankbardon/vellum"
	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/descriptor"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/examples"
	"github.com/frankbardon/vellum/mcp/toolmeta"
	"github.com/frankbardon/vellum/opc/zipdet"
	"github.com/frankbardon/vellum/skills"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/template/bind"
)

// Every handler below takes a *vellum.Vellum and one typed input, calls
// exactly one facade method, and returns one typed output or an error —
// "facade-only", per this package's own doc comment. None of them imports a
// format writer, resolve, template's own package (beyond the type aliased
// in types.go), or opc directly except where internal/cli's own verb file
// already established the identical necessity: zipdet.WriteOptions to
// serialise a filled *opc.Package to bytes is exactly what fill.go's CLI
// verb does, because the facade hands back a package, not bytes, and
// something has to bridge that once.

// parseFormat resolves raw to an [artifact.Format], or a
// VELLUM_MCP_INVALID_INPUT naming the accepted set — the MCP counterpart to
// internal/cli's own parseFormat (flags.go), which raises VELLUM_CLI_USAGE
// for the identical condition.
func parseFormat(raw string) (artifact.Format, error) {
	f, ok := artifact.ParseFormat(raw)
	if !ok {
		all := artifact.AllFormats()
		accepted := make([]string, len(all))
		for i, v := range all {
			accepted[i] = string(v)
		}
		return "", verr.NewCodedErrorWithDetails(verr.VELLUM_MCP_INVALID_INPUT,
			"unrecognised format value", map[string]any{"format": raw, "accepted": accepted})
	}
	return f, nil
}

// unknownDocErr builds the VELLUM_MCP_INVALID_INPUT failure [handleSkills]
// and [handleExamples] return when Name names no document in the pack — the
// same code parseFormat raises for an unrecognised format, since both are "a
// value the input contract's own semantic checks reject" rather than a
// decode failure. available carries every name that would have matched, so
// a client (or the model driving it) can retry without a second round trip
// to discover the valid set.
func unknownDocErr(tool, name string, available []string) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_MCP_INVALID_INPUT,
		"no document with that name in this pack", map[string]any{
			"tool":      tool,
			"name":      name,
			"available": available,
		})
}

func handleCompose(ctx context.Context, v *vellum.Vellum, in ComposeIn) (ComposeOut, error) {
	format, err := parseFormat(in.Format)
	if err != nil {
		return ComposeOut{}, err
	}
	s, err := spec.DecodeAuto(in.Spec)
	if err != nil {
		return ComposeOut{}, err
	}
	var buf bytes.Buffer
	report, err := v.Compose(ctx, s, format, &buf)
	if err != nil {
		return ComposeOut{}, err
	}
	return ComposeOut{
		Format:   string(report.Format),
		Artifact: buf.Bytes(),
		Warnings: report.Warnings,
	}, nil
}

func handleValidate(ctx context.Context, v *vellum.Vellum, in ValidateIn) (ValidateOut, error) {
	format, err := parseFormat(in.Format)
	if err != nil {
		return ValidateOut{}, err
	}
	s, err := spec.DecodeAuto(in.Spec)
	if err != nil {
		return ValidateOut{}, err
	}
	warnings, err := v.Validate(ctx, s, format)
	if err != nil {
		return ValidateOut{}, err
	}
	return ValidateOut{Valid: true, Warnings: warnings}, nil
}

func handleInspect(ctx context.Context, v *vellum.Vellum, in InspectIn) (InspectOut, error) {
	r := bytes.NewReader(in.Template)
	report, err := v.Inspect(ctx, r, int64(len(in.Template)))
	if err != nil {
		return InspectOut{}, err
	}
	return *report, nil
}

func handleFill(ctx context.Context, v *vellum.Vellum, in FillIn) (FillOut, error) {
	binding, err := bind.DecodeAuto(in.Binding)
	if err != nil {
		return FillOut{}, err
	}

	data := in.Data
	if len(bytes.TrimSpace(data)) == 0 {
		data = []byte("{}")
	}
	var scope bind.Scope
	if err := json.Unmarshal(data, &scope); err != nil {
		return FillOut{}, verr.NewCodedErrorWithDetails(verr.VELLUM_MCP_INVALID_INPUT,
			"the fill data is not valid JSON", map[string]any{"error": err.Error()})
	}

	r := bytes.NewReader(in.Template)
	result, err := v.Fill(ctx, r, int64(len(in.Template)), binding, scope)
	if err != nil {
		return FillOut{}, err
	}

	var buf bytes.Buffer
	if err := result.Package.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		return FillOut{}, err
	}
	return FillOut{Package: buf.Bytes(), Touched: result.Touched}, nil
}

func handleCapabilities(_ context.Context, v *vellum.Vellum, in CapabilitiesIn) (CapabilitiesOut, error) {
	format, err := parseFormat(in.Format)
	if err != nil {
		return CapabilitiesOut{}, err
	}
	return CapabilitiesOut{Capabilities: v.Capabilities(format)}, nil
}

func handleBoxes(ctx context.Context, v *vellum.Vellum, in BoxesIn) (BoxesOut, error) {
	format, err := parseFormat(in.Format)
	if err != nil {
		return BoxesOut{}, err
	}
	boxes, err := v.Boxes(ctx, in.Theme, format)
	if err != nil {
		return BoxesOut{}, err
	}
	return BoxesOut{Boxes: boxes}, nil
}

// handleSchema and handleManifest call no facade method: the published
// schema and the manifest both describe the library's own registries, which
// [descriptor] reads directly (the same way internal/cli's schema.go does),
// not through a *vellum.Vellum. Both still take v, for the same reason every
// handler in this file does — a uniform signature is what lets [bind] wrap
// all ten with one generic function — even though neither uses it.

func handleSchema(_ context.Context, _ *vellum.Vellum, _ SchemaIn) (SchemaOut, error) {
	return SchemaOut{Schema: descriptor.BuildPayloadSchema()}, nil
}

func handleManifest(_ context.Context, _ *vellum.Vellum, _ ManifestIn) (ManifestOut, error) {
	return ManifestOut{Manifest: descriptor.BuildManifest()}, nil
}

// handleSkills and handleExamples read the embedded skills/ and examples/
// packs directly — neither touches v, the facade, for the same reason
// handleSchema and handleManifest do not: both packs are fixed content
// baked into the binary at build time, not something a *vellum.Vellum
// instance computes. A non-empty In.Name looks the document up by
// [skills.Doc.Stem]/[examples.Doc.Stem] and returns its whole raw content;
// an empty one lists every available name instead. See [SkillsOut] and
// [ExamplesOut] for why those are two mutually exclusive fields on one
// struct rather than two tools.

func handleSkills(_ context.Context, _ *vellum.Vellum, in SkillsIn) (SkillsOut, error) {
	if in.Name != "" {
		doc, ok, err := skills.Get(in.Name)
		if err != nil {
			return SkillsOut{}, err
		}
		if ok {
			return SkillsOut{Content: doc.Raw}, nil
		}
	}
	names, err := skillNames()
	if err != nil {
		return SkillsOut{}, err
	}
	if in.Name != "" {
		return SkillsOut{}, unknownDocErr(toolmeta.NameSkills, in.Name, names)
	}
	return SkillsOut{Names: names}, nil
}

// skillNames returns every embedded skill document's [skills.Doc.Stem],
// already sorted since [skills.All] itself returns its documents sorted by
// filename.
func skillNames() ([]string, error) {
	docs, err := skills.All()
	if err != nil {
		return nil, err
	}
	names := make([]string, len(docs))
	for i, d := range docs {
		names[i] = d.Stem()
	}
	return names, nil
}

func handleExamples(_ context.Context, _ *vellum.Vellum, in ExamplesIn) (ExamplesOut, error) {
	if in.Name != "" {
		doc, ok, err := examples.Get(in.Name)
		if err != nil {
			return ExamplesOut{}, err
		}
		if ok {
			return ExamplesOut{Content: string(doc.Raw)}, nil
		}
	}
	names, err := exampleNames()
	if err != nil {
		return ExamplesOut{}, err
	}
	if in.Name != "" {
		return ExamplesOut{}, unknownDocErr(toolmeta.NameExamples, in.Name, names)
	}
	return ExamplesOut{Names: names}, nil
}

// exampleNames mirrors [skillNames] for the examples pack: every embedded
// document's [examples.Doc.Stem], already sorted since [examples.All]
// itself returns its documents sorted by filename.
func exampleNames() ([]string, error) {
	docs, err := examples.All()
	if err != nil {
		return nil, err
	}
	names := make([]string, len(docs))
	for i, d := range docs {
		names[i] = d.Stem()
	}
	return names, nil
}
