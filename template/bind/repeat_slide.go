package bind

// execSlideRepeat realizes [RepeatTargetSlide]: N whole pptx slide *parts*
// added to the output package where the template carried one un-repeated
// slide.
//
// # Why this cannot reuse execRepeat's own pipeline
//
// Every other [RepeatTarget] repeats a *region inside one part*: a
// <w:tr>, a <w:sdt>, a <row>. The container and every anchor its body
// reaches live in the same bytes, so execRepeat's pipeline — extract the
// container's own source once, run the body per item against a throwaway
// package wrapping the extracted slice, concatenate every iteration's
// filled copy, and produce one [xmlcopy.Replacement] covering the
// container's own span — is a byte-span operation from start to finish.
//
// A pptx repeat's anchors live in one slide's own part
// (ppt/slides/slideN.xml), but what gets repeated is *the whole slide as a
// distinct OPC part*: cloning it means creating brand-new parts
// (ppt/slides/slideN-fill1.xml, ...), each with its own relationships part,
// registering a [Content_Types].xml override for each, adding a
// relationship from ppt/presentation.xml to each new slide part, and
// inserting a new <p:sldId id="..." r:id="..."/> entry into
// ppt/presentation.xml's own <p:sldIdLst>. None of that is a byte-span
// replacement inside one part — it is real package-structure mutation, the
// same kind of "add a whole new part" operation splice/asset.go's own
// embedAsset already does for a media part, generalized here to a
// structurally richer part (its own relationships, its own content-type
// declaration, and an entry in a *third* part naming it).
//
// The one piece of discipline this function keeps in common with every
// sibling target: every iteration reads from the untouched original
// template bytes, never a previous iteration's own output, and the whole
// function is a pure function of the binding's own structure, the item
// list, and the template's own already-assigned identifiers — no
// [time.Now], no random or map-iteration-derived value, so the same binding
// against the same template and data produces byte-identical output every
// time.
//
// # Where things land
//
// A new slide's filled bytes, its own copied relationships part, its own
// content-type override, and the relationship ppt/presentation.xml gains to
// it, all land directly in assetPkg — the real output package, exactly the
// same package [execRepeat]'s own asset embedding already writes new media
// parts into (see this package's own top-level doc comment on assetPkg).
// That is deliberate here in a way it is only incidental for the DOCX/xlsx
// targets: a slide clone has no "throwaway, discarded" life stage the way a
// <w:tr>'s own extracted bytes do, because the thing being produced (a new
// OPC part) has no other home to be discarded from — it is either in the
// output package or it does not exist.
//
// Only the presentation part's own <p:sldId> entry for the original,
// un-repeated slide is replaced through the normal repls/[xmlcopy.Apply]
// path, mirroring the "the original placeholder becomes N real instances"
// semantics every other repeat target already has.
//
// # sldId numbering
//
// A cloned slide's own <p:sldId id="..."> must satisfy CLAUDE.md's own
// byte-layout invariant: at least 256 and strictly below 2147483648 (the
// first sldMasterId/sldLayoutId), the two identifier spaces disjoint by
// contract. The rule this function follows is one past the highest sldId
// value anywhere reachable at the moment it runs — either an original
// <p:sldId> in the template's own <p:sldIdLst>, or one already minted by an
// *earlier* slide repeat in the same Fill call. The second half matters
// because presentation.xml's own body bytes are never mutated between
// execSlideRepeat calls (only its relationships and the parts it names are)
// — every call sees the same pristine <p:sldIdLst> — so without also
// scanning what a prior call already *recorded* as a pending replacement
// (via repls.For, which every earlier call's own accumulated
// [xmlcopy.Replacement] for the presentation part is still visible through),
// two slide repeats in one binding would independently start numbering from
// the same base and collide. Scanning repls costs nothing extra and needs
// no new shared state beyond what [Execute] already threads through every
// statement in the tree.
//
// # The original slide part is left in place, unreferenced
//
// Once its own <p:sldId> entry is replaced or removed, nothing in the
// output package still points at the original template slide part — but
// this function does not delete it. A slide may be the target of a notes
// slide's own back-reference relationship (ppt/notesSlides/notesSlideN.xml
// -> its owning slide), a fact this story's own scope has not audited, and
// deleting the slide part without accounting for that risks leaving a
// dangling relationship elsewhere in the package — a worse failure than an
// unreferenced, harmless extra part. [opc.Package]'s own writeOrder already
// tolerates a relationships part whose owner is absent (see its own doc
// comment: "emitted rather than dropped... silently discarding content is
// the failure this library exists to avoid"), so leaving the original slide
// part and its own relationships part present-but-unreferenced is not a
// special case this function invents — it is the same tolerance the package
// format already extends to a part nothing points at, applied deliberately
// rather than accidentally.

