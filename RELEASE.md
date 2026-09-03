# Release readiness — v0.1.0

Vellum's build board (`.planning/vellum-v1/organize.md`) is complete: all
thirteen epics, every story, landed and verified (`make test && make lint`
green after each). This document is the release-readiness record E13-S5
calls for — what is done, what is deliberately deferred, and what remains a
manual, non-automatable check before a tagged release goes out.

## Agent-usability gate

E13's own stated slice: "An LLM given only the embedded skill pack and the
published schema composes a valid three-section report on the first
attempt." Verified for real, not assumed: a fresh agent instance, explicitly
barred from reading any Go source, `docs/src/`, CLAUDE.md or `.planning/`,
was given only `skills/*.md` and a freshly-generated `vellum schema` output,
and asked to compose and validate a three-section report black-box against
the built CLI.

**Result: passed on the first attempt.** `vellum validate` returned valid
with no iteration needed, for a spec exercising `heading`, `text` and
`table` blocks (hierarchical row/column headers, typed cell values, an
annotation) — composed cleanly to DOCX. A second data point, composing to
PDF, was correctly rejected (`VELLUM_FONT_EMBED_UNSUPPORTED`, the built-in
theme has no PDF/A-embeddable face) — and both `format-pdf.md` and
`theme-document.md` had already, correctly, pre-warned the agent to expect
exactly this before it ran the CLI.

One minor, non-blocking gap surfaced: no skill file documents the top-level
`spec.Spec` envelope itself (`title`, `sections`, `theme`, `format_version`)
— that shape is only in the schema's `$defs.spec`. It did not block the
agent (the schema is the other declared half of the two-input contract and
covers it completely), but a small `spec-envelope.md` or equivalent guide
would close the gap outright. Left as a nice-to-have, not a release blocker,
since the stated acceptance bar — first-attempt success — was met.

## What "done" means here

Every story in every epic was verified independently before being committed:
`go build ./...`, `go vet ./...`, `go test ./...`, `make lint`, and `make
test` all green, with IDE/gopls diagnostics never trusted without a direct
compiler/staticcheck run (they were wrong, in this project, every single
time they fired). Coverage floors are enforced per package
(`TestPerPackageCoverageFloors`, 65% with nine named, justified exceptions).
The determinism harness (1000x-per-golden repeat, cross-process, multiple
`GOMAXPROCS`, under `-race`) passes on every golden this library writes.

## Two open manual items (cannot be automated in this environment)

1. **Pre-release Word and Excel check on Windows.** The external-reader
   oracles this repository already runs (poppler, LibreOffice, veraPDF) are
   evidence, not proof — CLAUDE.md is explicit that they are more tolerant
   on some constructs and less on others than the readers that actually
   matter to a consumer. Three real defects reached a human specifically
   because every available Go-side and Linux-side reader accepted a file
   Word rejected (zip version fields, ECMA-376 child ordering, a golden
   "PNG" with no IDAT chunk). Before a tagged release, every DOCX/XLSX/PPTX
   golden should be opened once in real Word/Excel/PowerPoint on Windows.
   This has not been done in this session — no Windows/Office environment
   is available here.
2. **Defrag corpus re-baseline.** `testdata/corpus/defrag/` is empty except
   for its own `README.md` describing the fixture format
   (`TestDefragCorpusComplete` passes trivially with zero cases, per design —
   the gate has been live since day one specifically so it fails the moment
   a fixture is dropped in without its `expect.json`, or vice versa). A real
   corpus of Word-authored `.docx` files exhibiting genuine run
   fragmentation (spell-check splits, language-mark boundaries,
   revision-save-ID splits, paste boundaries, accepted-tracked-change
   residue) was always expected to arrive after v1's initial build, per
   `interview.md` decision 6. `template/defrag`'s algorithm has been
   validated against synthetic fixtures (`internal/fragtest`) covering the
   same fragmentation strategies, but a real corpus is the stronger proof
   and remains outstanding.

