package template

import (
	"sort"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/xmlcopy"
)

// nsWordprocessingMain is the WordprocessingML main namespace, matched by
// resolved URI rather than by a literal "w:" prefix — the same discipline
// template/anchor/docx.go documents for its own nsWordprocessing constant, so
// a family declared under whichever prefix an authoring tool happened to bind
// is still found.
const nsWordprocessingMain = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

// relStyles is the officeDocument relationship type a WordprocessingML main
// part uses to name its styles part — conventionally "word/styles.xml", but
// read from the relationship rather than assumed, the same discipline
// detectFormat already applies to the officeDocument relationship itself.
const relStyles = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles"

// rFontsAttrOrder is the fixed, canonical order font-requirement categories
// are reported in: ASCII text, high-ANSI text, complex-script text, East
// Asian text — the same sequence Word itself writes the w:rFonts attributes
// in, not an alphabetical sort. Fixed rather than derived from input, so two
// templates referencing the same family under the same categories always
// report those categories in the same order.
var rFontsAttrOrder = []string{"ascii", "hAnsi", "cs", "eastAsia"}

// FontRequirement is one distinct font family a template's own XML
// references via w:rFonts.
//
// This reports a name the template already states; it is not font discovery
// or validation. A DOCX template Vellum fills is very likely to reference
// fonts by family name only, with no embedded font program — Vellum's own
// compose-mode DOCX writer never writes a word/fontTable.xml part or embeds
// one either (font.embed.none, capability/matrix.go) — so nothing here
// inspects a fontTable part or an embedded font program even when a template
// happens to carry one. A caller wanting to know whether "Calibri" is
// actually available checks its own font inventory against Family; that
// check does not belong in fill mode, which has no theme and makes no
// substitution decision.
type FontRequirement struct {
	// Family is the font family name exactly as the template's XML states it
	// — byte-for-byte, no case-folding or other normalisation, since two
	// distinct attribute values are two distinct claims about what family is
	// wanted even if a human would read them as the same font.
	Family string `json:"family"`

	// Categories is the w:rFonts attribute categories this family was
	// referenced under somewhere in the template — "ascii", "hAnsi", "cs",
	// "eastAsia" — deduplicated and ordered per rFontsAttrOrder. A family
	// referenced as w:ascii fifty times across the document reports "ascii"
	// once.
	Categories []string `json:"categories"`
}

// categoriesJoined renders Categories as a single comma-separated string, for
// InspectReport.FontsTable.
func (f FontRequirement) categoriesJoined() string {
	return strings.Join(f.Categories, ", ")
}

// discoverFonts collects every distinct font family mainPart's own XML
// references via w:rFonts, plus — when the main part's relationships name
// one — its styles part's.
//
// Scanning the styles part closes a real under-reporting gap: the run font a
// document actually uses for most of its text is very often stated once, in
// styles.xml's docDefaults or Normal style, and never restated on every run
// that inherits it. A scan of only the main part would report nothing for
// such a template even though every paragraph in it depends on that family
// being available. The main part is always scanned; the styles part is
// scanned too whenever mainPart's own relationships resolve one to a part the
// package actually contains — its absence is not an error, since a
// relationship-free document (nothing here templated, or a non-DOCX caller
// that never reaches this function today) is a legitimate shape.
func discoverFonts(pkg *opc.Package, mainPart string) ([]FontRequirement, error) {
	families := make(map[string]map[string]struct{}) // scratch only: looked up by key, never ranged for its values

	mainSrc, err := fontScanPartBytes(pkg, mainPart)
	if err != nil {
		return nil, err
	}
	if err := scanRFonts(mainSrc, families); err != nil {
		return nil, err
	}

	if stylesPart, ok := discoverStylesPart(pkg, mainPart); ok {
		stylesSrc, err := fontScanPartBytes(pkg, stylesPart)
		if err != nil {
			return nil, err
		}
		if err := scanRFonts(stylesSrc, families); err != nil {
			return nil, err
		}
	}

	return buildFontRequirements(families), nil
}

