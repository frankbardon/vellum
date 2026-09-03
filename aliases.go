package vellum

import (
	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/capability"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/template"
	"github.com/frankbardon/vellum/template/bind"
	"github.com/frankbardon/vellum/theme"
)

// Root aliases so an embedder importing only "github.com/frankbardon/vellum"
// can name the types every facade method above already hands back or takes,
// without a second import for each leaf package. This is deliberately not
// the whole tree: CLAUDE.md's own convention is minimal and deliberate, and a
// consumer authoring a specification's full block vocabulary ([spec.Heading],
// [spec.Table] and so on) or a binding's full statement vocabulary
// ([bind.Repeat], [bind.If] and so on) already needs the [spec] and
// [template/bind] packages imported for that, so aliasing every type in them
// here would be noise, not ergonomics.

// Spec is the primary public model: blocks, sections, tables — the input to
// [Vellum.Compose] and [Vellum.Validate].
type Spec = spec.Spec

// Format names an output format: [FormatDOCX], [FormatXLSX], [FormatPPTX] or
// [FormatPDF].
type Format = artifact.Format

// The four output formats, aliased so a caller writes vellum.FormatDOCX
// rather than importing "github.com/frankbardon/vellum/artifact" for a
// constant [Vellum.Compose], [Vellum.Write], [Vellum.Boxes] and
// [Vellum.Capabilities] all take directly.
const (
	FormatDOCX = artifact.FormatDOCX
	FormatXLSX = artifact.FormatXLSX
	FormatPPTX = artifact.FormatPPTX
	FormatPDF  = artifact.FormatPDF
)

// Report is what [Vellum.Compose] and [Vellum.Write] return alongside a
// successful render: the format written and any warnings resolution raised.
type Report = artifact.Report

// Matrix is what [Vellum.Capabilities] returns: the declared (feature,
// outcome) rows for one format.
type Matrix = capability.Matrix

// BoxSet is what [Vellum.Boxes] returns: the asset slots a theme offers a
// format.
type BoxSet = theme.BoxSet

// CodedError is the concrete error type every Vellum failure mode is
// returned as — a stable [errors.Code] plus a message and structured
// details, never a per-package error type of its own.
type CodedError = verr.CodedError

// Binding is fill mode's declarative statement tree — the second argument to
// [Vellum.Fill].
type Binding = bind.Binding

// Scope is the data a [Binding]'s FEEL expressions evaluate against — the
// third argument to [Vellum.Fill].
type Scope = bind.Scope

// Evaluator is the FEEL evaluation seam [Options.Evaluator] configures. The
// default, used when Options.Evaluator is nil, is [bind.NewFEELEvaluator].
type Evaluator = bind.Evaluator

// FillResult is what [Vellum.Fill] returns: the filled package and the
// receipt of which parts it actually touched.
type FillResult = template.Result

// InspectReport is what [Vellum.Inspect] returns: every anchor and font
// family a template's own XML declares.
type InspectReport = template.InspectReport