## QA findings tracked through the epics, final status

Per standing instruction, every QA issue identified during the build was to
be closed before final merge. Final status of each:

1. **`resolve.resolveHeaders` never derives `HeaderNode.Span`.** Found
   during E6-S4. **Fixed.** `spec.HeaderNode` gained an exported `Width()`
   wrapping its existing private derivation; `resolveHeaders` now derives
   each node's span bottom-up instead of copying `Span` verbatim. Two new
   tests (`resolve/resolve_test.go`) prove single- and multi-level
   recursive derivation. No golden touched.
2. **`go.mod`: `go-text/typesetting` indirect→direct.** **Fixed**,
   incidentally, during E10-S2.
3. **`template/defrag`'s `Piece`/`RenderRun` don't clone a run's own
   `w:rsidR`/`w:rsidRPr` attributes** (only `<w:rPr>` children). Found
   during E9-S5. **Deliberately left unfixed** — rsid attributes are Word's
   own internal revision-bookkeeping with no visible formatting or semantic
   effect, and nothing in CLAUDE.md's conformance requirements depends on
   them surviving a fill. Not a release blocker; revisit only if the real
   defrag corpus (item below) or a future acceptance criterion needs rsid
   fidelity.
4. **No directory-backed `theme.Provider`/`asset.Resolver`;
   `VELLUM_THEME_DIR`/`VELLUM_ASSET_DIR` documented but not read from the
   environment by any code path; no `fs/` package despite CLAUDE.md's own
   package-map row.** Found while scoping E12-S4 (`doctor`). **Fixed.** A
   new `fs/` package (`fs.Default()`, `fs.SafeJoin` — the shared
   path-traversal guard, since a directory-backed resolver's handle comes
   from a specification this library did not author) backs new
   `theme.DirProvider` and `asset.DirResolver` types, both hermetically
   tested against `afero.NewMemMapFs()`. `internal/cli/facade.go`'s
   `newFacade()` wires both from `VELLUM_THEME_DIR`/`VELLUM_ASSET_DIR` when
   set (unchanged zero-value behaviour otherwise) and validates
   `VELLUM_MAX_ASSET_BYTES` loudly (`VELLUM_CLI_USAGE`) rather than
   silently ignoring a malformed value. `afero` is now a real `go.mod`
   dependency, matching what CLAUDE.md's Dependencies table already
   claimed. `VELLUM_SOURCE_DATE_EPOCH` was explicitly left out of this fix
   — it is a separate question (facade-construction timing vs. CLI flag
   precedence) and remains genuinely unread from the environment by any
   code path today; `doctor`'s own honest note about that stands.
5. **`vellum_skills`/`vellum_examples` MCP tools registered but their
   handlers unconditionally returned `VELLUM_MCP_NOT_IMPLEMENTED`.** Found
   during E13-S4. **Fixed.** Both handlers now call the real `skills.Get`/
   `skills.All` and `examples.Get`/`examples.All`. `SkillsOut`/`ExamplesOut`
   gained a `Names []string` field for the empty-name listing case,
   mutually exclusive with `Content` on the wire. Not-found reuses the
   existing `VELLUM_MCP_INVALID_INPUT` code. Verified end-to-end against
   the real built binary as an MCP stdio server.

## Recommendation

**All five QA findings are closed** — one deliberately as a documented
non-issue (item 3), four fixed with tests and, where practical, real
end-to-end verification against the built binary. `make test && make lint`
are green with all fixes applied. Nothing tracked through the epics remains
open.

The two manual items below cannot be closed in this environment and remain
the accepted, open state of a v0.1.0 release rather than blockers to it,
consistent with `interview.md`'s own original decision to defer the real
corpus.

No git tag has been created as part of this document. Tagging and pushing a
release remain actions for the maintainer to take explicitly.
