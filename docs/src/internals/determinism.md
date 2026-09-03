# Determinism

**The guarantee: identical spec, assets and theme produce byte-identical
output**, for a fixed Vellum version and Go toolchain minor version. This is
a hard requirement stated in CLAUDE.md, not an aspiration, and it is the
reason there is no converter subprocess anywhere on the render path —
conversion output varies with renderer version and installed fonts, which
would defeat both golden-file testing and any consumer that dedupes on
content.

This page summarises the guarantee for a human reader. The complete,
machine-checked source-by-source table lives at
[`.claude/reference/determinism.md`](https://github.com/frankbardon/vellum/blob/main/.claude/reference/determinism.md)
in the repository, and is what an engineer changing a byte-layout detail
actually consults and updates — this page will drift into a summary faster
than that table can, by design.

## Why it matters, concretely

Non-determinism does not fail loudly. The realistic failure mode is a
downstream consumer that dedupes renders on `input_hash UNIQUE` and returns
an existing artifact instead of re-rendering one it believes it already has.
Non-deterministic output silently defeats that dedupe: it produces a new
artifact on every run, with no error anywhere, and the failure looks like a
slow storage leak in *someone else's* product rather than like a Vellum bug.
That is the concrete failure every rule below exists to prevent.

## The general rules

- **No `time.Now()` on any output path.** Every timestamp Vellum writes
  comes from `Options.SourceDateEpoch`; unset selects a pinned 1980 epoch.
  `TestNoTimeNow` is an AST gate over non-test source outside the one
  deliberate opt-in (`provenance`, for a caller who explicitly wants a real
  timestamp recorded and accepts the determinism cost of doing so).
- **No random or counter-derived identifiers that depend on code path.**
  Every identifier is derived from content — sorted walks, content hashes,
  canonical orderings — never `math/rand`, never `github.com/google/uuid`
  (permanently ruled out for exactly this reason), never a counter whose
  value depends on the order operations happened to run in.
- **No map iteration on an ordered output path.** Registries return sorted
  copies. Where an API *could* accept a map on an ordered path, it takes an
  ordered slice instead, making the nondeterminism unrepresentable rather
  than merely tested against. `TestNoUnsortedMapIteration` is an AST gate
  over the output-path packages, catching a bare `for range <map>` with no
  documented, deliberate `//det:sorted` justification.
- **Bytewise `sort.Strings` only, never `x/text/collate`** — locale-aware
  ordering is nondeterminism with extra steps, since the same string sorts
  differently under different locales. `TestNoCollateImport` firewalls the
  import.
- **EMU is `int64`**; measurements never round-trip through `float64`.
  Accumulating layout in floating point is exactly how two runs of the same
  document come to differ in the last decimal place.
- **Text is measured once per styled run per line, never per glyph.**
  Converting each glyph to text space and summing rounds dozens of times
  down a single line of prose and the error accumulates; a paragraph
  measured one way and then re-measured the other way would break into
  different lines. Per-run is the boundary the author's own styling puts in
  the text, not an arbitrary compromise.
- **Line breaking is greedy over UAX#14 opportunities.** Not because greedy
  produces the prettiest ragged edge — an optimal-fit shaper would produce
  different breaks — but because a document's pagination must not silently
  change when a future version gets cleverer about line endings.
- **Assertions in the test suite are on raw bytes.** XML is normalised for
  failure *display* only, never for the comparison itself.

## Per-package pins, briefly

The full table names every specific pin; a few of the ones most likely to
surprise a reader coming from a general OOXML or PDF background:

- **Zip**: no extended-timestamp extra field (`zip.FileHeader.Modified`
  stays zero — setting it makes Go emit one), no data descriptors (entries
  are buffered so sizes and CRC are known up front), version fields pinned
  explicitly to `20` in both the local header and central directory (Go's
  `archive/zip` normally sets these inside `CreateHeader`, which the
  deterministic writer bypasses via `CreateRaw`).
- **OPC**: relationship IDs assigned by walking relationships sorted by
  `(Type, TargetMode, Target)`; media part names indexed by the sorted set
  of distinct content hashes, so identical assets collapse to one part and
  numbering stays stable.
- **PDF**: object numbering follows a fixed emission walk (catalog, page
  tree, pages in order, fonts sorted by subset tag, images sorted by
  content hash, ICC stream, metadata); font subset tags are base-26 over a
  hash of `(fontHash, sorted glyph IDs)`, never a counter; every page's
  content stream is built *before* any font object is written, because a
  subset is exactly the glyphs the stream actually drew.
- **Fill mode**: `now()`/`today()` are banned FEEL builtins (see
  [Bindings and FEEL](../fill/bindings.md#banned-builtins-determinism-reaches-into-feel-too));
  untouched parts are copied byte-for-byte; edited parts go through
  `xmlcopy`'s raw-token copy, never `encoding/xml` re-marshalling.

## The honest limit

Go's `compress/flate` output is stable for a pinned compression level
*within* one Go toolchain, but is **not** guaranteed stable across Go minor
versions. Rather than vendor a deflate implementation to work around this,
or pretend the limit does not exist, it is stated precisely:

> Byte-identical output is guaranteed for a fixed (spec, assets, theme,
> Vellum version, Go toolchain minor version), and is verified in CI against
> a pinned toolchain.

This does not weaken what a consumer actually depends on in practice,
because **artifact identity comes from `Spec.Hash()` plus the asset
hashes — inputs, not output bytes.** A dedupe check reads the spec hash and
never opens the rendered file at all. See [Artifact
identity](../spec/identity.md) for why that separation is deliberate.

## The determinism test harness

- **`TestDeterminismRepeat`** — every golden composed 1000× in one process
  per format (25× under `-short`); asserts one distinct SHA-256 across all
  of them.
- **`TestDeterminismCrossProcess`** — re-executes in fresh processes and
  compares digests, catching anything that happened to be stable only
  because it lived in one process's memory (an uninitialized map's
  iteration order, say).
- **`TestDeterminismGOMAXPROCS`** — the same goldens at `GOMAXPROCS=1` and
  `=8` under the race detector.
- **`TestDeterminismEpochInvariance`** — wall-clock time does not affect
  output; an explicit `SourceDateEpoch` produces a different but *stable*
  result.
- **`TestDeterminismOverflowIsPinned`** / **`...IsPinnedInPDF`** — an
  overflowing golden's row/page split is committed as explicit values (which
  row labels which page shows, and which it must not) and read back out of
  the written package — through poppler for PDF, since a PDF's text is
  glyph identifiers in a compressed stream and legible only through the
  font's own embedded ToUnicode mapping. This makes a boundary that silently
  moved by one row visible immediately, rather than hiding inside a
  few-thousand-byte golden rebaseline nobody reads closely.

## The external reader oracles

Every one of the assertions above compares Vellum's own bytes against
Vellum's own bytes. That proves determinism and, on its own, proves nothing
about whether a *real reader* actually opens the file — an artifact can be
byte-identical across a thousand runs and still be one no reader accepts.
Three defects reached a human that way historically: zip version fields left
at zero, ECMA-376 children emitted out of schema order, and a golden "PNG"
carrying no IDAT chunk at all — every Go-side reader in the toolchain
accepted all three; Word accepted none of them.

Three external oracles narrow that gap, sharing plumbing in
`internal/exttool`, and their output is examined for presence and content
and **never compared for byte equality** (a conversion's own bytes vary with
tool version and installed fonts — the exact nondeterminism this whole
discipline exists to exclude):

| Oracle | Question it answers | Where |
|---|---|---|
| poppler | Does the PDF open, and does its text extract? | `internal/pdfvalidate` |
| LibreOffice | Does the OOXML package open, and did its content survive? | `internal/oovalidate` |
| veraPDF | Is the PDF/A-2b claim the file makes about itself actually true? | `internal/pdfvalidate` |

They are evidence, not proof: LibreOffice is not Word, and is more tolerant
on several byte-layout constraints while being less tolerant of some
legitimate constructs; veraPDF implements the clauses its profile covers and
was observed *not* to check XMP/info-dictionary date agreement even though
ISO 19005 requires it, which is why that agreement is enforced structurally
in code (see [PDF and PDF/A](../formats/pdf.md)) rather than left to the
validator alone. `make test` runs poppler; `make test-office` and `make
test-pdfa` run the build-tagged LibreOffice and veraPDF checks, which skip
silently — with a printed warning — when the tool is absent locally, and
which CI turns from a skip into a hard failure via
`VELLUM_REQUIRE_OPTIONAL_GATES`.

Two mechanisms keep all of this off the library's own import graph: build
tags fence LibreOffice and veraPDF out of any build that did not ask for
them, and `TestNoExternalToolingOnTheLibraryPath` asserts no shipped package
reaches any of the three oracle packages. The library itself runs no
subprocess, ever, whether or not any of these tools happen to be installed
on the machine running it.

## The PDF/A preflight

`pdfa.Preflight` runs on every PDF document Vellum writes, unconditionally,
before any byte reaches the destination — see [PDF and
PDF/A](../formats/pdf.md#the-pdfa-preflight). It is the structural
complement to the veraPDF oracle above, not a duplicate of it: the oracle
implements the whole conformance profile and needs a provisioned tool to
run at all; the preflight implements the specific handful of clauses
Vellum's own code decides the answer to (the output intent, the XMP
packet's presence and agreement with the info dictionary, the file
identifier, every embedded font program, `Resources` living on the page and
not the tree) and runs on every machine, with nothing installed, every time.

## See also

- [Artifact identity](../spec/identity.md) — the input-hash guarantee that
  outlives byte-identity's toolchain-minor limit.
- [PDF and PDF/A](../formats/pdf.md) — the format with the most byte-layout
  invariants, and the preflight that checks them.
- [Formats](../formats/capabilities.md) — the byte-layout notes specific to
  DOCX, XLSX and PPTX.
