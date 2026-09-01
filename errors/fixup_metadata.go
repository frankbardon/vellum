package errors

// codeMetadata is the canonical message and guidance for every registered
// code. One row per code, no exceptions: TestCodesHaveFixups fails the build
// when a code in allCodes has no row here, and when a row has neither a fixup
// nor an explicit FixupNotApplicable.
//
// The hints are written as instructions to whoever must act, not as
// restatements of the error. "Re-export the template from Word" is useful;
// "the package is invalid" is what the message already said.
var codeMetadata = map[Code]Metadata{
	VELLUM_OPC_INVALID: {
		Message: "the file is not a well-formed OPC package",
		Fixups: []Fixup{{
			Action: FixupRepairInput,
			Hint:   "Confirm the file is a .docx, .xlsx or .pptx and not a renamed archive; re-export it from the authoring application if it was assembled by a script.",
		}},
	},

	VELLUM_OPC_PART_NOT_FOUND: {
		Message: "the package does not contain the named part",
		Fixups: []Fixup{{
			Action: FixupRepairInput,
			Hint:   "The package is internally inconsistent — a relationship points at a part that is not present. Re-export the document from the authoring application rather than editing the archive by hand.",
		}},
	},

	VELLUM_OPC_PART_DUPLICATE: {
		Message: "two parts in the package claim the same name",
		Fixups: []Fixup{{
			Action: FixupRepairInput,
			Hint:   "OPC part names are unique. Rebuild the archive; Vellum will not guess which of the two duplicates was intended.",
		}},
	},

	VELLUM_OPC_PART_NAME_INVALID: {
		Message: "the part name is not a valid OPC part name",
		Fixups: []Fixup{{
			Action:   FixupReplaceValue,
			Hint:     "Use an absolute, forward-slash part name with no traversal segments and no trailing slash.",
			Examples: []any{"/word/document.xml", "/xl/worksheets/sheet1.xml", "/ppt/slides/slide1.xml"},
		}},
	},

	VELLUM_OPC_CONTENT_TYPE_MISSING: {
		Message: "a part has no declared content type and none can be derived from its extension",
		Fixups: []Fixup{{
			Action: FixupSetField,
			Path:   []string{"Part", "ContentType"},
			Hint:   "Set the part's content type explicitly. Word and Excel refuse to open a package whose [Content_Types].xml does not cover every part.",
		}},
	},

	VELLUM_OPC_RELATIONSHIP_INVALID: {
		Message: "a relationship has an empty type, an empty target, or an unresolvable internal target",
		Fixups: []Fixup{{
			Action: FixupRepairInput,
			Hint:   "Re-export the document. A relationship whose target is absent means the package was assembled incorrectly, and following it would produce a file the consumer cannot open.",
		}},
	},

	VELLUM_ZIP_MALFORMED: {
		Message: "the archive could not be read",
		Fixups: []Fixup{{
			Action: FixupRepairInput,
			Hint:   "The file is truncated or corrupt. Re-download or re-export it; check that a transfer did not treat it as text.",
		}},
	},

	VELLUM_ZIP_TOO_LARGE: {
		Message: "an archive entry exceeds the configured uncompressed size bound",
		Fixups: []Fixup{
			{
				Action: FixupRaiseLimit,
				Path:   []string{"Options", "MaxPartBytes"},
				Hint:   "Raise the bound if the input is legitimately this large — decks with embedded media routinely are.",
			},
			{
				Action: FixupRepairInput,
				Hint:   "If the input is not expected to be large, treat it as hostile: a small archive declaring an enormous entry is a decompression bomb.",
			},
		},
	},

	VELLUM_ZIP_ENTRY_NAME_INVALID: {
		Message: "an archive entry name is absolute, uses backslashes, or contains a traversal segment",
		Fixups: []Fixup{{
			Action: FixupRepairInput,
			Hint:   "Rebuild the archive with relative forward-slash entry names. A traversal segment is an attack against any consumer that extracts to disk and is refused rather than sanitised.",
		}},
	},

	VELLUM_ZIP_ENTRY_DUPLICATE: {
		Message: "two archive entries share a name",
		Fixups: []Fixup{{
			Action: FixupRepairInput,
			Hint:   "Rebuild the archive. Which of two same-named entries a reader picks is unspecified, so Vellum refuses rather than choosing.",
		}},
	},

	VELLUM_SPEC_INVALID: {
		Message: "the specification is structurally invalid",
		Fixups: []Fixup{{
			Action: FixupSetField,
			Path:   []string{"Sections"},
			Hint:   "A specification needs at least one section, and every section needs at least one block. An empty document is almost always a construction bug rather than an intent.",
		}},
	},

	VELLUM_SPEC_BLOCK_KIND_UNKNOWN: {
		Message: "a block declares a kind that is not in the vocabulary",
		Fixups: []Fixup{{
			Action:   FixupReplaceValue,
			Path:     []string{"Sections", "*", "Blocks", "*", "Kind"},
			Hint:     "Use one of the declared block kinds. Semantic structure is the consumer's vocabulary and is expressed by composing these, not by inventing a kind.",
			Examples: []any{"heading", "text", "asset", "table", "page_break", "notes", "spacer"},
		}},
	},

	VELLUM_CAPABILITY_REJECTED: {
		Message: "the target format does not render this feature",
		Fixups: []Fixup{
			{
				Action: FixupChangeFormat,
				Hint:   "Render to a format that supports the feature. The capability matrix reports every outcome before a render is attempted, so this can be checked rather than discovered.",
			},
			{
				Action: FixupRemoveField,
				Path:   []string{"Sections", "*", "Blocks", "*"},
				Hint:   "Or remove the block. Vellum refuses rather than dropping it, because a silently missing section reaches a reader instead of the author.",
			},
		},
	},

	VELLUM_CAPABILITY_DEGRADED: {
		Message: "the feature became the stated alternative in this format",
		Fixups: []Fixup{{
			Action: FixupChangeFormat,
			Hint:   "No action is needed — this is a warning, reported so that no degradation is ever silent. Render to a format where the feature is native if the alternative will not do.",
		}},
	},

	VELLUM_CAPABILITY_UNDECLARED: {
		Message: "a feature and format pair has no declared outcome",
		// Always a Vellum bug; the completeness gate exists so it cannot reach
		// a release, and no caller input can resolve it.
		FixupNotApplicable: true,
	},

	VELLUM_ASSET_NOT_FOUND: {
		Message: "the resolver has no asset under this handle",
		Fixups: []Fixup{
			{
				Action: FixupSupplyAsset,
				Hint:   "Store the asset under the handle the block names, or correct the handle. Vellum owns no storage and fetches nothing, so it cannot tell a handle that never existed from one that has since been removed.",
			},
			{
				Action: FixupSetField,
				Path:   []string{"Sections", "*", "Blocks", "*", "Asset", "Handle"},
				Hint:   "If the handle is stale, point the block at one the resolver can produce.",
			},
		},
	},

	VELLUM_ASSET_RESOLVE_FAILED: {
		Message: "the host's asset resolver returned an error",
		Fixups: []Fixup{{
			Action: FixupRepairInput,
			Hint:   "The failure is inside the resolver this host wired, not inside Vellum. Unwrap the error to the resolver's own type for the cause; it is deliberately not serialised into the envelope, because a host's error prose can carry paths and credentials.",
		}},
	},

	VELLUM_ASSET_MEDIA_UNKNOWN: {
		Message: "the asset's media type was not declared and its bytes match no known signature",
		Fixups: []Fixup{{
			Action:   FixupSetField,
			Path:     []string{"Asset", "MediaType"},
			Hint:     "Return an explicit media type from the resolver. Vellum will not guess from the handle: the handle is the host's opaque identifier and may not be a filename, so a guess drawn from it would be a guess about the host's naming convention rather than about the content.",
			Examples: []any{"image/png", "image/jpeg", "image/svg+xml"},
		}},
	},

	VELLUM_ASSET_TOO_LARGE: {
		Message: "the resolved asset exceeds the configured per-asset size bound",
		Fixups: []Fixup{
			{
				Action: FixupSupplyAsset,
				Hint:   "Supply a smaller asset. Boxes reports the size the theme will render it at, so an asset far above that resolution is paying for detail the document cannot show.",
			},
			{
				Action: FixupRaiseLimit,
				Hint:   "Raise the bound with VELLUM_MAX_ASSET_BYTES, or with Options.MaxAssetBytes, if the asset is legitimately this large.",
			},
		},
	},

	VELLUM_ASSET_MEDIA_MISMATCH: {
		Message: "the asset's declared media type contradicts its own bytes",
		Fixups: []Fixup{
			{
				Action:   FixupSetField,
				Path:     []string{"Asset", "MediaType"},
				Hint:     "Return the media type the bytes actually are, or return none and let Vellum sniff them. Vellum will not write bytes into a package under a content type they do not match: a reader that checks refuses the whole document, and the failure then reads as \"this file is corrupt\" several layers from the mistake.",
				Examples: []any{"image/png", "image/jpeg", "image/svg+xml"},
			},
			{
				Action: FixupSupplyAsset,
				Hint:   "If the declaration is right, the stored bytes are the wrong ones. Re-upload the asset.",
			},
		},
	},

	VELLUM_THEME_NOT_FOUND: {
		Message: "the provider has no theme under this id",
		Fixups: []Fixup{
			{
				Action: FixupSupplyTheme,
				Hint:   "Register the theme with the provider, or leave the specification's theme empty to select the built-in one. Vellum will not silently fall back to the built-in theme: a document rendered against a theme it did not ask for is wrong in a way that looks right.",
			},
			{
				Action: FixupSetField,
				Path:   []string{"Theme"},
				Hint:   "Correct the theme id if it is a typo.",
			},
		},
	},

	VELLUM_THEME_INVALID: {
		Message: "the theme document is structurally invalid",
		Fixups: []Fixup{{
			Action: FixupRepairInput,
			Hint:   "The error's details name the offending field. Validate the theme against the published theme schema before storing it, so a broken theme fails where it is authored rather than on every render that uses it.",
		}},
	},

	VELLUM_THEME_LAYOUT_NOT_FOUND: {
		Message: "the theme declares no layout under this id for the target format",
		Fixups: []Fixup{
			{
				Action: FixupSetField,
				Path:   []string{"Sections", "*", "Layout"},
				Hint:   "Name a layout the theme declares for this format, or leave it empty to select the format's default layout. A theme may legitimately carry a layout for one format and not another, so the target format is part of the lookup.",
			},
			{
				Action: FixupSupplyTheme,
				Hint:   "Add the layout to the theme for this format.",
			},
		},
	},

	VELLUM_THEME_BOX_NOT_FOUND: {
		Message: "the resolved layout declares no box under this role",
		Fixups: []Fixup{
			{
				Action: FixupSetField,
				Path:   []string{"Sections", "*", "Blocks", "*", "Asset", "Role"},
				Hint:   "Name a role the layout declares, or leave it empty to select the default asset box. Call Boxes to enumerate the set; it answers before a specification exists, so the roles can be read once and cached against the theme.",
			},
			{
				Action: FixupSupplyTheme,
				Hint:   "Add the box to the layout if the role is one the theme should offer.",
			},
		},
	},

	VELLUM_MARK_UNKNOWN: {
		Message: "the theme declares no style for this mark, so the marked content rendered unstyled",
		Fixups: []Fixup{
			{
				Action: FixupSupplyTheme,
				Hint:   "Add a mark style to the theme if the mark is meant to be visible. This is a warning rather than an error because marks are the consumer's vocabulary and a theme need not style every one — but an invisible flag is indistinguishable from no flag, which is why it is never silent.",
			},
			{
				Action: FixupRemoveField,
				Path:   []string{"Sections", "*", "Blocks", "*", "Marks"},
				Hint:   "Remove the mark if it was not meant to reach the document.",
			},
		},
	},

	VELLUM_FONT_NOT_EMBEDDABLE: {
		Message: "the theme declares this font non-embeddable and names no substitute",
		Fixups: []Fixup{
			{
				Action:   FixupSetField,
				Path:     []string{"Fonts", "*", "Substitute"},
				Hint:     "Name the family to use in its place. Vellum will not fall back to whatever the machine has installed: a silent system fallback is exactly how one specification comes to render differently on two machines, which defeats byte-identical output and the consumer dedupe resting on it.",
				Examples: []any{"Arial", "Helvetica", "Georgia"},
			},
			{
				Action: FixupSetField,
				Path:   []string{"Fonts", "*", "Embeddable"},
				Hint:   "Set embeddable:true if the licence in fact permits embedding. Whoever authors the theme knows the licence; Vellum cannot, so it declines to guess.",
			},
		},
	},

	VELLUM_FONT_EMBED_UNSUPPORTED: {
		Message: "the theme demands an embedding mode Vellum cannot deliver for this face",
		Fixups: []Fixup{
			{
				Action: FixupSupplyAsset,
				Hint:   "Supply a TrueType-outlined build of the face. v1 subsets glyf outlines; CFF outlines embed whole.",
			},
			{
				Action:   FixupSetField,
				Path:     []string{"Fonts", "*", "Embed"},
				Hint:     "Relax the mode to \"auto\" if embedding the whole program is acceptable for this licence. Vellum will not relax it itself, because subset-only may be a licence condition and silently embedding the whole program would substitute its judgement for the vendor's.",
				Examples: []any{"auto", "whole"},
			},
		},
	},

	VELLUM_FONT_UNAVAILABLE: {
		Message: "the theme declares this font embeddable but its face could not be obtained",
		Fixups: []Fixup{
			{
				Action: FixupSetField,
				Path:   []string{"Fonts", "*", "Handle"},
				Hint:   "Give the font a handle the asset resolver can produce. A face declared embeddable with no handle is a promise the theme cannot keep.",
			},
			{
				Action: FixupSupplyAsset,
				Hint:   "Store the font file under that handle.",
			},
		},
	},

	VELLUM_PDF_OBJECT_UNRESOLVED: {
		Message:            "the document holds a reference to an object that was never supplied",
		FixupNotApplicable: true,
	},

	VELLUM_PDF_STREAM_INVALID: {
		Message:            "the stream could not be produced",
		FixupNotApplicable: true,
	},

	VELLUM_PDF_WRITE_FAILED: {
		Message:            "writing the document to its destination failed",
		FixupNotApplicable: true,
	},

	VELLUM_PDF_FONT_INVALID: {
		Message: "the font program could not be parsed or subsetted",
		Fixups: []Fixup{
			{
				Action: FixupSupplyAsset,
				Hint:   "Supply a TrueType or OpenType font with glyf outlines under this handle. Vellum subsets glyf; a CFF face is embedded whole, and a face that is neither is not a font program Vellum can read.",
			},
			{
				Action:   FixupSetField,
				Path:     []string{"Fonts", "*", "Embeddable"},
				Hint:     "Declare the face non-embeddable and name a substitute, if the program cannot be supplied.",
				Examples: []any{false},
			},
		},
	},

	VELLUM_PDF_GLYPH_MISSING: {
		Message: "the selected face has no glyph for a character in the text",
		Fixups: []Fixup{
			{
				Action: FixupSetField,
				Path:   []string{"Fonts", "*", "Handle"},
				Hint:   "Point the role at a face whose coverage includes this text. Vellum will not fall back to another installed font: a system fallback makes the same document render differently on two machines.",
			},
		},
	},

	VELLUM_PDF_SCRIPT_UNSUPPORTED: {
		Message: "the text is in a writing system Vellum does not lay out",
		Fixups: []Fixup{
			{
				Action: FixupSetField,
				Path:   []string{"Sections", "*", "Blocks", "*", "Text", "Content"},
				Hint:   "Vellum lays out horizontal left-to-right Latin, Greek and Cyrillic in v1. A script it has not established it can shape and break correctly is refused rather than rendered subtly wrong, because a reader notices that and a test does not.",
			},
			{
				Action:   FixupSetField,
				Path:     []string{"Format"},
				Hint:     "An OOXML target has no such restriction: Word does its own shaping, so the same text composes to .docx, .xlsx or .pptx.",
				Examples: []any{"docx", "pptx"},
			},
		},
	},

	VELLUM_PDF_TEXT_OVERFLOW: {
		Message: "a piece of text with no break opportunity is wider than the space available",
		Fixups: []Fixup{
			{
				Action: FixupSetField,
				Path:   []string{"Sections", "*", "Blocks", "*", "Text", "Content"},
				Hint:   "Give the text somewhere to break — a hyphen, a space, or a zero-width space in a long identifier. Vellum will not break mid-word: a break Unicode does not permit is one no reader would have made.",
			},
			{
				Action: FixupSetField,
				Path:   []string{"Theme"},
				Hint:   "Or widen the box the text is set in, in the theme's layout for this format.",
			},
		},
	},

	VELLUM_PDF_IMAGE_INVALID: {
		Message: "the asset's bytes do not parse as the image format they claim",
		Fixups: []Fixup{{
			Action: FixupSetField,
			Path:   []string{"Sections", "*", "Blocks", "*", "Asset", "Handle"},
			Hint:   "The bytes matched the format's signature and then failed the full read, which usually means a truncated or partially written file. Re-export the asset and check its byte length end to end.",
		}},
	},

	VELLUM_PDFA_NONCONFORMANT: {
		Message: "the assembled document would violate ISO 19005-2 level B",
		// No fixup, and deliberately so. Every conformance question a
		// specification can decide — whether a face is embeddable, whether an
		// image variant can be carried — is answered before the document is
		// assembled, with a code that names the field to change. Reaching this
		// one means Vellum built something the standard forbids, and there is
		// nothing in the specification for a consumer to edit.
		FixupNotApplicable: true,
	},

	VELLUM_PDF_IMAGE_UNSUPPORTED: {
		Message: "the image is in an encoding variant a PDF cannot carry unmodified",
		Fixups: []Fixup{
			{
				Action:   FixupSetField,
				Path:     []string{"Sections", "*", "Blocks", "*", "Asset", "Handle"},
				Hint:     "Save the asset in the plain form of the same format: a non-interlaced PNG, or a baseline RGB or greyscale JPEG. Vellum embeds the encoded bytes it is handed rather than decoding and recompressing them, so the variant has to be chosen where the asset is produced.",
				Examples: []any{"non-interlaced PNG", "baseline JPEG"},
			},
			{
				Action:   FixupSetField,
				Path:     []string{"Format"},
				Hint:     "An OOXML target has no such restriction: it stores the asset as a package part and the application decodes it, so every variant of PNG and JPEG passes through.",
				Examples: []any{"docx", "pptx"},
			},
		},
	},

	VELLUM_TABLE_FORMAT_INVALID: {
		Message: "the cell's number-format code does not parse",
		Fixups: []Fixup{{
			Action:   FixupChangeFormat,
			Path:     []string{"Sections", "*", "Blocks", "*", "Table", "Body", "*", "*", "Format"},
			Hint:     "Use an xlsx number-format code. That vocabulary is used for all four targets, so there is no second dialect to learn and no per-format drift.",
			Examples: []any{"0.0%", "#,##0", "yyyy-mm-dd", `_("$"* #,##0.00_)`},
		}},
	},

	VELLUM_ASSET_MEDIA_UNSUPPORTED: {
		Message: "the target format cannot embed an asset of this media type",
		Fixups: []Fixup{
			{
				Action: FixupSupplyAsset,
				Hint:   "Supply the asset in a media type the format accepts. The error names the accepted set, and Boxes reports the size to render at.",
			},
			{
				Action:   FixupSupplyAsset,
				Hint:     "For an SVG destined for a format that also accepts raster, supply a raster fallback alongside it; Vellum embeds the pair. It will not rasterise one for you — it never renders.",
				Examples: []any{"image/png", "image/jpeg"},
			},
		},
	},

	VELLUM_TABLE_INVALID: {
		Message: "the table is structurally invalid",
		Fixups: []Fixup{{
			Action: FixupSetField,
			Path:   []string{"Sections", "*", "Blocks", "*", "Table"},
			Hint:   "A table needs a body. Spans are counts, so they are at least one, and a cell carries either a typed value or text but is not required to carry both.",
		}},
	},

	VELLUM_TABLE_HEADER_SPAN_MISMATCH: {
		Message: "a header node's declared span does not match the total span of its children",
		Fixups: []Fixup{{
			Action: FixupSetField,
			Path:   []string{"Sections", "*", "Blocks", "*", "Table", "ColumnHeaders", "*", "Span"},
			Hint:   "Omit the span and let it be derived from the children, or set it to their total. A banner that does not tile its axis renders as a table with a hole in it, which reads as deliberate.",
		}},
	},

	VELLUM_TABLE_SPAN_OVERLAP: {
		Message: "two cells claim the same grid position, or a span runs past the edge of the table",
		Fixups: []Fixup{{
			Action: FixupSetField,
			Path:   []string{"Sections", "*", "Blocks", "*", "Table", "Body", "*", "*"},
			Hint:   "Reduce the row or column span, or remove the cell it collides with. The reported coordinates are the first collision, not necessarily the only one.",
		}},
	},

	VELLUM_TABLE_ROW_ARITY: {
		Message: "a row does not cover exactly the width the column headers declare",
		Fixups: []Fixup{{
			Action: FixupSetField,
			Path:   []string{"Sections", "*", "Blocks", "*", "Table", "Body", "*"},
			Hint:   "Add or remove cells so the row's total column span equals the header width. A short row is usually a missing margin column rather than a header that is too wide.",
		}},
	},

	VELLUM_DOC_BLOCK_UNSUPPORTED: {
		Message: "the DOCX writer does not render this block kind yet",
		Fixups: []Fixup{{
			Action: FixupRemoveField,
			Path:   []string{"Sections", "*", "Blocks", "*"},
			Hint:   "Remove the block, or target a format that supports it. The capability matrix reports which blocks render in which formats before a render is attempted, so this can be checked rather than discovered.",
		}},
	},

	VELLUM_OVERFLOW_NO_CAPACITY: {
		Message: "the container cannot hold even one row beneath its repeated headers",
		Fixups: []Fixup{
			{
				Action:   FixupSetField,
				Path:     []string{"Sections", "*", "Blocks", "*", "Table", "ColumnHeaders"},
				Hint:     "The repeated header banner is taller than the container. Flatten the header hierarchy — each level of nesting is another row repeated on every slide — or shorten it to fewer levels.",
				Examples: []any{"one banner level instead of three"},
			},
			{
				Action: FixupSetField,
				Path:   []string{"Format"},
				Hint:   "A flowing format has no fixed container height, so a table that cannot fit a slide fits a document page.",
				Examples: []any{"docx", "pdf"},
			},
		},
	},

	VELLUM_DECK_INVALID: {
		Message: "the deck cannot be serialised as PresentationML",
		// No fixup on the specification, and deliberately so. Nothing a
		// consumer can write in a spec produces one of these: the lowering
		// builds the masters, layouts and media table itself, so a dangling
		// layout reference or a picture indexing past the media is Vellum
		// having assembled a deck wrong. A consumer driving the deck model
		// directly gets the detail keys, which name the slide and the shape.
		FixupNotApplicable: true,
	},

	VELLUM_DECK_BLOCK_UNSUPPORTED: {
		Message: "the PPTX writer does not render this block kind yet",
		Fixups: []Fixup{{
			Action: FixupRemoveField,
			Path:   []string{"Sections", "*", "Blocks", "*"},
			Hint:   "Remove the block, or target a format that supports it. The capability matrix reports which blocks render in which formats before a render is attempted, so this can be checked rather than discovered.",
		}},
	},

	VELLUM_FONT_SUBSTITUTED: {
		Message: "a font was replaced by its declared substitute because the theme marks it as not embeddable",
		Fixups: []Fixup{
			{
				Action: FixupSupplyTheme,
				Path:   []string{"Theme", "Fonts", "*", "Embeddable"},
				Hint:   "If the licence permits embedding, set embeddable to true and supply the font file, and the substitution stops.",
			},
			{
				Action: FixupSupplyTheme,
				Path:   []string{"Theme", "Fonts", "*", "Substitute"},
				Hint:   "If the substitution is expected, no action is needed — this is a warning, reported so that no substitution is ever silent.",
			},
		},
	},

	VELLUM_INTERNAL_INVARIANT: {
		Message: "an internal invariant was violated",
		// No input change resolves this. It is always a Vellum bug, and
		// offering the author a field to edit would be misdirection.
		FixupNotApplicable: true,
	},
}
