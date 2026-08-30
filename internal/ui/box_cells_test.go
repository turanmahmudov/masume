package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query"
)

func TestMapCellsKeepsEveryColumn(t *testing.T) {
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000"))
	line := red.Render("ab") + "cd" + red.Render("é")
	cells := mapCells(line)
	if len(cells) != 5 {
		t.Fatalf("the line has %d cells, wanted 5", len(cells))
	}
	if cells[0].text != "a" || cells[1].text != "b" || cells[4].text != "é" {
		t.Errorf("the cells read %q", cells)
	}
	if cells[0].sgr == "" || cells[2].sgr != "" {
		t.Errorf("the escapes did not follow the cells: %q", cells)
	}
}

func TestWriteCellsRoundTrips(t *testing.T) {
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000"))
	blue := lipgloss.NewStyle().Background(lipgloss.Color("#0000ff"))
	line := red.Render("ab") + blue.Render("cd") + "ef"

	written := writeCells(mapCells(line))
	if lipgloss.Width(written) != lipgloss.Width(line) {
		t.Errorf("the width changed from %d to %d",
			lipgloss.Width(line), lipgloss.Width(written))
	}
	if stripEscapes(written) != stripEscapes(line) {
		t.Errorf("the text changed from %q to %q",
			stripEscapes(line), stripEscapes(written))
	}
	if !strings.Contains(written, "255;0;0") || !strings.Contains(written, "0;0;255") {
		t.Errorf("a colour was lost: %q", written)
	}
}

func TestMapCellsCountsAWideCellTwice(t *testing.T) {
	cells := mapCells("漢a")
	if len(cells) != 3 {
		t.Fatalf("a wide cell gave %d cells, wanted 3", len(cells))
	}
	if cells[1].text != "" || cells[2].text != "a" {
		t.Errorf("the column beside a wide cell holds %q", cells[1].text)
	}
}

func TestFollowColumnCursorMovesTheWindow(t *testing.T) {
	shape := GridShape{Widths: []int{10, 10, 10, 10, 10}}
	shape.Columns = make([]query.ResultColumn, len(shape.Widths))
	model := &Model{styles: NewStyles(NewThemeRegistry())}
	tab := &app.Tab{Frozen: map[int]bool{}}

	// Two columns fit, so the window follows the cursor to the last one.
	tab.GridColumn = 4
	plan := model.followColumnCursor(tab, shape, 22)
	if tab.GridColumnOffset != 4-plan.VisibleCount+1 {
		t.Errorf("the window opens at %d, showing %d of them",
			tab.GridColumnOffset, plan.VisibleCount)
	}
	if tab.GridColumn < plan.WindowStart ||
		tab.GridColumn >= plan.WindowStart+plan.VisibleCount {
		t.Errorf("the cursor sits outside the window it was moved to")
	}

	// Stepping back to the first column moves the window with it.
	tab.GridColumn = 0
	plan = model.followColumnCursor(tab, shape, 22)
	if tab.GridColumnOffset != 0 || plan.WindowStart != 0 {
		t.Errorf("the window opens at %d, wanted the first column", tab.GridColumnOffset)
	}

	// A frozen column is always drawn, so the window stays where it was.
	tab.GridColumnOffset = 3
	tab.Frozen[1] = true
	tab.GridColumn = 1
	model.followColumnCursor(tab, shape, 22)
	if tab.GridColumnOffset != 3 {
		t.Errorf("a frozen column moved the window to %d", tab.GridColumnOffset)
	}
}

func TestHiddenColumnCounts(t *testing.T) {
	tab := &app.Tab{Frozen: map[int]bool{0: true}}
	plan := present.ColumnPlan{WindowStart: 2, VisibleCount: 2}
	hidden := countHiddenColumns(tab, plan, 8)
	// The frozen column is drawn, so it is not one of the two left out on the left.
	if hidden.left != 1 || hidden.right != 4 {
		t.Errorf("the counts are %d left and %d right, wanted 1 and 4",
			hidden.left, hidden.right)
	}
}
