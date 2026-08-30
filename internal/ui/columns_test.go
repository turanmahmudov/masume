package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
)

// buildWideGridModel answers a model whose grid is drawn, with the cursor in it.
func buildWideGridModel(t *testing.T) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildLoadedModel(t, 1, 2, 12, 4)
	connection := model.Active()
	tab := connection.Active()
	tab.Focus = app.PaneResult
	model.render()
	return model, connection, tab
}

// A drag of the border after a column sets how wide it is drawn.
func TestADragOfAColumnBorderSetsItsWidth(t *testing.T) {
	model, connection, tab := buildWideGridModel(t)

	edge := model.layout.columnEdges[0]
	before := model.buildGridShape(connection, tab).Widths[edge.index]
	model.readMouse(tea.MouseClickMsg{
		X: edge.from, Y: model.layout.gridHeaderRow, Button: tea.MouseLeft,
	})
	model.readMouseMotion(tea.MouseMotionMsg{
		X: edge.from + 5, Y: model.layout.gridHeaderRow, Button: tea.MouseLeft,
	})
	model.readMouseRelease(tea.MouseReleaseMsg{
		X: edge.from + 5, Y: model.layout.gridHeaderRow, Button: tea.MouseLeft,
	})

	if after := model.buildGridShape(connection, tab).Widths[edge.index]; after != before+5 {
		t.Errorf("a drag of five cells left the column %d wide, and it was %d", after, before)
	}
}

// The border is dragged, so the drag lays no selection over the cells it crosses and orders
// the read by nothing.
func TestADragOfAColumnBorderSortsNothing(t *testing.T) {
	model, _, tab := buildWideGridModel(t)

	edge := model.layout.columnEdges[0]
	model.readMouse(tea.MouseClickMsg{
		X: edge.from, Y: model.layout.gridHeaderRow, Button: tea.MouseLeft,
	})
	if len(tab.Sort) != 0 {
		t.Error("a press on the border ordered the read by the column")
	}
	model.readMouseMotion(tea.MouseMotionMsg{
		X: edge.from + 4, Y: model.layout.gridHeaderRow, Button: tea.MouseLeft,
	})
	if model.holdsSelection() {
		t.Error("a drag of the border laid a selection over the cells it crossed")
	}
}

// A second press on the border gives the column back to the width of its widest value.
func TestASecondPressOnAColumnBorderFitsIt(t *testing.T) {
	model, connection, tab := buildWideGridModel(t)

	edge := model.layout.columnEdges[0]
	before := model.buildGridShape(connection, tab).Widths[edge.index]
	tab.ColumnWidths = map[int]int{edge.index: before + 9}
	model.render()

	row := model.layout.gridHeaderRow
	edge = model.layout.columnEdges[0]
	model.readMouse(tea.MouseClickMsg{X: edge.from, Y: row, Button: tea.MouseLeft})
	model.readMouse(tea.MouseClickMsg{X: edge.from, Y: row, Button: tea.MouseLeft})

	if after := model.buildGridShape(connection, tab).Widths[edge.index]; after != before {
		t.Errorf("a second press left the column %d wide, and its values ask for %d",
			after, before)
	}
}

// A width the reader set is what the grid draws, so the name and its values move together.
func TestTheWidthSetByHandIsWhatTheGridDraws(t *testing.T) {
	model, _, tab := buildWideGridModel(t)
	tab.ColumnWidths = map[int]int{0: 6}
	frame := strings.Split(model.render(), "\n")

	first, second := model.layout.gridColumns[0], model.layout.gridColumns[1]
	if width := first.to - first.from + 1; width != 6+columnGap {
		t.Errorf("the first column covers %d cells, and it was set to 6", width)
	}
	if second.from != first.to+1 {
		t.Errorf("the second column starts at %d, and the first ends at %d",
			second.from, first.to)
	}
	text := cutRowText(frame[model.layout.gridHeaderRow], second.from, second.to)
	if !strings.HasPrefix(text, "column_01") {
		t.Errorf("the name beside the narrowed column reads %q", text)
	}
}
