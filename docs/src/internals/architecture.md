# Architecture

Three content models, each with exactly one job. CLAUDE.md calls getting
this wrong "the most expensive mistake available in this codebase," and
states the reasoning plainly enough that it is worth reproducing rather than
paraphrasing away.

## The three models

**`spec` is unresolved and hashable.** It is author intent, nothing more: a
theme referenced *by name*, marks referenced by name, untyped values, no
measurements anywhere in it. This is precisely what makes `Spec.Hash()`
computable *before* a theme provider has even answered — which in turn is
what lets a consumer ask "does this artifact already exist?" and skip an
entire render, rather than discovering the answer only by paying for the
work. See [Artifact identity](../spec/identity.md).

**`fragment` is resolved and format-neutral.** Concrete font families,
sizes and colours; every length in EMU; every value typed with its format
code; every asset carrying real bytes, a media type and a content hash.
Theme application, font selection, number formatting, asset resolution and
mark resolution all happen exactly **once** here, and the result is shared
by all four writers and by fill's own splicer — none of DOCX, XLSX, PPTX or
PDF re-derives "what colour is `accent`" independently. `fragment` knows
nothing about pages, sheets, slides or XML; it carries the theme's **whole**
palette and type scale, not merely the values the content happened to use —
a document with no body paragraph at all still needs an answer for what
colour `background` is, and a format that declares a fixed palette up front
(PPTX's twelve `theme1.xml` slots) cannot reconstruct one from text that was
never there.

**`doc` / `sheet` / `deck` / `pdf` are resolved *and* laid out.** A flow
document, a workbook, a slide deck and a paginated page tree are genuinely
different things, and forcing all four into one intermediate representation
would produce a model that served none of them well. Each is public, so a
consumer needing format-specific reach — building a `*doc.Document` by hand
and handing it to `Vellum.Write`, say — genuinely has it. See [Public
facade](../library/facade.md#the-eight-methods).

`fragment` earns its own place in this pipeline specifically because it has
two genuinely different lowerings: whole-document, into one of the four
format models, and bounded-sequence, into a template's existing idiom during
a fill. **Fill mode never constructs a `doc.Document`** — it lowers a
`fragment.Sequence` directly into whatever the template's own anchors
already establish.

## The pipeline

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

Reading it left to right along the top row: a specification is decoded
strictly (an unrecognised field is a build failure, never a silent skip —
see [The block model](../spec/blocks.md#strict-decoding)), its hash names
the artifact it would produce *before* anything downstream runs, the
capability matrix is consulted to confirm the target format can honour what
the specification asks for, `resolve.Resolve` turns it into a `fragment.Doc`
carrying every warning that resolution raised, one of four `Lower` functions
turns that into the format's own model (this is where an overflow policy —
splitting a table across pages, say — is actually applied), and one writer
serialises it: the three OOXML formats through `opc`/`zipdet`, PDF through
its own object writer.

The middle row is the escape hatch [Public facade](../library/facade.md)
documents as `Vellum.Write`: a caller who already has a `*doc.Document` (or
built one by editing a resolved model directly) writes it without ever
touching `spec.Spec` or `resolve.Resolve` at all.

The bottom row is fill mode, which shares almost nothing with the top two —
it opens an *existing* package rather than building one, discovers anchors
read-only, evaluates FEEL against caller-supplied data to produce a bounded
`fragment.Sequence`, and splices only the parts that sequence actually
touches back into a clone of the source package via `xmlcopy`'s raw-token
copy. See [Non-destructiveness](../fill/non-destructive.md) for what
"only touched parts rewritten" means as a proven guarantee rather than an
aspiration.

## Import-graph invariants that keep this true

A handful of structural rules in CLAUDE.md exist specifically to keep the
diagram above from becoming aspirational over time — enforced by `go list
-deps` gates, not merely by convention:

- **`spec` never imports `theme`.** This is the single import-graph fact
  that keeps `Spec.Hash()` computable before any theme resolution happens
  at all — if `spec` could reach into `theme`, nothing would stop a future
  change from making the hash depend on resolved theme state.
- **`fragment` may not import `doc`, `sheet`, `deck`, `pdf`, `opc`, or
  `encoding/xml`.** The format-neutral IR staying genuinely format-neutral
  is what lets every writer share the one resolution pass.
- **`descriptor` never imports `resolve`, `doc`, `sheet`, `deck`, `pdf`, or
  `template`.** It is deliberately "no-execute": a caller can build the
  manifest and the payload schema — the machine-readable index of what
  Vellum can do — without linking in a single renderer.
- **`template/`, `defrag/`, `splice/` never import `encoding/xml`.** Fill
  mode's entire non-destructiveness guarantee depends on never
  re-marshalling a source part; this import ban makes the alternative
  unrepresentable rather than merely disallowed by convention. See
  [Non-destructiveness](../fill/non-destructive.md#how-this-is-actually-achieved-not-just-claimed).

## See also

- [The Spec](../spec/blocks.md), [Themes](../spec/themes.md) — what a
  `spec.Spec` and a resolved theme actually contain.
- [Fill Mode](../fill/anchors.md) — the bottom row of the diagram, in full.
- [Determinism](determinism.md) — the guarantee every stage of this
  pipeline is required to uphold.
- [Public facade](../library/facade.md) — the Go entry points that drive
  each row.
