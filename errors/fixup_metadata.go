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
