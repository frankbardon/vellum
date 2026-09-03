package errors

// Code identifies an error or warning category. Codes are of the form
// VELLUM_<AREA>_<CATEGORY>, screaming snake case, and are stable public API:
// a consumer may switch on one, and a released code is never renamed or
// repurposed.
//
// The AREA segment names the layer that raised it — OPC, ZIP, SPEC, PDF and so
// on — which is what lets a consumer route a failure without parsing prose.
// [Code.Domain] extracts it.
type Code string

// OPC domain — package structure, parts, relationships and content types.
const (
	// VELLUM_OPC_INVALID indicates the package is not a well-formed OPC
	// container: a missing or malformed [Content_Types].xml, an unreadable
	// root relationships part, or a structure no OOXML consumer would accept.
	VELLUM_OPC_INVALID Code = "VELLUM_OPC_INVALID"

	// VELLUM_OPC_PART_NOT_FOUND indicates a part was addressed by name and the
	// package does not contain it. Raised on lookup and on relationship
	// resolution, where a dangling target means the package is internally
	// inconsistent.
	VELLUM_OPC_PART_NOT_FOUND Code = "VELLUM_OPC_PART_NOT_FOUND"

	// VELLUM_OPC_PART_DUPLICATE indicates two parts claim the same name. OPC
	// part names are case-insensitively unique; a duplicate is either a
	// corrupt package or a writer bug, and is never resolved by preferring
	// one of the two.
	VELLUM_OPC_PART_DUPLICATE Code = "VELLUM_OPC_PART_DUPLICATE"

	// VELLUM_OPC_PART_NAME_INVALID indicates a part name that is not absolute,
	// does not use forward slashes, is empty, ends in a slash, or contains a
	// traversal segment. Path traversal in a part name is an attack against
	// any consumer that extracts a package to disk, so it is refused here
	// rather than sanitised.
	VELLUM_OPC_PART_NAME_INVALID Code = "VELLUM_OPC_PART_NAME_INVALID"

	// VELLUM_OPC_CONTENT_TYPE_MISSING indicates a part carries no content type
	// and none can be derived from its extension. Word and Excel both refuse
	// to open a package with an incomplete [Content_Types].xml, so an
	// undeclared part is an error at write time rather than a silent omission.
	VELLUM_OPC_CONTENT_TYPE_MISSING Code = "VELLUM_OPC_CONTENT_TYPE_MISSING"

	// VELLUM_OPC_RELATIONSHIP_INVALID indicates a relationship with an empty
	// type, an empty target, or an unresolvable internal target.
	VELLUM_OPC_RELATIONSHIP_INVALID Code = "VELLUM_OPC_RELATIONSHIP_INVALID"
)

// ZIP domain — the deterministic zip layer beneath OPC.
const (
	// VELLUM_ZIP_MALFORMED indicates the archive could not be read: a
	// truncated file, a bad central directory, a CRC mismatch, or a local
	// header that disagrees with its central directory entry.
	VELLUM_ZIP_MALFORMED Code = "VELLUM_ZIP_MALFORMED"

	// VELLUM_ZIP_TOO_LARGE indicates an entry whose declared or actual
	// uncompressed size exceeds the configured bound. The bound exists so a
	// decompression bomb in an untrusted template is a coded error rather than
	// an out-of-memory kill, and it is configurable because legitimate decks
	// get large.
	VELLUM_ZIP_TOO_LARGE Code = "VELLUM_ZIP_TOO_LARGE"

	// VELLUM_ZIP_ENTRY_NAME_INVALID indicates an archive entry name that is
	// absolute, contains a traversal segment, or uses a backslash separator.
	VELLUM_ZIP_ENTRY_NAME_INVALID Code = "VELLUM_ZIP_ENTRY_NAME_INVALID"

	// VELLUM_ZIP_ENTRY_DUPLICATE indicates two archive entries share a name.
	VELLUM_ZIP_ENTRY_DUPLICATE Code = "VELLUM_ZIP_ENTRY_DUPLICATE"
)

// SPEC domain — the declarative document specification.
const (
	// VELLUM_SPEC_INVALID indicates the specification is structurally invalid:
	// no sections, a section with no blocks, or a block whose kind does not
	// match the arm it carries.
	VELLUM_SPEC_INVALID Code = "VELLUM_SPEC_INVALID"

	// VELLUM_SPEC_BLOCK_KIND_UNKNOWN indicates a block declares a kind that is
	// not in the vocabulary.
	VELLUM_SPEC_BLOCK_KIND_UNKNOWN Code = "VELLUM_SPEC_BLOCK_KIND_UNKNOWN"
)

// CAPABILITY domain — the declared (feature x format) matrix.
const (
	// VELLUM_CAPABILITY_REJECTED indicates the specification uses a feature
	// the target format declares it will not render. Raised at validate time,
	// before any bytes are written, so a consumer learns about a gap from an
	// error rather than from a reader noticing an absence.
	VELLUM_CAPABILITY_REJECTED Code = "VELLUM_CAPABILITY_REJECTED"

	// VELLUM_CAPABILITY_DEGRADED is a WARNING. It reports that a feature
	// became the stated alternative in this format — notes rendered as a
	// footnote rather than as a speaker note, for instance. The alternative is
	// declared in advance, never discovered at render time.
	VELLUM_CAPABILITY_DEGRADED Code = "VELLUM_CAPABILITY_DEGRADED"

	// VELLUM_CAPABILITY_UNDECLARED indicates a (feature, format) pair with no
	// declared outcome. Always a Vellum bug: the completeness gate exists so
	// this cannot reach a release.
	VELLUM_CAPABILITY_UNDECLARED Code = "VELLUM_CAPABILITY_UNDECLARED"
)

// ASSET domain — asset resolution and media policy.
const (
	// VELLUM_ASSET_MEDIA_UNSUPPORTED indicates an asset whose media type the
	// target format cannot embed.
	//
	// PDF is the case that matters: it has no SVG mechanism at all. Vellum will
	// not rasterise — it never renders — and will not ship an SVG-to-PDF vector
	// translator, which would be a renderer with its own text layout and font
	// matching, free to drift from whatever produced the asset. So the target
	// format and its accepted media types travel with the request, and an
	// unsupported one is a loud error naming what is accepted.
	VELLUM_ASSET_MEDIA_UNSUPPORTED Code = "VELLUM_ASSET_MEDIA_UNSUPPORTED"

	// VELLUM_ASSET_NOT_FOUND indicates the resolver has no asset under the
	// handle a block names. Vellum owns no storage and fetches nothing, so it
	// cannot distinguish "never existed" from "no longer exists" — and does not
	// try. Both are the same coded failure with the handle in its details.
	VELLUM_ASSET_NOT_FOUND Code = "VELLUM_ASSET_NOT_FOUND"

	// VELLUM_ASSET_RESOLVE_FAILED indicates the host's resolver returned an
	// error of its own. The cause is wrapped so a host can unwrap to its own
	// error type; it is never serialised into the envelope, because a host's
	// error prose can carry paths and credentials Vellum has no business
	// publishing.
	VELLUM_ASSET_RESOLVE_FAILED Code = "VELLUM_ASSET_RESOLVE_FAILED"

	// VELLUM_ASSET_MEDIA_UNKNOWN indicates an asset whose media type the
	// resolver did not declare and whose bytes match no known signature.
	//
	// Guessing from a file extension is what this refuses to do: the handle is
	// the host's opaque identifier and may not be a filename at all, so a guess
	// drawn from it would be a guess about the host's naming convention rather
	// than about the content.
	VELLUM_ASSET_MEDIA_UNKNOWN Code = "VELLUM_ASSET_MEDIA_UNKNOWN"

	// VELLUM_ASSET_TOO_LARGE indicates a resolved asset exceeding the
	// configured per-asset bound. See VELLUM_MAX_ASSET_BYTES.
	VELLUM_ASSET_TOO_LARGE Code = "VELLUM_ASSET_TOO_LARGE"

	// VELLUM_ASSET_MEDIA_MISMATCH indicates a declared media type that the
	// asset's own bytes contradict.
	//
	// Trusting the declaration would put bytes into a package under a content
	// type they do not match, and a reader that checks — Word and Excel both do
	// — refuses the whole document rather than the one part. The failure then
	// surfaces as "this file is corrupt", several layers from the mistake.
	//
	// Checked only against the signatures Vellum knows, so it is a
	// contradiction rather than a failure to recognise: bytes that match no
	// signature at all are VELLUM_ASSET_MEDIA_UNKNOWN instead.
	VELLUM_ASSET_MEDIA_MISMATCH Code = "VELLUM_ASSET_MEDIA_MISMATCH"
)

