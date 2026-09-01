# Determinism — the full pinned-source table

CLAUDE.md carries the compressed statement. This is the complete list of every
known nondeterminism source and where it is pinned. A source that is not on this
list and not pinned is a bug.

The guarantee: **identical spec, assets and theme produce byte-identical output**
for a fixed Vellum version and Go toolchain minor version.

## Why it matters, concretely

Non-determinism does not fail loudly. The first downstream consumer dedupes runs
on `input_hash UNIQUE` and returns the existing artifact instead of re-rendering.
Non-deterministic output silently defeats that dedupe and produces a new
artifact every run — a slow storage leak in someone else's product, with no
error anywhere. That is the failure this whole discipline prevents.

## Zip — `opc/zipdet`

| Source | Pin |
|---|---|
| Entry modification time | `Options.SourceDateEpoch`; the zero value selects `artifact.PinnedEpoch` = 1980-01-01T00:00:00Z. The DOS epoch, because zip cannot represent the Go zero time. |
| Extended-timestamp / NTFS extra field | **Trap:** setting `zip.FileHeader.Modified` makes Go emit an extra field carrying a second copy of the time. `zipdet` leaves `Modified` zero and writes the legacy `ModifiedDate`/`ModifiedTime` fields directly. `TestWrite_NoExtraFields` reads raw local file headers and asserts the extra-field length is zero. |
| Entry order | `opc.CanonicalOrder`: `[Content_Types].xml` first, then `_rels/.rels`, then every other part sorted bytewise by name with each part's own `_rels/*.rels` immediately following its owner. |
| Compression method | A pure function of content type: `Store` for already-compressed media (png, jpeg, gif), `Deflate` otherwise. Never content sniffing, never size-dependent. |
| Compression level | A pinned constant. See "the honest limit" below. |
| Data descriptors | None. Each entry is buffered so sizes and CRC are written up front. |
| Version made by / needed to extract | **Trap:** `archive/zip` stamps both fields inside `CreateHeader`. `zipdet` uses `CreateRaw`, which takes the header verbatim, so both were reaching the byte stream as `0` — not a version any specification defines. `unzip`, Go's own reader and Python's `zipfile` all ignore the field, so the archive tested clean everywhere the build could see it; Word reads it and refuses the package as unreadable. Both are now pinned to `20`, with the high byte of "version made by" left at `0` (MS-DOS/FAT) so the archive does not vary with the writing machine. `TestWrite_VersionFieldsArePinned` checks the raw local headers and the central directory. |
| Zip64 | Threshold is a pure function of size, not a heuristic. |

## OPC — `opc`

| Source | Pin |
|---|---|
| Relationship IDs | `rId<N>` assigned by walking each part's relationships sorted by `(Type, TargetMode, Target)`. Never insertion order, never random. |
| Media part names | `/word/media/image<N>.<ext>` where N is the index of the asset's content hash within the sorted set of distinct hashes. Content-addressed, so identical assets collapse and numbering is stable. |
| `docProps/core.xml` dates | `SourceDateEpoch`. Never `time.Now()`. |
| `w:rsid*`, `w15:docId`, any GUID | Never emitted. Where a format demands an identifier, derive it as a UUIDv5-shaped digest of `(specHash, partName, ordinal)`. |
| `wp:docPr/@id`, `numId`, `abstractNumId`, `p:sldId`, bookmark ids | Deterministic counters over a canonical document walk. |

## Per-format

| Source | Pin |
|---|---|
| xlsx shared string table | First-seen order over a canonical walk: sheets in order, then rows, then columns. `count` and `uniqueCount` follow from it. |
| xlsx `styles.xml` | Fixed preamble with builtin `numFmts`/`fonts`/`fills`/`borders` indices intact. Fill index 0 and 1 are reserved by the spec; getting it wrong makes Excel refuse to open the file. Golden-pinned. |
| pptx overflow | Slide capacity is **theme-derived** (body box height divided by theme row height), never measured. Vellum does not lay out OOXML — PowerPoint does — so a measured capacity would not be reproducible. A theme-derived one is. |
| docx overflow | No-op. DOCX is a flow format and Word paginates. Declared in the capability matrix as a degradation to flow. |
| Map iteration | Banned on ordered-output paths. Enforced by construction (registries return sorted copies; `fragment.Doc.Assets` is a sorted slice, not a map) and by `TestNoUnsortedMapIteration`, an AST gate that flags `for ... := range <map>` without an inline `//det:sorted` justification. |
| Collation | Bytewise `sort.Strings` only. Never `x/text/collate` — locale-dependent ordering is nondeterminism with extra steps. |
| Float formatting | EMU is `int64`; measurements never round-trip through `float64`. Where a format demands a decimal, emit from integer thousandths at fixed precision. |

