---
name: format-pdf
description: PDF/A-2b, emitted directly rather than converted, laid out by Vellum itself.
kind: pdf
category: format
type: skill
applies_to: [pdf]
examples_tags: [format, pdf, pdfa]
---

## What it emits
PDF/A-2b, emitted directly — never converted, because conversion output
varies with renderer version and installed fonts. Every block renders
except `block.notes`, which degrades to a footnote — a PDF annotation is
not guaranteed visible everywhere and PDF/A restricts annotation types.

## Capability notes
- The only format Vellum lays out itself, so it can only promise scripts it
  has established it shapes and breaks correctly: `text.script.latin`,
  `text.script.greek` and `text.script.cyrillic` render. `text.script.other`
  — and two supported scripts mixed in one string — is **rejected**
  (`VELLUM_PDF_SCRIPT_UNSUPPORTED`): an unsupported script draws *wrong*,
  not absent.
- `text.bold`/`text.italic` **degrade** to the regular/upright cut of the
  same face — a PDF names one program per face (a theme declares one per
  role) and Vellum will not synthesise a stroked fake.
- `font.embed.none` **rejects** (`VELLUM_FONT_EMBED_UNSUPPORTED`) — PDF/A-2b
  requires every font embedded. This is why the built-in theme, whose three
  faces are all non-embeddable, cannot compose to PDF at all.
  `font.outlines.cff` degrades to whole-program embedding rather than a
  subset (no CFF subsetter); `font.embed.subset`/`font.embed.whole` on
  TrueType outlines render.
- Images: `asset.media.image/png`/`asset.media.image/jpeg` render;
  `asset.png.alpha` becomes an `/SMask`. `asset.png.interlaced`,
  `asset.jpeg.progressive` and `asset.jpeg.cmyk` all **reject**
  (`VELLUM_PDF_IMAGE_UNSUPPORTED`) — Vellum embeds bytes verbatim and will
  not decode-and-recompress to work around an encoding choice.
  `asset.media.image/svg+xml` rejects outright — PDF has no SVG mechanism.
- `asset.alt_text` degrades to "dropped" — it needs a structure tree
  PDF/A-2b does not require and Vellum does not write.
- `fill` and every `fill.bind.*` mode **reject** — a PDF is not an OPC
  package; there is no template to splice into.

## Gotchas
- `Resources` is never inherited on the page tree, even though ISO 32000-1
  allows it — PDF/A-2 clause 6.2.2 needs it explicit on the page.
- Every page's content stream is built before any font is written — a
  subset needs the glyphs the stream actually draws.
- Image bytes are embedded verbatim (a JPEG's own DCT stream, an opaque
  PNG's own IDAT); only an alpha PNG is unfiltered, split and recompressed,
  because PDF keeps colour and alpha in separate streams while PNG
  interleaves them.
- `/CreationDate`, `/ModDate` and the XMP dates come from one struct so
  they cannot disagree, and the file identifier is derived from content.

## See
- `block-notes.md`, `theme-document.md`