// TABLE domain — the analytical table model.
const (
	// VELLUM_TABLE_INVALID indicates a structurally invalid table: no body, a
	// negative span, or a cell carrying a value kind that does not match the
	// field it populates.
	VELLUM_TABLE_INVALID Code = "VELLUM_TABLE_INVALID"

	// VELLUM_TABLE_HEADER_SPAN_MISMATCH indicates a header node whose declared
	// span does not equal the total span of its children. A banner that does
	// not tile its axis renders as a table with a hole in it, which is worse
	// than a refusal because it looks deliberate.
	VELLUM_TABLE_HEADER_SPAN_MISMATCH Code = "VELLUM_TABLE_HEADER_SPAN_MISMATCH"

	// VELLUM_TABLE_SPAN_OVERLAP indicates two cells claim the same grid
	// position, or a span runs past the edge of the table.
	VELLUM_TABLE_SPAN_OVERLAP Code = "VELLUM_TABLE_SPAN_OVERLAP"

	// VELLUM_TABLE_ROW_ARITY indicates a row whose cells do not cover exactly
	// the width the column headers declare.
	VELLUM_TABLE_ROW_ARITY Code = "VELLUM_TABLE_ROW_ARITY"

	// VELLUM_TABLE_FORMAT_INVALID indicates a cell number-format code that does
	// not parse.
	//
	// It sits in the TABLE domain rather than a domain of its own because a
	// format code is only ever reached through a cell, and a consumer routing
	// this failure is looking at a table. The code vocabulary is xlsx's, used
	// for all four targets so there is no second dialect to learn.
	VELLUM_TABLE_FORMAT_INVALID Code = "VELLUM_TABLE_FORMAT_INVALID"
)

// DOC domain — WordprocessingML.
const (
	// VELLUM_DOC_BLOCK_UNSUPPORTED indicates a block kind the DOCX writer does
	// not yet render. It is a deliberate hard failure rather than a silent
	// omission: dropping content quietly is the failure mode this library
	// exists to prevent, and a consumer must learn about a gap from an error
	// rather than from a reader noticing a missing section.
	VELLUM_DOC_BLOCK_UNSUPPORTED Code = "VELLUM_DOC_BLOCK_UNSUPPORTED"
)

// OVERFLOW domain — content that does not fit the container it was given.
const (
	// VELLUM_OVERFLOW_NO_CAPACITY indicates a container that cannot hold even
	// the minimum the policy requires: a slide whose repeated headers leave no
	// room for a single body row.
	//
	// A hard error rather than a clipped table. Clipping drops rows, and a
	// table missing its last rows looks exactly like a table that had none —
	// the failure mode this library exists to prevent.
	VELLUM_OVERFLOW_NO_CAPACITY Code = "VELLUM_OVERFLOW_NO_CAPACITY"
)

// DECK domain — PresentationML.
const (
	// VELLUM_DECK_INVALID indicates a deck the PresentationML writer cannot
	// serialise: a slide naming a layout the deck does not carry, a layout
	// naming a missing master, a picture indexing past the media, a shape
	// carrying neither content nor a frame.
	//
	// Checked before anything is written rather than discovered part by part,
	// because every one of these produces a package that opens and is silently
	// wrong — a slide with no formatting, a picture that draws nothing — which
	// is the failure mode this library exists to prevent.
	VELLUM_DECK_INVALID Code = "VELLUM_DECK_INVALID"

	// VELLUM_DECK_BLOCK_UNSUPPORTED indicates a block kind the PPTX writer
	// does not yet render. A hard failure rather than a silent omission, for
	// the reason VELLUM_DOC_BLOCK_UNSUPPORTED is.
	VELLUM_DECK_BLOCK_UNSUPPORTED Code = "VELLUM_DECK_BLOCK_UNSUPPORTED"
)

// SHEET domain — SpreadsheetML.
const (
	// VELLUM_SHEET_INVALID indicates a workbook the SpreadsheetML writer
	// cannot serialise: a sheet with no name, two sheets sharing one, a table
	// referencing a cell style the styles part does not carry.
	//
	// Checked before anything is written rather than discovered part by part,
	// because every one of these produces a package that opens and is
	// silently wrong — a sheet Excel renames on load, a cell in the default
	// style instead of the one the theme chose — which is the failure mode
	// this library exists to prevent.
	VELLUM_SHEET_INVALID Code = "VELLUM_SHEET_INVALID"

	// VELLUM_SHEET_BLOCK_UNSUPPORTED indicates a block kind the XLSX writer
	// does not render. A hard failure rather than a silent omission, for the
	// reason VELLUM_DOC_BLOCK_UNSUPPORTED is.
	VELLUM_SHEET_BLOCK_UNSUPPORTED Code = "VELLUM_SHEET_BLOCK_UNSUPPORTED"
)

// INGEST domain — Pulse envelope JSON to spec.Table, at the protocol boundary.
const (
	// VELLUM_INGEST_INVALID indicates JSON that does not decode as the
	// documented matrix-payload shape, or decodes but is structurally
	// inconsistent with itself: a cell grid whose dimensions disagree with its
	// declared row and column keys, an axis header naming no fields, a margin
	// slice whose length does not match its axis.
	//
	// Strict rather than lenient, for the same reason spec decoding is: a
	// consumer authoring an integration against a payload shape that silently
	// tolerated a mismatch would get a table missing rows nobody was told
	// about, which is the failure mode this library exists to prevent.
	VELLUM_INGEST_INVALID Code = "VELLUM_INGEST_INVALID"

	// VELLUM_INGEST_VALUE_UNSUPPORTED indicates a cell whose value is present
	// but is not one of the scalar kinds a [spec.Value] can carry — a rich
	// aggregator payload (a per-label frequency map, a Welford triple) rather
	// than a number, a string or a boolean.
	//
	// Refused rather than flattened to its string form: silently turning a
	// structured result into text is exactly the kind of partial rendering
	// that gives an LLM-authored pipeline no signal that something was
	// dropped. A consumer needing that shape summarises it to a scalar before
	// handing the payload to Vellum.
	VELLUM_INGEST_VALUE_UNSUPPORTED Code = "VELLUM_INGEST_VALUE_UNSUPPORTED"
)

