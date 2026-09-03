# Artifact identity

`Spec.Hash()` plus the hashes of every asset a specification references
together name the artifact those inputs would produce. Both halves are
**inputs to a render, never outputs of one**, and that is the entire point:
an artifact's identity is knowable *before* the render runs, which is what
lets a consumer ask "does this artifact already exist" and skip producing it
entirely rather than discovering the answer only by paying for the work.

## `Spec.Hash()`

```go
func (s *Spec) Hash() string
```

A canonical content hash (`canon.CanonicalHash`, namespaced `"spec"`) with
four guarantees, every one of which is pinned by a committed vector table
(`TestSpecHashPinnedVectors`) rather than merely asserted in prose:

- The same logical specification hashes the same across processes and across
  Vellum versions.
- Field order does not affect it, so a specification authored as JSON and the
  same document authored as YAML hash identically.
- Defaults are normalised before hashing, so setting a field to its default
  value explicitly and omitting it entirely hash alike — a bare `"unit":
  "pt"` and a length whose unit was left unset (when `pt` is the implicit
  default) must not look like two different documents.
- Adding a new `omitempty` field to the `spec` model does not move the hash
  of a specification that omits it.

The last guarantee is the one that is easy to break and expensive to
discover. A consumer keyed on this hash treats a change in it as "this is a
different document, re-render it"; a hash that moved only because Vellum
itself gained a field would tell every downstream consumer that every
document in their system had changed, all at once, for no reason connected
to their own content.

`normalizedForHash` is what makes this true in practice: it works on a
cloned copy (asking for a hash must never rewrite the caller's own
specification as a side effect), applies `FormatVersion` and `Theme`
defaults explicitly, collapses a header node's stated `span` when it merely
restates what the tree shape already implies, collapses a `RowSpan`/`ColSpan`
of 1 to the zero value it is equivalent to, treats `AnnotationSuperscript`
(the default position) as unstated, and drops empty mark slices so `nil` and
`[]string{}` hash alike.

## `Vellum.ArtifactName`

```go
func (v *Vellum) ArtifactName(s *spec.Spec, assetHashes []string) string
```

Combines `s.Hash()` with the content hashes of every resolved asset into one
artifact name (`canon.CanonicalHash`, namespaced `"vellum.artifact"`).
`assetHashes` is an ordered slice, per Vellum's own determinism conventions —
never a map on an output path — but the *order the caller happens to list
assets in* is not itself part of a document's identity, so the slice is
sorted internally before hashing: two callers naming the same set of assets
in different orders name the same artifact. Order matters to a document only
through the specification's own section and block ordering, which
`s.Hash()` already accounts for.

`ArtifactName` calls no writer, performs no I/O, resolves no theme and
resolves no asset. Asset hashing happens once, at the point an asset is first
resolved (`asset.HashFor`) — see [Seams](../library/seams.md#asset-hashing-assethasher)
for the optional `asset.Hasher` seam that lets a host answer "what is this
asset's content hash" without moving the bytes at all, which is what makes
computing an artifact's name cheap enough to do before deciding whether
rendering is worth doing.

## Byte-identity is a separate, narrower guarantee

Artifact identity (spec hash plus asset hashes) and **byte-identity** (the
literal bytes Vellum writes) are two different guarantees with two different
lifetimes, and conflating them is a mistake this project is explicit about
avoiding:

- **Byte-identity** — the same specification, assets and theme produce
  byte-for-byte identical output — makes golden-file testing and
  attestation work. It holds for a fixed Vellum version *and* a fixed Go
  toolchain minor version, because `compress/flate`'s output is stable
  within a toolchain but not guaranteed stable across Go minor releases.
- **Spec-hash identity** is what makes deduplication work, and it holds
  across Vellum versions. A consumer dedupes on `input_hash UNIQUE` and
  should be able to trust that the same logical document produces the same
  hash whether it was rendered by last month's Vellum or today's.

See [Determinism](../internals/determinism.md) for the full pinned-source
table and the honest statement of that toolchain-minor limit.
