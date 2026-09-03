package splice

import (
	"strconv"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/xmlcopy"
)

// renderAssetParagraph resolves ref against seq's own asset manifest, embeds
// it into pkg as a new media part (mutating pkg in place — see the package
// doc for why an asset is the one exception to "splice returns a
// Replacement, nothing else"), and renders the inline picture wrapped in its
// own paragraph, since a w:drawing is run-level content and a native
// anchor's sdtContent needs block-level children.
func renderAssetParagraph(pkg *opc.Package, ownerPart string, seq fragment.Sequence, ref *fragment.AssetRef, drawingID int) ([]byte, error) {
	if ref.AssetIndex < 0 || ref.AssetIndex >= len(seq.Assets) {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"an asset block references an asset that is not in the sequence's own manifest",
			map[string]any{"asset_index": ref.AssetIndex, "asset_count": len(seq.Assets)})
	}
	a := seq.Assets[ref.AssetIndex]

	rID, err := embedAsset(pkg, ownerPart, a)
	if err != nil {
		return nil, err
	}

	drawing := renderDrawing(rID, ref, drawingID)
	var b strings.Builder
	b.WriteString(`<w:p><w:r>`)
	b.Write(drawing)
	b.WriteString(`</w:r></w:p>`)
	return []byte(b.String()), nil
}

// embedAsset adds a's bytes as a new media part under ownerPart's directory
// and registers (or reuses) the image relationship, returning the
// relationship id to embed as r:embed.
//
// The media part name is derived from a.Hash alone — content-hash-based
// rather than counter-based, per CLAUDE.md's determinism rules, and simple
// rather than doc's own mediaFileName scheme (index-plus-format-wide
// dedup manifest) because splice embeds exactly one asset at a time and has
// no document-wide manifest to deduplicate against.
func embedAsset(pkg *opc.Package, ownerPart string, a fragment.Asset) (string, error) {
	if a.MediaType != mediaPNG && a.MediaType != mediaJPEG {
		return "", verr.NewCodedErrorWithDetails(verr.VELLUM_TEMPLATE_ASSET_MEDIA_UNSUPPORTED,
			"the target format cannot embed an asset of this media type",
			map[string]any{
				"media_type": a.MediaType,
				"accepted":   []string{mediaJPEG, mediaPNG},
			})
	}

	dir := ownerDir(ownerPart)
	partName := dir + "media/img" + a.Hash + "." + mediaExtension(a.MediaType)
	target := "media/img" + a.Hash + "." + mediaExtension(a.MediaType)

	if err := pkg.Put(&opc.Part{Name: partName, ContentType: a.MediaType, Data: a.Bytes}); err != nil {
		return "", err
	}

	rels := pkg.Relationships(ownerPart)
	if id, ok := rels.IDFor(relImage, target); ok {
		return id, nil
	}
	if _, err := rels.Add(relImage, target, opc.TargetInternal); err != nil {
		return "", err
	}
	// Freeze locks in stable identifiers now, the same reason doc/write.go
	// freezes before reading its own relationships back: document.xml
	// references the relationship by id and the relationships part is
	// serialised later, so the two must already agree. For a relationship
	// set parsed from the opened template (the overwhelmingly common real
	// case — a real docx's main part always carries at least a styles and
	// settings relationship) this is a no-op: a parsed set is never
	// renumbered. It only does real work for a template whose main part
	// carried no relationships part at all, and in that rare case, calling
	// Splice repeatedly against the same still-unfrozen set for more than
	// one image is a known, narrow limitation — a later freeze can still
	// renumber an id an earlier call already embedded into its own
	// Replacement — accepted here rather than solved, since it requires no
	// change this story's scope reaches (opc/rels.go is out of scope) and
	// cannot occur against any template a real authoring tool produced.
	rels.Freeze()
	id, ok := rels.IDFor(relImage, target)
	if !ok {
		return "", verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"a relationship added and frozen was not found by its own type and target",
			map[string]any{"type": relImage, "target": target})
	}
	return id, nil
}

// ownerDir returns the directory an OPC part name sits in, with a trailing
// slash — "/word/document.xml" -> "/word/" — so a media part can be placed
// beside its owner rather than hardcoding "/word/".
func ownerDir(part string) string {
	if i := strings.LastIndexByte(part, '/'); i >= 0 {
		return part[:i+1]
	}
	return "/"
}

// renderDrawing emits an inline picture. xmlns:r is declared locally on the
// element rather than assumed inherited from the document root: splice edits
// an arbitrary template it did not author, and while every real docx's root
// w:document declares it (header/footer and section relationships need it
// too), redeclaring it here costs nothing and removes the assumption.
func renderDrawing(rID string, ref *fragment.AssetRef, id int) []byte {
	idStr := strconv.Itoa(id)
	width := strconv.FormatInt(ref.WidthEMU, 10)
	height := strconv.FormatInt(ref.HeightEMU, 10)
	name := "Picture " + idStr

	var b strings.Builder
	b.WriteString(`<w:drawing xmlns:r="` + nsRelationships + `">`)
	b.WriteString(`<wp:inline xmlns:wp="` + nsDrawingWP + `" distT="0" distB="0" distL="0" distR="0">`)
	b.WriteString(`<wp:extent cx="` + width + `" cy="` + height + `"/>`)
	b.WriteString(`<wp:effectExtent l="0" t="0" r="0" b="0"/>`)
	b.WriteString(`<wp:docPr id="` + idStr + `" name="` + xmlcopy.EscapeAttr(name) + `"`)
	if ref.AltText != "" {
		b.WriteString(` descr="` + xmlcopy.EscapeAttr(ref.AltText) + `"`)
	}
	b.WriteString(`/>`)
	b.WriteString(`<wp:cNvGraphicFramePr><a:graphicFrameLocks xmlns:a="` + nsDrawingMain + `" noChangeAspect="1"/></wp:cNvGraphicFramePr>`)
	b.WriteString(`<a:graphic xmlns:a="` + nsDrawingMain + `"><a:graphicData uri="` + nsDrawingPicture + `">`)
	b.WriteString(`<pic:pic xmlns:pic="` + nsDrawingPicture + `">`)
	b.WriteString(`<pic:nvPicPr><pic:cNvPr id="` + idStr + `" name="` + xmlcopy.EscapeAttr(name) + `"`)
	if ref.AltText != "" {
		b.WriteString(` descr="` + xmlcopy.EscapeAttr(ref.AltText) + `"`)
	}
	b.WriteString(`/><pic:cNvPicPr/></pic:nvPicPr>`)
	b.WriteString(`<pic:blipFill><a:blip r:embed="` + rID + `"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`)
	b.WriteString(`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="` + width + `" cy="` + height + `"/></a:xfrm>`)
	b.WriteString(`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>`)
	b.WriteString(`</pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing>`)
	return []byte(b.String())
}
