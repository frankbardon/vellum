package overflow_test

import (
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/overflow"
)

const (
	row    = int64(228600) // a quarter inch
	region = int64(5283200)
)

func plan(t *testing.T, rows, headers int) []overflow.Split {
	t.Helper()

	out, err := overflow.PlanTable(overflow.Table{
		Rows:         rows,
		HeaderRows:   headers,
		RowHeight:    row,
		HeaderHeight: row,
		Available:    region,
	})
	if err != nil {
		t.Fatalf("PlanTable: %v", err)
	}
	return out
}

// TestPlanTable_ATableThatFitsIsStillReported.
//
// A report that shows up only when something went wrong is one nobody can tell
// apart from a missing report.
func TestPlanTable_ATableThatFitsIsStillReported(t *testing.T) {
	got := plan(t, 3, 1)

	if len(got) != 1 {
		t.Fatalf("want one split, got %d", len(got))
	}
	if got[0] != (overflow.Split{Index: 0, From: 0, Count: 3}) {
		t.Errorf("got %+v", got[0])
	}
	if !got[0].Last(3) {
		t.Error("the only split is not reported as the last")
	}
}

// TestPlanTable_TheSplitsTileTheTable is the property everything else rests on.
func TestPlanTable_TheSplitsTileTheTable(t *testing.T) {
	for _, rows := range []int{1, 20, 21, 22, 100, 999} {
		got := plan(t, rows, 2)

		placed := 0
		for i, s := range got {
			if s.Index != i {
				t.Errorf("%d rows: split %d reports index %d", rows, i, s.Index)
			}
			if s.From != placed {
				t.Fatalf("%d rows: split %d starts at %d, want %d", rows, i, s.From, placed)
			}
			if s.Count <= 0 {
				t.Fatalf("%d rows: split %d carries %d rows", rows, i, s.Count)
			}
			placed += s.Count
		}
		if placed != rows {
			t.Errorf("%d rows: the splits carry %d between them", rows, placed)
		}
		if len(got) > 0 && !got[len(got)-1].Last(rows) {
			t.Errorf("%d rows: the final split is not reported as the last", rows)
		}
	}
}

// TestPlanTable_IsGreedy pins the rule, because balancing looks better and is
// worse: it makes every container's contents a function of the total, so
// appending a row reflows everything before it.
func TestPlanTable_IsGreedy(t *testing.T) {
	capacity := plan(t, 1000, 2)[0].Count

	first := plan(t, capacity+1, 2)
	if len(first) != 2 {
		t.Fatalf("one row past capacity should need two containers, got %d", len(first))
	}
	if first[0].Count != capacity || first[1].Count != 1 {
		t.Errorf("got %d then %d, want %d then 1 — the split is balancing rather than filling",
			first[0].Count, first[1].Count, capacity)
	}

	// And the first container does not change as the table grows.
	for _, rows := range []int{capacity + 1, capacity + 7, capacity * 3} {
		if got := plan(t, rows, 2)[0].Count; got != capacity {
			t.Errorf("%d rows: the first container carries %d, want %d", rows, got, capacity)
		}
	}
}

// TestPlanTable_HeadersCostCapacityOnEveryContainer.
func TestPlanTable_HeadersCostCapacityOnEveryContainer(t *testing.T) {
	none := plan(t, 1000, 0)[0].Count
	one := plan(t, 1000, 1)[0].Count
	three := plan(t, 1000, 3)[0].Count

	if !(none > one && one > three) {
		t.Fatalf("capacity does not fall as headers are added: %d, %d, %d", none, one, three)
	}
	if none-one != 1 || one-three != 2 {
		t.Errorf("each header row of equal height should cost one body row: %d, %d, %d",
			none, one, three)
	}
}

// TestPlanTable_ARegionWithNoRoomIsRefused.
//
// Clipping would drop rows, and a table missing its last rows looks exactly
// like a table that never had them.
func TestPlanTable_ARegionWithNoRoomIsRefused(t *testing.T) {
	for name, table := range map[string]overflow.Table{
		"headers fill the region": {
			Rows: 10, HeaderRows: 30, RowHeight: row, HeaderHeight: row, Available: region,
		},
		"the region is shorter than one row": {
			Rows: 10, RowHeight: row, Available: row - 1,
		},
		"the row height is unstated": {
			Rows: 10, RowHeight: 0, Available: region,
		},
		"the minimum cannot be met": {
			Rows: 10, RowHeight: row, Available: 2 * row, MinRows: 5,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := overflow.PlanTable(table)
			if !verr.HasCode(err, verr.VELLUM_OVERFLOW_NO_CAPACITY) {
				t.Fatalf("want VELLUM_OVERFLOW_NO_CAPACITY, got %v", err)
			}
		})
	}
}

// TestPlanTable_ATableOfHeadersAloneOccupiesOneContainer.
//
// Returning nothing would make the caller decide what an empty table means, and
// two callers would decide differently.
func TestPlanTable_ATableOfHeadersAloneOccupiesOneContainer(t *testing.T) {
	got := plan(t, 0, 2)

	if len(got) != 1 || got[0].Count != 0 {
		t.Fatalf("want one empty container, got %+v", got)
	}
}

// TestAllPolicies_IsACopy keeps the registry from being mutable through its own
// accessor.
func TestAllPolicies_IsACopy(t *testing.T) {
	first := overflow.AllPolicies()
	if len(first) == 0 {
		t.Fatal("no policies are declared")
	}
	first[0] = "tampered"

	if overflow.AllPolicies()[0] == "tampered" {
		t.Fatal("AllPolicies hands out the registry itself")
	}
}
