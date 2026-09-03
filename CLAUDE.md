# CLAUDE.md

Vellum is a declarative artifact emitter: **spec in, document out**. DOCX, XLSX,
PPTX and PDF, plus a *fill* mode that binds data into an existing OOXML template
without destroying the parts it does not understand.

It ships as an embeddable Go library, a thin CLI, and an MCP server with an
embedded skill pack. **The library is the deliverable; the CLI is an adapter.**

This file is the machine-checked contract. Hygiene tests parse it: they fail the
build if the current `format_version` is missing, if an environment variable in
source is undocumented here, or if a CI gate exists in source but is not listed
under "Non-Skippable CI Gates". Docs cannot rot silently.

**Status: bootstrap.** The repository furniture, conventions and contract are in
place; the packages are being built epic by epic. A row below that describes a
package which does not exist yet is a specification, not a description. It is
written first on purpose — the capability matrix, the determinism harness and
the error metadata table are all things that are cheap to establish and
expensive to retrofit.

## The Update Demand

Any change to Vellum code, configuration, file format, or public surface MUST
update the corresponding skill file(s) and CLAUDE.md in the same PR.
Non-skippable CI failure if a trigger fires without the required update.

| If you change... | You MUST also update... | Enforced by |
|---|---|---|
| A block kind (`spec.BlockKind`) | `skills/block-<kebab>.md` + `capability/matrix.go` (a row per format) + `descriptor/capabilities_blocks.go` | `TestSkillsCoverAllBlockKinds`, `TestCapabilityMatrixComplete`, `TestManifestBlocksComplete` |
| An output format (`artifact.Format`) | `skills/format-<kebab>.md` + a full column in `capability/matrix.go` + `descriptor/capabilities_formats.go` | `TestSkillsCoverAllFormats`, `TestCapabilityMatrixComplete`, `TestManifestFormatsComplete` |
| A capability matrix row (outcome, degradation, or the code it names) | the affected `skills/block-*.md` or `skills/format-*.md` "Gotchas" section + `descriptor/capabilities_*.go` | `TestCapabilityMatrixComplete`, `TestCapabilityCodesRegistered`, `TestSkillsCoverAllFeatures` |
| A theme slot, colour role, font field, or box role | `skills/theme-document.md` + `theme/builtin.json` + the `Boxes()` golden | `TestBoxesGolden`, `TestSkillsCoverThemeSlots` |
| An error code (add/remove/rename) | `errors/fixup_metadata.go` (`codeMetadata`) — `Message` plus at least one `Fixup`, or `FixupNotApplicable: true` | `TestCodesHaveFixups`, `TestManifestErrorCodesComplete` |
| A registered MCP tool | `skills/tool-<kebab>.md` (strip the `vellum_` prefix) + `mcp/toolmeta/meta.go` | `TestSkillsCoverAllMCPTools`, `TestManifestMCPToolsComplete` |
| A CLI leaf (add/remove) | the command index in `docs/src/cli/flags.md`, plus a dedicated page when `--help` is not enough | `TestSkillsCoverAllCliLeaves` |
| The `--json` envelope or the `format_version` value (currently `"1.0"`) | CLAUDE.md "Output Format Contract" + `descriptor.PayloadSchemaFormatVersion` + regenerate `descriptor/testdata/payload-schema.json` | `TestClaudeMdMentionsFormatVersion`, `TestPayloadSchemaVersionMatchesEnvelope` |
| Any `spec` type reachable from a payload, or a registry enum surfaced in the schema | regenerate `descriptor/testdata/payload-schema.json` (`go test ./descriptor/ -run TestPayloadSchemaGolden -update`) + `docs/src/contract/payload-schema.md` | `TestPayloadSchemaGolden`, `TestPayloadSchemaEnumsMatchRegistry`, `TestGoldensNotHandEdited` |
| The hash algorithm, the normalisation before hashing, or any `spec` field that participates in it | bump `format_version` + regenerate the pinned hash vectors + CLAUDE.md "Output Format Contract" | `TestSpecHashPinnedVectors`, `TestClaudeMdMentionsFormatVersion` |
| A byte-layout rule (zip, OPC, PDF object emission, font subsetting) | CLAUDE.md "Byte-layout invariants" + `.claude/reference/determinism.md` + rebaseline the affected goldens with `-update` | `TestGoldensNotHandEdited`, `TestDeterminismRepeat` |
| An anchor kind, binding mode, or repeat semantic | `skills/fill-<kebab>.md` + `capability/matrix.go` (`fill.*` rows) | `TestCapabilityMatrixComplete`, `TestSkillsCoverAllBindModes` |
| A banned FEEL builtin | `bind.AllBannedBuiltins()` + `skills/fill-binding.md` | `TestBindBannedBuiltinsComplete` |
| A dependency (add/remove/upgrade) | CLAUDE.md "Dependencies" with purpose, licence, and determinism hazard | reviewed, plus `TestNoFontscanImport` and the cgo build step |
| An environment variable | CLAUDE.md "Build / Env" | `TestClaudeMdMentionsAllEnvVars` |
| A new non-skippable CI gate | CLAUDE.md "Non-Skippable CI Gates" | `TestClaudeMdMentionsAllNonSkippableGates` |

Defer the doc or skill update to "a follow-up PR" and the follow-up will not
happen. Update in the same PR or do not merge.

## Architecture

Three content models, each with exactly one job. Getting this wrong is the most
expensive mistake available in this codebase, so it is stated plainly:

- **`spec` is unresolved and hashable.** Author intent. Theme *by reference*,
  marks by name, values untyped, no measurements. This is what makes
  `Spec.Hash()` computable *before* a theme provider has answered — which is
  what lets a consumer ask "does this artifact already exist" and skip the
  render entirely.
- **`fragment` is resolved and format-neutral.** Concrete font families, sizes
  and colours; every length EMU; every value typed with its format code; every
  asset carrying bytes, media type and content hash. Theme application, font
  selection, number formatting, asset resolution and mark resolution happen
  *once* here and are shared by all four writers and by fill's splicer. It knows
  nothing about pages, sheets, slides or XML.
  It carries the theme's **whole** palette and type scale, not only the values
  the content happened to use. A writer that reads them off the runs gets no
  answer at all from a sparse document — a file of nothing but headings has no
  body paragraph to read a body size from — and a format that declares a
  palette up front cannot reconstruct one from the text at all: a deck's colour
  scheme is twelve named slots and every one must be filled.
