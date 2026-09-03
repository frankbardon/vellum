// Package vellum is a declarative artifact emitter: spec in, document out.
//
// Vellum produces DOCX, XLSX, PPTX and PDF/A-2b files from a single generic
// block model, and fills existing OOXML templates with bound data without
// destroying the parts it does not understand. It ships as an embeddable Go
// library, a thin CLI, and an MCP server with an embedded skill pack. The
// library is the primary deliverable; the CLI is an adapter over it.
//
// # Determinism
//
// Identical inputs produce byte-identical outputs. This is a hard requirement
// rather than an aspiration, and it is why there is no converter subprocess
// anywhere on the render path: conversion output varies with renderer version
// and with the fonts installed on the converting machine, which would defeat
// both golden-file testing and any consumer that dedupes on content.
//
// # Identity before render
//
// An artifact's name derives from the spec hash and the asset hashes, both of
// which are inputs. The name is therefore knowable before the render runs — a
// consumer that could only learn an artifact's identity by producing it could
// not use identity to avoid producing it. See [Vellum.ArtifactName].
//
// # Three content models
//
// The spec is unresolved and hashable; the fragment IR is resolved and
// format-neutral, so theme, font, number and asset resolution happen once and
// are shared by every writer; the per-format models are resolved and laid out,
// and are public so a consumer needing format-specific reach has it.
//
// # Using the facade
//
// [New] constructs a [Vellum] from [Options], every field of which is a seam
// with an inert default — a host that wires nothing still gets a working
// library. From there:
//
//   - [Vellum.Compose] and [Vellum.Validate] cover the "compose from blocks"
//     path: a [spec.Spec] resolved, lowered and (for Compose) written to an
//     [io.Writer].
//   - [Vellum.Write] covers the "compose from a format model" path: a
//     caller-built *doc.Document, *sheet.Workbook, *deck.Deck or *pdf.Document
//     written directly.
//   - [Vellum.Fill] and [Vellum.Inspect] cover fill mode: binding data into an
//     existing OOXML template, or discovering what it declares.
//   - [Vellum.Boxes] and [Vellum.Capabilities] are answerable before any
//     specification exists: the asset slots a theme offers a format, and the
//     declared (feature, format) outcomes.
//   - [Vellum.ArtifactName] computes an artifact's content-addressed name
//     without rendering anything.
//
// See CLAUDE.md for the architecture and the conventions that govern it.
package vellum

import (
	"context"
	"io"
	"sort"
	"time"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/asset"
	"github.com/frankbardon/vellum/canon"
	"github.com/frankbardon/vellum/capability"
	"github.com/frankbardon/vellum/deck"
	"github.com/frankbardon/vellum/doc"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/pdf"
	"github.com/frankbardon/vellum/resolve"
	"github.com/frankbardon/vellum/sheet"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/template"
	"github.com/frankbardon/vellum/template/bind"
	"github.com/frankbardon/vellum/theme"
)

// Options configures a [Vellum]. Every field is a seam with an inert default:
// the zero value produces a working library serving the built-in theme,
// resolving only inline assets, evaluating bindings with the default FEEL
// engine, and writing with the pinned deterministic epoch — "a host that
// wires nothing still gets a working library rather than a construction
// failure," per CLAUDE.md's "Extension Points".
//
// This is a plain struct rather than functional options, per CLAUDE.md's
// "Code Conventions": facade config is an Options struct; the leaf packages
// this facade calls ([resolve], [template]) use their own With* functional
// options, and New assembles this struct's fields into theirs.
//
// None of the interface-typed fields are substituted with a concrete default
// at construction time. Every one of them is already nil-checked at the one
// place downstream that interprets it — [resolve.Resolve] via
// [theme.Resolve] and [asset.Ingest], [template.Fill] via its own default
// evaluator — so New leaves nil exactly nil, rather than becoming a second
// place that could decide the default differently.
type Options struct {
	// Assets resolves the bytes and media type behind an asset handle. Nil
	// resolves inline data URIs only.
	Assets asset.Resolver

	// AssetOptions bounds asset ingestion — today, the maximum size of one
	// resolved asset. The zero value selects [asset.DefaultMaxBytes].
	AssetOptions asset.Options

	// Themes resolves a theme id to a theme document. Nil serves the
	// built-in theme.
	Themes theme.Provider

	// Evaluator evaluates every FEEL expression a [Vellum.Fill] call's
	// binding carries. Nil selects [bind.NewFEELEvaluator], matching
	// [template.Fill]'s own default — the FEEL evaluator CLAUDE.md's
	// "Extension Points" names.
	Evaluator bind.Evaluator

	// SourceDateEpoch pins every date a written artifact carries: zip entry
	// timestamps, OOXML core properties, and the PDF information dictionary
	// and XMP packet alike. The zero value selects each writer's own pinned
	// 1980 epoch, which is what VELLUM_SOURCE_DATE_EPOCH's documented
	// unset behaviour names. Setting a real time is a deliberate opt-out of
	// byte-identical output.
	SourceDateEpoch time.Time

	// Producer names the software recorded in an OOXML package's extended
	// properties. Empty selects each writer's own default, "Vellum". PDF
	// carries no equivalent property and ignores this field.
	Producer string
}

