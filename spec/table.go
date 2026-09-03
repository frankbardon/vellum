package spec

import verr "github.com/frankbardon/vellum/errors"

// Table is an analytical table.
//
// A table is not a two-dimensional array of strings, and the model that treats
// it as one is the model that has to be thrown away. Four properties an
// analytical table actually carries, all first-class here:
//
//   - hierarchical headers with spanning cells on both axes, because nested
//     grouping variables produce multi-level banners;
//   - cell-level annotations that attach to a value rather than replacing it —
//     a significance letter beside a number, not instead of it;
//   - margins and totals distinguishable from data, so a renderer can style a
//     total row without string-matching on the word "Total";
//   - per-cell marks, so a consumer can flag a cell without Vellum learning
//     what the flag means.
//
// Vellum computes none of it. Significance letters, margins and low-base flags
// arrive already resolved.
type Table struct {
	// ColumnHeaders is the column banner, as a tree. Nested grouping variables
	// produce multiple levels.
	ColumnHeaders HeaderTree `json:"column_headers,omitempty"`

	// RowHeaders is the row stub, as a tree.
	RowHeaders HeaderTree `json:"row_headers,omitempty"`

	// Body is the data grid, row-major.
	Body [][]Cell `json:"body"`

	// Caption is the table's caption.
	Caption string `json:"caption,omitempty"`

	// Marks are consumer-defined style hooks for the table as a whole.
	Marks []string `json:"marks,omitempty"`
}

// HeaderNode is one node of a header tree.
type HeaderNode struct {
	// Label is the node's text.
	Label string `json:"label"`

	// Span is how many leaf positions this node covers. Zero means "derive
	// it": one for a leaf, the total of the children for a parent. Stating it
	// explicitly is allowed and is checked against the children, so a
	// disagreement is an error rather than a silently misaligned banner.
	Span int `json:"span,omitempty"`

	// Children are the nodes beneath this one.
	Children []HeaderNode `json:"children,omitempty"`

	// Marks are consumer-defined style hooks.
	Marks []string `json:"marks,omitempty"`
}

// HeaderTree is an ordered forest of header nodes forming one axis banner.
type HeaderTree []HeaderNode

// CellClass distinguishes a cell's role from its content.
//
// The point of the flag is that a renderer can give a total row a different
// theme role without inspecting its label. String-matching on "Total" works
// until the document is in another language, or until a data row happens to be
// called that.
type CellClass string

const (
	// CellBody is ordinary data. The zero value, so an unclassified cell is
	// data.
	CellBody CellClass = ""

	// CellMargin is a marginal figure — a row or column summarising across one
	// axis.
	CellMargin CellClass = "margin"

	// CellTotal is a grand total.
	CellTotal CellClass = "total"

	// CellHeader is a header rendered inside the body, for a stub column that
	// is structurally part of the grid.
	CellHeader CellClass = "header"
)

// AllCellClasses returns the cell classes, in declaration order.
func AllCellClasses() []CellClass {
	return []CellClass{CellBody, CellMargin, CellTotal, CellHeader}
}

// ValueKind names which arm of a [Value] carries content.
type ValueKind string

const (
	// ValueEmpty is a cell with no value.
	ValueEmpty ValueKind = ""

	// ValueNumber is a numeric value, formatted by the cell's format code.
	ValueNumber ValueKind = "number"

	// ValueText is a textual value.
	ValueText ValueKind = "text"

	// ValueBool is a boolean value.
	ValueBool ValueKind = "bool"

	// ValueDate is a date, in RFC 3339 form.
	ValueDate ValueKind = "date"
)

// AllValueKinds returns the value kinds, in declaration order.
func AllValueKinds() []ValueKind {
	return []ValueKind{ValueEmpty, ValueNumber, ValueText, ValueBool, ValueDate}
}