- **`doc` / `sheet` / `deck` / `pdf` are resolved and laid out.** A flow
  document, a workbook, a slide deck and a paginated page tree are genuinely
  different things; forcing them into one IR produces a model that serves none
  of them. Each is public, so a consumer needing format-specific reach has it.

`fragment` earns its place because it has two genuinely different lowerings —
whole-document into a format model, and bounded-sequence into a template's
existing idiom. Fill mode never constructs a `doc.Document`.

```
Compose from blocks
  JSON/YAML --strict decode--> spec.Spec
                                  |  Spec.Hash() -----------> artifact name (pre-render)
                                  v
             capability.Validate(spec, format)
                                  v
        resolve.Resolve(...) --> fragment.Doc + warnings
                                  v
        doc.Lower | sheet.Lower | deck.Lower | pdf.Lower   <- overflow policy applied here
                                  v
             artifact.Writer.WriteTo(ctx, w, opts)
                                  v
        opc -> zipdet -> io.Writer      |      pdf/object -> io.Writer

Compose from a format model
  consumer-built doc.Document --> artifact.Writer.WriteTo --> io.Writer

Fill
  template.Open -> opc.Package (parts held as sources, never re-serialised)
        |
        +-- Inspect --> anchor.Inventory
        |
        +-- Fill: bind (FEEL) --> fragment.Sequence --> splice --> xmlcopy
                                  only touched parts rewritten; Result.Touched is the receipt
```

### Package map

| Package | Role |
|---|---|
| `vellum.go`, `aliases.go` | Public facade: `Options`, `New`, `Compose`, `Validate`, `Fill`, `Inspect`, `Boxes`, `Capabilities`, `ArtifactName`, `Write`. Root aliases so embedders write `vellum.Spec`. |
| `artifact/` | `Format` enum, `AllFormats()`, `Report`. No unifying `Writer`/`WriteOptions`: each format's `WriteOptions` carries genuinely different fields (PDF's `PageTree`/`Uncompressed` have no OOXML counterpart, the OOXML formats' `Producer` has no PDF one), so `vellum.go`'s `Compose`/`Write` dispatch on `artifact.Format` and on the model's own concrete type directly rather than through a common interface that would need to erase that difference. |
| `errors/` | `VELLUM_*` code registry, `CodedError`, `Fixup`/`Metadata` table. |
| `canon/` | `CanonicalHash(tag, v)` and the JSON canonicaliser. Sole owner of the algorithm. |
| `fs/` | `afero.Fs` configuration. |
| `numfmt/` | xlsx number-format code engine. One formatting vocabulary for all four targets. |
| `overflow/` | Declared overflow policies and format-neutral pagination primitives. |
| `spec/` | The primary public model: blocks, sections, tables, strict decode, YAML normalisation, `Spec.Hash()`. |
| `theme/` | Theme document, fonts with embedding rights, colour roles, mark styles, master layouts, `Boxes(format)`, the `Provider` seam, the built-in theme. |
| `asset/` | `AssetResolver` seam, the inline default, media sniffing, the optional `Hasher` seam. |
| `capability/` | The (feature x format) matrix. `Profile(format)` projects the conformance allowlist out of it. |
| `fragment/` | The resolved, format-neutral IR. |
| `resolve/` | `spec` + theme + assets -> `fragment`. Emits the warnings. |
| `opc/`, `opc/zipdet/` | OPC packaging and the deterministic zip. |
| `xmlcopy/` | Token-level XML rewriter: raw-token copy with surgical subtree replacement. |
| `doc/`, `sheet/`, `deck/` | Per-format models and OOXML writers. |
| `pdf/` + `object|content|shape|text|font|font/sfnt|color|xmp|pdfa` | The PDF emitter. Vellum owns the object writer, the subsetter and PDF/A conformance. |
| `template/` + `anchor|defrag|bind|splice` | Fill mode. |
| `ingest/` | Pulse envelope JSON -> `spec.Table`. No Pulse import. |
| `provenance/` | Record, OOXML property embedding, PDF XMP embedding. |
| `descriptor/` | `Envelope`, `BuildManifest()`, `BuildPayloadSchema()`. **No-execute** — must not import a renderer. |
| `skills/`, `examples/` | `go:embed` content packs. |
| `mcp/`, `mcp/gosdk/`, `mcp/toolmeta/`, `mcpserve/` | MCP surface. Only `mcp/gosdk` imports the SDK. |
| `internal/cli/`, `cmd/vellum/` | The CLI. The only `internal/` in the tree. |

### Import-graph invariants

- `artifact`, `errors`, `canon`, `numfmt`, `overflow` import nothing from Vellum except `errors`.
- **`spec` never imports `theme`.** This is what keeps `Spec.Hash()` computable before theme resolution.
- `fragment` may not import `doc`, `sheet`, `deck`, `pdf`, `opc`, or `encoding/xml`.
- `descriptor` never imports `resolve`, `doc`, `sheet`, `deck`, `pdf`, or `template`.
- `template/`, `defrag/`, `splice/` never import `encoding/xml`.
- Nothing imports `go-text/typesetting/fontscan`.
- Nothing imports `"C"`. There is no carve-out.

## Code Conventions

### Naming

- Error codes: `VELLUM_<AREA>_<CATEGORY>`, SCREAMING_SNAKE. Areas: `SPEC`, `THEME`, `FONT`, `ASSET`, `TABLE`, `MARK`, `OVERFLOW`, `CAPABILITY`, `OPC`, `ZIP`, `DOC`, `SHEET`, `DECK`, `PDF`, `PDFA`, `TEMPLATE`, `ANCHOR`, `DEFRAG`, `BIND`, `INGEST`, `PROVENANCE`, `CLI`, `MCP`, `ARTIFACT`.
- Block kinds and formats are lowercase wire strings (`"heading"`, `"page_break"`, `"pptx"`).
- Constructors: `New<Thing>` exported, `new<thing>` unexported factories.
- Registries: `<thing>Registry` unexported package var; `All<Thing>s()` exported, copy-returning.
- Facade config is an `Options` struct; leaf packages use `With*` functional options.
- Booleans are named so the zero value is correct (`Disable*`, not `Enable*`).
- Test names `TestSubject_Scenario`. Gate tests use the reserved prefixes.

### Error handling

- One `errors` package, domain-prefixed codes. No per-package error types.
- `NewCodedErrorWithDetails` is the dominant form. Detail keys are `snake_case`.
- Every code needs a `codeMetadata` row. Absence of a fixup is a bug, not a
  feature — `FixupNotApplicable: true` is the honest signal for an internal
  invariant, and must be used deliberately.
