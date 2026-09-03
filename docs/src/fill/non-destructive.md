# Non-destructiveness

Naive template libraries parse a `.docx`, `.xlsx` or `.pptx` into an
incomplete object model and re-serialize it, which silently discards
everything that model does not understand: tracked changes, comments, custom
XML parts, embedded objects, footnotes, speaker notes on a slide nobody
touched. A designer's carefully built corporate template comes back mangled
— and worse, comes back mangled *quietly*, because the library that did it
has no concept of what it threw away. This is the exact failure mode fill
mode exists to make structurally impossible rather than merely rare.

## `Result.Touched`: the receipt

```go
type Result struct {
    Touched []string     // part names Fill actually rewrote, bytewise sorted
    Package *opc.Package // the filled package
}
```

`Vellum.Fill` returns a `template.Result`. `Touched` is not a log message or
a best-effort report — it is the assertable claim the whole guarantee rests
on: **every part in `Package` whose name does not appear in `Touched` is
byte-identical to that same part in the original template.** `Package` is
itself a clone of the template's own package (`opc.Package.Clone`), so
filling never mutates the `*Template` a caller opened — the same template can
be filled again with different data.

## How this is actually achieved, not just claimed

Fill never re-serializes a source part with `encoding/xml`. Doing so would
lose namespace prefixes, attribute order and self-closing form even for a
part it left semantically unchanged — a lossy round-trip Vellum treats as a
destructive one. Instead:

- **`xmlcopy`** is a token-level XML rewriter: raw-token copy with surgical
  subtree replacement. Everything before a spliced anchor's span, and
  everything after the last one, is copied through as bytes, verbatim —
  never reparsed, never reformatted.
- `xml:space="preserve"` is honoured, and whitespace is normalised **nowhere**
  on the fill path.
- The `encoding/xml` import is banned outright from `template/`, `defrag/`
  and `splice/` by a `go list -deps` CI gate
  (`TestNoEncodingXMLInFill`), so this is not a convention that could quietly
  regress — it is unrepresentable in a build that passes.

## What the corpus actually proves

`TestNonDestructiveCorpus` (DOCX) fills a hand-built, realistic package —
deliberately realistic rather than a systematic sweep of edge cases, because
the point is proving the property against the shape a real Word-authored
file actually has. Its `word/document.xml` carries, in order: a tracked
insertion, a tracked deletion, a comment range and its reference, a footnote
reference, an embedded OLE object, a `{{customer_name}}` marker anchor, a
native `"body"` content control, and a closing paragraph. Only the two
anchors are spliced; every other structural element — the tracked changes,
the comment, the footnote, the OLE object — is asserted **untouched**,
compared with `bytes.Equal`, never a semantic diff that might paper over a
byte that moved.

`TestNonDestructiveCorpus_XLSX` and `TestNonDestructiveCorpus_PPTX` prove the
identical property through the real `template.Fill` entry point, against
fixtures carrying each format's own native equivalents of "things a naive
library would drop":

- **XLSX**: a second, entirely untouched worksheet sitting beside the one
  being filled, plus a defined name deliberately left alone via
  `Binding.OptionalAnchors`.
- **PPTX**: a second, untouched slide carrying its own speaker notes and
  embedded media, sitting beside the slide(s) actually being filled or
  repeated.

Every one of these tests asserts two things, not one: that `Touched` names
*exactly* the parts that were supposed to change (no more, no fewer — a part
touched that should not have been is caught the same way a part that should
have changed but did not would be), and that every part outside it is
`bytes.Equal` to the source, part for part.

## Reading the receipt as a caller

```go
res, err := v.Fill(ctx, r, size, binding, data)
if err != nil {
    // ...
}
for _, part := range res.Touched {
    fmt.Println("rewrote:", part)
}
```

A caller with an audit requirement — "prove this fill did not alter anything
outside these three cells" — reads `Touched` directly rather than diffing
the whole package byte-for-byte themselves; the library has already done
that diffing, deterministically, as part of producing the result.

## See also

- [Templates and anchors](anchors.md) — what gets discovered before
  anything is spliced.
- [Bindings and FEEL](bindings.md) — what actually drives which anchors get
  touched.
- [Public facade](../library/facade.md) — `Vellum.Fill`'s exact signature.
- `skills/fill-anchors.md` — the same corpus description, terse, for an LLM.