// FONT domain — font resolution and embedding.
const (
	// VELLUM_FONT_SUBSTITUTED is a WARNING, not an error. It reports that a
	// theme font declared embeddable:false was replaced by its declared
	// substitute. Every substitution is surfaced because a silent one is
	// precisely how the same spec comes to render differently on two machines.
	VELLUM_FONT_SUBSTITUTED Code = "VELLUM_FONT_SUBSTITUTED"

	// VELLUM_FONT_NOT_EMBEDDABLE indicates a theme font declared
	// embeddable:false that carries no substitute.
	//
	// This is the case the font policy exists for, and it is a validate-time
	// error rather than a fallback to whatever the machine has installed. A
	// silent system fallback is precisely how one specification comes to render
	// differently on two machines, which defeats byte-identical output and the
	// consumer dedupe resting on it. Whoever authors the theme knows the
	// licence; Vellum cannot, so it declines to guess.
	VELLUM_FONT_NOT_EMBEDDABLE Code = "VELLUM_FONT_NOT_EMBEDDABLE"

	// VELLUM_FONT_EMBED_UNSUPPORTED indicates a theme demanding an embedding
	// mode Vellum cannot deliver for that face — a subset of a CFF-outlined
	// font in v1, where only TrueType outlines are subsetted.
	//
	// A hard error rather than the whole-font degradation, because subset-only
	// may be a licence condition. Silently embedding the whole program would
	// substitute Vellum's judgement for the font vendor's.
	VELLUM_FONT_EMBED_UNSUPPORTED Code = "VELLUM_FONT_EMBED_UNSUPPORTED"

	// VELLUM_FONT_UNAVAILABLE indicates a theme font declared embeddable that
	// names no handle, or whose handle the asset resolver cannot produce. The
	// theme promised a face Vellum was then unable to obtain.
	VELLUM_FONT_UNAVAILABLE Code = "VELLUM_FONT_UNAVAILABLE"
)

// PDF domain — the object layer, the emitter, and the font programs it embeds.
const (
	// VELLUM_PDF_OBJECT_UNRESOLVED indicates a document assembled with a
	// reference that names no object: a number reserved for a forward reference
	// and never filled, or a catalogue that was never set. It is caught before
	// the file is written, because a dangling reference produces a PDF that
	// opens to a blank page in one reader and an error in another.
	VELLUM_PDF_OBJECT_UNRESOLVED Code = "VELLUM_PDF_OBJECT_UNRESOLVED"

	// VELLUM_PDF_STREAM_INVALID indicates a stream that could not be produced —
	// compression failing, or data a filter cannot accept.
	VELLUM_PDF_STREAM_INVALID Code = "VELLUM_PDF_STREAM_INVALID"

	// VELLUM_PDF_WRITE_FAILED indicates the destination writer returned an
	// error. The document itself was well formed; what it was being written to
	// was not available.
	VELLUM_PDF_WRITE_FAILED Code = "VELLUM_PDF_WRITE_FAILED"

	// VELLUM_PDF_FONT_INVALID indicates a font program Vellum could not parse
	// or could not subset: a missing required table, a format it does not
	// implement, or a glyph outline it cannot follow.
	VELLUM_PDF_FONT_INVALID Code = "VELLUM_PDF_FONT_INVALID"

	// VELLUM_PDF_SCRIPT_UNSUPPORTED indicates text in a writing system Vellum
	// does not lay out. The supported set is declared in the capability matrix
	// under text.script rather than discovered: a script laid out incorrectly
	// produces a document that renders and is wrong, which a reader notices and
	// a test does not.
	VELLUM_PDF_SCRIPT_UNSUPPORTED Code = "VELLUM_PDF_SCRIPT_UNSUPPORTED"

	// VELLUM_PDF_TEXT_OVERFLOW indicates a single unbreakable piece of text
	// wider than the space it was given. It is an error rather than a silent
	// overhang, because text running off the edge of a page is a defect nobody
	// sees until the artifact is printed.
	VELLUM_PDF_TEXT_OVERFLOW Code = "VELLUM_PDF_TEXT_OVERFLOW"

	// VELLUM_PDF_GLYPH_MISSING indicates text containing a character the
	// selected face has no glyph for. It is an error rather than a substitution
	// with a notdef box, because a document that silently renders a row of
	// empty rectangles is one nobody notices until it has been sent.
	VELLUM_PDF_GLYPH_MISSING Code = "VELLUM_PDF_GLYPH_MISSING"

	// VELLUM_PDF_IMAGE_INVALID indicates asset bytes that do not parse as the
	// media type they were typed as: a truncated PNG, a chunk length running
	// past the end of the file, a JPEG with no frame header. The asset reached
	// the writer, so it passed the sniffer; this is the deeper read failing.
	VELLUM_PDF_IMAGE_INVALID Code = "VELLUM_PDF_IMAGE_INVALID"

	// VELLUM_PDF_IMAGE_UNSUPPORTED indicates a well-formed image in an encoding
	// form PDF cannot carry without re-encoding it.
	//
	// Distinct from VELLUM_ASSET_MEDIA_UNSUPPORTED, which is about the media
	// type. This is about a variant *within* an accepted type — an interlaced
	// PNG, a progressive or CMYK JPEG — and it is separate because the fix is
	// different: not "supply a different format" but "save this one plainly".
	// Re-encoding it here would mean decoding and recompressing an image, which
	// changes the pixels a consumer supplied without saying so.
	VELLUM_PDF_IMAGE_UNSUPPORTED Code = "VELLUM_PDF_IMAGE_UNSUPPORTED"
)

// PDFA domain — the conformance claim the file makes about itself.
const (
	// VELLUM_PDFA_NONCONFORMANT indicates the assembled document would violate
	// ISO 19005-2 level B, caught by the in-process preflight before any bytes
	// reach the writer.
	//
	// It is an internal invariant rather than a specification fault: nothing a
	// consumer can put in a spec produces a nonconformant document, because the
	// constructs that would — an unembedded font, an unsupported image variant
	// — are refused earlier with their own codes. This fires when Vellum itself
	// assembles something the standard forbids, which makes it a bug report
	// rather than a fixup.
	VELLUM_PDFA_NONCONFORMANT Code = "VELLUM_PDFA_NONCONFORMANT"
)