- Fixup paths are abstract pointers (`["Sections","*","Blocks","*","Kind"]`),
  never concrete indices.
- `fmt.Errorf(...%w)` only in the facade's own plumbing, never in domain
  packages.

### Determinism

The compressed statement; the full table is `.claude/reference/determinism.md`.

- No `time.Now()` on any output path. Timestamps come from `SourceDateEpoch`.
- No random or counter-derived identifiers that depend on code path. Derive from
  content: sorted walks, content hashes, canonical orderings.
- No map iteration on an ordered output path. Registries return sorted copies.
  Where an API *could* accept a map on an ordered path, take an ordered slice
  instead — make the nondeterminism unrepresentable rather than tested-against.
- No `x/text/collate`. Bytewise `sort.Strings` only.
- EMU is `int64`. Measurements never round-trip through `float64`.
- Text is measured in integer font units and converted to text space **once per
  styled run per line**, never per glyph. The distinction that matters is
  per-glyph against per-run: converting each glyph and summing rounds sixty
  times down a line of prose and accumulates, and a paragraph measured one way
  then re-measured the other breaks into different lines. Per run is one
  rounding for ordinary prose and a handful for a line carrying a bold word,
  bounded by the styling the author wrote rather than by the length of the text.
  A line cannot be measured with one division across the whole of it, because a
  styled run brings its own size and its own units per em and there is no single
  denominator; per run is where exactness ends and it is where the author's own
  structure puts the boundary.
- Line breaking is greedy over UAX#14 opportunities. Not because greedy is
  better: because an optimal-fit shaper produces different breaks, and a
  document's pagination must not change when a later version gets cleverer
  about ragged edges.
- Assertions are on raw bytes. Normalise XML for failure *display* only.

### Byte-layout invariants

- `[Content_Types].xml` is the first zip entry, always.
- Zip entries carry no extended-timestamp extra field: `zip.FileHeader.Modified`
  stays zero and the legacy `ModifiedDate`/`ModifiedTime` fields are written
  directly. Setting `Modified` makes Go emit the extra field.
- No data descriptors. Entries are buffered so sizes and CRC are written up
  front.
- Zip version fields are written explicitly as `20` (ZIP 2.0, the version that
  introduced deflate), in both the local header and the central directory.
  `archive/zip` sets them inside `CreateHeader`, which `zipdet` does not call —
  `CreateRaw` takes the header verbatim, so an unset field reaches the bytes as
  `0`. Every reader in the toolchain ignores it; Word does not, and reports the
  package as containing unreadable content.
- Relationship IDs are assigned by walking relationships sorted by
  `(Type, TargetMode, Target)`.
- Media part names are indexed by the sorted set of distinct content hashes.
- xlsx `styles.xml` keeps the fixed preamble with builtin indices intact. Fill
  index 0 and 1 are reserved by the spec; getting it wrong makes Excel refuse to
  open the file. Pinned by `TestStylesXML_ReservedFillIndicesArePresentEvenWithNoCustomFills`
  and `TestStylesXML_ElementOrderIsFixedByTheSchema`.
- **A legacy xlsx cell comment is two parts, not one.** `xl/comments1.xml`
  states the text; `xl/drawings/vmlDrawing1.vml` states the shape that draws
  it, and the worksheet references the drawing with `<legacyDrawing r:id="…"/>`.
  A comments part with no paired VML drawing is one Excel opens with the
  indicator triangle and nothing behind it. Threaded comments, the newer
  schema, need an author-identity part this library has no source for and are
  not written; the legacy form is the one every reader back to Excel 2007
  draws correctly. Pinned by `TestComments_PairsWithALegacyVMLDrawing`.
- **A table cell's annotation becomes a whole extra column, not an inline
  run.** `FeatureTableCellAnnotation` degrades to "text appended to the cell,
  with the typed value preserved in a neighbouring column" because a cell
  holds one typed value and appending text to a number would turn it into a
  string, defeating the reason to export a workbook at all. The column is
  added only when the table carries at least one annotation anywhere — an
  unannotated table gets no extra columns — and then uniformly across every
  data column, so the doubled structure is the same shape on every table that
  has one. Pinned by `TestLower_AnAnnotationAppendsAColumnAndPreservesTheValue`
  and `TestLower_NoAnnotationsMeansNoDoubling`.
- **A row-header stub spanning several body rows is anchored to the top of its
  merge, not left at Excel's own default.** Excel's default cell vertical
  alignment is the bottom, so a ten-row group label left unstated renders at
  the last row of the group it names — invisible until a reader has already
  scrolled past every row under it. `CellFormat.VerticalTop` states
  `vertical="top"` explicitly on a stub merge for exactly this reason.
- pptx slide and master identifier spaces are disjoint and both are bounded. A
  `sldId` is at least 256 and below 2147483648; a `sldMasterId` and a
  `sldLayoutId` are at or above it. A deck that mixes them is one PowerPoint
  repairs on open.
- A pptx shape identifier starts at 2. Identifier 1 belongs to the `spTree`'s
  own group shape, which every shape tree carries, and a shape numbered 1
  collides with it. Identifiers are assigned by position within the tree, so
  two slides carrying the same shapes produce the same identifiers.
- **A pptx title placeholder carries no `idx`; every other placeholder carries
  one.** The asymmetry is the schema's. A title matches its layout counterpart
  by type alone and everything else matches by index, so a title with an index
  or a body without one inherits from the wrong layout shape — the slide opens
  with its placeholders stacked on each other and nothing anywhere reports a
  problem. Pinned by `TestWrite_TitlePlaceholdersCarryNoIndex`.
- **Every colour a pptx master, layout or slide states is a scheme reference,
  never a literal.** `theme1.xml` is the only part carrying `srgbClr`. A literal
  elsewhere is a colour the theme cannot restyle, and it produces a deck that
  looks correct and cannot be restyled — worse than one that looks wrong,
  because nothing reports it. Pinned by
  `TestWrite_ColoursOnSlidesAreSchemeReferences`, which also asserts the master
  states a scheme colour so the check cannot pass by there being none.
- **A pptx table's row height is stated, never left to the reader.** Every cell
  writes its own insets (`marL/marR/marT/marB`) and every cell paragraph writes
  an exact line box (`a:lnSpc` as `a:spcPts`, not `a:spcPct`). Both are the
  numbers `overflow.PlanTable` computed the split from, and a capacity computed
  from values the file does not state is a capacity for a document the reader
  will not produce. Proportional line spacing is a proportion of the *font's*
  own line height, so a reader substituting a face would change the row height
  and a table measured to fit a slide would overflow it.
