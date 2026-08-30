package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/app"
)

// buildEditingModel answers a model whose editor holds a statement and the caret, which is
// what every key of the editor is read against.
func buildEditingModel(t *testing.T, text string, caret int) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildOfflineModel(t, 120, 34)
	connection := model.Active()
	tab := connection.Active()
	tab.Focus = app.PaneEditor
	tab.Editor = app.NewEditorBuffer(text, caret)
	return model, connection, tab
}

// pressEditorKey sends one press to the workspace, as the terminal reports it.
func pressEditorKey(model *Model, key tea.Key) *Model {
	next, _ := model.readWorkspaceKey(key)
	return next.(*Model)
}

// Shift and an arrow grow the selection. The registry must not take these keys while the
// caret is in the editor, or nothing is ever selected.
func TestShiftAndAnArrowSelectInTheEditor(t *testing.T) {
	model, _, tab := buildEditingModel(t, "select id\nfrom orders", 0)

	for range 6 {
		model = pressEditorKey(model, tea.Key{Code: tea.KeyRight, Mod: uv.ModShift})
	}
	if tab.Editor.Selection() != "select" {
		t.Fatalf("six presses of Shift and Right select %q", tab.Editor.Selection())
	}
	model = pressEditorKey(model, tea.Key{Code: tea.KeyDown, Mod: uv.ModShift})
	if tab.Editor.Selection() != "select id\nfrom o" {
		t.Errorf("Shift and Down selects %q", tab.Editor.Selection())
	}
	if !model.holdsSelection() {
		t.Error("the status bar was not told there is a selection")
	}
}

func TestControlAndASelectsTheWholeStatement(t *testing.T) {
	const text = "select id\nfrom orders"
	model, _, tab := buildEditingModel(t, text, 3)

	model = pressEditorKey(model, tea.Key{Code: 'a', Text: "a", Mod: uv.ModCtrl})
	if tab.Editor.Selection() != text {
		t.Errorf("Ctrl+A selects %q", tab.Editor.Selection())
	}
	if !model.holdsSelection() {
		t.Error("the status bar was not told there is a selection")
	}
}

func TestHomeAndEndReachTheEndsOfTheLineAndControlTheEndsOfTheStatement(t *testing.T) {
	const text = "select id\nfrom orders"
	model, _, tab := buildEditingModel(t, text, len("select id\nfrom"))

	model = pressEditorKey(model, tea.Key{Code: tea.KeyHome})
	if tab.Editor.Caret != len("select id\n") {
		t.Errorf("Home answers %d, wanted the start of the line", tab.Editor.Caret)
	}
	model = pressEditorKey(model, tea.Key{Code: tea.KeyEnd})
	if tab.Editor.Caret != len(text) {
		t.Errorf("End answers %d, wanted the end of the line", tab.Editor.Caret)
	}
	model = pressEditorKey(model, tea.Key{Code: tea.KeyHome, Mod: uv.ModCtrl})
	if tab.Editor.Caret != 0 {
		t.Errorf("Ctrl+Home answers %d, wanted the top of the statement", tab.Editor.Caret)
	}
	pressEditorKey(model, tea.Key{Code: tea.KeyEnd, Mod: uv.ModCtrl})
	if tab.Editor.Caret != len(text) {
		t.Errorf("Ctrl+End answers %d, wanted the foot of the statement", tab.Editor.Caret)
	}
}

func TestControlAndAnArrowStepsOverAWord(t *testing.T) {
	model, _, tab := buildEditingModel(t, "select id from orders", 0)

	model = pressEditorKey(model, tea.Key{Code: tea.KeyRight, Mod: uv.ModCtrl})
	if tab.Editor.Caret != len("select") {
		t.Errorf("Ctrl+Right answers %d", tab.Editor.Caret)
	}
	model = pressEditorKey(model, tea.Key{
		Code: tea.KeyRight, Mod: uv.ModCtrl | uv.ModShift})
	if tab.Editor.Selection() != " id" {
		t.Errorf("Ctrl+Shift+Right selects %q", tab.Editor.Selection())
	}
	pressEditorKey(model, tea.Key{Code: tea.KeyBackspace, Mod: uv.ModAlt})
	if tab.Editor.Text != "select from orders" {
		t.Errorf("Alt+Backspace answers %q", tab.Editor.Text)
	}
}

