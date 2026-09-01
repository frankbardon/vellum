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

	// VELLUM_PDF_GLYPH_MISSING indicates text containing a character the
	// selected face has no glyph for. It is an error rather than a substitution
	// with a notdef box, because a document that silently renders a row of
	// empty rectangles is one nobody notices until it has been sent.
	VELLUM_PDF_GLYPH_MISSING Code = "VELLUM_PDF_GLYPH_MISSING"
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