// discoverStylesPart resolves the part mainPart's own relationships name via
// relStyles, and reports whether one exists and the package actually
// contains it. A dangling or absent styles relationship is not this
// function's problem to raise — that is [opc.Package.Validate]'s job on the
// package as a whole — so it is reported as "no styles part found" rather
// than as an error.
func discoverStylesPart(pkg *opc.Package, mainPart string) (string, bool) {
	rels, ok := pkg.RelationshipsFor(mainPart)
	if !ok {
		return "", false
	}
	matches := rels.ByType(relStyles)
	if len(matches) == 0 {
		return "", false
	}
	// A well-formed part carries at most one styles relationship; ByType
	// returns them in serialised order, so taking the first is deterministic
	// even for a part that (incorrectly) declares more than one — the same
	// discipline detectFormat applies to the officeDocument relationship.
	target := resolveTarget(mainPart, matches[0].Target)
	if !pkg.Has(target) {
		return "", false
	}
	return target, true
}

// fontScanPartBytes reads a part's bytes for font scanning, reporting an
// internal-invariant error rather than panicking if the caller handed a part
// name the package does not actually contain. Reachable only if
// discoverStylesPart or Inspect's own caller passes an inconsistent
// (pkg, mainPart) pair — not something an untrusted template's own bytes can
// trigger, since discoverStylesPart already checks pkg.Has before returning a
// target.
func fontScanPartBytes(pkg *opc.Package, name string) ([]byte, error) {
	part, ok := pkg.Get(name)
	if !ok {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"font discovery was given a part name the package does not contain",
			map[string]any{"part": name})
	}
	return part.Bytes()
}

// scanRFonts walks src once, collecting every family named by a w:rFonts
// element's w:ascii, w:hAnsi, w:cs or w:eastAsia attribute into families,
// keyed by family name. families is mutated in place so a caller can fold
// several parts' results together.
//
// w:rFonts also carries w:asciiTheme, w:hAnsiTheme, w:csTheme, w:eastAsiaTheme
// (a reference to the theme's own font scheme, not a literal family name) and
// w:hint (which script a reader should treat an unmarked character as). None
// of the four name a font family directly, so none are scanned here — a
// theme-referencing template has no theme in fill mode to resolve one from
// anyway.
func scanRFonts(src []byte, families map[string]map[string]struct{}) error {
	return xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if e.Name.Space != nsWordprocessingMain || e.Name.Local != "rFonts" {
			return nil
		}
		for _, attr := range e.Attr {
			if attr.Name.Space != nsWordprocessingMain {
				continue
			}
			var category string
			switch attr.Name.Local {
			case "ascii", "hAnsi", "cs", "eastAsia":
				category = attr.Name.Local
			default:
				continue
			}
			family := strings.TrimSpace(attr.Value)
			if family == "" {
				continue
			}
			cats, ok := families[family]
			if !ok {
				cats = make(map[string]struct{})
				families[family] = cats
			}
			cats[category] = struct{}{}
		}
		return nil
	})
}

// buildFontRequirements turns the scratch families map into a deterministic,
// sorted []FontRequirement: family names sorted bytewise, each family's own
// categories ordered per rFontsAttrOrder rather than left in the map's
// (unspecified, per-run) iteration order.
func buildFontRequirements(families map[string]map[string]struct{}) []FontRequirement {
	names := make([]string, 0, len(families))
	for name := range families { // keys only: collected into a slice and sorted below, never ranged for values
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]FontRequirement, 0, len(names))
	for _, name := range names {
		cats := families[name]
		var ordered []string
		for _, c := range rFontsAttrOrder {
			if _, ok := cats[c]; ok {
				ordered = append(ordered, c)
			}
		}
		out = append(out, FontRequirement{Family: name, Categories: ordered})
	}
	return out
}