func TestTypingTakesThePlaceOfTheSelection(t *testing.T) {
	model, _, tab := buildEditingModel(t, "select id from orders", 0)

	model = pressEditorKey(model, tea.Key{Code: 'a', Text: "a", Mod: uv.ModCtrl})
	pressEditorKey(model, tea.Key{Code: 'x', Text: "x"})
	if tab.Editor.Text != "x" {
		t.Errorf("typing over the whole statement answers %q", tab.Editor.Text)
	}
}

// A line opened in the middle of a statement leaves no selection, or the character typed
// after it takes the place of everything below the caret.
func TestOpeningALineKeepsWhatIsUnderIt(t *testing.T) {
	model, _, tab := buildEditingModel(t, "select id\nfrom orders", len("select id"))

	model = pressEditorKey(model, tea.Key{Code: tea.KeyEnter})
	if tab.Editor.HasSelection() {
		t.Fatalf("the buffer came back holding %q", tab.Editor.Selection())
	}
	pressEditorKey(model, tea.Key{Code: 'x', Text: "x"})
	if tab.Editor.Text != "select id\nx\nfrom orders" {
		t.Errorf("typing on the new line answers %q", tab.Editor.Text)
	}
}

func TestControlAndZTakesTheLastEditBack(t *testing.T) {
	model, _, tab := buildEditingModel(t, "select id", len("select id"))

	for _, letter := range " x" {
		model = pressEditorKey(model, tea.Key{Code: letter, Text: string(letter)})
	}
	if tab.Editor.Text != "select id x" {
		t.Fatalf("typing answers %q", tab.Editor.Text)
	}
	model = pressEditorKey(model, tea.Key{Code: 'z', Text: "z", Mod: uv.ModCtrl})
	if tab.Editor.Text != "select id" {
		t.Errorf("Ctrl+Z answers %q", tab.Editor.Text)
	}
	pressEditorKey(model, tea.Key{Code: 'z', Text: "z", Mod: uv.ModAlt})
	if tab.Editor.Text != "select id x" {
		t.Errorf("Alt+Z answers %q", tab.Editor.Text)
	}
}

// The terminal pastes as one message, whatever key it was asked with, and the lines it
// carries are broken the way the buffer breaks its own.
func TestAPasteFromTheTerminalReachesTheStatement(t *testing.T) {
	model, _, tab := buildEditingModel(t, "", 0)

	next, _ := model.Update(tea.PasteMsg{Content: "select id\r\nfrom orders"})
	model = next.(*Model)
	if tab.Editor.Text != "select id\nfrom orders" {
		t.Errorf("the paste answers %q", tab.Editor.Text)
	}
	if tab.Editor.Caret != len(tab.Editor.Text) {
		t.Errorf("the caret stands at %d, wanted the end of the paste", tab.Editor.Caret)
	}
	// One press takes the whole paste back.
	pressEditorKey(model, tea.Key{Code: 'z', Text: "z", Mod: uv.ModCtrl})
	if tab.Editor.Text != "" {
		t.Errorf("the undo of a paste answers %q", tab.Editor.Text)
	}
}

func TestAPasteReachesNothingButTheStatement(t *testing.T) {
	model, _, tab := buildEditingModel(t, "", 0)
	tab.Focus = app.PaneSidebar

	model.Update(tea.PasteMsg{Content: "select id"})
	if tab.Editor.Text != "" {
		t.Errorf("a paste outside the editor wrote %q", tab.Editor.Text)
	}
}

func TestResolveLineSelection(t *testing.T) {
	for _, held := range []struct {
		name             string
		from, to, length int
		wantFrom, wantTo int
	}{
		{"inside the line", 2, 5, 10, 2, 5},
		{"opening before the line", -4, 5, 10, 0, 5},
		// The break at the end of the line takes a cell of its own.
		{"running past the line", 2, 40, 10, 2, 11},
		{"before the line", -8, -3, 10, 0, 0},
		{"after the line", 12, 20, 10, 0, 0},
	} {
		from, to := resolveLineSelection(held.from, held.to, held.length)
		if from != held.wantFrom || to != held.wantTo {
			t.Errorf("%s answers %d to %d, wanted %d to %d",
				held.name, from, to, held.wantFrom, held.wantTo)
		}
	}
}

