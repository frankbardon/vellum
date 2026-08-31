# Security Policy

## Reporting a vulnerability

Report privately through a GitHub security advisory on this repository. Do not
open a public issue.

Expect an acknowledgement within 72 hours.

## Scope

Vellum parses untrusted input in more places than a pure emitter would, because
fill mode accepts documents. The layers that matter:

- **`opc` / `opc/zipdet`** — ZIP and OPC package parsing. A template is
  attacker-controlled the moment a consumer accepts an upload. In scope: zip
  bombs, path traversal in entry names, truncated and malformed archives,
  duplicate parts, dangling and cyclic relationship targets.
- **`template` / `xmlcopy`** — XML token rewriting over untrusted parts. In
  scope: entity expansion, deeply nested structures, and any input that causes a
  panic rather than a coded error.
- **`bind`** — FEEL expression evaluation. Expressions are evaluated against
  caller-supplied data. In scope: unbounded evaluation and any escape from the
  expression sandbox.
- **`asset`** — resolver-supplied bytes. In scope: media-type confusion and
  decompression limits.
- **`pdf`** — font program parsing during subsetting. In scope: malformed SFNT
  tables causing out-of-bounds behaviour.

Out of scope: the correctness or appearance of a rendered document, and any
issue that requires the operator to have already been compromised.

## Known considerations

- Vellum performs **no network I/O**. Assets and themes arrive through
  caller-supplied seams; Vellum fetches nothing itself.
- Vellum executes **no external binaries**. There is no converter subprocess and
  no shelling out on any code path.
- All filesystem access goes through an `afero.Fs`, so an embedder can confine
  Vellum to a memory filesystem entirely.
- A single asset is bounded by `Options.MaxAssetBytes`, and archive expansion is
  bounded, to make decompression bombs a coded error rather than an OOM.