// THEME domain — the theme document and what a specification asks of it.
const (
	// VELLUM_THEME_NOT_FOUND indicates the provider has no theme under the id
	// the specification names. It is an error rather than a fallback to the
	// built-in theme, because a document rendered against a different theme
	// than the one it asked for is wrong in a way that looks right.
	VELLUM_THEME_NOT_FOUND Code = "VELLUM_THEME_NOT_FOUND"

	// VELLUM_THEME_INVALID indicates a structurally invalid theme document: a
	// missing required role, a layout declaring no page size, a box with a
	// zero width, a duplicate id.
	VELLUM_THEME_INVALID Code = "VELLUM_THEME_INVALID"

	// VELLUM_THEME_LAYOUT_NOT_FOUND indicates a section naming a master layout
	// the theme does not declare for the target format. A theme may legitimately
	// carry a layout for one format and not another, so the format is part of
	// the lookup and part of the error's details.
	VELLUM_THEME_LAYOUT_NOT_FOUND Code = "VELLUM_THEME_LAYOUT_NOT_FOUND"

	// VELLUM_THEME_BOX_NOT_FOUND indicates an asset block naming a box role the
	// resolved layout does not declare.
	//
	// The whole point of the layout query is that the set of boxes is small,
	// enumerable and knowable before a specification exists. A role outside it
	// means the host rendered its asset against a box that is not there, so the
	// size it chose is arbitrary — which is the exact failure Boxes exists to
	// prevent.
	VELLUM_THEME_BOX_NOT_FOUND Code = "VELLUM_THEME_BOX_NOT_FOUND"
)

// MARK domain — consumer-defined style hooks.
const (
	// VELLUM_MARK_UNKNOWN is a WARNING. It reports a mark the theme declares no
	// style for, so the marked content rendered unstyled.
	//
	// A warning rather than an error because marks are the consumer's
	// vocabulary and a theme is not required to style every one of them. A
	// warning rather than silence because the motivating case is a document
	// flagged as stale: rendering that flag invisibly is indistinguishable from
	// rendering a document that was never flagged.
	VELLUM_MARK_UNKNOWN Code = "VELLUM_MARK_UNKNOWN"
)

// TEMPLATE domain — fill mode's token-copy rewriter over a source OOXML part.
const (
	// VELLUM_TEMPLATE_XML_MALFORMED indicates a source part that does not
	// parse as well-formed XML: a mismatched close tag, a truncated document,
	// an unterminated attribute. Unlike a bad replacement span, this one can
	// genuinely originate in an untrusted template rather than in Vellum's own
	// code, so it carries a real fixup instead of being an internal invariant.
	VELLUM_TEMPLATE_XML_MALFORMED Code = "VELLUM_TEMPLATE_XML_MALFORMED"

	// VELLUM_TEMPLATE_XML_SPAN_INVALID indicates a replacement span
	// xmlcopy.Apply was handed that overlaps another, arrives out of the
	// ascending order Apply requires, or falls outside the bounds of the
	// source it is being applied to. Nothing in an untrusted template's own
	// bytes produces this — only a bug in the caller assembling replacements
	// — so it is an internal-invariant class of error.
	VELLUM_TEMPLATE_XML_SPAN_INVALID Code = "VELLUM_TEMPLATE_XML_SPAN_INVALID"

	// VELLUM_TEMPLATE_INVALID indicates a package [template.Open] cannot treat
	// as a fillable OOXML template: no package-relationship of type
	// officeDocument, one whose target the package does not contain, or one
	// whose target's declared content type is not a recognised DOCX, XLSX or
	// PPTX main part. PDF is not an OPC package at all and so never reaches
	// this check with real PDF bytes; a package that merely does not resolve
	// to one of the three OOXML main content types — including one built to
	// look PDF-shaped — is refused the same way, because fill mode edits an
	// OPC package surgically and a format outside that set is not one.
	VELLUM_TEMPLATE_INVALID Code = "VELLUM_TEMPLATE_INVALID"

	// VELLUM_TEMPLATE_FORMAT_UNSUPPORTED indicates [anchor.Discover] was asked
	// to inspect a template format with no discoverer wired yet — XLSX and
	// PPTX, whose anchor kinds (ListObject ranges, defined names, shape names)
	// are E11's job. Returning an empty inventory instead would be
	// indistinguishable from "this template genuinely has no anchors", which
	// is a different fact, so the gap is a loud, coded refusal rather than a
	// silent one.
	VELLUM_TEMPLATE_FORMAT_UNSUPPORTED Code = "VELLUM_TEMPLATE_FORMAT_UNSUPPORTED"

	// VELLUM_TEMPLATE_SDT_CONTENT_MISSING indicates a native anchor's w:sdt
	// element carries no w:sdtContent child. CT_SdtBlock declares sdtContent
	// optional, so a genuinely empty content control — one Word lets an
	// author create and never fill in — reaches splice with nothing to
	// replace, and there is no honest span to substitute into.
	VELLUM_TEMPLATE_SDT_CONTENT_MISSING Code = "VELLUM_TEMPLATE_SDT_CONTENT_MISSING"

	// VELLUM_TEMPLATE_BLOCK_UNSUPPORTED indicates a fragment.Block whose kind
	// template/splice does not render into a content control: none of
	// Paragraph, Table or Asset is set (a page break, notes or spacer block,
	// or a future kind splice has not been taught yet). Unlike
	// VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED this is not a permanent scope
	// boundary — a native content control is block-level and could in
	// principle carry one of these kinds — it is simply not implemented yet.
	VELLUM_TEMPLATE_BLOCK_UNSUPPORTED Code = "VELLUM_TEMPLATE_BLOCK_UNSUPPORTED"

	// VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED indicates a fragment.Sequence
	// handed to a marker anchor's splice that is not exactly one Paragraph
	// block: zero blocks, more than one, or a Table or Asset block. This is a
	// permanent scope boundary, not a gap: a {{marker}} sits inline inside a
	// run position in the middle of a paragraph, and block-level content — a
	// table, an image, several paragraphs — cannot be inserted there without
	// breaking well-formedness (a w:tbl cannot appear inside a w:r). Content
	// needing that shape belongs behind a native content control instead.
	VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED Code = "VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED"

	// VELLUM_TEMPLATE_ASSET_MEDIA_UNSUPPORTED indicates an asset block
	// splice was asked to embed whose media type is neither image/png nor
	// image/jpeg — the same accepted set the DOCX capability matrix already
	// declares, enforced again here because fill mode has no separate
	// validate-time capability check in front of a splice.
	VELLUM_TEMPLATE_ASSET_MEDIA_UNSUPPORTED Code = "VELLUM_TEMPLATE_ASSET_MEDIA_UNSUPPORTED"

	// VELLUM_TEMPLATE_REPEAT_CONTAINER_INVALID indicates a repeat
	// statement's body cannot be reconciled to the single structural
	// container its declared Target names: no anchor anywhere in the body
	// names a splice site at all, the referenced anchors sit in more than
	// one part, or no single <w:tr> (Target "row") or <w:sdt> (Target
	// "block") in the template contains every one of them. A repeat splices
	// N independent copies of exactly one region the template carries once;
	// a binding whose declared shape does not match where its own anchors
	// actually sit in the document has no such region to copy.
	VELLUM_TEMPLATE_REPEAT_CONTAINER_INVALID Code = "VELLUM_TEMPLATE_REPEAT_CONTAINER_INVALID"

	// VELLUM_TEMPLATE_TABLE_NOT_AT_SHEET_BOTTOM indicates a "table_row" repeat
	// targets an Excel Table whose sample data row is not the last content in
	// the worksheet: [xmlcopy.Walk] found a <row> with a higher r than the
	// table's own data row. Row-insert repetition splices N rows in where the
	// template carried one, which shifts every absolute cell reference below
	// the insertion point — a formula, a second table, a chart source range —
	// silently out from under whatever it pointed at. Checked before any
	// splicing happens, not after, so a caller never receives output that
	// looks correct and corrupts references invisibly.
	VELLUM_TEMPLATE_TABLE_NOT_AT_SHEET_BOTTOM Code = "VELLUM_TEMPLATE_TABLE_NOT_AT_SHEET_BOTTOM"

	// VELLUM_TEMPLATE_SHAPE_TXBODY_MISSING indicates a shape anchor's own
	// <p:sp> element carries no <p:txBody> child. CT_Shape declares txBody
	// optional — a purely decorative auto-shape with no text ever typed into
	// it is a real, if unusual, case — so a shape anchor discovered by its
	// own name can still reach splice with nothing to place text into, and
	// there is no honest span to substitute a new one at.
	VELLUM_TEMPLATE_SHAPE_TXBODY_MISSING Code = "VELLUM_TEMPLATE_SHAPE_TXBODY_MISSING"

	// VELLUM_TEMPLATE_SHAPE_BLOCK_UNSUPPORTED indicates a fragment.Sequence
	// handed to a pptx shape anchor's splice carries a block that is not a
	// Paragraph: a Table or an Asset block. This is a permanent scope
	// boundary, the shape-text counterpart of
	// VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED: a <p:txBody> is text-only by
	// the DrawingML schema itself — a table needs its own sibling
	// <p:graphicFrame> in the slide's shape tree and a picture its own
	// sibling <p:pic>, neither of which fits inside one shape's own text
	// body. Content needing that shape belongs behind its own shape in the
	// template, not spliced into another shape's text.
	VELLUM_TEMPLATE_SHAPE_BLOCK_UNSUPPORTED Code = "VELLUM_TEMPLATE_SHAPE_BLOCK_UNSUPPORTED"

	// VELLUM_TEMPLATE_SLIDE_REPEAT_EMPTIES_DECK indicates a "slide" repeat
	// evaluated to zero items and the slide it targets is the presentation's
	// only slide. A zero-item repeat legitimately removes the template's own
	// un-repeated slide the same way it removes a table row or a table_row's
	// worth of worksheet rows, but a deck left with zero total slides is not
	// a file PowerPoint opens — the same "would produce a file the target
	// reader refuses" reasoning as VELLUM_TEMPLATE_TABLE_NOT_AT_SHEET_BOTTOM,
	// checked before any part is written rather than after.
	VELLUM_TEMPLATE_SLIDE_REPEAT_EMPTIES_DECK Code = "VELLUM_TEMPLATE_SLIDE_REPEAT_EMPTIES_DECK"

	// VELLUM_TEMPLATE_SLIDE_ID_RANGE_EXCEEDED indicates a "slide" repeat's
	// own deterministic sldId numbering (one past the highest sldId already
	// in the template, incrementing per clone) would reach or exceed
	// 2147483648 — the identifier PresentationML reserves as the first
	// sldMasterId/sldLayoutId. The two identifier spaces are disjoint by
	// contract (see CLAUDE.md's own byte-layout invariant on the point), so
	// a slide repeat is refused rather than producing a deck whose slide and
	// master identifier spaces collide.
	VELLUM_TEMPLATE_SLIDE_ID_RANGE_EXCEEDED Code = "VELLUM_TEMPLATE_SLIDE_ID_RANGE_EXCEEDED"
)

