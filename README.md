# Vellum

A declarative artifact emitter for Go: **spec in, document out**. DOCX, XLSX,
PPTX and PDF/A-2b, from one generic block model, with byte-identical output.
Plus a *fill* mode that binds data into an existing OOXML template without
destroying the parts it does not understand.

No cgo. No external binaries. No network I/O.

> **Status: early construction.** The repository, conventions and contract are
> in place and the library is being built epic by epic. Nothing here is
> importable yet. This README describes what Vellum is being built to be; the
> [build board](#roadmap) says how far along it is.

## Why

Three problems, in order.

**There is no credible Go path to a governed document artifact.** `excelize`
handles xlsx well. Nothing in the Go ecosystem writes pptx. docx support is a
graveyard of abandoned half-libraries, and the one complete option is
commercially licensed. Every real Go service that needs a Word document today
either shells out to Python or gives up and emits HTML.

**Generated documents are not reproducible or attributable.** The same input
produces byte-different output on every run, because writers stamp timestamps
and generate random part identifiers. That makes documents undiffable,
untestable against golden files, and impossible to attest. For regulated work
the question "what produced this number, and can you prove this file has not
changed?" has no answer.

**Template filling destroys documents.** Naive libraries parse a `.docx` into an
incomplete object model and re-serialize it, silently discarding everything the
model does not understand: tracked changes, comments, custom XML parts, embedded
objects, footnotes. A designer's carefully built corporate template comes back
mangled.

## Principles

1. **Non-destructive.** Fill touches only the parts it must. Every other part is
   copied through byte-for-byte, and the result carries a receipt naming exactly
   what was rewritten.
2. **Deterministic.** Identical inputs produce byte-identical outputs. A hard
   requirement, not an aspiration — and the reason there is no converter
   subprocess anywhere on the render path.
3. **Attributable.** Every artifact carries machine-readable provenance
   describing what produced it.
4. **Fail loud.** Unknown spec fields, missing fonts, unbound anchors and
   unsupported features are errors. Vellum never silently drops content.
5. **Declared, not emergent.** Whether a block renders, degrades, or is rejected
   in a given format is a row in a queryable matrix *before* it is code — so a
   consumer scheduling an unattended job can learn the answer before the job
   runs.
6. **Subtractive.** Ship the conformance profile and nothing beyond it until a
   second real use case demands more.

## Design in one paragraph

Three content models, each with one job. **`spec`** is unresolved and hashable —
author intent, theme by reference, no measurements — which is what makes an
artifact's identity computable *before* it is rendered, so a caller can ask "does
this already exist" and skip the work. **`fragment`** is resolved and
format-neutral: concrete fonts, EMU lengths, typed values, resolved assets, with
theme application and font substitution done once and shared by every writer.
**`doc` / `sheet` / `deck` / `pdf`** are resolved *and laid out*, because a flow
document, a workbook, a deck and a paginated page tree are genuinely different
things and one IR would serve none of them. All four are public, so a consumer
needing format-specific reach has it.

## What Vellum is not

Vellum **never renders a chart** — it embeds an asset it is handed, at a size
the caller obtained from `Boxes(theme, format)`. It does no data access, no
authorization, and no network I/O. It emits presentation tables, not
spreadsheets: no formulas, no pivot tables. It knows nothing about what a
"report" is; semantic structure is the consumer's vocabulary.

## Roadmap

Thirteen epics, built in dependency order. The substrate and the determinism
harness come first — a non-deterministic packaging layer is unrecoverable
without a rewrite, and retrofitting a determinism suite onto an existing writer
finds bugs you then have to unwind.

| | Epic | What exists at the end |
|---|---|---|
| E1 | Deterministic substrate | A two-block spec becomes a `.docx` whose bytes are identical on every run, in every process, at any `GOMAXPROCS` |
| E2 | Spec surface and artifact identity | Validate, hash, and learn which blocks render in which formats — all before a byte is written |
| E3 | Themes, seams, fonts, layout query | Supply a theme and your own asset storage; ask what size to render a chart at |
| E4 | DOCX compose | A real Word report: headings, tables, images, headers, footnotes, TOC |
| E5 | PPTX compose | A deck, with notes and a table that continues onto extra slides with headers repeated |
| E6 | XLSX compose | Section to sheet, table to cell range, notes to cell comments |
| E7 | PDF substrate | Object writer, font subsetting, text layout |
| E8 | PDF/A-2b | An archival PDF that veraPDF passes with zero errors |
| E9 | Fill: surgical editing | A real template comes back with one slot filled and every other part byte-identical |
| E10 | Fill: binding and FEEL | `repeat` / `if` / `with` over FEEL expressions |
| E11 | Fill: XLSX and PPTX | Defined names, `ListObject` rows, a slide repeated per segment |
| E12 | CLI and MCP | The full verb set, and the same operations as MCP tools |
| E13 | Skills, examples, docs | An agent composes a valid report from the skill pack alone |

## Development

```
make build   # bin/vellum
make test
make lint    # go vet + staticcheck
make cover
make docs    # mdBook
```

Go 1.26 or later. `CGO_ENABLED=0` is exported by the Makefile as a build
contract, not a preference.

Conventions, invariants and the CI gate catalogue live in
[CLAUDE.md](CLAUDE.md). Contribution rules — including the Update Demand, which
requires doc and skill companions in the same PR — are in
[CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT. See [LICENSE](LICENSE).
