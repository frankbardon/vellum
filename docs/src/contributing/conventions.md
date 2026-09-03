# Contributing

The full contribution guide lives at
[`CONTRIBUTING.md`](https://github.com/frankbardon/vellum/blob/main/CONTRIBUTING.md)
at the repository root, and is worth reading in full before opening a pull
request — this page summarises its shape and links the details this book has
already covered elsewhere, rather than restating it wholesale.

## The short version

1. Fork, branch off `main`, make the change with tests.
2. `make test && make lint` — both are required CI contexts.
3. Update every companion the Update Demand table names, **in the same
   pull request**.
4. Open the PR with the template filled in honestly, including what was
   deliberately left undone.

## Library-first, always

The library is the deliverable; the CLI is a thin adapter over it. No
business logic lives in `cmd/vellum/` or `internal/cli/` — those packages
parse flags and call exactly one method on the facade, the same discipline
[Public facade](../library/facade.md) documents from the other direction.
A change that adds behaviour to a CLI verb file rather than to the facade
method it calls is very likely a change in the wrong place.

## The Update Demand

Any change to Vellum code, configuration, file format or public surface
**must** update the corresponding skill file(s) and `CLAUDE.md` in the same
pull request — this is a non-skippable CI failure, not a style suggestion.
CLAUDE.md's own Update Demand table is the authoritative, exhaustive list of
which trigger requires which companion update and which test enforces it;
skimming it before starting a change that touches a block kind, a format, a
capability row, an error code, an MCP tool, a CLI leaf, or a byte-layout
rule will save a second round trip through review. Deferring a doc or skill
update to "a follow-up PR" is explicitly called out as not happening in
practice — update in the same PR, or the change does not merge.

## Determinism is not optional

Never `time.Now()`, never a random or code-path-dependent identifier, never
map iteration on an output path. See
[Determinism](../internals/determinism.md) for the full reasoning and the
test harness that enforces it; `TestNoTimeNow`, `TestNoUnsortedMapIteration`
and `TestNoCollateImport` are AST- and `go list -deps`-based gates, not
review-time vigilance.

## Coded errors, not ad hoc ones

Every failure mode is a `VELLUM_<AREA>_<CATEGORY>` code via the typed
`errors.Code` — no per-package error types. A new code needs a
`codeMetadata` row carrying a `Message` and at least one `Fixup`, or an
explicit `FixupNotApplicable: true` when a fixup genuinely does not apply
(an internal invariant violation, say) — silence on that field is treated as
a bug, never as an implicit "no fixup needed."

## Declared, not emergent

A behaviour a consumer can actually observe — a block degrading in a given
format, an asset media type being refused, a table continuing onto another
page — is a row in `capability/matrix.go` *before* it is code. See
[Capability matrix](../formats/capabilities.md).

## Tests first, goldens never hand-edited

A pull request that changes behaviour without a test that would have caught
the old behaviour is not ready. Goldens are regenerated with `-update`, from
code, never hand-edited "just to fix formatting" — `TestGoldensNotHandEdited`
exists specifically because that phrase is exactly how a golden stops being
evidence of anything.

## Reporting a bug or suggesting a feature

For a rendering defect, attach the specification (or a minimised version of
it) and say which application opened the output, and at what version. For a
feature request, say what outcome you are trying to deliver, not which
specific OOXML or PDF element you want emitted — Vellum is deliberately
subtractive (see CLAUDE.md's "What NOT to Do"), and the useful framing is
the problem, not the implementation.