// ANCHOR domain — fill-mode anchor discovery: locating the native content
// controls and marker text a binding will later splice data into.
const (
	// VELLUM_ANCHOR_DUPLICATE indicates two anchors in the same part share one
	// name: two w:sdt content controls carrying the same w:tag, a {{marker}}
	// repeating the name of an earlier marker, or a marker colliding with a
	// content control's tag. A binding has no way to choose between two
	// anchors claiming the same name, so this is rejected at discovery time
	// rather than deferred to whichever binding story trips over it first.
	// Repeat semantics — the same anchor legitimately appearing once per
	// repeated row or slide — is an E10/E11 concept that does not exist yet
	// and this rule will need revisiting once it does.
	VELLUM_ANCHOR_DUPLICATE Code = "VELLUM_ANCHOR_DUPLICATE"

	// VELLUM_ANCHOR_MARKER_MALFORMED indicates {{ }} marker syntax in the
	// document text that does not close before the paragraph ends, or that
	// closes around an empty or all-whitespace name. Vellum has no lenient
	// mode: a malformed marker is refused rather than silently skipped, so an
	// author who mistyped a marker learns about it rather than getting a
	// document with a gap where the data was supposed to go.
	VELLUM_ANCHOR_MARKER_MALFORMED Code = "VELLUM_ANCHOR_MARKER_MALFORMED"

	// VELLUM_ANCHOR_DEFINED_NAME_UNSUPPORTED indicates an xlsx defined name
	// discovery cannot turn into a [KindDefinedName] anchor: its formula is not
	// a single-sheet, absolute, single-cell reference (a relative reference, a
	// multi-cell range, a reference to another defined name, or anything a
	// formula parser would be needed to resolve), or its formula does resolve
	// to that shape but names a cell the worksheet's own XML does not carry —
	// fill mode edits an existing <c> element and has no honest way to
	// introduce one that was never there. Raised at discovery time, before any
	// binding data exists, because whether a defined name is usable at all is
	// a fact about the template alone.
	VELLUM_ANCHOR_DEFINED_NAME_UNSUPPORTED Code = "VELLUM_ANCHOR_DEFINED_NAME_UNSUPPORTED"

	// VELLUM_ANCHOR_TABLE_UNSUPPORTED indicates an xlsx Excel Table
	// (ListObject) discovery cannot turn into [KindTableColumn] anchors: its
	// data region — the rows of its own ref below the header — does not carry
	// exactly one existing row (zero means there is no sample row to use as
	// the row-insert repeat's template; more than one means Vellum cannot tell
	// which is the sample), or a declared table column has no corresponding
	// placeholder <c> cell in that one data row to splice a value into.
	// Raised at discovery time for the same reason
	// VELLUM_ANCHOR_DEFINED_NAME_UNSUPPORTED is: usability is a fact about the
	// template, checkable before any binding runs.
	VELLUM_ANCHOR_TABLE_UNSUPPORTED Code = "VELLUM_ANCHOR_TABLE_UNSUPPORTED"
)

