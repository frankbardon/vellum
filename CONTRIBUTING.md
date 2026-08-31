# Contributing to Vellum

## Getting started

1. Fork the repository and clone your fork.
2. Create a branch off `main`.
3. Make your change, with tests.
4. Run `make test && make lint`. Both are required CI contexts.
5. Update the companions the Update Demand table names — in the same PR.
6. Push and open a pull request.
7. Fill in the PR template honestly, including what you did not do.

## Development setup

Go 1.26 or later. No C toolchain, ever — `CGO_ENABLED=0` is exported by the
Makefile as a build contract.

```
make build   # bin/vellum
make test    # go test ./...
make lint    # go vet + staticcheck
make cover   # coverage profile and per-function report
```

## Code conventions

- **Library-first.** The library is the deliverable; the CLI is a thin adapter.
  No business logic in `cmd/vellum/` or `internal/cli/` — they parse flags and
  call the facade.
- **Deterministic by construction.** Identical inputs produce byte-identical
  outputs. Never `time.Now()`, never a random identifier, never map iteration on
  an output path. Where an ordering is needed, sort it or carry an explicit
  order slice. Prefer making nondeterminism unrepresentable in an API over
  testing for its absence.
- **Fail loud.** Unknown spec fields, missing fonts, unbound anchors and
  unsupported features are errors. Vellum never silently drops content.
- **Coded errors only.** `VELLUM_<AREA>_<CATEGORY>` via the typed
  `errors.Code`. Every new code needs a `codeMetadata` row with a `Message` and
  at least one `Fixup`, or an explicit `FixupNotApplicable: true`.
- **Declared, not emergent.** A behaviour a consumer can observe — a block
  degrading, an asset media type being refused — is a row in the capability
  matrix before it is code.
- **No `fmt.Sprintf` for JSON.** Use `encoding/json` and
  `descriptor.NewEnvelope`.
- **All file I/O through `afero.Fs`.** Never `os.Open` in library code. Tests
  are hermetic on `afero.NewMemMapFs()`.
- **No logging.** Diagnostics are returned as data: coded errors with details,
  warnings on the envelope, report structs.
- `descriptor/` is no-execute: it must not import a renderer.

## The Update Demand

Any change to code, configuration, file format or public surface MUST update the
corresponding skill file(s) and `CLAUDE.md` in the same PR. This is a
non-skippable CI failure, not a convention.

Defer a doc or skill update to "a follow-up PR" and the follow-up will not
happen. Update in the same PR or do not merge.

## Pull requests

Tests first. A PR that changes behaviour without a test that would have caught
the old behaviour is not ready. Goldens are regenerated with `-update`, never
hand-edited.

## Reporting bugs

Open an issue with the bug template. For a rendering defect, attach the spec (or
a minimised version of it) and say which application you opened the output in
and at what version.

## Suggesting features

Open an issue with the feature template. Vellum is deliberately subtractive: say
what you are trying to deliver, not which OOXML element you want emitted.
