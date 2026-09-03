# Themes

A theme is every appearance decision a specification does not make, held
entirely apart from what the specification says. A `spec.Spec` names a theme
*by reference* (`theme: "acme-brand"`, or empty for the built-in theme) — it
never carries a colour, a font family or a measurement itself. That
separation is what keeps `Spec.Hash()` computable before any theme has been
resolved: two specifications differing only in which theme they name are, by
construction, different documents, but a specification's *identity* does not
depend on how a theme happens to render it today. See [Artifact
identity](identity.md).

Vellum ships exactly one theme, the built-in default, plus the schema. It
never ships a list of brands, because which brands exist is entirely the
consumer's business — the same reasoning that keeps a semantic section
vocabulary out of the block model applies here too.

## Why the theme model holds no maps

Every collection in `theme.Theme` — colour roles, font roles, mark styles,
boxes — is an ordered slice, even though each is logically keyed. A Go map
ranges in a different order on every process run, and a theme is read
directly on the output path, so a map here would be a nondeterminism source
sitting one hop upstream of the actual bytes. Taking a slice instead makes
that disorder unrepresentable rather than merely tested against; lookup is a
linear scan, and the collections are small enough by construction that this
costs nothing that matters — a theme needing a hash map to find a colour role
would have too many colour roles.

## Fonts

Three typographic slots (`theme.FontRole`): `body` (running prose), `heading`
(titles) and `mono` (fixed-width text). Each declared `Font` states a
`Family` name, a `Handle` the asset resolver serves the program bytes from,
and — critically — an `Embeddable` bool plus an `EmbedMode`
(`""`/auto, `subset`, `whole`).

Embedding rights are a **licence condition**, and only whoever authored the
theme knows what a face's licence permits. Vellum cannot infer this and does
not try: `Embeddable: true` means embed it; `false` means substitute the
named `Substitute` family and warn; `false` with no `Substitute` set is a
validate-time error, never a silent system fallback. `font.embed.none` — a
face referenced by name only, with no embedding at all — is what every OOXML
format gets in v1 regardless of what a theme asks for, because DOCX, XLSX and
PPTX author no font-embedding settings today; asking for `subset` or `whole`
there degrades quietly to a name reference. PDF is the opposite: PDF/A-2b
*requires* every font embedded, so `font.embed.none` there is a hard
rejection (`VELLUM_FONT_EMBED_UNSUPPORTED`) rather than a degradation — which
is exactly why the built-in theme, whose three faces are all
non-embeddable, cannot compose to PDF at all until a theme with at least one
embeddable face is supplied.

## Colours

Ten semantic slots (`theme.ColorRole`), every one of them required of every
theme: `background`, `text`, `text_muted`, `heading`, `accent`,
`accent_text`, `rule`, `table_header_background`, `table_header_text`,
`table_stripe`. A theme with a hole in its palette would otherwise be
discovered by whichever document first used the missing role — a long way
from where the theme was authored — so validation checks completeness up
front instead.

The [`fragment`](../internals/architecture.md) IR that resolution produces
carries the theme's **whole** palette, not only the colours the content
happened to reference. A writer reading colours off the runs it happened to
draw gets no answer at all from a sparse document — a file of nothing but
headings has no body paragraph to read a body colour from — and a format
that declares a palette up front (PPTX's `theme1.xml` slots) cannot
reconstruct one from the text alone. Twelve named slots, every one filled,
every time.

## Mark styles

A block's, cell's or annotation's `marks: []string` names entries in the
theme's own `Marks` table. Each `MarkStyle` maps one consumer-chosen name to
character-level switches (`bold`, `italic`, `underline`) plus a `color` and a
`background` — both colour *roles*, not raw values, so a mark follows the
palette rather than pinning a colour the theme has since moved away from.
This is the only place in Vellum a mark name is ever interpreted, and it is
interpreted purely as data: nothing branches on what a mark's name *is*. A
mark with no matching style is a resolve-time warning
(`VELLUM_MARK_UNKNOWN`), never a hard error — an author flagging content with
a mark the current theme has not yet defined a style for should still get a
document, just one that says so.

## Layouts, pages and box roles

A theme carries one or more `Layout` per target format — because the same
brand's page is A4 in a document and 16:9 in a deck, and because a document's
landscape section is a genuinely different page setup, not a flag on one
layout. Exactly one layout per format must be marked `Default`; a section
naming no `layout` gets that one, and naming one the theme does not declare
for the target format is `VELLUM_THEME_LAYOUT_NOT_FOUND` rather than a silent
fallback to the default. Each `Layout` states its own `Page` geometry (width,
height, four margins) and the `Box`es it offers.

A **box** is an asset slot: the size a host should render a picture at.
`theme.BoxRole` is a small, closed, enumerable set on purpose —
`asset.full` (the full content width, and the default an `asset` block with
no `role` selects), `asset.half` (a two-up row), `asset.quarter`, and `logo`
(the brand slot placed in a header, footer or master). Each `Box` states a
`Width`; a fixed-geometry format (PPTX) states a `Height` too, while a
flowing format (DOCX, PDF) leaves `Height` zero to mean "follow the asset's
own aspect ratio" — the column width is fixed there, but the vertical extent
genuinely is not.

The set is theme-level rather than per-instance deliberately: the host
renders one artifact per *distinct box*, so a per-block query would make the
artifact set grow with a document's layouts rather than with its content, and
near-identical boxes would produce near-identical artifacts no cache could
ever unify. Bounding it at the theme means every full-width chart in a
document — indeed, in every document composed against this theme — renders
at exactly the same size, which is the property `Theme.Boxes(format)` exists
to deliver, and it is answerable **before any specification exists at all**.
See [`vellum_boxes`](../mcp/index.md) and [Seams](../library/seams.md).

## The `theme.Provider` seam

A specification names a theme id; resolving that id to a document is a seam
(`theme.Provider`), with `theme.BuiltinProvider` as the inert default that
serves only the theme Vellum ships. A host implements the interface directly
to serve themes from its own storage, or uses `theme.NewStaticProvider` for a
small, construction-time-known set. Whatever the provider, a theme id it does
not carry must fail as `VELLUM_THEME_NOT_FOUND` — serving a *different* theme
than the one asked for would produce a document that is wrong in a way that
looks right, which CLAUDE.md calls out as the worst kind of wrong a document
library can produce. See [Seams](../library/seams.md#themeprovider).

## See also

- [Artifact identity](identity.md) — why a theme is referenced, not embedded.
- [Capability matrix](../formats/capabilities.md) — the `font.*` rows in
  full, per format.
- `skills/theme-document.md` — the same material, terser, for an LLM
  authoring a theme document.
