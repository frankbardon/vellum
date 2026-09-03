# Vellum

A declarative artifact emitter for Go: **spec in, document out**. DOCX, XLSX,
PPTX and PDF/A-2b from one generic block model, with byte-identical output —
plus a *fill* mode that binds data into an existing OOXML template without
destroying the parts it does not understand.

No cgo. No external binaries. No network I/O.

**Status: built.** Epics E1 through E13 are complete. The library composes
all four formats from a `spec.Spec`, fills DOCX/XLSX/PPTX templates
non-destructively, ships a full CLI (`vellum compose` / `validate` / `fill` /
`inspect` / `boxes` / `capabilities` / `schema` / `provenance` / `mcp` /
`doctor`), a Model Context Protocol server exposing ten tools, and an
embedded skill pack an LLM reads over MCP at runtime. Every package carries
enforced per-package coverage floors and a determinism harness that composes
each golden a thousand times, across processes, at multiple `GOMAXPROCS`,
and checks for exactly one SHA-256. This README describes what exists today,
not a roadmap.

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
   copied through byte-for-byte, and the result (`Result.Touched`) carries a
   receipt naming exactly what was rewritten — proven against real
   Word/Excel/PowerPoint-shaped fixtures carrying tracked changes, comments,
   custom XML, footnotes and embedded objects.
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

## Installation

Go 1.26 or later. As a library:

```sh
go get github.com/frankbardon/vellum
```

As a CLI:

```sh
go install github.com/frankbardon/vellum/cmd/vellum@latest
```