// BIND domain — the fill-mode binding specification: the declarative,
// hashable document mapping a template's anchors onto FEEL expressions,
// with a thin control layer of repeat, if and with statements.
const (
	// VELLUM_BIND_INVALID indicates a binding that is structurally invalid:
	// malformed JSON or YAML, an unknown field, no top-level statements, a
	// statement whose kind and carried arm disagree, or a statement missing
	// one of its own required fields (a bind with no anchor or expression, a
	// repeat with no over or loop-variable name, an if with no when
	// expression, a with with no scope-variable name or value expression).
	// It does not cover the content of any FEEL expression, which this
	// package never parses.
	VELLUM_BIND_INVALID Code = "VELLUM_BIND_INVALID"

	// VELLUM_BIND_STATEMENT_KIND_UNKNOWN indicates a statement declares a
	// kind that is not in the vocabulary (bind, repeat, if, with).
	VELLUM_BIND_STATEMENT_KIND_UNKNOWN Code = "VELLUM_BIND_STATEMENT_KIND_UNKNOWN"

	// VELLUM_BIND_REPEAT_TARGET_UNKNOWN indicates a repeat statement whose
	// target is not one of "row", "block" or "table_row". The zero value is
	// not defaulted: a template format realizes repetition several
	// structurally different ways — splicing rows into a DOCX table,
	// splicing copies of a DOCX native content control's content, inserting
	// rows into an xlsx Excel Table — and guessing between them from the
	// document rather than the binding is exactly what this field exists to
	// avoid.
	VELLUM_BIND_REPEAT_TARGET_UNKNOWN Code = "VELLUM_BIND_REPEAT_TARGET_UNKNOWN"

	// VELLUM_BIND_EXPR_MALFORMED indicates a FEEL expression string — a
	// Bind.Expr, a Repeat.Over, an If.When, a With.Value or a Skip — that
	// does not parse as FEEL. Raised by [Validate] before any binding data
	// exists, and again by the default [Evaluator] if an unvalidated
	// binding reaches evaluation, so a caller who skips Validate still gets
	// a coded error rather than an opaque one from pbinitiative/feel.
	VELLUM_BIND_EXPR_MALFORMED Code = "VELLUM_BIND_EXPR_MALFORMED"

	// VELLUM_BIND_NONDETERMINISTIC_EXPR indicates a FEEL expression calls a
	// builtin from the banned registry — see [AllBannedBuiltins] — whose
	// result depends on wall-clock time. Raised by [Validate], which parses
	// every expression and walks its AST rather than evaluating it, because
	// evaluating an expression to decide whether it is safe to evaluate
	// would call the very builtin it exists to forbid.
	VELLUM_BIND_NONDETERMINISTIC_EXPR Code = "VELLUM_BIND_NONDETERMINISTIC_EXPR"

	// VELLUM_BIND_EVAL_FAILED indicates a FEEL expression parsed
	// successfully but failed while being evaluated against a scope: an
	// undefined dotted path, a wrong argument count, a type a builtin
	// rejects, or a panic recovered from pbinitiative/feel itself (its
	// arbitrary-precision arithmetic panics rather than erroring on a
	// division by zero, and Vellum recovers that panic at the evaluator
	// boundary rather than letting it escape into a caller's fill). Distinct
	// from VELLUM_BIND_EXPR_MALFORMED, which is a syntax fault caught before
	// any data is involved.
	VELLUM_BIND_EVAL_FAILED Code = "VELLUM_BIND_EVAL_FAILED"

	// VELLUM_BIND_VALUE_NOT_SCALAR indicates a Bind.Expr evaluated to a FEEL
	// list or context (map) where a single value was required — a bind
	// statement fills exactly one anchor with exactly one value, and a
	// caller wanting several values out of one expression is describing a
	// Repeat, not a Bind.
	VELLUM_BIND_VALUE_NOT_SCALAR Code = "VELLUM_BIND_VALUE_NOT_SCALAR"

	// VELLUM_BIND_VALUE_NOT_LIST indicates a Repeat.Over evaluated to
	// something other than a FEEL list — a scalar or a context has no
	// well-defined number of copies to produce.
	VELLUM_BIND_VALUE_NOT_LIST Code = "VELLUM_BIND_VALUE_NOT_LIST"

	// VELLUM_BIND_VALUE_UNSUPPORTED_TYPE indicates a scalar FEEL result
	// whose type has no corresponding [numfmt.Value] variant — a duration,
	// a function value, or a range. numfmt.Value's vocabulary is number,
	// text, bool and date; a duration in particular has no honest date-like
	// reading and must be turned into text inside the FEEL expression itself
	// (for example with FEEL's own string() builtin) before it can fill an
	// anchor.
	VELLUM_BIND_VALUE_UNSUPPORTED_TYPE Code = "VELLUM_BIND_VALUE_UNSUPPORTED_TYPE"

	// VELLUM_BIND_ANCHOR_UNKNOWN indicates a binding statement — a Bind's own
	// Anchor, or an anchor name a Repeat's body reaches while reconciling its
	// anchors to one splice container — names an anchor the template's
	// discovered [anchor.Inventory] does not contain. Raised at execution
	// time (there is no anchor list at binding-authoring time to check
	// against), and distinct from VELLUM_ANCHOR_DUPLICATE and
	// VELLUM_ANCHOR_MARKER_MALFORMED, which are about the template's own
	// anchors being ill-formed rather than about a binding naming one that
	// is not there at all.
	VELLUM_BIND_ANCHOR_UNKNOWN Code = "VELLUM_BIND_ANCHOR_UNKNOWN"

	// VELLUM_BIND_ANCHOR_UNRECONCILED indicates a pre-flight mismatch
	// between a template's discovered anchor.Inventory and a binding's
	// statement tree, found by [bind.Reconcile] before [bind.Execute] runs
	// anything — FR-F6's "error on anchors present in the template but
	// absent from the binding, and the reverse, unless explicitly marked
	// optional". Unlike VELLUM_BIND_ANCHOR_UNKNOWN, which is raised by
	// execution the first time it trips over one mismatch, this is raised
	// once, structurally, and its details carry every mismatch found in one
	// pass rather than only the first: a "problems" list, each entry naming
	// the anchor and which direction it failed in — the binding references
	// an anchor the template does not have and the reference is not marked
	// [bind.Bind.Optional], or the template has an anchor the binding's
	// statement tree never references and the anchor's name is not listed
	// in [bind.Binding.OptionalAnchors].
	VELLUM_BIND_ANCHOR_UNRECONCILED Code = "VELLUM_BIND_ANCHOR_UNRECONCILED"
)

// DEFRAG domain — fill-mode run defragmentation: flattening a container's
// w:r runs into matchable text and computing the resplice site around a
// match.
const (
	// VELLUM_DEFRAG_CONTAINER_NOT_FOUND indicates [Flatten] was given a
	// container span that does not match the span of any element found while
	// walking the source. A caller assembles the span from a Walk over the
	// same bytes — typically an anchor.Anchor.Span from anchor.Discover
	// re-walking the same part — so nothing an untrusted template's own bytes
	// can do produces this; only a caller passing a span from a different
	// source, or a stale one, does.
	VELLUM_DEFRAG_CONTAINER_NOT_FOUND Code = "VELLUM_DEFRAG_CONTAINER_NOT_FOUND"

	// VELLUM_DEFRAG_RANGE_INVALID indicates a [Flat.Locate] call whose
	// matchStart or matchEnd falls outside the flattened text's own rune
	// bounds, or where matchStart is greater than matchEnd. A caller derives
	// both from the same Flat.Text it is calling Locate on, so this is always
	// a bug in the caller assembling the match, never something a template
	// author's own document can trigger.
	VELLUM_DEFRAG_RANGE_INVALID Code = "VELLUM_DEFRAG_RANGE_INVALID"
)