- **An empty pptx table cell's paragraph mark carries the table's type size.**
  An empty paragraph takes its height from its mark, and a mark stating nothing
  is sized at DrawingML's default of eighteen points. A crosstab is mostly
  merged corners and continued stub cells, so one unstated empty paragraph per
  row doubles every row's height and the table silently overflows the slide the
  split measured it to fit.
- **A DrawingML spanning cell does not replace the cells it covers.** It
  declares `gridSpan` or `rowSpan` and every covered cell stays present carrying
  `hMerge="1"` or `vMerge="1"`, so a row always holds exactly one `a:tc` per
  grid column. This is the opposite of WordprocessingML, where a `gridSpan` cell
  stands in for the cells it covers. A row that is short by the covered cells is
  one some readers draw with a hole and others draw shifted left.
- **A merge clipped by an overflow split restarts with its label.** A stub merge
  computed over the whole table names a span reaching past the container it
  lands in — twenty-six rows inside a table carrying eighteen — and a reader
  handed that grows every row trying to honour it. `fragment.ClipStub` restarts
  the merge at the window's first row, which is the same rule the column banner
  follows.
- **pptx masters, layouts and the theme part are authored from the theme, not
  shipped.** A .pptx carries no loose formatting: a slide inherits from a
  layout, a layout from a master, and the master from a theme part naming its
  colours and fonts by slot. A deck with none of those is not a deck with
  default styling but a file PowerPoint refuses. Shipping a fixed template and
  copying its parts through would make every consumer's deck look like Vellum's.
  See `deck.Author`.
- PDF uses a classic xref table and trailer, never object streams.
- **A PDF table is drawn in three passes: every fill, then every hairline, then
  every cell's text.** PDF paints in the order the operators appear, so a fill
  emitted after a neighbour's hairline covers the hairline, and text emitted
  before a fill vanishes under it. One pass per cell is fewer operators and
  produces a table missing a line here and a number there, in a pattern that
  looks like a rendering artefact rather than like a bug. Pinned by
  `TestTable_FillsAndRulesPrecedeText`.
- **A PDF table's capacity is measured, not theme-derived** — the one place
  Vellum departs from the rule stated in `overflow`'s own doc comment, and the
  departure is what the rule is about. That rule is about who lays the content
  out: Word and PowerPoint do, with the fonts installed where the file is
  opened, so a measured capacity there would not be reproducible. Vellum lays
  PDF out completely, so every row height is a number it computed and then drew.
  The greedy fill stays shared — `overflow.PlanRows` holds it and
  `overflow.PlanTable` delegates — so a boundary cannot fall in two places.
- **A PDF table's rows are measured before any split is planned, and every stub
  position is measured carrying its own label.** `fragment.ClipStub` restarts a
  merge at the first row of a continuation page, so a row that carried no label
  under one plan carries one under another — and a height that changed with the
  plan would be a capacity computed for a table nobody draws. The cost is white
  space under a long stub label; the alternative is a table that overflows the
  page it was measured to fit.
- **Image data is embedded verbatim.** A JPEG's own bytes become the DCTDecode
  stream and an opaque PNG's own IDAT becomes the FlateDecode stream, carrying
  the PNG predictor its scanlines are already filtered with. Nothing is decoded
  and nothing is recompressed, so the picture in the document is the picture on
  disk and a consumer's choice of compression survives. The single exception is
  a PNG with an alpha channel: PNG interleaves alpha with colour on one
  scanline and PDF keeps them in two streams, so those are unfiltered, split and
  recompressed — every sample preserved, only the arrangement changed. The
  variants no PDF filter describes are named rejections in the matrix, not
  silent re-encodes.
- The PDF page tree is balanced at a fixed branching factor, folded from the
  leaves up. Its shape is a function of the page count alone. Pages are
  numbered before interior nodes, in order, so page one is the lowest-numbered
  page object.
- **`Resources` is never inherited.** It is inheritable under ISO 32000-1 and
  lifting it onto the page tree root is an obvious saving. ISO 19005-2 clause
  6.2.2 requires a content stream referencing a font or an image to have an
  *explicitly associated* resource dictionary, and veraPDF rejects the
  inherited form. `MediaBox`, `CropBox` and `Rotate` are hoisted; `Resources`
  is not. Pinned by `TestBuildPageTree_DoesNotHoistResources` so re-adding it
  fails in a second rather than in the conformance gate, and re-checked on
  every document written by `pdfa.Preflight`.
- A PDF page's font and image resources are **derived from its items**, not
  listed beside them. Listing them separately gives a caller two ways to say one
  thing, and the interesting half of the failure — a stream selecting a font the
  resource dictionary does not name — draws nothing and reports nothing.
- Every page's content stream is built **before** any font is written. A subset
  contains the glyphs the document draws, and a face learns which those are by
  being drawn with, so writing fonts first embeds subsets of nothing. Enforced
  by the font writer refusing an empty subset rather than by a comment.
- PDF `/CreationDate`, `/ModDate` and the XMP dates are generated from one
  struct so they cannot disagree. ISO 19005 requires them to match; veraPDF's
  2b profile was observed *not* to check it, which is a reason to keep the
  invariant structural rather than to rely on the validator for it — the next
  profile, and every other conforming reader, may well check.
- The PDF file identifier is derived from the document's content. A generated
  one is the easiest way to lose byte-identity and the hardest to notice: the
  file differs between runs in sixteen bytes buried in the trailer and
  everything else matches.
- The sRGB output-intent profile is built rather than embedded as a shipped
  blob, and contains no floating-point computation: a sampled transfer curve
  would need `math.Pow`, whose results are not guaranteed identical across
  platforms and Go versions.
- Font subset tags are base-26 over a hash of `(fontHash, sorted glyph IDs)`.
- Subset font programs pin `head.created` and `head.modified`, use a fixed table
  order and padding, and recompute checksums with `head.checkSumAdjustment`
  written last.

### The external reader oracles

Every assertion in the determinism harness compares our bytes against our bytes.
That proves determinism and proves nothing about whether the file opens: an
artifact can be byte-identical across a thousand runs and still be one no reader
accepts. Three defects have reached a human that way — zip version fields left
at zero, ECMA-376 children emitted out of schema order, and a golden "PNG"
carrying no IDAT chunk. Every Go-side reader accepted all three. Word accepted
none.

