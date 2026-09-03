package present_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/present"
)

// A key press or a click lands on a screen column, and these functions return the target. A
// wrong result sorts the wrong column or folds the wrong row.

func TestIsOnFoldMarkerIsTrueOnlyOnTheMarkItself(t *testing.T) {
	// A click on the name selects the row. Only the mark folds it.
	for _, held := range []struct {
		name   string
		offset int
		depth  int
		want   bool
	}{
		{"the mark of a row at the top", 0, 0, true},
		{"the last cell of that mark", 1, 0, true},
		{"the name beside it", 2, 0, false},
		{"the mark of a row one level in", 2, 1, true},
		{"the indent before that mark", 1, 1, false},
		{"the name beside it", 4, 1, false},
		{"the mark of a row two levels in", 4, 2, true},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := present.IsOnFoldMarker(held.offset, held.depth, 2, 2)
			if got != held.want {
				t.Errorf("IsOnFoldMarker(%d, depth %d) = %v, want %v",
					held.offset, held.depth, got, held.want)
			}
		})
	}
}

func TestPlanVisibleTabsShowsThemAllWhereTheyFit(t *testing.T) {
	window := present.PlanVisibleTabs([]int{10, 10, 10}, 0, 100, 0)
	if window.Start != 0 || window.Count != 3 {
		t.Errorf("got start %d count %d, want 0 and 3", window.Start, window.Count)
	}
}

func TestPlanVisibleTabsAnswersNothingForNoTabs(t *testing.T) {
	window := present.PlanVisibleTabs(nil, 0, 100, 0)
	if window.Start != 0 || window.Count != 0 {
		t.Errorf("got start %d count %d, want 0 and 0", window.Start, window.Count)
	}
}

func TestPlanVisibleTabsHoldsTheWindowStillWhileTheOpenTabIsInIt(t *testing.T) {
	widths := []int{20, 20, 20, 20, 20, 20}
	first := present.PlanVisibleTabs(widths, 2, 60, 2)
	second := present.PlanVisibleTabs(widths, first.Start, 60, first.Start)
	if second.Start != first.Start {
		t.Errorf("the window moved from %d to %d with the tab still inside it",
			first.Start, second.Start)
	}
}

func TestPlanVisibleTabsScrollsToBringTheOpenTabIntoTheWindow(t *testing.T) {
	widths := []int{20, 20, 20, 20, 20, 20}
	window := present.PlanVisibleTabs(widths, 5, 60, 0)
	if 5 < window.Start || 5 >= window.Start+window.Count {
		t.Errorf("tab 5 is outside the window start %d count %d",
			window.Start, window.Count)
	}
}

func TestPlanVisibleTabsMovesTheWindowBackForATabBeforeIt(t *testing.T) {
	widths := []int{20, 20, 20, 20, 20, 20}
	window := present.PlanVisibleTabs(widths, 0, 60, 4)
	if window.Start != 0 {
		t.Errorf("start = %d, want the window moved back to 0", window.Start)
	}
}

func TestPlanVisibleTabsAlwaysDrawsTheOpenTabEvenWhereItDoesNotFit(t *testing.T) {
	// Without this a tab wider than the row would leave the row empty.
	window := present.PlanVisibleTabs([]int{5, 200, 5}, 1, 20, 1)
	if window.Count < 1 {
		t.Errorf("count = %d, want at least the open tab", window.Count)
	}
	if 1 < window.Start || 1 >= window.Start+window.Count {
		t.Errorf("the open tab is outside the window start %d count %d",
			window.Start, window.Count)
	}
}

func TestPlanVisibleColumnsSpansTheColumnsThatFitBesideTheFrozenOnes(t *testing.T) {
	// A frozen column always draws and takes its width from the same total, so fewer of
	// the other columns fit next to it.
	widths := []int{10, 10, 10, 10, 10}
	free := present.PlanVisibleColumns(present.ColumnPlanInput{
		Widths: widths, Available: 35, Gap: 1, ColumnOffset: 0,
	})
	frozen := present.PlanVisibleColumns(present.ColumnPlanInput{
		Widths: widths, Frozen: map[int]bool{0: true, 1: true},
		Available: 35, Gap: 1, ColumnOffset: 0,
	})
	if frozen.VisibleCount > free.VisibleCount {
		t.Errorf("with two frozen columns %d span, with none %d",
			frozen.VisibleCount, free.VisibleCount)
	}
}

func TestPlanVisibleColumnsHoldsTheWindowInsideTheColumnsThereAre(t *testing.T) {
	widths := []int{10, 10, 10}
	// The caller adds an offset to a cursor inside the result, so the offset is never
	// after the last column. A negative offset is clamped to the first column.
	for _, offset := range []int{-5, 0, 2} {
		plan := present.PlanVisibleColumns(present.ColumnPlanInput{
			Widths: widths, Available: 25, Gap: 1, ColumnOffset: offset,
		})
		if plan.WindowStart < 0 || plan.WindowStart > len(widths) {
			t.Errorf("offset %d gave WindowStart %d", offset, plan.WindowStart)
		}
		if plan.WindowStart+plan.VisibleCount > len(widths) {
			t.Errorf("offset %d spans past the last column: start %d count %d",
				offset, plan.WindowStart, plan.VisibleCount)
		}
	}
}

func TestPlanVisibleColumnsAlwaysSpansAtLeastOneColumn(t *testing.T) {
	// Without this a column wider than the pane would leave the grid empty.
	plan := present.PlanVisibleColumns(present.ColumnPlanInput{
		Widths: []int{200, 10}, Available: 20, Gap: 1, ColumnOffset: 0,
	})
	if plan.VisibleCount < 1 {
		t.Errorf("VisibleCount = %d, want at least 1", plan.VisibleCount)
	}
}

func TestPlanVisibleColumnsSpansThemAllWhereTheyFit(t *testing.T) {
	plan := present.PlanVisibleColumns(present.ColumnPlanInput{
		Widths: []int{10, 10, 10}, Available: 200, Gap: 1, ColumnOffset: 0,
	})
	if plan.WindowStart != 0 || plan.VisibleCount != 3 {
		t.Errorf("got start %d count %d, want 0 and 3", plan.WindowStart, plan.VisibleCount)
	}
}