// resolveOptions projects the seams this Options carries into a
// [resolve.Options] for one format. Format-specific, because resolution
// itself is: the layout comes from a per-format master and the accepted
// asset media differ by container.
func (o Options) resolveOptions(format artifact.Format) resolve.Options {
	return resolve.Options{
		Format:       format,
		Themes:       o.Themes,
		Assets:       o.Assets,
		AssetOptions: o.AssetOptions,
	}
}

// Vellum is the library facade: one configured entry point over every
// composition, validation, fill and query path CLAUDE.md's architecture
// diagram names.
//
// It holds nothing but the [Options] it was constructed with, and every
// method reads that configuration fresh rather than caching anything derived
// from it — a theme resolved for one [Vellum.Boxes] call is not reused by the
// next, because a [theme.Provider] answering differently between two calls
// is the provider's prerogative, not a staleness bug in this type. A *Vellum
// is therefore safe for concurrent use: nothing about it is mutated after
// [New] returns.
type Vellum struct {
	opts Options
}

// New constructs a Vellum from opts, validating eagerly per CLAUDE.md's
// "Code Conventions" — a construction that would fail on every later call
// should fail once, here, rather than be discovered by whichever call
// happens to make first use of the bad field.
//
// Every field of [Options] is a seam whose zero value already names a
// working default — a nil [asset.Resolver], a nil [theme.Provider], a nil
// [bind.Evaluator], a zero [time.Time], an empty Producer — so there is,
// today, no combination of field values this rejects. The error return
// exists for the same reason every other constructor in this codebase
// returns one rather than a bare value: a future seam can become rejectable
// without changing every caller's call site.
func New(opts Options) (*Vellum, error) {
	return &Vellum{opts: opts}, nil
}

// resolve runs the shared spec-to-fragment step every compose path uses:
// [resolve.Resolve], configured from v's own seams for format. Every
// capability rejection, theme lookup failure and asset resolution failure
// surfaces here, before [Compose] or [Write] would touch a writer.
func (v *Vellum) resolve(ctx context.Context, s *spec.Spec, format artifact.Format) (*resolve.Result, error) {
	return resolve.Resolve(ctx, s, v.opts.resolveOptions(format))
}

// Compose resolves s against format — theme application, font selection,
// asset resolution and capability enforcement, all inside [resolve.Resolve]
// — lowers the result into the format's own model, and writes it to w. This
// is the "Compose from blocks" path in CLAUDE.md's architecture diagram.
//
// The returned [artifact.Report] carries the format written and every
// warning resolution raised — a degraded feature, a substituted font — so a
// caller composing unattended learns about a degradation from the report
// rather than from a reader noticing an absence later. No byte reaches w
// until resolution and lowering have both succeeded: a rejected
// specification or a block the target format's writer cannot render returns
// an error before w.Write is ever called.
func (v *Vellum) Compose(ctx context.Context, s *spec.Spec, format artifact.Format, w io.Writer) (*artifact.Report, error) {
	res, err := v.resolve(ctx, s, format)
	if err != nil {
		return nil, err
	}
	if err := v.lowerAndWrite(res.Doc, format, w); err != nil {
		return nil, err
	}
	return &artifact.Report{Format: format, Warnings: res.Warnings}, nil
}