// Value is a cell's typed value.
//
// A closed struct rather than an any-typed field, deliberately. An interface
// value would round-trip through JSON as whatever the decoder felt like —
// every number a float64 — and would make the canonical hash depend on decode
// details rather than on content.
type Value struct {
	// Kind names which arm carries content.
	Kind ValueKind `json:"kind"`

	// Number is set when Kind is ValueNumber.
	Number float64 `json:"number,omitempty"`

	// Text is set when Kind is ValueText.
	Text string `json:"text,omitempty"`

	// Bool is set when Kind is ValueBool.
	Bool bool `json:"bool,omitempty"`

	// Date is set when Kind is ValueDate, in RFC 3339 form.
	Date string `json:"date,omitempty"`
}

// Cell is one grid position.
type Cell struct {
	// Value is the typed value. Spreadsheet targets prefer it, with Format
	// applied; flowing targets prefer Text when set.
	Value *Value `json:"value,omitempty"`

	// Text is the rendered representation, when the consumer has already
	// formatted the value or the cell is purely textual.
	Text string `json:"text,omitempty"`

	// Format is an xlsx number-format code — one formatting vocabulary across
	// every target, so there is no second dialect to learn or to drift.
	Format string `json:"format,omitempty"`

	// Annotations attach to the value rather than replacing it.
	Annotations []Annotation `json:"annotations,omitempty"`

	// RowSpan and ColSpan are counts, so the natural value is one. Zero means
	// one, so an unspanned cell needs no ceremony.
	RowSpan int `json:"row_span,omitempty"`
	ColSpan int `json:"col_span,omitempty"`

	// Class distinguishes the cell's role from its content.
	Class CellClass `json:"class,omitempty"`

	// Marks are consumer-defined style hooks.
	Marks []string `json:"marks,omitempty"`
}

// AnnotationPosition says where an annotation sits relative to its value.
type AnnotationPosition string

const (
	// AnnotationSuperscript renders raised after the value — the conventional
	// place for a significance letter.
	AnnotationSuperscript AnnotationPosition = "superscript"

	// AnnotationSuffix renders inline after the value.
	AnnotationSuffix AnnotationPosition = "suffix"

	// AnnotationPrefix renders inline before the value.
	AnnotationPrefix AnnotationPosition = "prefix"

	// AnnotationNote renders away from the value, as a footnote or comment
	// depending on format.
	AnnotationNote AnnotationPosition = "note"
)

// AllAnnotationPositions returns the positions, in declaration order.
func AllAnnotationPositions() []AnnotationPosition {
	return []AnnotationPosition{AnnotationSuperscript, AnnotationSuffix, AnnotationPrefix, AnnotationNote}
}

// Annotation is a marker attached to a cell's value.
type Annotation struct {
	// Text is the annotation's content — a significance letter, a footnote
	// marker.
	Text string `json:"text"`

	// Position says where it sits. Empty means superscript, the conventional
	// default.
	Position AnnotationPosition `json:"position,omitempty"`

	// Marks are consumer-defined style hooks.
	Marks []string `json:"marks,omitempty"`
}

// span returns a cell's effective row and column span, treating zero as one.
func (c *Cell) span() (rows, cols int) {
	rows, cols = c.RowSpan, c.ColSpan
	if rows == 0 {
		rows = 1
	}
	if cols == 0 {
		cols = 1
	}
	return rows, cols
}

// Width returns the number of leaf positions a header tree covers.
//
// A node with an explicit span is checked against its children rather than
// trusted, because a banner that does not tile its axis renders as a table
// with a hole in it — and a hole looks deliberate in a way a refusal does not.
func (t HeaderTree) Width() (int, error) {
	total := 0
	for i := range t {
		w, err := t[i].width(nil)
		if err != nil {
			return 0, err
		}
		total += w
	}
	return total, nil
}

// Width returns this node's own effective span: its declared Span when
// non-zero (checked against its children, for a parent), or the derived
// value when Span is left at zero — one for a leaf, the sum of the
// children's own derived widths for a parent. It is the same derivation
// [HeaderTree.Width] applies across a forest, exposed per node so a caller
// building a downstream tree can carry the derived value forward rather than
// a zero a reader would otherwise clamp.
func (n *HeaderNode) Width() (int, error) {
	return n.width(nil)
}

