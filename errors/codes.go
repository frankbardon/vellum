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

	// DOC
	VELLUM_DOC_BLOCK_UNSUPPORTED,

	// ZIP
	VELLUM_ZIP_MALFORMED,
	VELLUM_ZIP_TOO_LARGE,
	VELLUM_ZIP_ENTRY_NAME_INVALID,
	VELLUM_ZIP_ENTRY_DUPLICATE,

	// FONT
	VELLUM_FONT_SUBSTITUTED,

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
