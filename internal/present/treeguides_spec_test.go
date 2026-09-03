package present_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/present"
)

// buildGuidedRows returns rows at every depth of the tree, with branches that end at each
// depth, so a guide has siblings below it, siblings above it, and no siblings.
func buildGuidedRows() []present.TreeRow {
	depths := []int{0, 1, 2, 2, 1, 2, 2, 2, 0, 1, 1, 2, 0}
	rows := make([]present.TreeRow, 0, len(depths))
	for _, depth := range depths {
		rows = append(rows, present.TreeRow{Depth: depth})
	}
	return rows
}

// The pane draws forty rows of a catalog with thousands of rows, so the guides are built for
// the visible rows only. A guide depends on the rows below it, so a window must give the same
// result as a build of every row.
func TestBuildTreeGuidesWithinAgreesWithTheWhole(t *testing.T) {
	rows := buildGuidedRows()
	whole := present.BuildTreeGuidesWithin(rows, 0, len(rows))

	for from := 0; from <= len(rows); from++ {
		for to := from; to <= len(rows); to++ {
			held := present.BuildTreeGuidesWithin(rows, from, to)
			if len(held) != to-from {
				t.Fatalf("rows %d to %d answered %d guides, want %d",
					from, to, len(held), to-from)
			}
			for at, guide := range held {
				if guide != whole[from+at] {
					t.Fatalf("row %d of the window %d to %d reads %q, want %q",
						at, from, to, guide, whole[from+at])
				}
			}
		}
	}
}

// A window outside the rows returns nothing, and a window after the last row stops at that
// row.
func TestBuildTreeGuidesWithinHoldsToTheRows(t *testing.T) {
	rows := buildGuidedRows()
	whole := present.BuildTreeGuidesWithin(rows, 0, len(rows))

	if held := present.BuildTreeGuidesWithin(rows, -4, 0); held != nil {
		t.Fatalf("a window above the first row answered %d guides", len(held))
	}
	if held := present.BuildTreeGuidesWithin(rows, len(rows), len(rows)+9); held != nil {
		t.Fatalf("a window under the last row answered %d guides", len(held))
	}
	if held := present.BuildTreeGuidesWithin(rows, 5, 5); held != nil {
		t.Fatalf("an empty window answered %d guides", len(held))
	}

	held := present.BuildTreeGuidesWithin(rows, -3, len(rows)+7)
	if len(held) != len(rows) {
		t.Fatalf("a window over every row answered %d guides, want %d",
			len(held), len(rows))
	}
	for at, guide := range held {
		if guide != whole[at] {
			t.Fatalf("row %d reads %q, want %q", at, guide, whole[at])
		}
	}
}

// The guides of a real catalog come from its rows, so this tests the window on the tree the
// pane draws and not on depth values alone.
func TestBuildTreeGuidesWithinDrawsTheOpenCatalog(t *testing.T) {
	input := buildTreeInput()
	input.Expanded = map[string]bool{
		core.BuildSchemaID("public"):  true,
		core.BuildSchemaID("archive"): true,
	}
	rows := present.BuildTree(input).Rows
	if len(rows) < 6 {
		t.Fatalf("the open catalog drew %d rows, too few to window", len(rows))
	}
	whole := present.BuildTreeGuidesWithin(rows, 0, len(rows))

	held := present.BuildTreeGuidesWithin(rows, 2, 5)
	for at, guide := range held {
		if guide != whole[2+at] {
			t.Fatalf("row %d reads %q, want %q", 2+at, guide, whole[2+at])
		}
	}
}
