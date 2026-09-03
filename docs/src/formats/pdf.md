# PDF and PDF/A

PDF/A-2b, emitted directly — never converted from an intermediate format,
because conversion output varies with renderer version and installed fonts,
which would defeat byte-identical output outright. PDF is also the **only**
format Vellum lays out itself: there is no application behind a PDF to
paginate it afterward the way Word paginates a `.docx`, so every page break,
every line break and every table split here is a decision Vellum's own code
made and then drew, rather than one a reader's own layout engine will make
later.

Every block renders except `notes`, which degrades to a footnote — a PDF
annotation is not guaranteed visible in every reader and PDF/A itself
restricts which annotation types are conformant, so a footnote in the flow
of the page is the honest choice.

## Script support is a real restriction, not a formality

Because PDF is the one format Vellum lays out itself, it can only promise
scripts it has actually established it shapes and breaks correctly:
`text.script.latin`, `text.script.greek` and `text.script.cyrillic` render.
`text.script.other` — and two supported scripts mixed within a single string
— is **rejected** (`VELLUM_PDF_SCRIPT_UNSUPPORTED`), not silently drawn. This
is a deliberate asymmetry with the three OOXML formats, which impose no such
restriction at all, because Word, Excel and PowerPoint do their own shaping
and Vellum's line breaker never touches their text. An unsupported script
draws *wrong* here, not merely absent, and Vellum treats "wrong" as strictly
worse than "refused."

`text.bold` and `text.italic` both **degrade** to the regular, upright cut of
the same face, rather than rendering — a PDF names exactly one font program
per face (a theme declares one face per role), and Vellum will not
synthesise a stroked-fake bold or an algorithmically-slanted italic. A theme
wanting real bold or italic in PDF needs to declare a face for each cut it
wants.

## Fonts: the format where embedding actually matters

PDF is the one format where an embed demand is honoured rather than
degraded, and correspondingly the one format where a licence mismatch is
fatal rather than warned about:

- `font.embed.none` **rejects** (`VELLUM_FONT_EMBED_UNSUPPORTED`) — PDF/A-2b
  requires every font embedded, full stop. This is exactly why the built-in
  theme, whose three faces are all declared non-embeddable, cannot compose
  to PDF at all until a theme naming at least one embeddable face is
  supplied — see [Themes](../spec/themes.md#fonts).
- `font.outlines.cff` degrades to whole-program embedding rather than a real
  subset, because Vellum ships no CFF subsetter — see
  `.claude/reference/scope.md`'s "Deferred, designed for" table, where CFF
  subsetting is named as the deferred item with the largest correctness cost
  of anything in scope.
- `font.embed.subset` and `font.embed.whole` on TrueType outlines both
  **render** as stated — a genuine subset containing only the glyphs the
  document actually drew, or the whole program, exactly per the theme's own
  demand.

## Images

`image/png` and `image/jpeg` render; a PNG's alpha channel becomes a real PDF
`/SMask`. `image/svg+xml` is rejected outright — PDF has no SVG mechanism at
all, and Vellum will not ship an SVG-to-PDF translator, which would amount to
a second renderer with its own text layout and font matching, free to drift
from whatever produced the original asset. An interlaced PNG, a progressive
JPEG and a CMYK JPEG are all rejected too
(`VELLUM_PDF_IMAGE_UNSUPPORTED`): Vellum embeds image bytes verbatim and will
not decode-and-recompress an asset to work around an encoding choice someone
else made when they saved the file. `asset.alt_text` degrades to "dropped" —
alt text needs a structure tree that PDF/A-2b does not require and Vellum
does not write.

## Fill mode does not apply here

`fill` and every `fill.bind.*` mode **reject** outright for PDF. A PDF is not
an OPC package; there is no template structure to splice bound data into.
Fill mode is exclusively a DOCX/XLSX/PPTX capability — see [Templates and
anchors](../fill/anchors.md).

## Byte-layout notes worth knowing

- `Resources` is deliberately **never inherited** on the page tree, even
  though ISO 32000-1 permits it as an obvious space saving — PDF/A-2 clause
  6.2.2 requires a content stream referencing a font or image to have an
  *explicitly associated* resource dictionary, and veraPDF rejects the
  inherited form. `MediaBox`, `CropBox` and `Rotate` are hoisted onto the
  tree; `Resources` is not.
- Every page's content stream is built **before** any font object is
  written, because a font subset is exactly the glyphs the page's own
  stream drew, and a face learns which glyphs those are by being drawn
  with.
- Image data is embedded verbatim — a JPEG's own DCT stream, an opaque PNG's
  own IDAT stream carrying the PNG predictor its scanlines are already
  filtered with. The single exception is an alpha-carrying PNG, which is
  unfiltered, split and recompressed, because PNG interleaves colour and
  alpha on one scanline and PDF keeps them in two separate streams.
- `/CreationDate`, `/ModDate` and the XMP packet's dates are all generated
  from one shared struct, so they cannot ever disagree — ISO 19005 requires
  them to match, and veraPDF was observed *not* to check that agreement
  under its 2b profile, which is the actual reason the invariant is kept
  structural rather than left to the validator.
- The file identifier (`/ID`) is derived from the document's own content,
  never generated randomly — a random one is the easiest way to lose
  byte-identity and the hardest way to notice it, since the file would then
  differ between runs in exactly sixteen bytes buried in the trailer while
  everything else matched.

## The PDF/A preflight

`pdfa.Preflight` runs on **every** PDF document Vellum writes, before any
byte reaches the destination, with no option to skip it — the only reason to
want that option would be to write a file whose own metadata falsely claims
conformance, and a false conformance claim is worse than no claim at all. See
[Determinism](../internals/determinism.md#the-pdfa-preflight) for what it
checks and, just as importantly, what it deliberately does not.

## See also

- [Tables](../spec/tables.md#what-each-format-actually-does-with-this-structure)
  — PDF's three-pass table drawing order and measured (not theme-derived)
  row capacity.
- [Determinism](../internals/determinism.md) — the "external reader
  oracles" (poppler, veraPDF) that check a written PDF against a real
  reader, not just against itself.
- `skills/format-pdf.md` — the same material, terse, for an LLM composing
  against this target.
