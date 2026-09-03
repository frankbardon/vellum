# The capability matrix

Whether a given feature — a block kind, a table property, a font embed mode,
a script, a fill binding mode — behaves in a given target format is a
**declared fact**, sitting in `capability/matrix.go` as data, before it is
ever code. That ordering is deliberate and is stated as a project-wide
principle in CLAUDE.md: "a behaviour a consumer can observe... is a row in
`capability/matrix.go` **before** it is code." A consumer scheduling an
unattended batch render must be able to learn the answer by *asking*, not by
running the job and reading a support ticket afterward.

## Three outcomes, and only three

Every `(feature, format)` pair in the matrix carries exactly one of:

- **`renders`** — the feature is honoured as specified, faithfully.
- **`degrades`** — the feature is honoured as a stated, named alternative,
  and a warning naming the feature is raised so a caller learns about the
  substitution rather than discovering it later. A `heading` block in XLSX
  degrading to "a styled cell above the table" is the example CLAUDE.md
  itself uses.
- **`rejects`** — the feature is refused outright, at validate time, naming a
  coded error. An `asset` block in XLSX is rejected rather than approximated,
  because a workbook is where a reader goes for the numbers behind a chart,
  not the chart.

`TestCapabilityOutcomesAreWellFormed` keeps the three outcomes distinguishable
by construction: a `degrades` row must name its alternative and a `rejects`
row must name its code, so neither can quietly collapse into the other. The
row is only half the promise, though — the other half is that a `degrades`
row's warning is machine-checked to actually fire:
`TestCapabilityDegradationsAreReported` requires every degrading row to be
exercised by a specification that provokes a warning naming the feature it
degrades, or to carry a stated reason no specification can provoke it (there
are exactly three such rows today, all `font.outlines.cff` in the OOXML
formats — see below).

## Reading it as a caller

Three ways to ask the same question, all backed by the identical matrix:

- **CLI**: `vellum capabilities --format docx` — see [Commands and
  flags](../cli/flags.md).
- **MCP**: the `vellum_capabilities` tool — see [MCP
  Integration](../mcp/index.md).
- **Library**: `v.Capabilities(artifact.FormatDOCX)` — pure data, takes no
  context, and cannot fail; an unrecognised format returns an empty matrix
  rather than an error, because the matrix genuinely has nothing to say about
  a format it does not know, and saying nothing is the honest answer there.

`vellum validate` and `vellum compose` both run the identical resolution
against the identical matrix — `validate` simply stops one step short of
writing.

## What a feature name looks like

Feature names are dotted, hierarchical strings: `block.<kind>` for every
`spec.BlockKind` (`FeatureForBlockKind` derives this automatically, so a new
block kind cannot go undeclared for a format), `table.hierarchical_headers`,
`table.cell_annotation`, `asset.media.image/png`, `asset.png.alpha`,
`font.embed.subset`, `font.outlines.cff`, `text.bold`, `text.script.other`,
`overflow.continue_repeat_headers`, `fill`, `fill.bind.repeat`,
`fill.bind.if`, `fill.bind.with`, `fill.bind.skip`, and so on. Each is
enumerable and answerable per format — `Profile(format)` projects out exactly
the features a given format renders, which is what conformance profiling
reads.

## A worked example: `font.outlines.cff` and the honest-reason rows

`font.outlines.cff` is a genuinely interesting row to read closely, because
it shows the matrix reasoning about *why* an outcome is what it is, not just
recording it. An embed *mode* (`font.embed.subset`, `font.embed.whole`) is a
licence condition on **how** a font program may be embedded. A format that
embeds *no* font programs at all — every OOXML format, in v1 — cannot violate
that condition, because there is nothing being embedded for it to apply to.
So in DOCX, XLSX and PPTX, `font.embed.subset` and `font.embed.whole` both
*degrade* (to "the family referenced by name") rather than reject, and
`font.outlines.cff` in those same three formats is one of the three rows
`TestCapabilityDegradationsAreReported` exempts from needing its own
exercising specification: the row states plainly that the outline format
cannot change the answer there, because no font program is ever loaded to
have an outline format at all — the degradation that row names is the one
`font.embed.*` already reports, and a second specification testing the
identical warning under a different name would test nothing new. PDF is
where the outline format finally matters, because PDF is the one format that
actually embeds programs: a CFF-outline face there degrades to whole-program
embedding (no CFF subsetter exists) while a TrueType-outline face on the
identical embed demand renders as a real subset.

## Format-by-format

Each output format has its own page, written from the matrix rows and the
byte-layout invariants that produce them, not merely restating the matrix:

- [DOCX](docx.md) — the richest target for prose, and fill mode's richest
  template.
- [XLSX](xlsx.md) — presentation tables, deliberately not a spreadsheet.
- [PPTX](pptx.md) — the only format with a native speaker-note channel.
- [PDF and PDF/A](pdf.md) — the only format Vellum lays out completely
  itself.

## See also

- [The block model](../spec/blocks.md) and [Tables](../spec/tables.md) —
  what each feature actually is.
- [Themes](../spec/themes.md) — the font and colour rows this matrix
  constrains.
- `skills/format-*.md` and every `skills/block-*.md`'s own "Gotchas" section
  — the identical facts, written for an LLM composing against a specific
  target.