// The selection has to be drawn, or Shift and the arrows move a caret nobody can follow.
func TestTheEditorDrawsTheSelectionItHolds(t *testing.T) {
	model, _, tab := buildEditingModel(t, "select id\nfrom orders", 0)
	selected := resolveOpening(model.styles.Theme.Text, model.styles.Theme.Selection)
	if selected == "" {
		t.Fatal("the theme names no ground for a selection")
	}

	plain := strings.Join(model.renderEditor(model.Active(), tab, 60, 10), "\n")
	if strings.Contains(plain, selected) {
		t.Error("the ground of a selection is drawn although nothing is selected")
	}

	tab.Editor.SelectAll()
	drawn := strings.Join(model.renderEditor(model.Active(), tab, 60, 10), "\n")
	if !strings.Contains(drawn, selected) {
		t.Error("the statement is selected and the ground of the selection is not drawn")
	}
}

// A press of the pointer puts the caret where it stands, two take the word under it, and a
// drag grows the selection to where it is dragged.
func TestThePointerPlacesTheCaretInTheStatement(t *testing.T) {
	model, connection, tab := buildEditingModel(t, "select id\nfrom orders", 0)
	model.render()

	left := model.layout.editorTextLeft
	top := model.layout.editorTextTop
	model.pressEditor(connection, tab, tea.Mouse{X: left + 3, Y: top, Button: tea.MouseLeft})
	if tab.Editor.Caret != 3 {
		t.Errorf("a press answers a caret at %d, wanted 3", tab.Editor.Caret)
	}

	model.dragEditor(tea.Mouse{X: left + 2, Y: top + 1, Button: tea.MouseLeft})
	if tab.Editor.Selection() != "ect id\nfr" {
		t.Errorf("a drag selects %q", tab.Editor.Selection())
	}
	if !model.holdsSelection() {
		t.Error("the status bar was not told there is a selection")
	}
}

func TestAPressOnTheGutterReachesTheStartOfTheLine(t *testing.T) {
	model, connection, tab := buildEditingModel(t, "select id\nfrom orders", 0)
	model.render()

	model.pressEditor(connection, tab, tea.Mouse{
		X: model.layout.editorTextLeft - 2,
		Y: model.layout.editorTextTop + 1, Button: tea.MouseLeft,
	})
	if tab.Editor.Caret != len("select id\n") {
		t.Errorf("a press on the gutter answers %d", tab.Editor.Caret)
	}
}

// A long line is moved left only as far as it must be, so the words before the caret stay
// in view while the caret walks along it.
func TestTheEditorHoldsALongLineWhereItStands(t *testing.T) {
	line := strings.Repeat("a", 300)
	model, _, tab := buildEditingModel(t, line, 200)
	model.renderEditor(model.Active(), tab, 60, 10)
	moved := tab.EditorColumnOffset

	if moved == 0 {
		t.Fatal("a caret 200 cells along a line left the pane where it was")
	}
	// A step back along the line leaves the pane where it stands, because the caret is
	// still on it.
	tab.Editor.PlaceCaret(190, false)
	model.renderEditor(model.Active(), tab, 60, 10)
	if tab.EditorColumnOffset != moved {
		t.Errorf("a step back along the line moved the pane from %d to %d",
			moved, tab.EditorColumnOffset)
	}
	tab.Editor.PlaceCaret(0, false)
	model.renderEditor(model.Active(), tab, 60, 10)
	if tab.EditorColumnOffset != 0 {
		t.Errorf("the caret is at the start and the pane stands at %d",
			tab.EditorColumnOffset)
	}
}

// Two presses take the word under the pointer, and three take the line.
func TestTwoPressesTakeAWordAndThreeTakeTheLine(t *testing.T) {
	model, connection, tab := buildEditingModel(t, "select id\nfrom orders", 0)
	model.render()
	at := tea.Mouse{
		X: model.layout.editorTextLeft + 7, Y: model.layout.editorTextTop,
		Button: tea.MouseLeft,
	}

	model.pressEditor(connection, tab, at)
	model.pressEditor(connection, tab, at)
	if tab.Editor.Selection() != "id" {
		t.Errorf("two presses select %q", tab.Editor.Selection())
	}
	model.pressEditor(connection, tab, at)
	if tab.Editor.Selection() != "select id\n" {
		t.Errorf("three presses select %q", tab.Editor.Selection())
	}
}

