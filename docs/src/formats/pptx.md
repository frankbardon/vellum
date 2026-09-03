# PPTX

PresentationML — a slide deck, and the only one of the four formats with a
real, native speaker-note channel. Every block kind renders, including
`notes` (`Renders`, not degraded — the only format where it isn't).
`page_break` degrades to a new slide.

## Assets and overflow

`image/png` and `image/jpeg` render with every encoding variant intact, the
same as DOCX; `image/svg+xml` degrades to raster-plus-vector for the same
Word/PowerPoint reason. `overflow.continue_repeat_headers` **renders** here —
a table too long for one slide continues onto the next with its header band
repeated — with capacity **theme-derived** rather than measured, so the split
stays reproducible across machines rather than depending on whatever fonts
happen to be installed where a measurement was taken. (PDF, which lays itself
out completely, is the one place capacity actually is measured instead — see
[PDF and PDF/A](pdf.md).)

## Fonts

As DOCX: `font.embed.subset` and `font.embed.whole` both degrade to a name
reference, because PPTX authors no embedding settings in v1 either. A deck is
where a missing face is most visible — large type, a fixed layout with no
reflow to absorb the substitution — so the degradation is warned rather than
assumed harmless, the same way it would be anywhere else, but worth calling
out because a slide deck is the format where an operator is most likely to
actually *notice*.

## Fill mode

`fill.bind.repeat` gains a fourth `RepeatTarget` no other format has:
`RepeatTargetSlide` clones an entire slide **part** N times — new
relationships, new `[Content_Types].xml` overrides, and fresh `<p:sldId>`
values, a real OPC structure mutation rather than a byte-span splice the way
row and block splicing are. A zero-item `slide` repeat targeting the deck's
only slide is rejected outright
(`VELLUM_TEMPLATE_SLIDE_REPEAT_EMPTIES_DECK`) rather than silently emptying
the deck to zero slides. See [Bindings and FEEL](../fill/bindings.md).

## Byte-layout notes worth knowing

PPTX carries the most byte-layout invariants of any format Vellum writes,
because PowerPoint is unusually strict about schema order and identifier
spaces on open:

- `sldId`, `sldMasterId` and `sldLayoutId` occupy **disjoint, bounded**
  identifier spaces: a `sldId` is at least 256 and below 2³¹; a master or
  layout id sits at or above that boundary. A deck mixing the two ranges is
  one PowerPoint repairs on open.
- A shape identifier starts at **2** — identifier 1 belongs to the shape
  tree's own group shape, which every shape tree carries, and a shape
  numbered 1 collides with it. Identifiers are assigned by tree position, so
  two slides carrying the same shapes in the same order produce the same
  identifiers.
- A **title** placeholder carries no `idx`; every other placeholder does. The
  asymmetry is the schema's own, not a bug: a title matches its layout
  counterpart by type alone, and everything else matches by index — a title
  with an index, or a body without one, inherits from the wrong layout shape
  and the slide opens with placeholders stacked on top of each other with no
  error reported anywhere.
- Every colour a master, layout or slide states is a **scheme reference**,
  never a literal `srgbClr` — that literal form lives only in `theme1.xml`
  itself. A literal colour elsewhere is one the theme cannot restyle, and it
  produces a deck that looks correct and cannot be restyled, which is worse
  than looking wrong because nothing reports it.
- Masters, layouts and the theme part are **authored from the theme**, never
  shipped as a fixed template and copied through — a `.pptx` with none of
  those parts is not a deck with default styling, it is a file PowerPoint
  refuses to open.
- In a table, every row's height is stated explicitly (cell insets plus an
  exact `a:lnSpc` line box on every paragraph, including empty ones — an
  unstated empty paragraph mark takes DrawingML's 18pt default and silently
  doubles a crosstab's mostly-empty rows), and a spanning cell keeps every
  covered cell present with `hMerge`/`vMerge` rather than replacing them the
  way WordprocessingML's `gridSpan` does.

## See also

- [Tables](../spec/tables.md#what-each-format-actually-does-with-this-structure)
  — the table-specific byte-layout rules in more depth.
- [Themes](../spec/themes.md) — masters and layouts are authored *from* a
  theme document; this is where that document lives.
- [Bindings and FEEL](../fill/bindings.md) — `RepeatTargetSlide` in full.
- `skills/format-pptx.md` — the same material, terse, for an LLM composing
  against this target.