The gap is structural: **the readers available to the build are more tolerant
than the readers that matter.** Three oracles narrow it, sharing the plumbing in
`internal/exttool`:

| Oracle | Question it answers | Tag | Where |
|---|---|---|---|
| poppler | Does the PDF open, and does its text extract? | none | `internal/pdfvalidate` |
| LibreOffice | Does the OOXML package open, and did its content survive? | `soffice` | `internal/oovalidate` |
| veraPDF | Is the PDF/A-2b claim the file makes about itself true? | `verapdf` | `internal/pdfvalidate` |

This does not reopen the LibreOffice question. That ruling is about *producing*
artifacts and stands unchanged. Here these are oracles: they read bytes Vellum
already wrote and report whether they could. Their output is examined for
presence and content and **never compared for equality**, because a conversion's
bytes vary with tool version and installed fonts — the exact nondeterminism the
ruling exists to exclude.

Two mechanisms keep them out of the library. The heavier two are behind build
tags, so they cannot link into a build that did not ask for them, and
`TestNoExternalToolingOnTheLibraryPath` asserts no shipped package reaches any of
them. The poppler oracle deliberately carries no tag, so for that one the import
graph check is the only thing holding the line.

**They are evidence, not proof.** LibreOffice is not Word; it is more tolerant on
several of the byte-layout constraints above and less tolerant on some
legitimate constructs, so a failure is read before it is believed. veraPDF
implements the clauses its profile covers and not every clause of the standard —
observed directly: an XMP date deliberately made to disagree with the
information dictionary still passes 2b. They are worth having because the
failures they do catch — a package that will not open, content that silently
vanished, a conformance claim that is false — are the expensive ones, and
nothing else catches them before a human opens the file by hand.

Run them with `make test-office` and `make test-pdfa`; poppler runs in `make
test`. When a tool is absent the check skips and `make test` prints a warning
naming what went unchecked, because a skip is invisible in a non-verbose run and
an optional gate nobody is told about is one nobody provisions. CI sets
`VELLUM_REQUIRE_OPTIONAL_GATES`, which turns every such skip into a failure.

### The PDF/A preflight

`pdfa.Preflight` runs on **every** document the PDF writer emits, before any
byte reaches the destination, and there is no option to skip it. The only
reason to want one would be to write a file that does not conform while its own
metadata claims it does, and a false conformance claim is worse than no claim.

It is the complement to the veraPDF oracle rather than a second copy of it. The
oracle implements the profile and needs provisioning; the preflight implements
the handful of clauses where Vellum's own code decides the answer — the output
intent and its embedded profile, the XMP packet's presence, filtering and
conformance claim, agreement between the packet and the information dictionary
field by field, the file identifier, every descendant font's embedded program,
`Resources` on the page and not on the tree, and the image constraints on
`/Interpolate`, `/ColorSpace` and `/LZWDecode`. Those are the ones a refactor
can break, so they fail in `make test` on a machine with nothing installed.

What it deliberately does **not** check is anything unrepresentable in this
writer: encryption, object streams, JavaScript, embedded files, annotations
without appearance streams. A check for a condition that cannot arise passes
forever while proving nothing, which is the failure this repository has already
been bitten by twice. Every check has a test that breaks the document in the one
way that check exists to catch.

### Declared, not emergent

A behaviour a consumer can observe — a block degrading, an asset media type
being refused, a table continuing onto another slide — is a row in
`capability/matrix.go` **before** it is code. There are exactly three legitimate
outcomes per (feature, format): `renders`, `degrades` to a stated alternative, or
`rejects` at validate time. A consumer scheduling an unattended job must be able
to learn the answer before the job runs; if they learn it from a support ticket,
the matrix has failed.

The row is half the promise. The other half is that a `degrades` row produces a
warning the consumer can actually read, and that is machine-checked:
`TestCapabilityDegradationsAreReported` requires every degrading row to be
exercised by a specification that provokes a warning naming the feature, or to
carry a stated reason no specification can. Three rows carry that reason today —
`font.outlines.cff` in the three OOXML formats, where no font program is ever
loaded so the outline format is never seen, and where the degradation the row
names is the one `font.embed.*` already reports.

An embed mode is a licence condition on **how** a program may be embedded, so a
format that embeds none of them cannot violate it: `font.embed.subset` and
`font.embed.whole` degrade there rather than being refused. The refusal belongs
where the format does embed and Vellum cannot honour the mode, which is CFF in
PDF.

## Output Format Contract

### `--json` envelope

Current `format_version` is `"1.0"`.

```json
{
  "format_version": "1.0",
  "data": { },
  "request": { },
  "errors":   [ { "code": "VELLUM_...", "message": "...", "details": { } } ],
  "warnings": [ { "code": "VELLUM_...", "message": "...", "details": { } } ]
}
```

`errors` and `warnings` are always present as arrays, never `null` and never
omitted. `request` is omitted when absent. Every `--json` and MCP output path
goes through `descriptor.NewEnvelope`; no `fmt.Sprintf` builds JSON.

Additive slots must be `omitempty` so pre-existing wire output stays
byte-identical, and `format_version` is **not** bumped for an additive slot.

`vellum schema` writes the raw JSON Schema unwrapped, because the artifact is
self-contained and carries its own `$schema` and `$id`.

### Artifact identity

`Spec.Hash()` plus the asset hashes name the output, and both are **inputs**. The
name is therefore knowable before the render runs — which is the whole point,
because a consumer that can only learn an artifact's identity by producing it
cannot use identity to avoid producing it.

Byte-identity and spec-hash identity are separate guarantees with separate
lifetimes. Byte-identity makes goldens and attestation work and holds for a
fixed Go toolchain minor. Spec-hash identity makes dedupe work and holds across
versions. Do not conflate them.

## Non-Skippable CI Gates

CLAUDE.md hygiene:
- `TestClaudeMdMentionsFormatVersion` — CLAUDE.md must mention the current `format_version` `"1.0"`.
- `TestClaudeMdMentionsAllEnvVars` — every `VELLUM_*` environment variable in Go source must appear in CLAUDE.md "Build / Env".
- `TestClaudeMdMentionsAllNonSkippableGates` — every test name with a reserved prefix (`TestClaudeMd`, `TestUpdateDemand`, `TestGoldensNot`, `TestSkillsCover`, `TestCapability`, `TestDeterminism`, `TestNo`, `TestPerPackageCoverage`, `TestManifest`, `TestPayloadSchema`, `TestSpecHash`, `TestBind`) must be listed here.
- `TestUpdateDemandTableCovers` — the Update Demand table must cover: block kind, output format, capability, theme, error code, MCP tool, CLI leaf, format_version, environment variable, CI gate, dependency, byte layout.