import (
	"bytes"
	"strconv"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/xmlcopy"
)

const (
	// relOfficeDocumentSlide is the package-relationship type naming a
	// package's own main part — this file's own copy of the constant
	// template.go and template/anchor's xlsx.go each already carry, per this
	// subtree's "own your own constants" convention.
	relOfficeDocumentSlide = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"

	// relSlideRepeat is the relationship type ppt/presentation.xml's own
	// <p:sldId r:id="..."/> resolves through to a slide part — this file's
	// own copy of the same constant template/anchor's pptx.go carries as
	// relSlide (different package, so a different name to avoid a stray
	// impression the two are the same symbol).
	relSlideRepeat = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide"

	// nsPresentationRepeat is the PresentationML main namespace.
	nsPresentationRepeat = "http://schemas.openxmlformats.org/presentationml/2006/main"

	// nsRelationshipsRepeat is the officeDocument relationships namespace,
	// resolving a <p:sldId>'s own r:id attribute.
	nsRelationshipsRepeat = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

	// relsContentTypeRepeat is the content type every OPC relationships part
	// carries — opc's own relsContentType constant is unexported, so this is
	// this file's own copy, matching it exactly.
	relsContentTypeRepeat = "application/vnd.openxmlformats-package.relationships+xml"

	// firstMasterIDRepeat is the first identifier PresentationML reserves
	// for a sldMasterId/sldLayoutId — the bound a cloned slide's own sldId
	// must stay strictly below, per CLAUDE.md's own byte-layout invariant.
	firstMasterIDRepeat = 2147483648
)

// sldIDEntry is one <p:sldId id="..." r:id="..."/> entry read from the
// presentation part's own <p:sldIdLst>.
type sldIDEntry struct {
	id   int
	rID  string
	span xmlcopy.Span
}