func (n *HeaderNode) width(path []string) (int, error) {
	path = append(path, n.Label)

	if n.Span < 0 {
		return 0, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_INVALID,
			"header span is negative",
			map[string]any{"header_path": joinPath(path), "span": n.Span})
	}

	if len(n.Children) == 0 {
		if n.Span == 0 {
			return 1, nil
		}
		return n.Span, nil
	}

	sum := 0
	for i := range n.Children {
		w, err := n.Children[i].width(path)
		if err != nil {
			return 0, err
		}
		sum += w
	}
	if n.Span != 0 && n.Span != sum {
		return 0, verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_HEADER_SPAN_MISMATCH,
			"header node's declared span does not match the total span of its children",
			map[string]any{"header_path": joinPath(path), "declared_span": n.Span, "children_span": sum})
	}
	return sum, nil
}

func joinPath(path []string) string {
	out := ""
	for i, p := range path {
		if i > 0 {
			out += " / "
		}
		out += p
	}
	return out
}

// Validate checks the table's structure.
//
// It verifies that the header trees tile their axes, that every row covers
// exactly the declared width, and that no two cells claim the same grid
// position. The last of those needs a real occupancy check rather than an
// arithmetic one, because a row span reaches into rows that have not been read
// yet.
func (t *Table) Validate() error {
	if t == nil {
		return verr.NewCodedError(verr.VELLUM_TABLE_INVALID, "table is nil")
	}
	if len(t.Body) == 0 {
		return verr.NewCodedError(verr.VELLUM_TABLE_INVALID, "table has no body rows")
	}

	width, err := t.ColumnHeaders.Width()
	if err != nil {
		return err
	}
	if _, err := t.RowHeaders.Width(); err != nil {
		return err
	}

	if width == 0 {
		// No column headers: the width is whatever the widest row covers, so a
		// caption-only or header-less table is still expressible.
		for r := range t.Body {
			rowWidth := 0
			for c := range t.Body[r] {
				_, cols := t.Body[r][c].span()
				rowWidth += cols
			}
			if rowWidth > width {
				width = rowWidth
			}
		}
	}

	return t.checkOccupancy(width)
}

// checkOccupancy places every cell on a grid and reports the first collision.
//
// Cells are placed left to right into the first free column of their row,
// which is how a table with row spans is read: a spanned cell occupies the
// position beneath it, and the next row's cells flow around it.
func (t *Table) checkOccupancy(width int) error {
	occupied := make([]map[int]bool, len(t.Body))
	for i := range occupied {
		occupied[i] = make(map[int]bool)
	}

	for r := range t.Body {
		col := 0
		for c := range t.Body[r] {
			cell := &t.Body[r][c]
			if cell.RowSpan < 0 || cell.ColSpan < 0 {
				return verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_INVALID,
					"cell span is negative",
					map[string]any{"row": r, "cell": c, "row_span": cell.RowSpan, "col_span": cell.ColSpan})
			}
			rows, cols := cell.span()

			for occupied[r][col] {
				col++
			}
			if col+cols > width {
				return verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_SPAN_OVERLAP,
					"cell extends past the right edge of the table",
					map[string]any{"row": r, "cell": c, "column": col, "col_span": cols, "table_width": width})
			}
			if r+rows > len(t.Body) {
				return verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_SPAN_OVERLAP,
					"cell extends past the last row of the table",
					map[string]any{"row": r, "cell": c, "row_span": rows, "table_rows": len(t.Body)})
			}

			for dr := range rows {
				for dc := range cols {
					if occupied[r+dr][col+dc] {
						return verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_SPAN_OVERLAP,
							"two cells claim the same grid position",
							map[string]any{"row": r, "cell": c, "grid_row": r + dr, "grid_column": col + dc})
					}
					occupied[r+dr][col+dc] = true
				}
			}
			col += cols
		}
	}

	for r := range occupied {
		if len(occupied[r]) != width {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_TABLE_ROW_ARITY,
				"row does not cover exactly the declared table width",
				map[string]any{"row": r, "covered": len(occupied[r]), "table_width": width})
		}
	}
	return nil
}