Contract and registry completeness:
- `TestCapabilityMatrixComplete` — every (feature x format) pair has a declared outcome.
- `TestCapabilityCodesRegistered` — every code named by a matrix row parses via `errors.ParseCode`.
- `TestCapabilityEveryBlockKindHasAFeature` — no block kind is absent from the matrix.
- `TestCapabilityOutcomesAreWellFormed` — a `degrades` row names its alternative and a `rejects` row names its code; the three outcomes stay distinguishable.
- `TestCapabilityKnownDecisions` — the decisions that were argued, pinned as cases, so a matrix edit that reverses one is deliberate.
- `TestCapabilityDegradationsAreReported` — every `degrades` row is either exercised by a specification that provokes a warning naming the feature, or carries a stated reason no specification can. A row promising a degradation is a promise the consumer will be told; a row with neither fails the build. The exercise table is also checked for orphans, so a row that stops degrading cannot leave a passing test behind that checks nothing.
- `TestManifestCapabilitiesComplete` — the manifest enumerates the live matrix.
- `TestPayloadSchemaEmbedsSpecDefinitions` — the schema carries the `spec` definitions rather than referencing them out of band.
- `TestNoPulseCodes` — no `PULSE_*` error code leaks in through `ingest`. Vellum reads a Pulse envelope; it does not import Pulse and does not re-emit its vocabulary.
- `TestNoSemanticSectionVocabulary` — no built-in section type ("cover", "methodology"). That vocabulary belongs to the consumer.
- `TestNoExternalToolingOnTheLibraryPath` — no shipped package reaches `internal/exttool`, `internal/oovalidate` or `internal/pdfvalidate`. The library runs no subprocess. See "The external reader oracles".
- `TestNoUnpinnedValidatorImage` — the veraPDF reference is a content digest, not a tag, and is named once, in `internal/pdfvalidate/pin.go`; `.github/workflows/ci.yml` reads it via `internal/cmd/validatorpin` rather than stating a digest of its own. A validator that can change without a commit makes the PDF/A conformance verdict unattributable.
- `TestNoTestFontOnTheLibraryPath` — no shipped package reaches the test face. Fonts come from the theme, only; a face reachable from the library is a fallback waiting to be used. The other half of `TestNoFontscanImport`.
- `TestNoImagetestOnTheLibraryPath` — no shipped package reaches `internal/imagetest`. Pictures come from the host, only. It cannot be drawn at `image/png` and `image/jpeg`, which `go-text/typesetting` already pulls in transitively for colour bitmap glyph tables; that is worth knowing rather than worth pretending.
- `TestCodesHaveFixups` — every error code has a `codeMetadata` row with a `Message` and a `Fixup`, or `FixupNotApplicable: true`.
- `TestManifestBlocksComplete`, `TestManifestFormatsComplete`, `TestManifestErrorCodesComplete`, `TestManifestMCPToolsComplete` — the manifest enumerates the live registries.
- `TestPayloadSchemaGolden`, `TestPayloadSchemaEnumsMatchRegistry`, `TestPayloadSchemaVersionMatchesEnvelope`.
- `TestGoldensNotHandEdited` — goldens end with a valid `// golden-hash: <sha256>` line, or carry a `.sha256` sidecar listed in a hashed manifest.
- `TestSpecHashPinnedVectors` — committed (spec, hash) vectors. Changing one requires a `format_version` bump in the same PR.

Determinism:
- `TestDeterminismRepeat` — every golden composed 1000x in one process per format; one distinct SHA-256. 25x under `-short`.
- `TestDeterminismCrossProcess` — re-exec in fresh processes and compare digests.
- `TestDeterminismGOMAXPROCS` — the same goldens at `GOMAXPROCS=1` and `=8` under `-race`.
- `TestDeterminismEpochInvariance` — wall-clock time does not affect output; an explicit `SourceDateEpoch` produces a different but stable result.
- `TestDeterminismOverflowIsPinnedInPDF` — the overflowing PDF golden's split is committed as values and read back through poppler, page by page: which row labels each page shows and which it must not. A PDF's text is glyph identifiers in a compressed stream, so the label on a row is only legible through the font's own ToUnicode mapping — which is why this one needs a reader where the deck's arm needs only a scan of the XML. Both halves are asserted, presence and absence: a boundary that moved by one row still shows every label it did before, one page earlier.
- `TestDeterminismOverflowIsPinned` — each overflowing golden's split is committed as values: rows per slide and the label at each boundary, read back out of the written package. The harness already proves the split cannot drift between runs; this makes it visible, because a change to the row-height arithmetic otherwise moves the boundary and hides inside a few thousand bytes of rebaselined XML.
- `TestNoTimeNow` — no `time.Now(` in non-test source outside the `provenance` opt-in.
- `TestNoUnsortedMapIteration` — AST gate over the output-path packages.
- `TestNoFontscanImport` — `go list -deps` firewall against system font scanning.
- `TestNoCollateImport` — `go list -deps` firewall against `x/text/collate`; ordering is bytewise, always.
- `TestNoEncodingXMLInFill` — `encoding/xml` is not imported from `template/`, `defrag/` or `splice/`.
- `TestNoCgoImports` — nothing imports `"C"`; paired with the `CGO_ENABLED=0` build step.
- `TestNoGoSDKImport` — `go list -deps` firewall: `mcp/` (the SDK-free core) never transitively depends on `modelcontextprotocol/go-sdk`.
- `TestNoGoSDKImport_NonVacuous` — proves the query above can actually detect the SDK, by running it against `mcp/gosdk`, which must find it.
- `TestNoGoSDKImport_DirectOnlyForToolmetaAndServe` — `mcp/toolmeta` and `mcpserve` never *directly* import `modelcontextprotocol/go-sdk`; both reach its types, when they need to, through `mcp/gosdk`'s own re-exports.

