# The real-corpus adapter's gap, carried openly

This directory is the drop point for a corpus of real, Word-authored `.docx`
templates: files with the run-fragmentation Word's own editing actually
produces (spell-check splits, language-mark boundaries, revision-save-ID
splits, paste boundaries, accepted-tracked-change residue) rather than the
synthetic shapes a test author would hand-type.

**That corpus does not exist yet.** It is expected to be supplied later, per
`interview.md` decision 6. Until then this directory is empty except for this
file, and `TestDefragCorpusComplete` (in `template/corpus_test.go`) passes
trivially: zero case directories means zero cases to check.

The gate is live from day one anyway, per the same decision. When a real
file lands, drop it in its own subdirectory here alongside an `expect.json`
describing what discovery (and, optionally, a splice) should find. The gate
fails the build the moment either half is missing:

- a subdirectory with a `.docx` fixture but no `expect.json`,
- a subdirectory with an `expect.json` but no `.docx` fixture (or more than
  one),
- an `expect.json` that names zero expected anchors.

## `expect.json` schema

Deliberately minimal — three fields, room to grow once real files reveal what
is actually needed:

```json
{
  "description": "one line: where this fixture came from and what it exercises",
  "anchors": [
    { "name": "customer_name", "kind": "marker", "splice": true },
    { "name": "body", "kind": "native" }
  ]
}
```

- `description` — free text, for a human reading the corpus later.
- `anchors` — every anchor `TestDefragCorpusComplete` requires
  `anchor.Discover` to find in this fixture. `name` and `kind` (`"native"` or
  `"marker"`) are checked against the discovered [`anchor.Anchor`]; `splice`,
  when `true`, additionally requires `template/splice.Splice` to succeed
  against that anchor with a trivial one-paragraph `fragment.Sequence`.

Nothing here validates the fixture's byte layout beyond what `template.Open`
and `anchor.Discover` already require to run at all — this manifest checks
what fill mode is expected to *find and do*, not the file's own bytes.
