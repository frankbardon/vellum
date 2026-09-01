package capability

import (
	"github.com/frankbardon/vellum/artifact"
	verr "github.com/frankbardon/vellum/errors"
)

// matrix is the declaration. Every (feature, format) pair appears exactly
// once, and TestCapabilityMatrixComplete asserts it by cardinality — so adding
// a feature or a format fails the build until every cell is filled in.
//
// Grouped by format, and within a format in feature declaration order, so the
// file reads as four columns rather than as a list.
var matrix = Matrix{
	// ---- DOCX: a flowing document. The richest target for prose. ----
	{Feature: FeatureBlockHeading, Format: artifact.FormatDOCX, Outcome: Renders},
	{Feature: FeatureBlockText, Format: artifact.FormatDOCX, Outcome: Renders},
	{Feature: FeatureBlockAsset, Format: artifact.FormatDOCX, Outcome: Renders},
	{Feature: FeatureBlockTable, Format: artifact.FormatDOCX, Outcome: Renders},
	{Feature: FeatureBlockPageBreak, Format: artifact.FormatDOCX, Outcome: Renders},
	{
		Feature: FeatureBlockNotes, Format: artifact.FormatDOCX, Outcome: Degrades,
		Degrade: "footnote", Code: verr.VELLUM_CAPABILITY_DEGRADED,
		Note: "A flowing document has no speaker-note channel, so a note becomes a footnote anchored where the block sits.",
	},
	{Feature: FeatureBlockSpacer, Format: artifact.FormatDOCX, Outcome: Renders},

	{Feature: FeatureTableHierarchicalHeaders, Format: artifact.FormatDOCX, Outcome: Renders},
	{Feature: FeatureTableCellAnnotation, Format: artifact.FormatDOCX, Outcome: Renders},
	{Feature: FeatureTableMargins, Format: artifact.FormatDOCX, Outcome: Renders},
	{Feature: FeatureTableCellSpan, Format: artifact.FormatDOCX, Outcome: Renders},

	{Feature: FeatureAssetPNG, Format: artifact.FormatDOCX, Outcome: Renders},
	{Feature: FeatureAssetJPEG, Format: artifact.FormatDOCX, Outcome: Renders},
	{
		Feature: FeatureAssetSVG, Format: artifact.FormatDOCX, Outcome: Degrades,
		Degrade: "raster fallback with the vector embedded alongside", Code: verr.VELLUM_CAPABILITY_DEGRADED,
		Note: "Word 2016 and later read an SVG only when a raster blip accompanies it. The caller supplies the pair; Vellum will not rasterise, because it never renders.",
	},

	{
		Feature: FeatureFontEmbedSubset, Format: artifact.FormatDOCX, Outcome: Degrades,
		Degrade: "the family referenced by name", Code: verr.VELLUM_CAPABILITY_DEGRADED,
		Note: "Font embedding in OOXML is a document-settings feature Vellum does not author in v1, so a theme's faces are referenced by name and the reader resolves them. Degrades rather than rejects because a theme that merely permits embedding should still render; a theme that explicitly demands it gets VELLUM_FONT_EMBED_UNSUPPORTED instead, because an explicit demand is a statement about a licence and must not be silently downgraded.",
	},
	{
		Feature: FeatureFontEmbedWhole, Format: artifact.FormatDOCX, Outcome: Degrades,
		Degrade: "the family referenced by name", Code: verr.VELLUM_CAPABILITY_DEGRADED,
		Note: "As above.",
	},

	{
		Feature: FeatureFontOutlinesCFF, Format: artifact.FormatDOCX, Outcome: Degrades,
		Degrade: "the family referenced by name", Code: verr.VELLUM_FONT_SUBSTITUTED,
		Note: "The outline format does not change the answer here, because this format embeds no font programs in v1: every face is referenced by name whatever its outlines are.",
	},

	{Feature: FeatureScriptLatin, Format: artifact.FormatDOCX, Outcome: Renders},
	{Feature: FeatureScriptGreek, Format: artifact.FormatDOCX, Outcome: Renders},
	{Feature: FeatureScriptCyrillic, Format: artifact.FormatDOCX, Outcome: Renders},
	{Feature: FeatureScriptOther, Format: artifact.FormatDOCX, Outcome: Renders,
		Note: "Vellum writes the characters and the application lays them out. It does no shaping or line breaking for this format, so it imposes no restriction on the writing system."},

	{
		Feature: FeatureOverflowContinue, Format: artifact.FormatDOCX, Outcome: Degrades,
		Degrade: "flow", Code: verr.VELLUM_CAPABILITY_DEGRADED,
		Note: "A flowing format paginates itself. Vellum does not lay out OOXML, and a split it computed would disagree with the one Word performs.",
	},
	{Feature: FeatureFill, Format: artifact.FormatDOCX, Outcome: Renders},

	// ---- XLSX: presentation tables. Not a spreadsheet. ----
	{
		Feature: FeatureBlockHeading, Format: artifact.FormatXLSX, Outcome: Degrades,
		Degrade: "a styled cell above the table", Code: verr.VELLUM_CAPABILITY_DEGRADED,
		Note: "A sheet has no heading construct; the text occupies a cell.",
	},
	{
		Feature: FeatureBlockText, Format: artifact.FormatXLSX, Outcome: Degrades,
		Degrade: "a wrapped cell", Code: verr.VELLUM_CAPABILITY_DEGRADED,
	},
	{
		Feature: FeatureBlockAsset, Format: artifact.FormatXLSX, Outcome: Rejects,
		Code: verr.VELLUM_CAPABILITY_REJECTED,
		Note: "Declared unsupported rather than approximated. A workbook is where a reader goes for the numbers behind a chart, not for the chart.",
	},
	{Feature: FeatureBlockTable, Format: artifact.FormatXLSX, Outcome: Renders},
	{
		Feature: FeatureBlockPageBreak, Format: artifact.FormatXLSX, Outcome: Degrades,
		Degrade: "a new sheet", Code: verr.VELLUM_CAPABILITY_DEGRADED,
	},
	{
		Feature: FeatureBlockNotes, Format: artifact.FormatXLSX, Outcome: Degrades,
		Degrade: "a cell comment", Code: verr.VELLUM_CAPABILITY_DEGRADED,
	},
	{
		Feature: FeatureBlockSpacer, Format: artifact.FormatXLSX, Outcome: Degrades,
		Degrade: "a blank row", Code: verr.VELLUM_CAPABILITY_DEGRADED,
	},

	{Feature: FeatureTableHierarchicalHeaders, Format: artifact.FormatXLSX, Outcome: Renders},
	{
		Feature: FeatureTableCellAnnotation, Format: artifact.FormatXLSX, Outcome: Degrades,
		Degrade: "text appended to the cell, with the typed value preserved in a neighbouring column", Code: verr.VELLUM_CAPABILITY_DEGRADED,
		Note: "A cell holds one typed value. Appending the annotation to the number would turn it into text and defeat the reason to export a workbook at all.",
	},
	{Feature: FeatureTableMargins, Format: artifact.FormatXLSX, Outcome: Renders},
	{Feature: FeatureTableCellSpan, Format: artifact.FormatXLSX, Outcome: Renders},

	{Feature: FeatureAssetPNG, Format: artifact.FormatXLSX, Outcome: Rejects, Code: verr.VELLUM_CAPABILITY_REJECTED},
	{Feature: FeatureAssetJPEG, Format: artifact.FormatXLSX, Outcome: Rejects, Code: verr.VELLUM_CAPABILITY_REJECTED},
	{Feature: FeatureAssetSVG, Format: artifact.FormatXLSX, Outcome: Rejects, Code: verr.VELLUM_CAPABILITY_REJECTED},

	{
		Feature: FeatureFontEmbedSubset, Format: artifact.FormatXLSX, Outcome: Degrades,
		Degrade: "the family referenced by name", Code: verr.VELLUM_CAPABILITY_DEGRADED,
		Note: "As DOCX: xlsx references a family by name and never carries the program.",
	},
	{
		Feature: FeatureFontEmbedWhole, Format: artifact.FormatXLSX, Outcome: Degrades,
		Degrade: "the family referenced by name", Code: verr.VELLUM_CAPABILITY_DEGRADED,
		Note: "As above.",
	},

	{
		Feature: FeatureFontOutlinesCFF, Format: artifact.FormatXLSX, Outcome: Degrades,
		Degrade: "the family referenced by name", Code: verr.VELLUM_FONT_SUBSTITUTED,
		Note: "The outline format does not change the answer here, because this format embeds no font programs in v1: every face is referenced by name whatever its outlines are.",
	},

	{Feature: FeatureScriptLatin, Format: artifact.FormatXLSX, Outcome: Renders},
	{Feature: FeatureScriptGreek, Format: artifact.FormatXLSX, Outcome: Renders},
	{Feature: FeatureScriptCyrillic, Format: artifact.FormatXLSX, Outcome: Renders},
	{Feature: FeatureScriptOther, Format: artifact.FormatXLSX, Outcome: Renders,
		Note: "Vellum writes the characters and the application lays them out. It does no shaping or line breaking for this format, so it imposes no restriction on the writing system."},

	{
		Feature: FeatureOverflowContinue, Format: artifact.FormatXLSX, Outcome: Degrades,
		Degrade: "one continuous sheet", Code: verr.VELLUM_CAPABILITY_DEGRADED,
		Note: "A sheet has no page, so a long table simply continues.",
	},
	{Feature: FeatureFill, Format: artifact.FormatXLSX, Outcome: Renders},

	// ---- PPTX: a deck. Notes are native here and nowhere else. ----
	{Feature: FeatureBlockHeading, Format: artifact.FormatPPTX, Outcome: Renders},
	{Feature: FeatureBlockText, Format: artifact.FormatPPTX, Outcome: Renders},
	{Feature: FeatureBlockAsset, Format: artifact.FormatPPTX, Outcome: Renders},
	{Feature: FeatureBlockTable, Format: artifact.FormatPPTX, Outcome: Renders},
	{Feature: FeatureBlockPageBreak, Format: artifact.FormatPPTX, Outcome: Degrades,
		Degrade: "a new slide", Code: verr.VELLUM_CAPABILITY_DEGRADED},
	{Feature: FeatureBlockNotes, Format: artifact.FormatPPTX, Outcome: Renders,
		Note: "The only format with a native speaker-note channel."},
	{Feature: FeatureBlockSpacer, Format: artifact.FormatPPTX, Outcome: Renders},

	{Feature: FeatureTableHierarchicalHeaders, Format: artifact.FormatPPTX, Outcome: Renders},
	{Feature: FeatureTableCellAnnotation, Format: artifact.FormatPPTX, Outcome: Renders},
	{Feature: FeatureTableMargins, Format: artifact.FormatPPTX, Outcome: Renders},
	{Feature: FeatureTableCellSpan, Format: artifact.FormatPPTX, Outcome: Renders},

	{Feature: FeatureAssetPNG, Format: artifact.FormatPPTX, Outcome: Renders},
	{Feature: FeatureAssetJPEG, Format: artifact.FormatPPTX, Outcome: Renders},
	{
		Feature: FeatureAssetSVG, Format: artifact.FormatPPTX, Outcome: Degrades,
		Degrade: "raster fallback with the vector embedded alongside", Code: verr.VELLUM_CAPABILITY_DEGRADED,
	},

	{
		Feature: FeatureFontEmbedSubset, Format: artifact.FormatPPTX, Outcome: Degrades,
		Degrade: "the family referenced by name", Code: verr.VELLUM_CAPABILITY_DEGRADED,
		Note: "As DOCX. A deck is the case where a missing face is most visible, because a slide's type is large and its layout is fixed — which is why the substitution is warned rather than assumed harmless.",
	},
	{
		Feature: FeatureFontEmbedWhole, Format: artifact.FormatPPTX, Outcome: Degrades,
		Degrade: "the family referenced by name", Code: verr.VELLUM_CAPABILITY_DEGRADED,
		Note: "As above.",
	},

	{
		Feature: FeatureFontOutlinesCFF, Format: artifact.FormatPPTX, Outcome: Degrades,
		Degrade: "the family referenced by name", Code: verr.VELLUM_FONT_SUBSTITUTED,
		Note: "The outline format does not change the answer here, because this format embeds no font programs in v1: every face is referenced by name whatever its outlines are.",
	},

	{Feature: FeatureScriptLatin, Format: artifact.FormatPPTX, Outcome: Renders},
	{Feature: FeatureScriptGreek, Format: artifact.FormatPPTX, Outcome: Renders},
	{Feature: FeatureScriptCyrillic, Format: artifact.FormatPPTX, Outcome: Renders},
	{Feature: FeatureScriptOther, Format: artifact.FormatPPTX, Outcome: Renders,
		Note: "Vellum writes the characters and the application lays them out. It does no shaping or line breaking for this format, so it imposes no restriction on the writing system."},

	{Feature: FeatureOverflowContinue, Format: artifact.FormatPPTX, Outcome: Renders,
		Note: "A table longer than a slide continues onto the next with its headers repeated. Capacity is theme-derived rather than measured, so the split is reproducible."},
	{Feature: FeatureFill, Format: artifact.FormatPPTX, Outcome: Renders},

	// ---- PDF: archival output, emitted directly. ----
	{Feature: FeatureBlockHeading, Format: artifact.FormatPDF, Outcome: Renders},
	{Feature: FeatureBlockText, Format: artifact.FormatPDF, Outcome: Renders},
	{Feature: FeatureBlockAsset, Format: artifact.FormatPDF, Outcome: Renders},
	{Feature: FeatureBlockTable, Format: artifact.FormatPDF, Outcome: Renders},
	{Feature: FeatureBlockPageBreak, Format: artifact.FormatPDF, Outcome: Renders},
	{
		Feature: FeatureBlockNotes, Format: artifact.FormatPDF, Outcome: Degrades,
		Degrade: "a footnote", Code: verr.VELLUM_CAPABILITY_DEGRADED,
		Note: "A PDF annotation would be closer in spirit but is not guaranteed visible in every reader, and PDF/A restricts annotation types. A footnote is legible everywhere.",
	},
	{Feature: FeatureBlockSpacer, Format: artifact.FormatPDF, Outcome: Renders},

	{Feature: FeatureTableHierarchicalHeaders, Format: artifact.FormatPDF, Outcome: Renders},
	{Feature: FeatureTableCellAnnotation, Format: artifact.FormatPDF, Outcome: Renders},
	{Feature: FeatureTableMargins, Format: artifact.FormatPDF, Outcome: Renders},
	{Feature: FeatureTableCellSpan, Format: artifact.FormatPDF, Outcome: Renders},

	{Feature: FeatureAssetPNG, Format: artifact.FormatPDF, Outcome: Renders},
	{Feature: FeatureAssetJPEG, Format: artifact.FormatPDF, Outcome: Renders},
	{
		Feature: FeatureAssetSVG, Format: artifact.FormatPDF, Outcome: Rejects,
		Code: verr.VELLUM_ASSET_MEDIA_UNSUPPORTED,
		Note: "PDF has no SVG mechanism. Vellum will not rasterise, because it never renders, and will not ship an SVG-to-PDF translator, which would be a second renderer with its own text layout and font matching, free to drift from whatever produced the asset. Supply a raster, or ask Boxes what size to render at.",
	},

	{Feature: FeatureFontEmbedSubset, Format: artifact.FormatPDF, Outcome: Renders,
		Note: "PDF/A-2b requires every font embedded, and TrueType outlines are subsetted. A face with CFF outlines cannot be subsetted; see font.outlines.cff, which is where that answer lives."},
	{Feature: FeatureFontEmbedWhole, Format: artifact.FormatPDF, Outcome: Renders,
		Note: "The whole program is embedded, which is what the theme asked for. A licence forbidding subsetting is the usual reason to ask."},
	{
		Feature: FeatureFontOutlinesCFF, Format: artifact.FormatPDF, Outcome: Degrades,
		Degrade: "the whole font program embedded rather than a subset", Code: verr.VELLUM_CAPABILITY_DEGRADED,
		Note: "Vellum subsets glyf and not CFF: a CFF subsetter means a second outline format and a charstring interpreter, and the size it saves is not worth the ways it can be subtly wrong. The face is embedded whole and the document is conforming. A theme demanding embed: \"subset\" of a CFF face gets VELLUM_FONT_EMBED_UNSUPPORTED rather than this degradation, because subset-only may be a licence condition and must never be silently ignored.",
	},

	{Feature: FeatureScriptLatin, Format: artifact.FormatPDF, Outcome: Renders},
	{Feature: FeatureScriptGreek, Format: artifact.FormatPDF, Outcome: Renders},
	{Feature: FeatureScriptCyrillic, Format: artifact.FormatPDF, Outcome: Renders},
	{
		Feature: FeatureScriptOther, Format: artifact.FormatPDF, Outcome: Rejects,
		Code: verr.VELLUM_PDF_SCRIPT_UNSUPPORTED,
		Note: "Vellum lays PDF out itself, so it can only promise the systems it has established it shapes and breaks correctly: horizontal left-to-right Latin, Greek and Cyrillic in v1. Anything else is refused at compose time rather than rendered wrong — an unsupported script does not fail to draw, it draws incorrectly, and that is a defect a reader of the language finds and a test does not. Two supported systems mixed in one string is also refused, for the same reason. Compose to an OOXML target, which does its own layout.",
	},

	{Feature: FeatureOverflowContinue, Format: artifact.FormatPDF, Outcome: Renders,
		Note: "Vellum paginates PDF itself, so the policy is honoured exactly."},
	{
		Feature: FeatureFill, Format: artifact.FormatPDF, Outcome: Rejects,
		Code: verr.VELLUM_CAPABILITY_REJECTED,
		Note: "Fill mode edits an OPC package surgically. A PDF is not one, and editing PDF in place is a different problem with none of the same guarantees.",
	},
}
