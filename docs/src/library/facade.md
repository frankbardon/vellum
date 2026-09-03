# Public facade

Everything documented on this page lives in `vellum.go` at the module root.
This is the single entry point CLAUDE.md means when it says "the library is
the deliverable; the CLI is an adapter" — the CLI, the MCP surface, and a
Go embedder all call the identical `*Vellum` methods.

## Constructing one

```go
type Options struct {
    Assets          asset.Resolver
    AssetOptions    asset.Options
    Themes          theme.Provider
    Evaluator       bind.Evaluator
    SourceDateEpoch time.Time
    Producer        string
}

v, err := vellum.New(vellum.Options{})
```

`Options` is a plain struct, per CLAUDE.md's naming convention ("facade
config is an `Options` struct; leaf packages use `With*` functional
options") — `New` assembles this struct's fields into the functional options
the leaf packages it calls (`resolve`, `template`) actually take. Every
field is a seam with an inert default: the zero-value `Options{}` produces a
fully working library serving the built-in theme, resolving only inline
(`data:`) assets, evaluating FEEL with the default engine, and writing
against the pinned deterministic 1980 epoch. None of the interface-typed
fields are substituted with a concrete default *at construction time* —
each is nil-checked exactly once, at the one place downstream that actually
interprets it, so `New` never becomes a second place a default could be
decided differently than the place that matters. `*Vellum` holds nothing but
the `Options` it was built from and caches nothing derived from it, which is
what makes a `*Vellum` safe for concurrent use without any locking: nothing
about it is mutated after `New` returns.

## The eight methods

**`Compose(ctx, s *spec.Spec, format artifact.Format, w io.Writer) (*artifact.Report, error)`**
— resolve `s` against `format` (theme, font, asset resolution and capability
enforcement, all inside `resolve.Resolve`), lower the result into the
format's own model, and write it to `w`. No byte reaches `w` until
resolution *and* lowering have both succeeded — a rejected specification or
an unrenderable block returns an error before `w.Write` is ever called. The
returned `artifact.Report` carries every warning resolution raised (a
degraded feature, a substituted font), so a caller composing unattended
learns about a degradation from the report rather than from a reader
noticing an absence weeks later.

**`Validate(ctx, s *spec.Spec, format artifact.Format) ([]*errors.CodedError, error)`**
— runs the identical resolution `Compose` does and returns every warning it
raised, without lowering into a format model or writing a single byte. A
specification that would be rejected is returned as the error, exactly as
`Compose` would return it; the only difference is that a specification which
*would* render never reaches a writer here. This is "identity before
render" in method form.

**`Write(ctx, model any, w io.Writer) (*artifact.Report, error)`** — the
"compose from a format model" path: writes a caller-built `*doc.Document`,
`*sheet.Workbook`, `*deck.Deck` or `*pdf.Document` directly, bypassing
`spec.Spec` and `resolve.Resolve` entirely. The model's own concrete Go type
selects the writer — there is deliberately no separate format parameter that
could disagree with it. A value of a type `Write` does not recognise is
`VELLUM_ARTIFACT_MODEL_UNSUPPORTED`. This path carries no warnings in its
report, because warnings are raised during resolution and this path never
resolves anything; a caller who needs them uses `Compose`.

**`Fill(ctx, r io.ReaderAt, size int64, b *bind.Binding, data bind.Scope, opts ...template.FillOption) (*template.Result, error)`**
— `template.Open` then `template.Fill`. `v`'s own `Options.Evaluator`, when
set, is prepended to `opts`, so an explicit `template.WithEvaluator` passed
at the call site still wins — the caller asking at the point of the call is
taken as the more specific instruction. See [Fill mode](../fill/anchors.md).

**`Inspect(ctx, r io.ReaderAt, size int64) (*template.InspectReport, error)`**
— `template.Open` then `template.Inspect`: every anchor and font family a
template's own XML declares, answerable with no binding at all.

**`Boxes(ctx, themeID string, format artifact.Format) (theme.BoxSet, error)`**
— resolves `themeID` through the identical `theme.Resolve` call
`resolve.Resolve` itself makes, and returns the format's asset slots. A
caller asking `Boxes` for a theme id and later composing a specification
against that same id are guaranteed to see the *same* theme document a
provider might otherwise have answered differently between two calls.
Answerable before any specification exists.

**`Capabilities(format artifact.Format) capability.Matrix`** — the declared
(feature, outcome) matrix for `format`, in feature declaration order. Pure
data: consults none of `v`'s seams, touches no I/O, takes no `context.Context`
and cannot fail. An unrecognised format returns an empty matrix rather than
an error, matching `capability.ForFormat`'s own behaviour — the matrix
genuinely has nothing to say about a format it does not know.

**`ArtifactName(s *spec.Spec, assetHashes []string) string`** — see
[Artifact identity](../spec/identity.md). Calls no writer, resolves nothing,
performs no I/O; combines `s.Hash()` with the (internally sorted)
`assetHashes` into one content-addressed name.

## Root aliases

`aliases.go` re-exports the types an embedder reaches for constantly —
`vellum.Spec` for `spec.Spec`, and similarly for the other common leaf types
— so the common case is a single import (`"github.com/frankbardon/vellum"`)
without giving up the ability to import a leaf package (`theme`, `asset`,
`template`) directly for deeper configuration when that is actually needed.

## No unifying writer interface, on purpose

`artifact/`'s own package doc explains why `Compose`, `Write` and
`lowerAndWrite` dispatch on `artifact.Format` and on a model's own concrete
Go type directly, rather than through one shared `Writer` interface: each
format's `WriteOptions` genuinely carries different fields — PDF's
`PageTree`/`Uncompressed` options have no OOXML counterpart at all, and the
three OOXML formats' shared `Producer` field has no PDF equivalent — and an
interface abstract enough to unify them would need to erase exactly the
difference that makes each format's options meaningful.

## See also

- [Seams](seams.md) — the three interfaces `Options` wires: `asset.Resolver`,
  `theme.Provider`, `bind.Evaluator`.
- [The Spec](../spec/blocks.md) — what `s *spec.Spec` actually is.
- [Fill Mode](../fill/anchors.md) — the `Fill`/`Inspect` half of this facade
  in depth.
- [Architecture](../internals/architecture.md) — where `resolve.Resolve` and
  lowering sit in the pipeline this facade drives.
