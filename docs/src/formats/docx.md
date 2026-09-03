# DOCX

WordprocessingML — a flowing document, and the richest of the four targets
for prose. Every block kind renders except `notes`, which degrades to a
footnote anchored at the block's own position in the section, and
`overflow.continue_repeat_headers`, which degrades to plain flow: Word
paginates the document itself, and a split Vellum computed in advance would
disagree with the one Word actually performs when the file is opened. DOCX is
also fill mode's richest template format — see below.

## Assets

`image/png` and `image/jpeg` render, and every encoding variant — an alpha
channel, interlacing, progressive JPEG, CMYK JPEG — passes through untouched,
because the asset becomes a package part verbatim and Word decodes it itself;
Vellum never re-encodes an accepted media type. `image/svg+xml` degrades to a
raster fallback with the vector embedded alongside it: Word 2016 and later
only reads an SVG when a raster blip accompanies it, and since Vellum never
rasterises, the caller supplies both encodings and Vellum embeds the pair.

## Fonts

Font embedding is genuinely absent here in v1: DOCX authors no
font-embedding settings at all, so `font.embed.subset` and `font.embed.whole`
both degrade to "the family referenced by name" regardless of what a theme
asks for. `font.embed.none` — a name reference and nothing else — is what
renders, which is the common case and the only case DOCX can actually
produce. The font's own outline format (`font.outlines.cff`) does not change
this answer, because no font program is ever loaded here to have an outline
format in the first place.

## Text and script

`text.bold` and `text.italic` both render — the target application selects
or synthesises the appropriate cut of the family. Every script renders,
including `text.script.other`: Vellum writes characters and Word shapes and
breaks them, so unlike PDF this format carries no restriction on writing
system at all.

## Fill mode

DOCX is fill mode's richest target. `fill` and every `fill.bind.repeat` /
`fill.bind.if` / `fill.bind.with` / `fill.bind.skip` combination render. Both
`RepeatTarget` realizations documented in [Bindings and
FEEL](../fill/bindings.md) are available here: `row` splices copies of a
table row, and `block` splices copies of a native content control's whole
content — no other format offers the second. See [Templates and
anchors](../fill/anchors.md) for DOCX's own `native` (`w:sdt` content
control) and `marker` (`{{name}}` text) anchor kinds.

The non-destructiveness fixture proving fill leaves everything else alone
carries tracked changes, a comment, a custom XML part, footnotes and an
embedded OLE object — none of which fill mode disturbs. See
[Non-destructiveness](../fill/non-destructive.md).

## Byte-layout notes worth knowing

A handful of WordprocessingML-specific invariants from CLAUDE.md's
byte-layout section are worth surfacing here directly, because getting any
one of them wrong produces a file Word repairs or refuses on open rather than
one that merely looks different:

- No `w:rsid*`, `w15:docId`, or any GUID is ever emitted. Where the format
  demands an identifier (`wp:docPr/@id`, `numId`, bookmark ids), it is a
  deterministic counter over a canonical document walk, never random and
  never a UUID.
- A `block.notes` block always becomes a footnote here — never a margin
  comment (that's XLSX's degradation) and never a speaker note (PPTX's
  native rendering).

## See also

- [Templates and anchors](../fill/anchors.md), [Bindings and
  FEEL](../fill/bindings.md), [Non-destructiveness](../fill/non-destructive.md)
  — DOCX is where all three are demonstrated most fully.
- [Themes](../spec/themes.md) — the font and colour roles this format's
  master styles draw from.
- `skills/format-docx.md` — the same material, terse, for an LLM composing
  against this target.