Non-destructiveness and fill:
- `TestNonDestructiveCorpus` — after a fill, every part outside the anchors' own part is byte-identical to the source, checked with `bytes.Equal` part for part (there is no `Result.Touched` receipt yet — that lands with E10's `Fill` orchestrator; this story drives discovery, defrag and splice directly). The fixture carries tracked changes, a comment, a custom XML part, footnotes and an embedded OLE object. This is the DOCX case; `TestNonDestructiveCorpus_XLSX` (`template/nondestructive_xlsx_test.go`) and `TestNonDestructiveCorpus_PPTX` (`template/nondestructive_pptx_test.go`) prove the same "every part outside `Result.Touched` is byte-identical" property through the real `template.Fill` entry point for the other two fillable formats, against fixtures carrying their own format-native equivalents — a second untouched worksheet, a legacy comment paired with its own VML drawing, and a defined name left alone via `Binding.OptionalAnchors` for xlsx; a second untouched slide with its own speaker notes and embedded media for pptx — rather than the DOCX-specific tracked-changes/footnote/OLE shape, which has no equivalent in either format.
- `TestDefragCorpusComplete` — a case directory under `testdata/corpus/defrag/` missing its `expect.json` fails the build, and so does an `expect.json` with no matching fixture. Live from day one, including while the corpus is empty.
- `TestBindBannedBuiltinsComplete` — the nondeterministic-builtin registry covers everything `bind.Validate` rejects.

Fuzz (bounded smoke run per PR; the job is to keep the targets compiling and re-exercise the committed corpus, not to run a campaign):
- `FuzzRead`, `FuzzWriteNames` over the archive reader and entry-name validation.
- `FuzzOpen`, `FuzzReadWriteRoundTrip` over the package reader. The round-trip target also asserts the writer is idempotent, which is the determinism guarantee restated over arbitrary input rather than over fixtures.
- `FuzzWriteDocument` over the PDF object writer. The input is a *program* — each byte appends an object — because the writer is not a parser and fuzzing its output would test the test reader instead. It asserts the file parses back, that every cross-reference offset lands on the object it names, and that writing the same document twice is byte-identical.
- `FuzzImage`, `FuzzImageFingerprint` over the PNG and JPEG readers, which are the only place in the library that parses bytes Vellum did not write. Every rejection must be coded, every acceptance must write, and every accepted stream's `/Length` must match its data.
- The hostile seed corpus lives at `opc/testdata/seeds/` with a documented expectation per file. A seed with no expectation, or an expectation with no seed, fails the build.

Conformance (externally provisioned; skipped when the tool is absent unless `VELLUM_REQUIRE_OPTIONAL_GATES` is set):
- `TestPDFAConformance` — build-tagged `verapdf`; runs a digest-pinned veraPDF against every PDF golden and asserts zero errors.
- `TestOfficeReaderOpensGoldens` — build-tagged `soffice`; opens every OOXML golden with an installed LibreOffice and asserts the content a reader sees.
- `TestPDFReaderSeesTheContent` — poppler over every PDF golden: the document opens, and its text extracts. Untagged, because poppler is small and fast enough for the ordinary suite; it still skips when absent.

Skill and example coverage:
- `TestSkillsCoverAllBlockKinds`, `TestSkillsCoverAllFormats`, `TestSkillsCoverAllFeatures`, `TestSkillsCoverAllMCPTools`, `TestSkillsCoverAllBindModes`, `TestSkillsCoverThemeSlots`, `TestSkillsCoverAllCliLeaves`.
- `TestSkillsHaveRequiredSections`, `TestSkillTokenBudget`.
- `TestPerPackageCoverageFloors`.

## Build / Env

```
make build   # bin/vellum, version-stamped via -X main.version=$(VERSION)
make test
make lint    # go vet + staticcheck
make cover
make test-office  # build-tagged; needs a LibreOffice installation
make test-pdfa    # build-tagged; needs veraPDF
make bench   # manual; asserts no threshold
make docs    # mdBook
```

`CGO_ENABLED=0` is exported by the Makefile as a contract and asserted by a CI
step. Required CI contexts: `lint` and `test (1.26)`.

Environment variables:

- `VELLUM_THEME_DIR` — directory the default `ThemeProvider` reads theme documents from. Unset means the built-in theme only.
- `VELLUM_ASSET_DIR` — directory the default `AssetResolver` reads asset handles from. Unset means inline assets only.
- `VELLUM_SOURCE_DATE_EPOCH` — RFC 3339 timestamp pinning every date Vellum writes. Unset selects the pinned 1980 epoch, which is the deterministic default. Setting a real time is a deliberate opt-out of byte-identical output and is recorded in provenance.
- `VELLUM_MAX_ASSET_BYTES` — cap on a single resolved asset. Unset selects the built-in default.
- `VELLUM_VERAPDF` — path to a local veraPDF installation, or a `container:` reference (e.g. `container:docker.io/verapdf/cli@sha256:...`) run via Docker or Podman, for the build-tagged conformance gate. A container reference must name a digest, not a tag; `TestNoUnpinnedValidatorImage` enforces it. CI runs the container form, pinned in `internal/pdfvalidate/pin.go` and read by the workflow through `internal/cmd/validatorpin` rather than being stated a second time.
- `VELLUM_SOFFICE` — path to a LibreOffice `soffice` binary, for the build-tagged office-reader checks. Unset searches the `PATH` and the platform's conventional install location. Test-only: the library runs no subprocess, and this changes nothing a consumer can observe. See "The office-reader oracle".
- `VELLUM_REQUIRE_OPTIONAL_GATES` — turns a missing external tool from a skip into a failure. Unset lets a developer without veraPDF or LibreOffice run the suite; CI sets it so a gate cannot pass forever without ever running. Shared across the externally-provisioned gates rather than one variable per tool.
- `VELLUM_DETTEST_CASE` — names the single case a re-executed child process emits, for `TestDeterminismCrossProcess`. Set by the harness, never by hand.
- `VELLUM_SPEC_HASH_CHILD` — marks the re-executed child process in `TestSpecHashPinnedVectors`' cross-process arm. Set by the harness, never by hand.
- `VELLUM_BIND_HASH_CHILD` — marks the re-executed child process in `template/bind`'s `TestHash_StableAcrossProcesses`, the same cross-process hash-stability check `TestSpecHashPinnedVectors` runs for `spec.Spec`. Set by the harness, never by hand.
- `VELLUM_COVERAGE_FLOOR_CHILD` — marks the `go test ./...` subprocess `TestPerPackageCoverageFloors` spawns to collect per-package statement coverage, so that subprocess's own run of `internal/gates` does not spawn another subprocess in turn. Set by the harness, never by hand.

## Dependencies

Deliberately small. Every entry justified, every licence checked, every
determinism hazard named.