## PDF

| Source | Pin |
|---|---|
| `/CreationDate`, `/ModDate` | `SourceDateEpoch`, and the *same value* in XMP `xmp:CreateDate` / `xmp:ModifyDate`. PDF/A requires the two to agree; they are generated from one struct so they cannot diverge. Divergence is the most common way a nearly-correct PDF/A file fails veraPDF. |
| `/Producer` | `"Vellum <version>"`. `/Creator` from spec metadata or pinned. |
| `/ID` | `[<H> <H>]`, both elements equal, where H is a canonical hash of `(specHash, themeHash, sorted asset hashes, version, format)`. |
| Object numbering | A fixed emission walk: catalog, pages tree, per page in order, then fonts sorted by subset tag, then images sorted by content hash, then the ICC stream, then metadata. |
| Font subset tag | Base-26 over the first 30 bits of a canonical hash of `(fontHash, sorted glyph IDs)`. Never a counter, never random. |
| Subset font bytes | Our SFNT writer pins `head.created` and `head.modified`, uses a fixed table order and 4-byte padding, and recomputes checksums with `head.checkSumAdjustment` written last. |
| Cross-reference | Classic xref table and trailer. No object streams, no xref streams — both are permitted by PDF/A-2b, and the classic form is byte-simpler, easier to diff, and friendlier to veraPDF triage. |
| Line breaking | Integer 1/1000-em arithmetic throughout. Never accumulate widths in `float64`. |
| Font source | The theme, only. `go-text/typesetting/fontscan` scans system fonts and is firewalled by `TestNoFontscanImport`, a `go list -deps` gate. |

## Fill mode

| Source | Pin |
|---|---|
| FEEL `now()` / `today()` | `pbinitiative/feel` binds both to `time.Now()`. `bind.Validate` walks the parsed AST and rejects them with `VELLUM_BIND_NONDETERMINISTIC_EXPR`, naming the alternative: put `as_of` in the binding data and read it. The banned list is a registry with a completeness gate, not an ad-hoc check. |
| Untouched parts | Copied byte-for-byte. `Result.Touched` is the assertable receipt. |
| Edited parts | `xmlcopy` raw-token copy with surgical subtree replacement. `encoding/xml` never re-marshals a source part — it does not preserve namespace prefixes, attribute order or self-closing form. A CI gate bans the import from the fill subtree. |
| Whitespace | `xml:space="preserve"` honoured; whitespace normalised nowhere on the fill path. |

## The honest limit

Go's `compress/flate` output is stable for a pinned level within a toolchain but
is **not** guaranteed stable across Go minor versions. Rather than vendor a
deflate implementation or pretend otherwise, the guarantee is stated precisely:

> Byte-identical output is guaranteed for a fixed (spec, assets, theme, Vellum
> version, Go toolchain minor version), and is verified in CI against a pinned
> toolchain.

This does not weaken what consumers actually depend on, because **artifact
identity comes from `Spec.Hash()` plus the asset hashes — inputs, not output
bytes**. A dedupe reads the spec hash and never opens the file. Byte-identity is
what makes goldens and attestation work; spec-hash identity is what makes dedupe
work. They are separate guarantees with separate lifetimes, and conflating them
makes the flate question look like a crisis instead of a footnote.

An `Uncompressed` write option exists for callers who need byte-identity across
toolchains as well.

## The test harness

- `TestDeterminism_Repeat` — each golden composed 1000x in one process per format; one distinct SHA-256. Reduced to 25x under `-short`.
- `TestDeterminism_CrossProcess` — re-exec in ten fresh processes and compare. Catches address-dependent and init-order leaks that in-process repetition cannot.
- `TestDeterminism_GOMAXPROCS` — the same goldens at `GOMAXPROCS=1` and `=8`, under `-race`. Surfaces map-iteration leaks.
- `TestDeterminism_EpochInvariance` — two runs at different wall-clock times produce identical bytes; an explicit `SourceDateEpoch` produces a different but stable result.
- `TestNoTimeNow` — grep gate over non-test source, allowlisted only in `provenance` behind the non-deterministic opt-in.

Assertions are on **raw bytes**. XML is normalised for the failure *display*
only, so a failure reads as "three attributes differ in `word/styles.xml`"
rather than as a binary mismatch.
