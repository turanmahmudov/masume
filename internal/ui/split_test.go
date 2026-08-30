package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
)

// findSplitRow answers the screen row of the line between the editor and the result.
func findSplitRow(model *Model) int {
	return model.layout.editorTop + model.layout.editorRows - 1
}

// A drag of the line between the two panes moves it, so the reader sets how much of the
// screen the statement takes.
func TestADragOfTheSplitMovesIt(t *testing.T) {
	model := buildLoadedModel(t, 1, 3, 40, 3)
	connection := model.Active()
	model.render()

	line := findSplitRow(model)
	before := model.layout.editorRows
	model.readMouse(tea.MouseClickMsg{X: 60, Y: line, Button: tea.MouseLeft})
	model.readMouseMotion(tea.MouseMotionMsg{X: 60, Y: line + 6, Button: tea.MouseLeft})
	model.readMouseRelease(tea.MouseReleaseMsg{X: 60, Y: line + 6, Button: tea.MouseLeft})
	model.render()

	if model.layout.editorRows != before+6 {
		t.Errorf("a drag of six rows left the editor %d rows, and it was %d",
			model.layout.editorRows, before)
	}
	if !connection.ResultVisible {
		t.Error("a drag of the line hid the result")
	}
}

// A press on the line that never moves hides the result, as it did before the line could be
// dragged.
func TestAPressOnTheSplitHidesTheResult(t *testing.T) {
	model := buildLoadedModel(t, 1, 3, 40, 3)
	connection := model.Active()
	model.render()

	line := findSplitRow(model)
	model.readMouse(tea.MouseClickMsg{X: 60, Y: line, Button: tea.MouseLeft})
	if !connection.ResultVisible {
		t.Error("the press hid the result before it was let go")
	}
	model.readMouseRelease(tea.MouseReleaseMsg{X: 60, Y: line, Button: tea.MouseLeft})
	if connection.ResultVisible {
		t.Error("a press on the line left the result on show")
	}
}

// A drag never leaves a pane with fewer rows than it is drawn in.
func TestADragOfTheSplitStopsAtTheFewestRows(t *testing.T) {
	model := buildLoadedModel(t, 1, 3, 40, 3)
	connection := model.Active()
	model.render()

	line := findSplitRow(model)
	model.readMouse(tea.MouseClickMsg{X: 60, Y: line, Button: tea.MouseLeft})
	model.readMouseMotion(tea.MouseMotionMsg{X: 60, Y: model.height, Button: tea.MouseLeft})
	model.render()
	if model.layout.resultRows < minPaneHeight {
		t.Errorf("a drag to the foot left the result %d rows", model.layout.resultRows)
	}

	model.readMouseMotion(tea.MouseMotionMsg{X: 60, Y: 0, Button: tea.MouseLeft})
	model.render()
	if model.layout.editorRows < minPaneHeight {
		t.Errorf("a drag to the top left the editor %d rows", model.layout.editorRows)
	}
	_ = connection
}

// The line the pointer is dragged over belongs to the panes, so the drag lays no selection
// over the cells it crosses.
func TestADragOfTheSplitSelectsNothing(t *testing.T) {
	model := buildLoadedModel(t, 1, 3, 40, 3)
	model.Active().Active().Focus = app.PaneEditor
	model.render()

	line := findSplitRow(model)
	model.readMouse(tea.MouseClickMsg{X: 60, Y: line, Button: tea.MouseLeft})
	model.readMouseMotion(tea.MouseMotionMsg{X: 70, Y: line + 3, Button: tea.MouseLeft})
	if model.holdsSelection() {
		t.Error("a drag of the line laid a selection over the cells it crossed")
	}
}