| Dependency | Purpose | Licence | Hazard |
|---|---|---|---|
| `github.com/spf13/afero` | Filesystem seam; hermetic tests | Apache-2.0 | none |
| `github.com/go-text/typesetting` | Shaping (harfbuzz), UAX#14 line breaking, bidi, OpenType parsing | Unlicense/BSD-3 | **`fontscan` scans system fonts** — firewalled by CI gate. Has no subsetter. |
| `github.com/pbinitiative/feel` | FEEL evaluation for bindings; used directly, no wrapper | MIT | **`now()`/`today()` call `time.Now()`** — banned at validate time |
| `github.com/santhosh-tekuri/jsonschema/v6` | Spec validation | Apache-2.0 | none |
| `github.com/google/jsonschema-go` | MCP schema reflection | BSD-3 | confined to `mcp/` |
| `sigs.k8s.io/yaml` | YAML to JSON at the boundary | Apache-2.0 + MIT | chosen *because* it routes through JSON — the round-trip is the requirement |
| `golang.org/x/text` | `message`/`language`, to render JSON Schema validator faults | BSD-3 | `jsonschema`'s `LocalizedString` dereferences its printer without a nil check, so a printer is required rather than optional. Confined to error prose; **never** use `x/text/collate` — locale-aware ordering is nondeterminism with extra steps |
| `github.com/modelcontextprotocol/go-sdk` | MCP transport | MIT | only `mcp/gosdk` imports it |
| `github.com/urfave/cli/v3` | CLI | MIT | confined to `internal/cli` and `cmd/vellum` |
| `golang.org/x/image` | The test face (`gofont/goregular`) and an independent SFNT parser to check the subsetter against | BSD-3 | **test-only.** Firewalled by `TestNoTestFontOnTheLibraryPath`: a font reachable from the library is a fallback waiting to be used |
| `image/png`, `image/jpeg` (stdlib) | Generating the raster fixtures in `internal/imagetest` | BSD-3 | **test-only**, and firewalled at `internal/imagetest` rather than at the stdlib packages: `go-text/typesetting`'s font package reads colour bitmap glyph tables, so both are already in the graph transitively. Vellum asks for neither |

Permanently ruled out, with reasons, so they are not re-proposed:

- **`seehuhn.de/go/pdf`** — GPL-3.0. Otherwise the ideal fit (subsetting, ICC, XMP, output intents). A permissively licensed library cannot take it. This is why Vellum owns its subsetter.
- **`github.com/tdewolff/font`** — MIT with a real subsetter, but its write path stamps `time.Now()` into `head.modified`, inside the exact byte stream we are required to pin. Never tagged. Read as reference; do not import.
- **`github.com/pdfcpu/pdfcpu`** — a processor for existing PDFs; a deterministic writer needs total control of object numbering and xref layout.
- **`github.com/google/uuid`** — Vellum generates no random identifiers, ever.
- **`sRGB2014.icc`, or any shipped ICC blob** — the output-intent profile is built in `pdf/color` instead. Around eight hundred bytes of entirely specified structure, which removes a redistributed binary and its licence notice, removes the possibility of blob and code disagreeing, and makes the bytes reviewable. veraPDF accepts it.
- **LibreOffice, in any form, on the output path** — conversion output varies with renderer version and installed fonts, which defeats byte-identical output and the consumer dedupe that rests on it. Vellum never converts. It is permitted in one place and one only: as a build-tagged *test oracle* that reads artifacts Vellum already wrote and reports whether a real reader accepts them, never comparing bytes. See "The office-reader oracle". It is not a module dependency and the library runs no subprocess.

## Extension Points

Seams, each an interface with an inert default, so a host that wires nothing
still gets a working library rather than a construction failure:

- **`asset.Resolver`** — handle to bytes plus media type. Default resolves
  inline assets only. Vellum owns nothing and fetches nothing itself. The
  optional `asset.Hasher` seam lets a host supply content hashes without
  transferring bytes, which is what makes `ArtifactName` cheap enough to call
  before deciding whether to render; a resolver that does not implement it is
  not an error, the assertion simply fails and Vellum hashes the bytes itself.
- **`theme.Provider`** — theme id to theme document. Default serves the built-in
  theme.
- **`bind.Evaluator`** — expression evaluation. Default is FEEL.

The asset request carries the target `Format` and a ranked `Accept` list,
because the target format constrains what can be embedded. **PDF has no SVG
mechanism**, so an SVG handed to a PDF render is a coded error naming the
accepted set — not a silent drop and not an in-library rasteriser.

## Skill Pack

Flat `skills/*.md`, `//go:embed *.md`. Categories come from filename prefixes,
not directories: `block-<kind>`, `format-<name>`, `tool-<name>` (one per MCP
tool, `vellum_` stripped), `theme-<topic>`, `fill-<topic>`, and unprefixed design
guides. Frontmatter carries `name`, `description`, `kind`, `category`, `type`,
`applies_to`, `examples_tags`. Per-prefix required headings and per-family token
budgets are both enforced by tests.

`docs/` is for humans; `skills/` is for LLMs and is loaded via MCP at runtime.
They are not the same document written twice.

## What NOT to Do

- Do not add a semantic section vocabulary ("cover", "executive summary",
  "methodology appendix"). That is the consumer's vocabulary. A library that
  ships one product's section types makes the next consumer fight it.
- Do not compute anything statistical. Significance letters, margins and
  low-base flags arrive already resolved. There is no `compute_totals` option
  and there will not be one.
- Do not render a chart. Vellum embeds an asset it is handed.
- Do not rasterise anything.
- Do not add a "lenient" or "best effort" decode mode, not even behind a flag.
  It will be used. Silent tolerance of unknown fields is poison when an LLM
  authors the spec: the model gets no signal that its output was partially
  ignored.
- Do not hand-edit a golden, including "just to fix formatting". The gate exists
  because that is exactly how goldens stop being evidence.
- Do not re-marshal a source part with `encoding/xml`.
- Do not shell out to anything **from the library**. A consumer embedding Vellum gets the same bytes whether or not any external tool is installed. The build-tagged test oracles are the sole exception, fenced by a tag and by `TestNoOfficeToolingOnTheLibraryPath`.
- Do not perform network I/O.
- Do not log. Diagnostics are returned as data.
- Do not scaffold empty `convert/` or worker directories. They are cut
  permanently; an empty directory invites someone to fill it.

## Reference Docs

- [`.claude/reference/determinism.md`](.claude/reference/determinism.md) — the full pinned-source table, the test harness, and the honest limit on byte-identity.
- [`.claude/reference/scope.md`](.claude/reference/scope.md) — what is deliberately not in v1, and where each deferred item lands.
