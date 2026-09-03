# Payload schema

`descriptor.BuildPayloadSchema()` returns one JSON Schema document covering
everything on Vellum's wire: the envelope shape itself, the specification
model, and every request/result shape a caller can send or receive. It is
published at
`https://frankbardon.github.io/vellum/payload-schema-1.0.json` — the id
carries the format version, so a consumer pinning one version is never
handed a different one silently.

Get it three ways, all backed by the identical generated document:

- **CLI**: `vellum schema` — writes the raw schema, unwrapped (see [The
  output envelope](envelope.md#the-one-exception-vellum-schema)).
- **MCP**: the `vellum_schema` tool — carries the schema as one field of a
  normal envelope, since MCP has no unwrapped transport mode.
- **Static file**: the [docs deploy workflow](#published-alongside-this-book)
  publishes a copy of the golden alongside this book.

## Why the specification schema is embedded, not referenced

`BuildPayloadSchema` unmarshals `spec.Schema()` — the specification's own,
independently generated schema — and folds its `$defs` directly into this
larger document's own `$defs`, dropping only the inner `$schema`/`$id`/`$defs`
wrapper keywords that belong to a document, not to a definition living
inside one. This is a deliberate choice, not an implementation shortcut:
there is exactly **one** definition of what a `heading` block or a table
`Cell` looks like, embedded here, rather than two documents that could
independently describe the same shape and quietly drift apart.
`TestPayloadSchemaEmbedsSpecDefinitions` pins this.

That single definition is what actually drives two different things at
once: **runtime validation** — the same schema Vellum's own strict decoder
checks a specification against — and **the tool contract an agent authors
against** when composing a specification through `vellum_compose`. Because
it is one document serving both roles, a drift between "what the schema
says a block looks like" and "what the decoder actually accepts" is not a
documentation bug waiting to be noticed — it would be a validation failure
the moment the two disagreed, caught by `TestPayloadSchemaGolden` and
`TestPayloadSchemaEnumsMatchRegistry` before it ever reached a consumer.

## What the top-level document contains

Beyond the embedded `spec` definitions, the schema's own `$defs` declare:

- **`envelope`** — the exact shape documented in [The output
  envelope](envelope.md): `format_version`, `data`, an optional `request`,
  and `errors`/`warnings` arrays of `entry` objects.
- **`entry`** — one error or warning: a `code` matching
  `^VELLUM_[A-Z0-9_]+$`, a `message`, and optional structured `details`.
- **`format`** — an enum of every `artifact.Format` value, generated from
  `artifact.AllFormats()` rather than hand-typed, so a new output format
  cannot go undeclared here.
- **`composeRequest`** / **`validateRequest`** — the `{ spec, format, theme?
  }` shape both `vellum_compose` and `vellum_validate` (and their CLI
  equivalents) accept.
- **`validateResult`** — `{ ok, rejections?, degradations? }`, each of the
  latter two an array of `capabilityFault`.
- **`capabilityFault`** — one capability-matrix outcome located in a
  specific block: `feature`, `outcome`, an optional `degrade` alternative, a
  `code`, and the block's own coordinates (`section_index`, optional
  `section_id`, `block_index`, `kind`).

Every enum-valued field anywhere in this document — block kinds, formats,
value kinds, annotation positions, and so on — is generated from the live Go
registry it names (`enumOf`), never hand-maintained as a parallel list.
`TestPayloadSchemaEnumsMatchRegistry` is the gate that keeps that promise
honest.

## Regenerating the golden

The published document is a committed golden
(`descriptor/testdata/payload-schema.json`), asserted byte-for-byte by
`TestPayloadSchemaGolden` and never hand-edited — `TestGoldensNotHandEdited`
enforces that every golden in the repository ends with a valid
`// golden-hash:` trailer or a hashed `.sha256` sidecar. Any `spec` type
reachable from a payload, or any registry enum surfaced in this schema,
regenerates it:

```sh
go test ./descriptor/ -run TestPayloadSchemaGolden -update
```

per CLAUDE.md's Update Demand table, in the same PR as the change that
required it.

## Published alongside this book

The [docs-deploy workflow](https://github.com/frankbardon/vellum/blob/main/.github/workflows/docs.yml)
copies `descriptor/testdata/payload-schema.json` into the built book as
`payload-schema.json`, stripping the golden-hash trailer line the source
file carries (that trailer is a build-time integrity marker, not part of
the schema document itself) — so the schema published alongside this site
is always exactly the golden the test suite already pins, with nothing
translated or hand-copied in between.

## See also

- [The output envelope](envelope.md) — the wire shape this schema's
  `envelope` definition describes.
- [The Spec](../spec/blocks.md) — the model whose schema is embedded here.
- `vellum_schema` in [MCP Integration](../mcp/index.md).