// A drag over the statement belongs to the buffer, so the cells of the frame keep no
// selection of their own and the two are never painted over each other.
func TestADragOverTheStatementLeavesTheCellsAlone(t *testing.T) {
	model, _, tab := buildEditingModel(t, "select id\nfrom orders", 0)
	model.render()

	next, _ := model.readMouse(tea.MouseClickMsg{
		X: model.layout.editorTextLeft + 2, Y: model.layout.editorTextTop,
		Button: tea.MouseLeft,
	})
	model = next.(*Model)
	if model.selection.dragging {
		t.Error("a press on the statement began a drag over the cells of the frame")
	}
	if !model.drag.holds(dragEditorText) {
		t.Fatal("a press on the statement began no drag of the buffer")
	}

	next, _ = model.readMouseMotion(tea.MouseMotionMsg{
		X: model.layout.editorTextLeft + 6, Y: model.layout.editorTextTop,
		Button: tea.MouseLeft,
	})
	model = next.(*Model)
	if tab.Editor.Selection() != "lect" {
		t.Errorf("the drag selects %q", tab.Editor.Selection())
	}

	next, _ = model.readMouseRelease(tea.MouseReleaseMsg{
		X: model.layout.editorTextLeft + 6, Y: model.layout.editorTextTop,
	})
	model = next.(*Model)
	if model.drag.holds(dragEditorText) {
		t.Error("the drag of the buffer outlived the release")
	}
	if tab.Editor.Selection() != "lect" {
		t.Errorf("the release let the selection go, leaving %q", tab.Editor.Selection())
	}
}

// The wheel over the statement moves its lines, and the caret comes back into view at the
// next press of a key.
func TestTheWheelMovesTheLinesOfTheStatement(t *testing.T) {
	lines := make([]string, 40)
	for at := range lines {
		lines[at] = "select " + strings.Repeat("a", at%5+1)
	}
	model, _, tab := buildEditingModel(t, strings.Join(lines, "\n"), 0)
	model.render()

	next, _ := model.readMouseWheel(tea.MouseWheelMsg{
		X: model.layout.editorTextLeft, Y: model.layout.editorTextTop,
		Button: tea.MouseWheelDown,
	})
	model = next.(*Model)
	if tab.EditorRowOffset != 1 || !tab.EditorRolled {
		t.Errorf("one turn answers offset %d, rolled %v", tab.EditorRowOffset, tab.EditorRolled)
	}
	model.render()
	if tab.EditorRowOffset != 1 {
		t.Errorf("the frame pulled the lines back to %d", tab.EditorRowOffset)
	}

	pressEditorKey(model, tea.Key{Code: tea.KeyRight})
	model.render()
	if tab.EditorRowOffset != 0 {
		t.Errorf("a key left the lines at %d, wanted the caret back in view",
			tab.EditorRowOffset)
	}
}

// selectionContrastFloor is the least the ground of a selection may stand out from the
// ground of the pane. Below it the selection cannot be seen at all.
const selectionContrastFloor = 1.35

// Every theme has to give the selection a ground of its own: one that can be seen against
// the pane, and one the text still reads on.
func TestEveryThemeDrawsASelectionThatCanBeRead(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)
	for _, choice := range model.styles.registry.ListThemeChoices() {
		problems, applied := model.styles.ApplyThemeByName(choice.Name)
		if !applied || len(problems) > 0 {
			t.Errorf("theme %q reports %v", choice.Name, problems)
			continue
		}
		theme := model.styles.Theme
		if theme.Selection == nil {
			t.Errorf("theme %q names no ground for a selection", choice.Name)
			continue
		}
		if stood := CalculateContrastRatio(
			theme.Selection, theme.Panel); stood < selectionContrastFloor {
			t.Errorf("the selection of theme %q stands at %.2f against the pane",
				choice.Name, stood)
		}
		if read := CalculateContrastRatio(
			theme.Text, theme.Selection); read < TextContrastFloor {
			t.Errorf("the text of theme %q reads at %.2f on its selection",
				choice.Name, read)
		}
	}
}

// An empty line inside a selection still shows one cell, or a blank line reads as a hole in
// what was selected.
func TestAnEmptyLineInsideASelectionShowsACell(t *testing.T) {
	model, _, tab := buildEditingModel(t, "select id\n\nfrom orders", 0)
	tab.Editor.SelectAll()
	selected := resolveOpening(model.styles.Theme.Text, model.styles.Theme.Selection)

	rows := model.renderEditor(model.Active(), tab, 60, 10)
	// The border takes the first row, so the second line of the statement is the third.
	if !strings.Contains(rows[2], selected) {
		t.Errorf("the empty line inside the selection is drawn as %q", rows[2])
	}
}
