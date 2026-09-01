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

// TestPlanRows_UnevenRowsFillEachContainerToWhatItHolds is the whole reason
// this shape exists beside [overflow.Table].
//
// A single row height would have to be the tallest, and every container would
// then carry the number of rows that fits if all of them were that tall — which
// is white space at the foot of every page of a table whose rows are mostly
// short.
func TestPlanRows_UnevenRowsFillEachContainerToWhatItHolds(t *testing.T) {
	got, err := overflow.PlanRows(overflow.Rows{
		Heights:   []int64{10, 10, 10, 40, 10, 10},
		Available: 50,
	})
	if err != nil {
		t.Fatalf("PlanRows: %v", err)
	}

	want := []overflow.Split{
		// Three short rows, and the tall one would take the container past 50.
		{Index: 0, From: 0, Count: 3},
		// The tall row and one short one fill the next exactly.
		{Index: 1, From: 3, Count: 2},
		{Index: 2, From: 5, Count: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("PlanRows = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("split %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestPlanRows_TheFirstContainerMayBeShorter covers a table that begins part
// way down a page.
//
// Without it the only honest split is to start every table that does not fit
// entirely on a container of its own, which strands the heading above it alone
// at the foot of the page before.
func TestPlanRows_TheFirstContainerMayBeShorter(t *testing.T) {
	rows := overflow.Rows{
		Heights:   []int64{10, 10, 10, 10, 10, 10},
		Available: 50,
		First:     20,
	}
	got, err := overflow.PlanRows(rows)
	if err != nil {
		t.Fatalf("PlanRows: %v", err)
	}

	want := []overflow.Split{
		{Index: 0, From: 0, Count: 2},
		{Index: 1, From: 2, Count: 4},
	}
	if len(got) != len(want) {
		t.Fatalf("PlanRows = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("split %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	// And the zero value means "the same as every other container", rather than
	// meaning a first container of no height at all.
	rows.First = 0
	plain, err := overflow.PlanRows(rows)
	if err != nil {
		t.Fatalf("PlanRows: %v", err)
	}
	if len(plain) != 2 || plain[0].Count != 5 {
		t.Errorf("with First unset the plan is %+v, want the full container first", plain)
	}
}

// TestPlanRows_HeadersCostCapacityOnEveryContainer pins that the repeated
// header is subtracted from each container rather than from the first.
func TestPlanRows_HeadersCostCapacityOnEveryContainer(t *testing.T) {
	got, err := overflow.PlanRows(overflow.Rows{
		Heights:      []int64{10, 10, 10, 10, 10, 10},
		HeaderHeight: 30,
		Available:    50,
	})
	if err != nil {
		t.Fatalf("PlanRows: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("PlanRows = %+v, want three containers of two rows each", got)
	}
	for i, s := range got {
		if s.Count != 2 {
			t.Errorf("container %d carries %d rows, want 2", i, s.Count)
		}
	}
}

// TestPlanRows_ARowThatFitsNowhereIsRefused is the failure the caller has to be
// told about rather than shown.
//
// A container that cannot hold one row beneath its headers produces either an
// empty container or an infinite sequence of them, and neither is something a
// writer can render.
func TestPlanRows_ARowThatFitsNowhereIsRefused(t *testing.T) {
	_, err := overflow.PlanRows(overflow.Rows{
		Heights:      []int64{10, 90, 10},
		HeaderHeight: 20,
		Available:    50,
	})
	if !verr.HasCode(err, verr.VELLUM_OVERFLOW_NO_CAPACITY) {
		t.Fatalf("error = %v, want VELLUM_OVERFLOW_NO_CAPACITY", err)
	}
}

// TestPlanRows_TheMinimumIsAFloorOnCapacityNotOnTheRemainder pins the
// distinction that decides whether an ordinary table is refused.
//
// A table of eleven rows across a capacity of ten leaves one row over. Testing
// the remainder against the minimum would refuse the last container of every
// table that did not divide evenly, which is nearly all of them.
func TestPlanRows_TheMinimumIsAFloorOnCapacityNotOnTheRemainder(t *testing.T) {
	got, err := overflow.PlanRows(overflow.Rows{
		Heights:   []int64{10, 10, 10, 10, 10},
		Available: 40,
		MinRows:   3,
	})
	if err != nil {
		t.Fatalf("PlanRows: %v", err)
	}
	if len(got) != 2 || got[1].Count != 1 {
		t.Fatalf("PlanRows = %+v, want a final container carrying the one row left over", got)
	}

	// The floor still bites where the container genuinely cannot hold it.
	_, err = overflow.PlanRows(overflow.Rows{
		Heights:   []int64{10, 10, 10, 10, 10},
		Available: 20,
		MinRows:   3,
	})
	if !verr.HasCode(err, verr.VELLUM_OVERFLOW_NO_CAPACITY) {
		t.Fatalf("error = %v, want VELLUM_OVERFLOW_NO_CAPACITY", err)
	}
}
