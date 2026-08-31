# Scope — what is deliberately not in v1, and where it lands

Vellum is subtractive by principle: ship the conformance profile and nothing
beyond it until a second real use case demands more. This file records what was
considered and set aside, so the decisions are not re-litigated and so a
"missing" feature can be recognised as a choice rather than an oversight.

## Never

| Item | Why |
|---|---|
| **Chart rendering** | Vellum embeds an asset it is handed. The host renders, at sizes obtained from `Boxes()`. This is the line that keeps Vellum ignorant of any particular charting library and therefore reusable. |
| **Rasterisation of any kind** | No rasteriser, no network I/O, no cgo. An SVG needing a PNG companion gets one from the caller. |
| **Calling a rendering endpoint** | Protocol coupling only. Siblings are consumed as data, never as package imports or network dependencies. |
| **Reading OOXML into a full object model / round-trip editing** | Fill parses surgically. A full model is precisely how naive libraries silently discard tracked changes, comments, custom XML parts and embedded objects. |
| **Spreadsheet authoring** | Vellum emits presentation tables, not spreadsheets: no formulas, no pivot tables, no macros, no array formulas, no external references. A consumer wanting a live model builds it elsewhere. |
| **A LibreOffice-backed converter** | Conversion output varies with renderer version and installed fonts, so byte-identical output is unachievable and the consumer dedupe that rests on it fails. It would also put an external binary on the render path. |
| **Legacy binary formats** (`.doc`, `.xls`, `.ppt`) | |
| **Automated Word render-fidelity checking** | Not automatable within scope. A documented pre-release manual checklist on Windows is. |
| **Random identifiers** | Every identifier Vellum emits is derived from content. |

## Deferred, designed for

| Item | Why deferred | Where it lands |
|---|---|---|
| **Native DrawingML charts (`chart` block)** | Requires modelling chart types, series and axes — a second charting engine that can drift from the first, coupled to whatever produced the data. A real architectural cost, not just more work. | v2, its own PRD. The block model admits the kind without restructuring and the capability matrix already expresses per-format support. |
| **CFF subsetting** | Charstring subsetting with local and global subroutine renumbering is the hardest correctness problem in the emitter. v1 subsets `glyf` and embeds CFF whole, as a declared degradation. A theme that asks for `embed: "subset"` on a CFF font gets a hard error, because subset-only may be a licence condition and must never be silently ignored. | A later release. Provenance records which subset profile produced a file, and the goldens rebaseline as a deliberate act. |
| **RTL and complex scripts, CJK line breaking, vertical writing** | `typesetting` can shape them; the line breaking and visual reordering are the work. v1 is horizontal LTR Latin, Greek and Cyrillic, with everything else a named rejection rather than a silent mis-render. | A later release. |
| **Hyphenation and Knuth–Plass paragraph optimisation** | Greedy line breaking is deterministic and adequate. Hyphenation would add a locale dictionary — a nondeterminism surface and a licensing surface. | A later release; K–P is a strict improvement whose only cost is a golden rebaseline. |
| **PDF/A-1b and PDF/A-3b** | 2b is the archival target the consumer contract names. 1b and 3b are conformance variants over the same emitter. | v1.1, additive. |
| **Tagged PDF / PDF/UA** | That is conformance level 2a/2u, a different and much larger undertaking. | Demand-driven. |
| **`application/pdf` as an embeddable asset** | Single-page Form XObject import would give crisp vector charts in an archival PDF with no renderer. A badly imported XObject is a PDF/A failure with an unpleasant diagnostic path, so it needs a strict import profile and its own conformance corpus. | First post-v1 item. |
| **xlsx repeat regions above the sheet bottom** | Inserting rows invalidates every absolute reference below. | v1.1. v1 restricts repeat regions to the bottom of a sheet, declared as a matrix row rather than a footnote. |
| **Locale-correct CLDR formatting** | xlsx number-format codes are required for the sheet writer regardless, so reusing them avoids a second formatting dialect and a dependency. | An alternate formatter behind the same interface, if a vertical requires it. |
| **Floating objects, text boxes, columns, drop caps, equations, comment and tracked-change *authoring*** | Outside the `doc` conformance profile. All of it survives a fill untouched — preserved but not authored. | Demand-driven. |
| **Animations, transitions, SmartArt, embedded media** | Outside the `deck` conformance profile. | Demand-driven. |
| **HTTP transport for MCP** | Schema binding rebinds tools per session, which is correct for stdio (one process, one session) and needs the server-factory pattern over HTTP. That is a host's concern. | Demand-driven. |

## Known gaps carried openly

**The defragmentation corpus.** Fill mode's run-defragmentation algorithm is
built against synthetic fixtures because the real Word-authored corpus does not
exist yet. The TRD is explicit that synthetic XML will not reproduce the
fragmentation patterns that matter and will give false confidence, and that is
accepted rather than papered over.

Mitigation: a fragmentation *generator* covering mid-word spell-check splits,
language-mark boundaries, revision-save-ID splits, paste boundaries and
accepted-tracked-change residue, so the algorithm meets combinatorial
fragmentation rather than three hand-written cases; plus a corpus loader whose
directory walk *is* the manifest, so dropping real files into
`testdata/corpus/defrag/` requires no code change. `TestDefragCorpusComplete` is
live from day one and fails on a case directory missing its expectation file,
even while the corpus is empty.

A re-baseline story stays open until the real files land.