// lowerAndWrite dispatches a resolved [fragment.Doc] to the one writer that
// matches format: Lower into the format's own model, then WriteTo it against
// v's configured epoch and producer. The four format packages' Lower and
// WriteTo shapes agree closely enough to make this dispatch a formality —
// they do not agree closely enough to share one interface; see artifact's
// own package doc for why WriteOptions is not unified across them.
func (v *Vellum) lowerAndWrite(fd *fragment.Doc, format artifact.Format, w io.Writer) error {
	switch format {
	case artifact.FormatDOCX:
		d, err := doc.Lower(fd)
		if err != nil {
			return err
		}
		return d.WriteTo(w, doc.WriteOptions{SourceDateEpoch: v.opts.SourceDateEpoch, Producer: v.opts.Producer})
	case artifact.FormatXLSX:
		wb, err := sheet.Lower(fd)
		if err != nil {
			return err
		}
		return wb.WriteTo(w, sheet.WriteOptions{SourceDateEpoch: v.opts.SourceDateEpoch, Producer: v.opts.Producer})
	case artifact.FormatPPTX:
		dk, err := deck.Lower(fd)
		if err != nil {
			return err
		}
		return dk.WriteTo(w, deck.WriteOptions{SourceDateEpoch: v.opts.SourceDateEpoch, Producer: v.opts.Producer})
	case artifact.FormatPDF:
		pd, err := pdf.Lower(fd)
		if err != nil {
			return err
		}
		return pd.WriteTo(w, pdf.WriteOptions{SourceDateEpoch: v.opts.SourceDateEpoch})
	default:
		// Unreachable in practice: resolve.Resolve already rejects any format
		// outside artifact.AllFormats() with VELLUM_SPEC_INVALID before this
		// is ever called. Guarded anyway, because a silent no-op here would
		// be a caller told nothing was wrong while nothing was written.
		return verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_INVALID,
			"unknown output format", map[string]any{"format": string(format)})
	}
}

// Validate runs the same resolution [Vellum.Compose] does — capability
// enforcement, theme, font and asset resolution — and returns every warning
// it raised, without lowering the result into a format model or writing a
// single byte. This is "identity before render": a caller learns what will
// degrade and what will outright reject before committing to a render.
//
// A specification that would be rejected is returned as the error, exactly
// as Compose would return it. The only difference from Compose is that a
// specification which *would* render never reaches a writer here.
func (v *Vellum) Validate(ctx context.Context, s *spec.Spec, format artifact.Format) ([]*verr.CodedError, error) {
	res, err := v.resolve(ctx, s, format)
	if err != nil {
		return nil, err
	}
	return res.Warnings, nil
}

// Write emits a format model a caller built directly — a *doc.Document, a
// *sheet.Workbook, a *deck.Deck or a *pdf.Document — bypassing [spec.Spec]
// and [resolve.Resolve] entirely. This is the "Compose from a format model"
// path in CLAUDE.md's architecture diagram: a consumer with format-specific
// reach who built, or edited, the resolved model itself and only wants it
// written.
//
// model's own concrete type selects the writer; there is deliberately no
// separate format parameter that could disagree with it — the type already
// says which of the four writers applies, and asking the caller to name it a
// second time would only be a second place for the two to say different
// things. A model of a type Write does not recognise is
// VELLUM_ARTIFACT_MODEL_UNSUPPORTED.
//
// The returned [artifact.Report] carries the format written and no
// warnings: a warning is raised during resolution, and this path never runs
// resolution. A caller who needs warnings resolves through Compose instead.
func (v *Vellum) Write(_ context.Context, model any, w io.Writer) (*artifact.Report, error) {
	switch m := model.(type) {
	case *doc.Document:
		if err := m.WriteTo(w, doc.WriteOptions{SourceDateEpoch: v.opts.SourceDateEpoch, Producer: v.opts.Producer}); err != nil {
			return nil, err
		}
		return &artifact.Report{Format: artifact.FormatDOCX}, nil
	case *sheet.Workbook:
		if err := m.WriteTo(w, sheet.WriteOptions{SourceDateEpoch: v.opts.SourceDateEpoch, Producer: v.opts.Producer}); err != nil {
			return nil, err
		}
		return &artifact.Report{Format: artifact.FormatXLSX}, nil
	case *deck.Deck:
		if err := m.WriteTo(w, deck.WriteOptions{SourceDateEpoch: v.opts.SourceDateEpoch, Producer: v.opts.Producer}); err != nil {
			return nil, err
		}
		return &artifact.Report{Format: artifact.FormatPPTX}, nil
	case *pdf.Document:
		if err := m.WriteTo(w, pdf.WriteOptions{SourceDateEpoch: v.opts.SourceDateEpoch}); err != nil {
			return nil, err
		}
		return &artifact.Report{Format: artifact.FormatPDF}, nil
	default:
		return nil, verr.NewCodedError(verr.VELLUM_ARTIFACT_MODEL_UNSUPPORTED,
			"the value is not one of the four resolved format models a Vellum writer accepts")
	}
}