// execSlideRepeat is [RepeatTargetSlide]'s own execution path. See this
// file's own doc comment for the full design.
func execSlideRepeat(r *Repeat, scope Scope, ev Evaluator, frame Frame, assetPkg *opc.Package, repls *ReplacementSet) error {
	items, err := EvaluateList(ev, r.Over, scope)
	if err != nil {
		return err
	}

	names := collectAnchorNames(r.Body)
	if len(names) == 0 {
		return repeatContainerErr(RepeatTargetSlide, names,
			"the repeat's body names no anchor anywhere in it, so there is no slide to identify")
	}

	resolved := make(map[string]anchor.Anchor, len(names))
	slidePart := ""
	for _, name := range names {
		a, ok := frame.Anchors[name]
		if !ok {
			return unknownAnchorErr(name)
		}
		if a.Kind != anchor.KindShape {
			return repeatContainerErr(RepeatTargetSlide, names,
				"every anchor a slide repeat's body references must be a shape anchor")
		}
		if slidePart == "" {
			slidePart = a.Part
		} else if slidePart != a.Part {
			return repeatContainerErr(RepeatTargetSlide, names,
				"the repeat's body anchors are not all on the same slide")
		}
		resolved[name] = a
	}

	presPart, err := presentationPartName(assetPkg)
	if err != nil {
		return err
	}
	presSrc, err := partBytes(assetPkg, presPart)
	if err != nil {
		return err
	}

	entries, err := scanSldIDEntries(presSrc)
	if err != nil {
		return err
	}

	var target *sldIDEntry
	for i := range entries {
		resolvedPart, ok := resolveSlideRelTarget(assetPkg, presPart, entries[i].rID)
		if ok && resolvedPart == slidePart {
			target = &entries[i]
			break
		}
	}
	if target == nil {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"no <p:sldId> entry in the presentation resolves to the repeat's own slide part",
			map[string]any{"slide_part": slidePart})
	}

	if len(items) == 0 && len(entries) == 1 {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_TEMPLATE_SLIDE_REPEAT_EMPTIES_DECK,
			"a zero-item slide repeat would remove the presentation's only slide",
			map[string]any{"slide_part": slidePart})
	}

	// The next sldId to mint: one past the highest id anywhere already
	// reachable — the original sldIdLst, or a replacement an earlier slide
	// repeat in this same Fill call already recorded against presPart. See
	// this file's own doc comment for why both sources matter.
	nextID := 0
	for _, e := range entries {
		if e.id > nextID {
			nextID = e.id
		}
	}
	for _, prior := range repls.For(presPart) {
		for _, id := range scanSldIDValues(prior.Data) {
			if id > nextID {
				nextID = id
			}
		}
	}
	nextID++

	origSlidePart, ok := frame.SrcPkg.Get(slidePart)
	if !ok {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"the repeat's own slide part is missing from the source package", map[string]any{"part": slidePart})
	}
	origSlideSrc, err := origSlidePart.Bytes()
	if err != nil {
		return err
	}
	slideContentType := origSlidePart.ContentType

	origRelsName := opc.RelsNameFor(slidePart)
	origRelsBytes, hasRels, err := partBytesOptional(frame.SrcPkg, origRelsName)
	if err != nil {
		return err
	}

	type clone struct {
		partName string
		target   string
		sldID    int
	}
	clones := make([]clone, 0, len(items))

	for i, item := range items {
		iterScope := extendScope(scope, r.As, item)

		// relocated is a plain copy: unlike a DOCX/xlsx repeat's own
		// throwaway container (a sub-slice, needing coordinate translation —
		// see repeatWrapOpen's own doc comment), the throwaway package below
		// wraps the *whole, complete* original slide part bytes, which every
		// resolved anchor's own Span already addresses correctly with no
		// offset shift.
		relocated := make(map[string]anchor.Anchor, len(names))
		for _, name := range names {
			relocated[name] = resolved[name]
		}

		throwaway := opc.New()
		if err := throwaway.Put(&opc.Part{Name: slidePart, Data: origSlideSrc}); err != nil {
			return err
		}

		childFrame := Frame{SrcPkg: throwaway, Anchors: relocated}
		childRepls := NewReplacementSet()
		if err := Execute(r.Body, iterScope, ev, childFrame, assetPkg, childRepls); err != nil {
			return err
		}

		applied, err := xmlcopy.Apply(origSlideSrc, childRepls.For(slidePart))
		if err != nil {
			return err
		}

		newName := freshSlidePartName(assetPkg, slidePart, i)
		if err := assetPkg.Put(&opc.Part{Name: newName, ContentType: slideContentType, Data: applied}); err != nil {
			return err
		}
		if hasRels {
			if err := assetPkg.Put(&opc.Part{
				Name:        opc.RelsNameFor(newName),
				ContentType: relsContentTypeRepeat,
				Data:        origRelsBytes,
			}); err != nil {
				return err
			}
		}

		relTarget := relativeTargetRepeat(presPart, newName)
		if _, err := assetPkg.Relationships(presPart).Add(relSlideRepeat, relTarget, opc.TargetInternal); err != nil {
			return err
		}

		sldID := nextID + i
		if sldID >= firstMasterIDRepeat {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_TEMPLATE_SLIDE_ID_RANGE_EXCEEDED,
				"a slide repeat would need a sldId at or beyond the master/layout identifier space",
				map[string]any{"slide_part": slidePart, "computed_id": sldID})
		}

		clones = append(clones, clone{partName: newName, target: relTarget, sldID: sldID})
	}

	// Freeze locks in stable relationship identifiers now, the same reason
	// splice/asset.go's own embedAsset freezes before reading identifiers
	// back: for a parsed set (the overwhelmingly common real case — a real
	// pptx's own presentation.xml always carries relationships to its
	// masters/theme, so its own rels part is always parsed rather than
	// built) this is a no-op, because Add already assigned final identifiers
	// on a parsed set. It only does real work for a synthetic template whose
	// presentation part carried no relationships part at all.
	assetPkg.Relationships(presPart).Freeze()

	var out []byte
	for _, c := range clones {
		rID, ok := assetPkg.Relationships(presPart).IDFor(relSlideRepeat, c.target)
		if !ok {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
				"a relationship added for a cloned slide was not found by its own type and target",
				map[string]any{"target": c.target})
		}
		out = append(out, []byte(`<p:sldId id="`+strconv.Itoa(c.sldID)+`" r:id="`+rID+`"/>`)...)
	}

	repls.Add(presPart, target.span.Replace(out))
	return nil
}

