package cli

import (
	"fmt"
	"io"
	"strings"
)

// printTable writes rows as column-padded text: the first row is a header,
// every subsequent row is data, and every column is padded to the widest
// cell across the whole table.
//
// Hand-rolled fmt.Fprintf padding rather than a table library, per CLAUDE.md's
// Dependencies table: none is listed, and none should be added for this.
// text/tabwriter would work too, but a manual width pass keeps this package's
// only formatting dependency the standard library's string and fmt packages,
// with no writer state to flush.
//
// Row order is never computed here — it is read out of rows in the order the
// caller built it, which is itself built from an already-ordered source
// (capability.Matrix's own declaration order, a BoxSet's own role order, and
// so on) rather than from a map range, per CLAUDE.md's determinism
// conventions.
func printTable(w io.Writer, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	cols := len(rows[0])
	widths := make([]int, cols)
	for _, row := range rows {
		for i, cell := range row {
			if i >= cols {
				continue
			}
			if l := len(cell); l > widths[i] {
				widths[i] = l
			}
		}
	}
	for _, row := range rows {
		var b strings.Builder
		for i, cell := range row {
			if i >= cols {
				continue
			}
			b.WriteString(cell)
			if i < cols-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-len(cell)+2))
			}
		}
		fmt.Fprintln(w, b.String())
	}
}
