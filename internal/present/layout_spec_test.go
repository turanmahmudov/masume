package present_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/present"
)

// The panes are laid out for whatever the terminal gives, down to a very narrow one. A width
// below zero or a count of zero must answer something a renderer can use rather than a
// negative width that would panic where it is sliced.
func TestPlanLabelWidthNeverAnswersBelowZero(t *testing.T) {
	for _, count := range []int{0, 1, 2, 5, 40} {
		for _, available := range []int{-10, 0, 1, 10, 80, 400} {
			if held := present.PlanLabelWidth(count, available); held < 0 {
				t.Errorf("%d labels in %d cells answered %d", count, available, held)
			}
		}
	}
}

// A label keeps a readable width even where the terminal is narrow, because a label cut to
// nothing names no column at all.
func TestPlanLabelWidthKeepsALabelReadable(t *testing.T) {
	narrow := present.PlanLabelWidth(8, 20)
	if narrow <= 0 {
		t.Fatalf("eight labels in twenty cells answered %d", narrow)
	}
	// A wide terminal does not give one label the whole row either.
	wide := present.PlanLabelWidth(2, 4000)
	if wide >= 4000 {
		t.Errorf("two labels in four thousand cells answered %d", wide)
	}
	if wide < narrow {
		t.Errorf("a wide terminal answered %d, narrower than the %d of a small one",
			wide, narrow)
	}
}

// The name and the type of a field share the room the value does not need. Both keep a
// readable width, so a pane too narrow for even that is given the floor and the renderer cuts
// what will not fit. The widths never fall below zero and never grow as the pane shrinks.
func TestPlanFieldColumnsNeverFallBelowTheReadableFloor(t *testing.T) {
	last := present.PlanFieldColumns(-5)
	for _, available := range []int{-5, 0, 1, 10, 20, 40, 80, 200} {
		held := present.PlanFieldColumns(available)
		if held.Name <= 0 || held.Type <= 0 {
			t.Errorf("%d cells answered name=%d type=%d", available, held.Name, held.Type)
		}
		if held.Name < last.Name || held.Type < last.Type {
			t.Errorf("%d cells answered name=%d type=%d, narrower than a smaller pane gave",
				available, held.Name, held.Type)
		}
		last = held
	}
}

// A wide pane gives both columns their full width, and a narrow one takes from both rather
// than starving one of them.
func TestPlanFieldColumnsTakeFromBothWhereItIsNarrow(t *testing.T) {
	wide := present.PlanFieldColumns(200)
	narrow := present.PlanFieldColumns(30)

	if narrow.Name >= wide.Name && narrow.Type >= wide.Type {
		t.Errorf("a narrow pane answered name=%d type=%d, no smaller than the wide %d and %d",
			narrow.Name, narrow.Type, wide.Name, wide.Type)
	}
	if narrow.Name <= 0 || narrow.Type <= 0 {
		t.Errorf("a narrow pane starved a column: name=%d type=%d", narrow.Name, narrow.Type)
	}
}

// The strip of views names them where there is room, numbers them where there is less, and
// drops the hint first, because the names are what the user reads.
func TestPlanViewStripGivesUpTheHintBeforeTheNames(t *testing.T) {
	names := []string{"data", "columns", "indexes", "constraints", "ddl", "plan"}

	wide := present.PlanViewStrip(names, 0, 200)
	if !wide.Named || !wide.ShowsHint {
		t.Errorf("a wide strip answered %+v, wanted the names and the hint", wide)
	}

	// Narrower: the names stay and the hint goes.
	middle := present.PlanViewStrip(names, 0, 60)
	if !middle.Named {
		t.Errorf("a strip of sixty cells answered %+v, wanted the names kept", middle)
	}

	// Narrower still: the numbers alone.
	narrow := present.PlanViewStrip(names, 0, 10)
	if narrow.Named {
		t.Errorf("a strip of ten cells answered %+v, wanted the numbers alone", narrow)
	}
}

func TestPlanViewStripHoldsAnEmptyListAndANarrowTerminal(t *testing.T) {
	for _, available := range []int{-5, 0, 1, 5} {
		// It has to answer rather than reach past the end of the list.
		present.PlanViewStrip(nil, 0, available)
		present.PlanViewStrip([]string{"data"}, 0, available)
		// An index outside the list must not reach past it either.
		present.PlanViewStrip([]string{"data"}, 5, available)
	}
}

// The tree shrinks with the terminal, because a tree of its full width would leave the editor
// and the result no room beside it.
func TestPlanSidebarWidthLeavesRoomForThePanes(t *testing.T) {
	const preferred = 40

	wide := present.PlanSidebarWidth(200, preferred)
	if wide != preferred {
		t.Errorf("a wide terminal answered %d, wanted the width asked for", wide)
	}

	narrow := present.PlanSidebarWidth(60, preferred)
	if narrow >= 60 {
		t.Errorf("a terminal of sixty cells gave the tree %d, leaving nothing beside it", narrow)
	}
	if narrow <= 0 {
		t.Errorf("a terminal of sixty cells gave the tree %d", narrow)
	}
	if narrow > wide {
		t.Errorf("a narrow terminal gave the tree %d, more than the %d of a wide one",
			narrow, wide)
	}
}

// A terminal with no room gets no tree, and one that has room never gets a tree wider than
// itself. The width never falls below zero, because it is sliced with.
func TestPlanSidebarWidthNeverPassesWhatThereIs(t *testing.T) {
	for _, available := range []int{-10, 0, 1, 10, 20, 40, 80, 200} {
		held := present.PlanSidebarWidth(available, 40)
		if held < 0 {
			t.Errorf("%d cells gave the tree %d", available, held)
		}
		if available >= 0 && held > available {
			t.Errorf("%d cells gave the tree %d, wider than the terminal", available, held)
		}
	}
}
