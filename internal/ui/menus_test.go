package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
)

// openMenuWith presses the right button on this cell and answers what the menu offers.
func openMenuWith(t *testing.T, model *Model, connection *app.Connection, x, y int) app.Overlay {
	t.Helper()
	model.readMouse(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseRight})
	return connection.Overlay
}

// The right button on a tab offers what a reader does to the tab itself.
func TestTheRightButtonOnATabOpensItsMenu(t *testing.T) {
	model, connection, _ := buildEditingModel(t, "select id from orders", 0)
	connection.OpenQueryTab("select two from three")
	model.render()

	held := model.layout.tabs[0]
	overlay := openMenuWith(t, model, connection, held.from, model.layout.tabRow)
	if overlay.Kind != app.OverlayActionMenu {
		t.Fatalf("the right button on a tab opened %q", overlay.Kind)
	}
	if connection.ActiveIndex != 0 {
		t.Errorf("the menu opened on tab %d", connection.ActiveIndex)
	}
	checkMenuOffers(t, overlay, ActionCloseTab, ActionNameTab, ActionNewQueryTab)
}

// The right button on a row of the connection list offers what a reader does to the
// connection.
func TestTheRightButtonOnAConnectionOpensItsMenu(t *testing.T) {
	model := buildLoadedModel(t, 1, 2, 4, 2)
	connection := model.Active()
	model.render()

	block := model.layout.connections
	overlay := openMenuWith(t, model, connection, block.from+2, block.top)
	if overlay.Kind != app.OverlayActionMenu {
		t.Fatalf("the right button on a connection opened %q", overlay.Kind)
	}
	checkMenuOffers(t, overlay, ActionCloseConnection, ActionRefreshObjects)
}

// The right button on the statement offers what a reader does to it.
func TestTheRightButtonOnTheStatementOpensItsMenu(t *testing.T) {
	model, connection, tab := buildEditingModel(t, "select id from orders", 0)
	tab.Focus = app.PaneEditor
	model.render()

	overlay := openMenuWith(t, model, connection,
		model.layout.editorTextLeft+2, model.layout.editorTextTop)
	if overlay.Kind != app.OverlayActionMenu {
		t.Fatalf("the right button on the statement opened %q", overlay.Kind)
	}
	checkMenuOffers(t, overlay, ActionRunAtCursor, ActionFormatSQL, ActionPasteText)
}

// The right button on the name of a column offers what a reader does to the column.
func TestTheRightButtonOnAColumnNameOpensItsMenu(t *testing.T) {
	model := buildLoadedModel(t, 1, 2, 12, 4)
	connection := model.Active()
	tab := connection.Active()
	tab.Focus = app.PaneResult
	model.render()

	column := model.layout.gridColumns[1]
	overlay := openMenuWith(t, model, connection, column.from+1, model.layout.gridHeaderRow)
	if overlay.Kind != app.OverlayActionMenu {
		t.Fatalf("the right button on a name opened %q", overlay.Kind)
	}
	if tab.GridColumn != column.index {
		t.Errorf("the menu opened on column %d, and the press landed on %d",
			tab.GridColumn, column.index)
	}
	checkMenuOffers(t, overlay, ActionSortColumn, ActionFreezeColumns, ActionGoToColumn)
}

// The left button on a name still orders the read by it, as it did before the menu.
func TestTheLeftButtonOnAColumnNameStillSortsByIt(t *testing.T) {
	model := buildLoadedModel(t, 1, 2, 12, 4)
	connection := model.Active()
	tab := connection.Active()
	tab.Focus = app.PaneResult
	model.render()

	column := model.layout.gridColumns[1]
	model.readMouse(tea.MouseClickMsg{
		X: column.from + 1, Y: model.layout.gridHeaderRow, Button: tea.MouseLeft,
	})
	if connection.Overlay.IsOpen() {
		t.Error("the left button on a name opened a menu")
	}
	if len(tab.Sort) == 0 {
		t.Error("the left button on a name ordered the read by nothing")
	}
}

// An entry chosen from a menu of actions runs the action it names.
func TestAnEntryOfAnActionMenuRunsIt(t *testing.T) {
	model, connection, _ := buildEditingModel(t, "select id from orders", 0)
	connection.OpenQueryTab("select two from three")
	model.render()

	held := model.layout.tabs[1]
	overlay := openMenuWith(t, model, connection, held.from, model.layout.tabRow)
	at := findMenuRow(overlay, ActionCloseTab)
	if at < 0 {
		t.Fatal("the tab menu offered no way to close the tab")
	}
	connection.Overlay.List.Cursor = at
	model.render()

	block := model.layout.overlayRows
	before := len(connection.Tabs)
	model.readMouse(tea.MouseClickMsg{
		X: block.from + 4, Y: block.top + at, Button: tea.MouseLeft,
	})
	if len(connection.Tabs) != before-1 {
		t.Errorf("the close entry left %d tabs", len(connection.Tabs))
	}
}

// checkMenuOffers fails where the menu does not offer every one of these actions.
func checkMenuOffers(t *testing.T, overlay app.Overlay, wanted ...ActionID) {
	t.Helper()
	for _, action := range wanted {
		if findMenuRow(overlay, action) < 0 {
			offered := make([]string, 0, len(overlay.Actions))
			for _, entry := range overlay.Actions {
				offered = append(offered, entry.ID)
			}
			t.Errorf("the menu does not offer %q, and offers %s",
				action, strings.Join(offered, ", "))
		}
	}
}

// findMenuRow answers which row of the menu runs this action.
func findMenuRow(overlay app.Overlay, action ActionID) int {
	for at, entry := range overlay.Actions {
		if entry.ID == string(action) {
			return at
		}
	}
	return -1
}

// The report this work began with: a right button on a table opens the object menu, and a
// press on the row that draws the ER diagram runs that row and not the one under it.
func TestAPressOnAMenuRowRunsTheRowItLooksLike(t *testing.T) {
	model, connection := buildObjectMenuModel(t)
	model.render()

	at := findMenuRow(connection.Overlay, "er-diagram")
	if at < 0 {
		t.Fatal("the object menu offered no ER diagram")
	}
	block := model.layout.overlayRows
	frame := strings.Split(model.render(), "\n")
	if text := cutRowText(frame[block.top+at], block.from, block.to); !strings.Contains(
		text, "ER diagram") {
		t.Fatalf("the row the press lands on reads %q", strings.TrimSpace(text))
	}

	model.readMouse(tea.MouseClickMsg{
		X: block.from + 6, Y: block.top + at, Button: tea.MouseLeft,
	})
	// The offline catalog holds no foreign key, so the diagram reports that it found
	// nothing to draw. Either answer is the ER diagram row and not the row under it.
	switch connection.Overlay.Kind {
	case app.OverlayDiagram:
	case app.OverlayMessage:
		if !strings.Contains(strings.ToLower(connection.Overlay.Body), "relation") &&
			!strings.Contains(strings.ToLower(connection.Overlay.Title), "diagram") {
			t.Errorf("the press reported %q", connection.Overlay.Body)
		}
	default:
		t.Errorf("the press opened %q, and the row it landed on draws the diagram",
			connection.Overlay.Kind)
	}
}