// ARTIFACT domain — the public facade's dispatch between a resolved format
// model and the writer that emits it.
const (
	// VELLUM_ARTIFACT_MODEL_UNSUPPORTED indicates the public facade's Write
	// method was given a value that is not one of the four resolved format
	// models a Vellum writer accepts: *doc.Document, *sheet.Workbook,
	// *deck.Deck or *pdf.Document. Always a bug in the caller assembling the
	// value — never something a specification's own content can trigger,
	// since this path never runs resolve.Resolve at all. A caller composing
	// from spec.Spec blocks wants Compose, not Write.
	VELLUM_ARTIFACT_MODEL_UNSUPPORTED Code = "VELLUM_ARTIFACT_MODEL_UNSUPPORTED"
)

// PROVENANCE domain — reading an artifact's own embedded provenance record
// back out, the inverse of the OOXML custom-properties and PDF XMP embedding
// [provenance.Record] writes.
const (
	// VELLUM_PROVENANCE_MALFORMED indicates an artifact carries something that
	// is recognisably an attempt at an embedded provenance record — a
	// docProps/custom.xml property under the Vellum prefix, or a vellum:*
	// property inside an XMP packet — that does not parse as the shape
	// [provenance.Extract] or [provenance.ExtractPDF] expects: a malformed
	// timestamp, a vellum:record property whose value is not the JSON
	// [provenance.Record.Hash] digests, or custom-properties XML that does
	// not parse at all. Distinct from an artifact carrying no provenance at
	// all, which both functions report honestly as an absent record rather
	// than as an error — a file most callers hand them, since automatic
	// embedding during Compose is not wired in yet.
	VELLUM_PROVENANCE_MALFORMED Code = "VELLUM_PROVENANCE_MALFORMED"
)

// CLI domain — the command-line shell over the library facade. Every failure
// here is about how the CLI itself was invoked, not about the content the
// facade was asked to act on; the facade's own coded errors (VELLUM_SPEC_*,
// VELLUM_TEMPLATE_*, and so on) pass through the CLI unchanged rather than
// being wrapped in one of these.
const (
	// VELLUM_CLI_USAGE indicates a flag or argument combination the CLI
	// cannot act on: an unrecognised --format value, a required flag or
	// positional argument left unset, or two flags whose combination is
	// meaningless together. Raised before the facade is ever called.
	VELLUM_CLI_USAGE Code = "VELLUM_CLI_USAGE"

	// VELLUM_CLI_INPUT_NOT_FOUND indicates a file path a flag or positional
	// argument named could not be opened — it does not exist, or the process
	// lacks permission to read it. Distinct from VELLUM_CLI_USAGE because the
	// flag itself was well-formed; what it pointed at was not there.
	VELLUM_CLI_INPUT_NOT_FOUND Code = "VELLUM_CLI_INPUT_NOT_FOUND"

	// VELLUM_CLI_OUTPUT_CONFLICT indicates a command was asked to write both
	// a binary artifact and a --json envelope to the same stdout stream with
	// no -o/--output file named. The two cannot share one stream: a reader
	// expecting one JSON document would receive JSON with an artifact's raw
	// bytes spliced into the middle of it. Raised before either is written,
	// rather than corrupting stdout and reporting nothing.
	VELLUM_CLI_OUTPUT_CONFLICT Code = "VELLUM_CLI_OUTPUT_CONFLICT"

	// VELLUM_CLI_NOT_IMPLEMENTED indicates a verb the CLI registers — so it
	// appears in --help and in shell completion — but does not yet run: mcp
	// and doctor, which land with E12-S2 and E12-S4. A stub registration
	// rather than an absent command, so those stories extend a working
	// framework instead of each wiring CLI plumbing from scratch.
	VELLUM_CLI_NOT_IMPLEMENTED Code = "VELLUM_CLI_NOT_IMPLEMENTED"
)

// MCP domain — the Model Context Protocol surface: typed tool contracts over
// the same facade the CLI wraps, translated to and from JSON at one seam.
// Every facade failure a tool's handler surfaces (VELLUM_SPEC_*,
// VELLUM_TEMPLATE_*, and so on) passes through unchanged rather than being
// wrapped in one of these — these three are specifically about the MCP
// transport's own contract: whether a call named a real tool and supplied
// arguments that tool's schema accepts.
const (
	// VELLUM_MCP_INVALID_INPUT indicates a tool call's arguments do not
	// decode against its declared input contract, or name a value — an
	// unrecognised --format, for instance — the contract's own semantic
	// checks reject. Raised before the facade is ever called, the MCP
	// counterpart to VELLUM_CLI_USAGE.
	VELLUM_MCP_INVALID_INPUT Code = "VELLUM_MCP_INVALID_INPUT"

	// VELLUM_MCP_UNKNOWN_TOOL indicates a call named a tool the catalog does
	// not register.
	VELLUM_MCP_UNKNOWN_TOOL Code = "VELLUM_MCP_UNKNOWN_TOOL"

	// VELLUM_MCP_NOT_IMPLEMENTED indicates a tool the catalog registers — so
	// a client discovers it via tools/list — but whose content is not yet
	// available: skills and examples serve go:embed packs E13 builds, not
	// this story. A stub registration rather than an absent tool, the same
	// discipline VELLUM_CLI_NOT_IMPLEMENTED already established for the
	// CLI's own mcp and doctor stubs.
	VELLUM_MCP_NOT_IMPLEMENTED Code = "VELLUM_MCP_NOT_IMPLEMENTED"
)

// INTERNAL domain — invariants that no author input can violate.
const (
	// VELLUM_INTERNAL_INVARIANT indicates a condition Vellum believed
	// impossible. It is always a bug in Vellum and never something a caller
	// can fix by changing their input, which is why it is the canonical
	// FixupNotApplicable case.
	VELLUM_INTERNAL_INVARIANT Code = "VELLUM_INTERNAL_INVARIANT"
)