// presentationPartName resolves pkg's own root officeDocument relationship
// to the presentation part's own name, mirroring template.detectFormat's own
// resolution exactly — this package cannot import template (template
// imports bind through fill.go), so it carries its own copy, per this
// subtree's "own your own constants" convention.
func presentationPartName(pkg *opc.Package) (string, error) {
	rels, ok := pkg.RelationshipsFor("/")
	if !ok {
		return "", verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT,
			"the output package declares no root relationships")
	}
	matches := rels.ByType(relOfficeDocumentSlide)
	if len(matches) == 0 {
		return "", verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT,
			"the output package declares no officeDocument relationship")
	}
	return resolveTargetRepeat("/", matches[0].Target), nil
}

// scanSldIDEntries walks the presentation part once for its own
// <p:sldIdLst><p:sldId id="..." r:id="..."/></p:sldIdLst> entries, in
// document order.
func scanSldIDEntries(src []byte) ([]sldIDEntry, error) {
	var entries []sldIDEntry
	err := xmlcopy.Walk(src, func(e xmlcopy.Element) error {
		if e.Name.Space != nsPresentationRepeat || e.Name.Local != "sldId" {
			return nil
		}
		idStr, hasID := readAttr(e, "id")
		rID, hasRID := readAttrNS(e, nsRelationshipsRepeat, "id")
		if !hasID || !hasRID {
			return nil
		}
		id, convErr := strconv.Atoi(idStr)
		if convErr != nil {
			return nil
		}
		entries = append(entries, sldIDEntry{id: id, rID: rID, span: e.Span})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// readAttrNS returns the value of element e's own attribute named local in
// namespace space, from the Attr slice xmlcopy.Walk already populated —
// readAttr's own namespaced counterpart, needed here because r:id is
// namespaced and readAttr only ever looks at unprefixed attributes.
func readAttrNS(e xmlcopy.Element, space, local string) (string, bool) {
	for _, a := range e.Attr {
		if a.Name.Space == space && a.Name.Local == local {
			return a.Value, true
		}
	}
	return "", false
}

// resolveSlideRelTarget resolves rID, a relationship id owned by presPart,
// to its target's absolute part name.
func resolveSlideRelTarget(pkg *opc.Package, presPart, rID string) (string, bool) {
	rels, ok := pkg.RelationshipsFor(presPart)
	if !ok {
		return "", false
	}
	rel, ok := rels.Resolve(rID)
	if !ok {
		return "", false
	}
	return resolveTargetRepeat(presPart, rel.Target), true
}

// scanSldIDValues finds every ` id="N"` occurrence in data — the numeric
// slide identifiers in a <p:sldId id="N" r:id="..."/> fragment this
// function itself produced in an earlier call (data comes from
// repls.For(presPart), whose entries are exactly the concatenated
// <p:sldId .../> strings this function builds), or in a slice of the
// original presentation part's own bytes. The leading space in the needle
// is what keeps this from also matching the namespaced ` r:id="..."`
// attribute immediately preceding it.
func scanSldIDValues(data []byte) []int {
	var out []int
	needle := []byte(` id="`)
	i := 0
	for {
		idx := bytes.Index(data[i:], needle)
		if idx < 0 {
			break
		}
		start := i + idx + len(needle)
		end := start
		for end < len(data) && data[end] != '"' {
			end++
		}
		if n, err := strconv.Atoi(string(data[start:end])); err == nil {
			out = append(out, n)
		}
		i = end
	}
	return out
}

// partBytesOptional fetches name's own bytes when the package contains it,
// or reports found=false when it does not — partBytes' own permissive
// counterpart, needed because a slide legitimately may carry no
// relationships part at all (one referencing no layout, no media — never a
// real docx/pptx a real authoring tool writes, but not something this
// function should error on either).
func partBytesOptional(pkg *opc.Package, name string) ([]byte, bool, error) {
	part, ok := pkg.Get(name)
	if !ok {
		return nil, false, nil
	}
	b, err := part.Bytes()
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

// freshSlidePartName derives a new slide part name for iteration index
// (0-based) of a clone of original, guaranteed not to collide with any part
// name already in pkg — checked with [opc.Package.Has], per this story's own
// guidance to check collision rather than assume the template follows
// Vellum's own compose-mode naming convention.
func freshSlidePartName(pkg *opc.Package, original string, index int) string {
	dir, stem := splitPartRepeat(original)
	candidate := dir + stem + "-fill" + strconv.Itoa(index+1) + ".xml"
	for n := 0; pkg.Has(candidate); n++ {
		candidate = dir + stem + "-fill" + strconv.Itoa(index+1) + "-" + strconv.Itoa(n) + ".xml"
	}
	return candidate
}

// splitPartRepeat splits an OPC part name into its own directory (with
// trailing slash) and its own file stem (basename, without ".xml").
func splitPartRepeat(name string) (dir, stem string) {
	i := strings.LastIndexByte(name, '/')
	d := name[:i+1]
	base := name[i+1:]
	return d, strings.TrimSuffix(base, ".xml")
}

// relativeTargetRepeat computes a relationship Target string for target,
// relative to owner's own directory — this file's own copy of the same
// algorithm deck/write.go's own relative() helper implements, per this
// subtree's "own your own constants" convention (deck is read-only
// reference here, never imported).
func relativeTargetRepeat(owner, target string) string {
	ownerDir := owner[:strings.LastIndexByte(owner, '/')+1]
	if len(target) > len(ownerDir) && target[:len(ownerDir)] == ownerDir {
		return target[len(ownerDir):]
	}

	up := 0
	dir := ownerDir
	for dir != "/" {
		dir = dir[:strings.LastIndexByte(dir[:len(dir)-1], '/')+1]
		up++
		if len(target) > len(dir) && target[:len(dir)] == dir {
			return strings.Repeat("../", up) + target[len(dir):]
		}
	}
	return target
}

// resolveTargetRepeat resolves a relationship target against owner's
// directory — this file's own copy of the same algorithm template.go's own
// resolveTarget and template/anchor's xlsx.go's own resolveRelTarget already
// carry, per this subtree's established "own your own constants" convention
// for a helper no package here can reach across an import boundary.
func resolveTargetRepeat(owner, target string) string {
	if strings.HasPrefix(target, "/") {
		return target
	}

	base := "/"
	if owner != "/" && owner != "" {
		if i := strings.LastIndexByte(owner, '/'); i >= 0 {
			base = owner[:i+1]
		}
	}

	segments := strings.Split(strings.TrimPrefix(base, "/"), "/")
	if len(segments) > 0 && segments[len(segments)-1] == "" {
		segments = segments[:len(segments)-1]
	}
	for _, seg := range strings.Split(target, "/") {
		switch seg {
		case "", ".":
		case "..":
			if len(segments) > 0 {
				segments = segments[:len(segments)-1]
			}
		default:
			segments = append(segments, seg)
		}
	}
	return "/" + strings.Join(segments, "/")
}
