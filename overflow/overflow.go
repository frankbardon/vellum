// Package overflow declares what happens to content that does not fit the
// container it was given, and computes the split.
//
// # Declared, not measured
//
// Capacity here is derived from the theme's own measurements — a container
// height, a row height, the number of header rows — and never from asking a
// reader how tall anything actually is. Vellum does not lay out OOXML: Word and
// PowerPoint do, with the fonts installed on the machine that opens the file.
// A capacity measured that way would put the installed fonts into the split,
// so the same specification would break across slides differently on two
// machines and the artifact would stop being reproducible.
//
// The price is that the split is approximate. A theme whose row height
// overstates what a row occupies leaves white space at the foot of a slide, and
// one that understates it overflows. That is a theme's error to make and a
// theme's error to correct, and it is visible in the same place on every
// machine — which is the property that matters.
//
// # Greedy, and why it stays greedy
//
// Rows fill each container to capacity before the next begins. The obvious
// improvement is to balance them, so eleven rows across a capacity of ten
// become six and five rather than ten and one.
//
// It is not taken, because balancing makes every container's contents a
// function of the total. Adding one row to the end of a table reflows every
// container before it, and a deck regenerated after an author appended a row is
// a deck where every slide changed. Greedy keeps the first container's rows the
// same whatever comes after them.
//
// This package imports nothing from Vellum except the error registry, and knows
// nothing about slides, pages or XML.
package overflow

import (
	verr "github.com/frankbardon/vellum/errors"
)

// Policy is the declared answer to content that does not fit.
//
// One value in v1, and it is still an enumerated type rather than an implicit
// behaviour: a consumer scheduling an unattended job has to be able to learn
// what will happen before the job runs, and "whatever the writer does" is not
// something they can learn.
type Policy string

const (
	// Continue moves the remainder to another container of the same kind, with
	// the header rows repeated at the top of each.
	Continue Policy = "continue"
)

var allPolicies = []Policy{Continue}

// AllPolicies returns the policies, in declaration order.
func AllPolicies() []Policy { return append([]Policy(nil), allPolicies...) }

// Table describes a table to be split, in EMU.
//
// Every field is a measurement the theme already decided. Nothing here is
// measured from content, and nothing is optional in a way that would let a
// caller omit the number that decides the answer.
type Table struct {
	// Rows is the number of body rows to place.
	Rows int

	// HeaderRows is how many rows repeat at the top of every container.
	HeaderRows int

	// RowHeight is one body row's height.
	RowHeight int64

	// HeaderHeight is one header row's height. Header rows are usually taller
	// than body rows, so they are measured separately rather than assumed
	// equal — assuming equal is how a three-level banner comes to overflow by
	// exactly the amount it is taller.
	HeaderHeight int64

	// Available is the container's usable height.
	Available int64

	// MinRows is the fewest body rows a container may carry. Zero means one.
	//
	// It exists so a caller can refuse a split that produces containers too
	// sparse to be worth turning to. Below it the split fails rather than
	// producing them.
	MinRows int
}

// Split is one container's share of a table.
type Split struct {
	// Index is the container's ordinal, from zero.
	Index int

	// From is the first body row this container carries.
	From int

	// Count is how many body rows it carries.
	Count int
}

// Last reports whether this is the final container of the table.
func (s Split) Last(total int) bool { return s.From+s.Count >= total }

// PlanTable computes the split.
//
// A table that fits still returns one split. The result is the report a
// consumer reads, and a report that appears only when something went wrong is
// one nobody can distinguish from a missing report.
func PlanTable(t Table) ([]Split, error) {
	minimum := t.MinRows
	if minimum < 1 {
		minimum = 1
	}

	capacity, err := t.capacity()
	if err != nil {
		return nil, err
	}
	if capacity < minimum {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_OVERFLOW_NO_CAPACITY,
			"the container cannot hold even one row beneath its repeated headers",
			map[string]any{
				"available_emu":     t.Available,
				"header_rows":       t.HeaderRows,
				"header_height_emu": t.HeaderHeight,
				"row_height_emu":    t.RowHeight,
				"rows_that_fit":     capacity,
				"minimum_rows":      minimum,
			})
	}

	if t.Rows <= 0 {
		// A table of headers alone still occupies one container. Returning no
		// splits would make the caller decide what an empty table means, and
		// two callers would decide differently.
		return []Split{{Index: 0, From: 0, Count: 0}}, nil
	}

	// The greedy fill itself lives in [PlanRows], so a table measured once and
	// a table measured row by row cannot disagree about where a boundary falls.
	// The capacity check above stays here because it is an answer this shape can
	// give and the measured one cannot: a single row height either fits or it
	// does not, whatever the rows happen to contain.
	heights := make([]int64, t.Rows)
	for i := range heights {
		heights[i] = t.RowHeight
	}
	headers := int64(0)
	if t.HeaderRows > 0 {
		headers = int64(t.HeaderRows) * t.HeaderHeight
	}
	return PlanRows(Rows{
		Heights:      heights,
		HeaderHeight: headers,
		Available:    t.Available,
		MinRows:      minimum,
	})
}

