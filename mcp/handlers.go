package mcp

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/frankbardon/vellum"
	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/descriptor"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/mcp/toolmeta"
	"github.com/frankbardon/vellum/opc/zipdet"
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

// notImplementedErr builds the VELLUM_MCP_NOT_IMPLEMENTED failure
// [handleSkills] and [handleExamples] return: a tool the catalog registers,
// so a client sees it via tools/list, but whose content E13 has not yet
// embedded.
func notImplementedErr(tool, landsIn string) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_MCP_NOT_IMPLEMENTED,
		"this tool is registered but its content is not yet available",
		map[string]any{"tool": tool, "lands_in": landsIn})
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

// handleSkills and handleExamples are stubs: the tools are registered — a
// client sees them in tools/list, with a description saying as much — but
// serve go:embed packs (skills/, examples/) E13 builds, not this story. See
// this package's doc comment and toolmeta's own doc comment for the same
// "stub, not a gap pretending to be done" note internal/cli's mcp and
// doctor stubs carried before E12-S2 landed them for real.

func handleSkills(_ context.Context, _ *vellum.Vellum, _ SkillsIn) (SkillsOut, error) {
	return SkillsOut{}, notImplementedErr(toolmeta.NameSkills, "E13")
}

func handleExamples(_ context.Context, _ *vellum.Vellum, _ ExamplesIn) (ExamplesOut, error) {
	return ExamplesOut{}, notImplementedErr(toolmeta.NameExamples, "E13")
}