See [Installation](https://frankbardon.github.io/vellum/getting-started/installation.html)
for building from a checkout and verifying the environment with `vellum
doctor`.

## Quickstart

```go
package main

import (
	"context"
	"os"

	"github.com/frankbardon/vellum"
	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/spec"
)

func main() {
	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Title:         "Quarterly Business Review",
		Sections: []spec.Section{{
			Blocks: []spec.Block{
				{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: 1, Content: "Quarterly Business Review"}},
				{Kind: spec.BlockText, Text: &spec.Text{Content: "Revenue grew across every region this quarter."}},
			},
		}},
	}

	v, _ := vellum.New(vellum.Options{})
	out, _ := os.Create("report.docx")
	defer out.Close()

	if _, err := v.Compose(context.Background(), s, artifact.FormatDOCX, out); err != nil {
		panic(err)
	}
}
```

`vellum.New(vellum.Options{})` is the zero-configuration path — every seam
(theme, assets, FEEL evaluator) defaults to something that works over the
built-in theme and inline (`data:`) assets. See the full walkthrough,
including a realistic multi-section report composed from
[`examples/format-docx.json`](examples/format-docx.json), in the
[Quickstart](https://frankbardon.github.io/vellum/getting-started/quickstart.html).

## CLI reference

```sh
vellum compose report.json --format docx -o report.docx
vellum validate report.json --format pdf
vellum fill --template template.docx --binding binding.json --data data.json -o filled.docx
vellum inspect template.docx
vellum boxes --format pptx
vellum capabilities --format xlsx
vellum schema
vellum provenance report.docx
vellum mcp
vellum doctor
```

Every command accepts `--json` (except `schema`, which always writes raw,
unwrapped JSON Schema) and follows one exit-code convention: `0` success,
`1` a well-formed request that failed at compose/fill time, `2` a malformed
invocation. See
[Commands and flags](https://frankbardon.github.io/vellum/cli/flags.html)
for the full index, or `vellum <command> --help`.

## MCP usage

```sh
vellum mcp
```

Runs a Model Context Protocol server over stdin/stdout for a client to
launch as a subprocess. Ten tools are registered —
`vellum_compose`, `vellum_validate`, `vellum_inspect`, `vellum_fill`,
`vellum_capabilities`, `vellum_boxes`, `vellum_schema`, `vellum_manifest`,
`vellum_skills`, `vellum_examples` — backed by the identical facade the CLI
and library use, with every response carrying the same envelope shape
(below). Embed the server directly with `mcpserve.New` +
`mcpserve.ServeStdio` rather than shelling out to the binary. See
[MCP Integration](https://frankbardon.github.io/vellum/mcp/index.html),
including the current, honestly-documented state of `vellum_skills` and
`vellum_examples`.

## Embedding in Go

```go
v, err := vellum.New(vellum.Options{
	Assets:    myAssetResolver{},   // asset.Resolver; nil serves data: URIs only
	Themes:    myThemeProvider{},   // theme.Provider; nil serves the built-in theme
	Evaluator: myFEELEvaluator{},   // bind.Evaluator; nil selects the default FEEL engine
})
```

`Vellum` exposes eight methods: `Compose`, `Validate`, `Write` (a
caller-built `*doc.Document`/`*sheet.Workbook`/`*deck.Deck`/`*pdf.Document`,
bypassing `spec.Spec` entirely), `Fill`, `Inspect`, `Boxes`, `Capabilities`
and `ArtifactName`. Every seam is an interface with an inert default, so
`vellum.New(vellum.Options{})` is already a complete, working library. See
[Public facade](https://frankbardon.github.io/vellum/library/facade.html)
and [Seams](https://frankbardon.github.io/vellum/library/seams.html).

## Skill pack

`skills/` is a flat set of markdown files, embedded into the binary and
served over MCP, written for an LLM composing a spec or a binding at
runtime rather than for a human reading prose — one file per block kind
(`block-heading.md`), per output format (`format-docx.md`), per MCP tool
(`tool-compose.md`), per fill-mode concept (`fill-binding.md`), plus the
theme document. Every skill's coverage is enforced: a new block kind, format
or MCP tool without a matching skill file fails the build. This is a
distinct document from this site's own [prose
documentation](https://frankbardon.github.io/vellum) — `docs/` explains why
something is shaped the way it is; a skill file answers "how do I do this
correctly, right now" as tersely as the token budget allows.

`examples/` is the companion runnable pack: a proven, composing-or-filling
JSON specification per block kind, per format, and per fill-mode statement
kind — never merely parsed, always actually exercised through the facade by
its own test suite.

## Spec format

```json
{
  "format_version": "1.0",
  "title": "…",
  "sections": [
    { "id": "…", "blocks": [
      { "kind": "heading", "heading": { "level": 1, "content": "…" } },
      { "kind": "text", "text": { "content": "…" } }
    ] }
  ]
}
```

Seven block kinds, closed and format-agnostic — `heading`, `text`, `asset`,
`table`, `page_break`, `notes`, `spacer` — decoded strictly (an unrecognised
field is a build failure, never a silent skip) and validated for shape
before a target format's capability matrix is ever consulted. A theme is
referenced by id, never embedded, which is what makes `Spec.Hash()`
computable before any theme resolution happens at all. See [The block
model](https://frankbardon.github.io/vellum/spec/blocks.html), [The
capability matrix](https://frankbardon.github.io/vellum/formats/capabilities.html),
and [Artifact identity](https://frankbardon.github.io/vellum/spec/identity.html).

## Configuration

| Variable | Purpose |
|---|---|
| `VELLUM_THEME_DIR` | Directory the default `ThemeProvider` reads theme documents from. Unset means the built-in theme only — no directory-backed provider reads this today. |
| `VELLUM_ASSET_DIR` | Directory the default `AssetResolver` reads asset handles from. Unset means inline (`data:`) assets only — no directory-backed resolver reads this today. |
| `VELLUM_SOURCE_DATE_EPOCH` | RFC 3339 timestamp pinning every date Vellum writes. Unset selects a pinned 1980 epoch, the deterministic default. |
| `VELLUM_MAX_ASSET_BYTES` | Cap on a single resolved asset. Unset selects the built-in default (64 MiB). |
| `VELLUM_VERAPDF` | Path to a local veraPDF install, or a digest-pinned `container:` reference, for the build-tagged PDF/A conformance gate. Test-only. |
| `VELLUM_SOFFICE` | Path to a LibreOffice `soffice` binary, for the build-tagged OOXML reader-oracle gate. Test-only; the library itself runs no subprocess. |
| `VELLUM_REQUIRE_OPTIONAL_GATES` | Turns a missing external tool from a skipped test into a failed one. Set in CI. |

`VELLUM_THEME_DIR` and `VELLUM_ASSET_DIR` are read honestly above: both name
an intended directory-backed seam that has no implementation shipping yet —
`vellum doctor` reports exactly this when either is set. `vellum doctor`
checks every one of these, plus the built-in theme's fonts and the PDF/A
sRGB ICC profile, in one pass.

## Output format contract

Every `--json` CLI invocation and every MCP tool response writes one
envelope shape (`format_version` currently `"1.0"`):

```json
{
  "format_version": "1.0",
  "data": { },
  "request": { },
  "errors":   [ { "code": "VELLUM_...", "message": "...", "details": { } } ],
  "warnings": [ { "code": "VELLUM_...", "message": "...", "details": { } } ]
}
```

`errors` and `warnings` are always arrays, never `null`, never omitted.
`vellum schema` publishes the full JSON Schema for this contract and for
`spec.Spec` itself — one document, embedded rather than referenced twice, so
runtime validation and the schema an agent authors against cannot drift
apart. See [The output
envelope](https://frankbardon.github.io/vellum/contract/envelope.html) and
[Payload schema](https://frankbardon.github.io/vellum/contract/payload-schema.html).

## Development

```
make build   # bin/vellum
make test
make lint    # go vet + staticcheck
make cover
make docs    # mdBook
```

`CGO_ENABLED=0` is exported by the Makefile as a build contract, not a
preference.

Conventions, invariants and the CI gate catalogue live in
[CLAUDE.md](CLAUDE.md). Contribution rules — including the Update Demand,
which requires doc and skill companions in the same PR — are in
[CONTRIBUTING.md](CONTRIBUTING.md), summarised at
[Contributing](https://frankbardon.github.io/vellum/contributing/conventions.html).

## License

MIT. See [LICENSE](LICENSE).