// allCodes lists every defined code, in domain order. It is hand-maintained:
// a code that is declared but not listed here is invisible to AllCodes, to the
// manifest, and to the metadata gate, so the gate that catches the omission is
// TestCodesHaveFixups reading this slice.
var allCodes = []Code{
	// OPC
	VELLUM_OPC_INVALID,
	VELLUM_OPC_PART_NOT_FOUND,
	VELLUM_OPC_PART_DUPLICATE,
	VELLUM_OPC_PART_NAME_INVALID,
	VELLUM_OPC_CONTENT_TYPE_MISSING,
	VELLUM_OPC_RELATIONSHIP_INVALID,

	// SPEC
	VELLUM_SPEC_INVALID,
	VELLUM_SPEC_BLOCK_KIND_UNKNOWN,

	// CAPABILITY
	VELLUM_CAPABILITY_REJECTED,
	VELLUM_CAPABILITY_DEGRADED,
	VELLUM_CAPABILITY_UNDECLARED,

	// ASSET
	VELLUM_ASSET_MEDIA_UNSUPPORTED,
	VELLUM_ASSET_NOT_FOUND,
	VELLUM_ASSET_RESOLVE_FAILED,
	VELLUM_ASSET_MEDIA_UNKNOWN,
	VELLUM_ASSET_TOO_LARGE,
	VELLUM_ASSET_MEDIA_MISMATCH,

	// THEME
	VELLUM_THEME_NOT_FOUND,
	VELLUM_THEME_INVALID,
	VELLUM_THEME_LAYOUT_NOT_FOUND,
	VELLUM_THEME_BOX_NOT_FOUND,

	// MARK
	VELLUM_MARK_UNKNOWN,

	// TABLE
	VELLUM_TABLE_INVALID,
	VELLUM_TABLE_HEADER_SPAN_MISMATCH,
	VELLUM_TABLE_SPAN_OVERLAP,
	VELLUM_TABLE_ROW_ARITY,
	VELLUM_TABLE_FORMAT_INVALID,

	// DOC
	VELLUM_DOC_BLOCK_UNSUPPORTED,

	// OVERFLOW
	VELLUM_OVERFLOW_NO_CAPACITY,

	// DECK
	VELLUM_DECK_INVALID,
	VELLUM_DECK_BLOCK_UNSUPPORTED,

	// SHEET
	VELLUM_SHEET_INVALID,
	VELLUM_SHEET_BLOCK_UNSUPPORTED,

	// INGEST
	VELLUM_INGEST_INVALID,
	VELLUM_INGEST_VALUE_UNSUPPORTED,

	// ZIP
	VELLUM_ZIP_MALFORMED,
	VELLUM_ZIP_TOO_LARGE,
	VELLUM_ZIP_ENTRY_NAME_INVALID,
	VELLUM_ZIP_ENTRY_DUPLICATE,

	// FONT
	VELLUM_FONT_SUBSTITUTED,
	VELLUM_FONT_NOT_EMBEDDABLE,
	VELLUM_FONT_EMBED_UNSUPPORTED,
	VELLUM_FONT_UNAVAILABLE,

	// PDF
	VELLUM_PDF_OBJECT_UNRESOLVED,
	VELLUM_PDF_STREAM_INVALID,
	VELLUM_PDF_WRITE_FAILED,
	VELLUM_PDF_FONT_INVALID,
	VELLUM_PDF_GLYPH_MISSING,
	VELLUM_PDF_SCRIPT_UNSUPPORTED,
	VELLUM_PDF_TEXT_OVERFLOW,
	VELLUM_PDF_IMAGE_INVALID,
	VELLUM_PDF_IMAGE_UNSUPPORTED,

	// PDFA
	VELLUM_PDFA_NONCONFORMANT,

	// TEMPLATE
	VELLUM_TEMPLATE_XML_MALFORMED,
	VELLUM_TEMPLATE_XML_SPAN_INVALID,
	VELLUM_TEMPLATE_INVALID,
	VELLUM_TEMPLATE_FORMAT_UNSUPPORTED,
	VELLUM_TEMPLATE_SDT_CONTENT_MISSING,
	VELLUM_TEMPLATE_BLOCK_UNSUPPORTED,
	VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED,
	VELLUM_TEMPLATE_ASSET_MEDIA_UNSUPPORTED,
	VELLUM_TEMPLATE_REPEAT_CONTAINER_INVALID,
	VELLUM_TEMPLATE_TABLE_NOT_AT_SHEET_BOTTOM,
	VELLUM_TEMPLATE_SHAPE_TXBODY_MISSING,
	VELLUM_TEMPLATE_SHAPE_BLOCK_UNSUPPORTED,
	VELLUM_TEMPLATE_SLIDE_REPEAT_EMPTIES_DECK,
	VELLUM_TEMPLATE_SLIDE_ID_RANGE_EXCEEDED,

	// ANCHOR
	VELLUM_ANCHOR_DUPLICATE,
	VELLUM_ANCHOR_MARKER_MALFORMED,
	VELLUM_ANCHOR_DEFINED_NAME_UNSUPPORTED,
	VELLUM_ANCHOR_TABLE_UNSUPPORTED,

	// DEFRAG
	VELLUM_DEFRAG_CONTAINER_NOT_FOUND,
	VELLUM_DEFRAG_RANGE_INVALID,

	// BIND
	VELLUM_BIND_INVALID,
	VELLUM_BIND_STATEMENT_KIND_UNKNOWN,
	VELLUM_BIND_REPEAT_TARGET_UNKNOWN,
	VELLUM_BIND_EXPR_MALFORMED,
	VELLUM_BIND_NONDETERMINISTIC_EXPR,
	VELLUM_BIND_EVAL_FAILED,
	VELLUM_BIND_VALUE_NOT_SCALAR,
	VELLUM_BIND_VALUE_NOT_LIST,
	VELLUM_BIND_VALUE_UNSUPPORTED_TYPE,
	VELLUM_BIND_ANCHOR_UNKNOWN,
	VELLUM_BIND_ANCHOR_UNRECONCILED,

	// ARTIFACT
	VELLUM_ARTIFACT_MODEL_UNSUPPORTED,

	// PROVENANCE
	VELLUM_PROVENANCE_MALFORMED,

	// CLI
	VELLUM_CLI_USAGE,
	VELLUM_CLI_INPUT_NOT_FOUND,
	VELLUM_CLI_OUTPUT_CONFLICT,
	VELLUM_CLI_NOT_IMPLEMENTED,

	// MCP
	VELLUM_MCP_INVALID_INPUT,
	VELLUM_MCP_UNKNOWN_TOOL,
	VELLUM_MCP_NOT_IMPLEMENTED,

	// INTERNAL
	VELLUM_INTERNAL_INVARIANT,
}

// codeIndex is the string-to-Code lookup table, built once at init so
// ParseCode is a map hit rather than a scan.
var codeIndex map[string]Code

func init() {
	codeIndex = make(map[string]Code, len(allCodes))
	for _, c := range allCodes {
		codeIndex[string(c)] = c
	}
}

// AllCodes returns a copy of every defined code, in declaration order. The
// copy is deliberate: the registry backs the manifest and the payload schema,
// and a caller that could mutate it could move a golden.
func AllCodes() []Code {
	out := make([]Code, len(allCodes))
	copy(out, allCodes)
	return out
}

// ParseCode resolves s to a known Code. The second result reports whether s
// named one; unknown, empty and differently-cased strings all fail.
func ParseCode(s string) (Code, bool) {
	c, ok := codeIndex[s]
	return c, ok
}

// Domain returns the AREA segment of the code — "OPC", "ZIP", "FONT" — or the
// empty string if the code is not of the VELLUM_<AREA>_<CATEGORY> form.
func (c Code) Domain() string {
	s := string(c)
	const prefix = "VELLUM_"
	if len(s) <= len(prefix) || s[:len(prefix)] != prefix {
		return ""
	}
	rest := s[len(prefix):]
	for i := 0; i < len(rest); i++ {
		if rest[i] == '_' {
			if i == 0 {
				return ""
			}
			return rest[:i]
		}
	}
	return ""
}

// AllDomains returns the distinct domains present in the registry, in first-
// seen order. Ordering is first-seen rather than sorted so it tracks the
// declaration order in this file, which groups by layer.
func AllDomains() []string {
	seen := make(map[string]bool, len(allCodes))
	out := make([]string, 0, len(allCodes))
	for _, c := range allCodes {
		d := c.Domain()
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

// ByDomain returns every code in the named domain, in declaration order.
func ByDomain(domain string) []Code {
	out := make([]Code, 0, 8)
	for _, c := range allCodes {
		if c.Domain() == domain {
			out = append(out, c)
		}
	}
	return out
}
