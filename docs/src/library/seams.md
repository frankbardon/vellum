# Seams

CLAUDE.md names three "Extension Points": interfaces with an inert default,
so a host that wires nothing still gets a working library rather than a
construction failure. Every one of them is a field on `vellum.Options` — see
[Public facade](facade.md) — and every one follows the identical shape: a
seam interface, one obvious default implementation, and a `Resolve`/`Ingest`
dispatch point that treats a `nil` seam as "use the default" rather than
panicking.

## `asset.Resolver`

```go
type Resolver interface {
    Resolve(ctx context.Context, req Request) (*Asset, error)
}
```

Turns an `Asset` block's opaque `handle` into bytes, a media type and
(optionally) intrinsic pixel dimensions. Vellum owns nothing and fetches
nothing itself — a specification references an asset by handle, never by
bytes, which is what keeps the library entirely ignorant of wherever the
host actually keeps its pictures, and what makes an asset's content hash
cheap to compute before deciding whether a render is even worth doing.

The **default** is `asset.Inline{}`: it serves only `data:` URIs, resolved
straight from the handle itself, with no filesystem, network or process
access at all. This is what makes "wire nothing and it still works" literally
true rather than aspirational — the [Quickstart](../getting-started/quickstart.md)
composes a real document with a real embedded logo using nothing but this
default.

A resolver's request carries the **target format**, because what can be
embedded is format-constrained: PDF has no SVG mechanism at all, and Vellum
will not rasterise and will not ship an SVG-to-PDF translator (that would be
a second renderer, free to drift from whatever produced the original asset).
A host holding several encodings of the same picture uses `Request.Accept`
to serve the right one for the render actually being asked for; a host
holding only the wrong one gets a loud, coded error naming the accepted set,
never a silent drop.

### Asset hashing (`asset.Hasher`)

```go
type Hasher interface {
    AssetHash(ctx context.Context, handle string) (string, bool, error)
}
```

An **optional** second interface a `Resolver` may also implement, letting a
host answer "what is this asset's content hash" without moving the bytes at
all — exactly what makes `Vellum.ArtifactName` cheap enough to call *before*
deciding whether to render. A resolver that does not implement it is not an
error and not a degradation: the type assertion simply fails, and Vellum
falls back to resolving the asset and hashing the bytes itself — the same
answer, by the slower route.

`asset.Map`, an in-memory resolver over a fixed handle set, is a convenience
for tests and small known-at-construction-time asset sets — not the seam
itself. A host reading assets from real storage implements `Resolver`
directly.

## `theme.Provider`

```go
type Provider interface {
    Theme(ctx context.Context, id string) (*Theme, error)
}
```

Resolves a theme id (an empty id means the built-in theme) to a
`*theme.Theme` document. The **default** is `theme.BuiltinProvider{}`, which
serves the one theme Vellum ships and nothing else. An id the provider does
not carry must fail as `VELLUM_THEME_NOT_FOUND` — never a substitution of a
different theme than the one asked for, which CLAUDE.md calls out directly
as producing a document that is "wrong in a way that looks right," the worst
kind of wrong a document library can produce.

`theme.NewStaticProvider(...*Theme)` validates every theme it is given at
*registration* time, not at first use — a broken theme should fail where it
is wired into a service, not on whichever render happens to reach for it
first in production. Whatever provider is wired, `theme.Resolve` (the one
dispatch point every caller, including the facade, actually goes through)
validates the returned document a second time regardless, because a
`Provider` is host-supplied code, and this is the boundary where a host's
own output becomes Vellum's input — trusting it here would move a theme bug
downstream into whichever writer first dereferenced the field that was
missing.

See [Themes](../spec/themes.md) for what a theme document actually contains.

## `bind.Evaluator`

```go
type Evaluator interface {
    Evaluate(expr string, scope Scope) (result any, err error)
}
```

Evaluates one FEEL expression against a `bind.Scope` (`map[string]any`) —
every `bind`, `if.when`, `repeat.over`, `with` value and `skip` condition in
a binding document goes through this seam. The **default**,
`bind.NewFEELEvaluator()`, wraps `pbinitiative/feel` directly, with one
deliberate addition: `bind.Validate` walks the parsed AST and rejects
`now()`/`today()` before evaluation ever runs, because both call
`time.Now()` internally and would otherwise silently break byte-identical
output. See [Bindings and FEEL](../fill/bindings.md#banned-builtins-determinism-reaches-into-feel-too).

Substituting this seam is how a host wires a sandboxed or metered evaluator,
or an entirely different expression language, without Vellum needing an
opinion about which — the interface's one method is genuinely all the
splicer needs from it.

## Why a seam, and not a plugin registry

All three follow the same shape deliberately: a Go interface with exactly
the methods the library actually calls, plus one working default
implementation shipped alongside it. There is no plugin discovery, no
registration-by-string-name, and no configuration file naming an
implementation — a host wires a concrete value into a struct field, in Go,
at construction time, and that is the entire mechanism. This keeps every
seam statically type-checked and keeps "what does this Vellum instance
actually do when asked for a theme" answerable by reading the one call to
`vellum.New` that constructed it.

## See also

- [Public facade](facade.md) — where these three fields live on `Options`.
- [Themes](../spec/themes.md) — the document `theme.Provider` serves.
- [Bindings and FEEL](../fill/bindings.md) — what `bind.Evaluator` actually
  evaluates.