// Fill opens the template named by r and size and binds data into it
// according to b, following the "Fill" path of CLAUDE.md's architecture
// diagram: [template.Open], then [template.Fill]. opts are appended after
// v's own configured evaluator, so a caller's own [template.WithEvaluator]
// wins over Options.Evaluator when both are given — the caller asking
// explicitly, at the call site, is taken as the more specific instruction.
//
// ctx is accepted for consistency with every other I/O-touching method on
// Vellum but is not forwarded anywhere: neither template.Open nor
// template.Fill takes one today.
func (v *Vellum) Fill(_ context.Context, r io.ReaderAt, size int64, b *bind.Binding, data bind.Scope, opts ...template.FillOption) (*template.Result, error) {
	t, err := template.Open(r, size)
	if err != nil {
		return nil, err
	}
	if v.opts.Evaluator != nil {
		merged := make([]template.FillOption, 0, len(opts)+1)
		merged = append(merged, template.WithEvaluator(v.opts.Evaluator))
		merged = append(merged, opts...)
		opts = merged
	}
	return template.Fill(t, b, data, opts...)
}

// Inspect opens the template named by r and size and reports every anchor
// and font family its own XML declares — the "Inspect" path of CLAUDE.md's
// architecture diagram, answerable without a binding.
//
// ctx is accepted for the same reason [Vellum.Fill]'s is: consistency with
// every other I/O-touching method, even though neither template.Open nor
// template.Inspect takes one today.
func (v *Vellum) Inspect(_ context.Context, r io.ReaderAt, size int64) (*template.InspectReport, error) {
	t, err := template.Open(r, size)
	if err != nil {
		return nil, err
	}
	return template.Inspect(t)
}

// Boxes reports the asset slots the theme named by themeID offers for
// format, resolved through Options.Themes by the same [theme.Resolve] call
// [resolve.Resolve] itself makes — so a caller asking Boxes for a theme id
// and later composing a specification against that same id are guaranteed to
// see the same theme document, not two providers' possibly different
// answers.
//
// Answerable with no specification: [theme.Theme.Boxes] is a (theme, format)
// query the theme's own master layouts answer, so a host can pre-render,
// cache and warm against a theme before any document exists.
func (v *Vellum) Boxes(ctx context.Context, themeID string, format artifact.Format) (theme.BoxSet, error) {
	if !artifact.ValidFormat(format) {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_INVALID,
			"unknown output format", map[string]any{"format": string(format)})
	}
	th, err := theme.Resolve(ctx, v.opts.Themes, themeID)
	if err != nil {
		return nil, err
	}
	return th.Boxes(format), nil
}

// Capabilities returns the declared (feature, outcome) matrix for format —
// every row [capability.ForFormat] carries, in feature declaration order.
//
// Pure data: it consults none of v's seams and touches no I/O, so it takes
// no context and cannot fail. An unrecognised format returns an empty
// matrix, matching capability.ForFormat's own behaviour, rather than an
// error — the same reason [capability.ForFormat] itself does not error: the
// matrix has nothing to say about a format it does not know, and saying
// nothing is the honest answer.
func (v *Vellum) Capabilities(format artifact.Format) capability.Matrix {
	return capability.ForFormat(format)
}

// artifactHashTag namespaces the artifact-identity hash. See
// [canon.CanonicalHash].
const artifactHashTag = "vellum.artifact"

// ArtifactName returns the content-addressed name for an artifact composed
// from s and carrying the given resolved asset content hashes.
//
// Both s.Hash() and assetHashes are inputs to a render, never outputs of
// one, which is what makes this callable *before* [Vellum.Compose] runs: a
// consumer can ask "does this artifact already exist" and skip the render
// entirely. See CLAUDE.md's "Artifact identity".
//
// assetHashes is an ordered slice — never a map, per CLAUDE.md's
// determinism conventions — but the order a caller happens to list assets in
// is not part of a document's identity, so it is sorted internally before
// hashing: two callers naming the same set of assets in a different order
// still name the same artifact. Order matters to the document only through
// the specification itself, which s.Hash() already accounts for.
//
// ArtifactName calls no writer and performs no I/O. It never resolves a
// theme, never resolves an asset, and never lowers anything — asset hashing
// happens once, when an asset is first resolved (see [asset.HashFor]), and
// this only combines hashes already computed.
func (v *Vellum) ArtifactName(s *spec.Spec, assetHashes []string) string {
	sorted := make([]string, len(assetHashes))
	copy(sorted, assetHashes)
	sort.Strings(sorted)

	return canon.CanonicalHash(artifactHashTag, struct {
		SpecHash    string   `json:"spec_hash"`
		AssetHashes []string `json:"asset_hashes"`
	}{
		SpecHash:    s.Hash(),
		AssetHashes: sorted,
	})
}