// capacity is how many body rows fit beneath the repeated headers.
func (t Table) capacity() (int, error) {
	if t.RowHeight <= 0 {
		return 0, verr.NewCodedErrorWithDetails(verr.VELLUM_OVERFLOW_NO_CAPACITY,
			"the table declares no row height, so no number of rows fits any container",
			map[string]any{"row_height_emu": t.RowHeight})
	}

	headers := int64(0)
	if t.HeaderRows > 0 {
		headers = int64(t.HeaderRows) * t.HeaderHeight
	}

	free := t.Available - headers
	if free <= 0 {
		return 0, nil
	}
	return int(free / t.RowHeight), nil
}

// Rows describes a table whose body rows have already been measured, one by
// one.
//
// Lengths here are in whatever unit the caller measures in — EMU for the OOXML
// writers, thousandths of a point for PDF — and the only requirement is that
// one call uses one of them. Converting to a common unit would round, and a
// planner that rounds hands back a capacity for rows of a height nobody draws,
// which is the exact failure this type exists to remove.
//
// Where [Table] states a single row height for the whole table, this states one
// per row. The distinction is not a convenience. A format that lays its own
// text out — which is PDF, and only PDF — knows how tall each row actually
// came out, and flattening that back to one height would leave white space at
// the foot of some containers and overflow at others for no reason but the
// shape of this struct.
//
// The declared-not-measured rule in this package's doc comment is not weakened
// by that. It is a rule about who does the laying out: Vellum does not lay out
// OOXML, so it may not measure there. It lays out PDF completely, so the
// measurement is its own and is the same on every machine.
type Rows struct {
	// Heights is one entry per body row, in row order.
	Heights []int64

	// HeaderHeight is the total height the repeated headers occupy at the top
	// of every container.
	//
	// A total rather than a count multiplied by a height, because the levels of
	// a multi-level banner are not the same height as each other once each has
	// been measured — which is the whole reason this type exists.
	HeaderHeight int64

	// Available is a container's usable height.
	Available int64

	// First is the first container's usable height, when it differs. Zero means
	// it does not.
	//
	// A table beginning part way down a page has less room than the pages that
	// follow it. Without this the only honest split is to start every table
	// that does not fit entirely on a container of its own, which strands the
	// heading above it alone at the foot of the page before — a widow the
	// author did not write and cannot remove.
	First int64

	// MinRows is the fewest body rows a container may carry. Zero means one.
	// It is a floor on capacity, not on the final container: a table with two
	// rows left over does not fail because two is fewer than the minimum.
	MinRows int
}

// PlanRows computes the split of a measured table.
//
// The greedy fill lives here and [PlanTable] delegates to it, so the two cannot
// disagree about where a boundary falls. Greedy for the reason stated at the
// top of this file: balancing makes every container's contents a function of
// the total.
func PlanRows(r Rows) ([]Split, error) {
	minimum := r.MinRows
	if minimum < 1 {
		minimum = 1
	}

	if len(r.Heights) == 0 {
		// Headers alone still occupy a container. Returning no splits would
		// make the caller decide what an empty table means, and two callers
		// would decide differently.
		return []Split{{Index: 0, From: 0, Count: 0}}, nil
	}

	var out []Split
	for from := 0; from < len(r.Heights); {
		free := r.Available
		if len(out) == 0 && r.First > 0 {
			free = r.First
		}
		free -= r.HeaderHeight

		used, fit := int64(0), 0
		for i := from; i < len(r.Heights); i++ {
			if used+r.Heights[i] > free {
				break
			}
			used += r.Heights[i]
			fit++
		}

		// The floor is on capacity rather than on what is left, so a table
		// whose remainder is smaller than the minimum still places it. Testing
		// the remainder instead would refuse the last container of every table
		// that did not divide evenly.
		if fit == 0 || (fit < minimum && len(r.Heights)-from >= minimum) {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_OVERFLOW_NO_CAPACITY,
				"the container cannot hold even one row beneath its repeated headers",
				map[string]any{
					"available":     free + r.HeaderHeight,
					"header_height": r.HeaderHeight,
					"row_index":     from,
					"row_height":    r.Heights[from],
					"rows_that_fit": fit,
					"minimum_rows":  minimum,
				})
		}

		out = append(out, Split{Index: len(out), From: from, Count: fit})
		from += fit
	}
	return out, nil
}
