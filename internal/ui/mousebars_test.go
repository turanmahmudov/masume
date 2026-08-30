package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/core"
)

// readRowCells answers the cells of a drawn row, with the escapes taken off, so a hit box can
// be read against the text a reader sees. A glyph two columns wide keeps both of them, so one
// cell of the row is one column of the screen and a box is read where it was recorded.
func readRowCells(row string) []string {
	held := mapCells(row)
	cells := make([]string, 0, len(held))
	for _, cell := range held {
		cells = append(cells, cell.text)
	}
	return cells
}

// cutRowText answers the text a hit box covers.
func cutRowText(row string, from, to int) string {
	cells := readRowCells(row)
	if from < 0 || to >= len(cells) || from > to {
		return ""
	}
	return strings.Join(cells[from:to+1], "")
}

// Every key of the two bars is a button, so the cells it is recorded at have to be the cells
// its own text was drawn in. One cell out and a press runs the key beside it.
func TestTheKeysOfTheBarsStandWhereTheyAreDrawn(t *testing.T) {
	model, _, _ := buildEditingModel(t, "select id from orders", 0)
	frame := strings.Split(model.render(), "\n")
	title, bar := frame[model.layout.titleRow], frame[model.layout.hintRow]

	titleKeys, hints := 0, 0
	for _, held := range model.layout.buttons {
		switch held.row {
		case model.layout.titleRow:
			titleKeys++
			if text := cutRowText(title, held.from, held.to); !strings.Contains(text, "palette") &&
				!strings.Contains(text, "help") && !strings.Contains(text, "ask ai") {
				t.Errorf("the key of %q covers %q", held.action, text)
			}
		case model.layout.hintRow:
			hints++
			text := cutRowText(bar, held.from, held.to)
			if strings.TrimSpace(text) == "" {
				t.Errorf("the key of %q covers %q", held.action, text)
			}
			if strings.HasPrefix(text, " ") || strings.HasSuffix(text, " ") {
				t.Errorf("the key of %q covers %q, which is not the key alone",
					held.action, text)
			}
		}
	}
	if titleKeys == 0 {
		t.Error("the title bar recorded no key at all")
	}
	if hints == 0 {
		t.Error("the status bar recorded no key at all")
	}
}

// A press on a key of a bar runs it, so the bars work as the rows of buttons they look like.
func TestAPressOnAKeyOfABarRunsIt(t *testing.T) {
	model, connection, _ := buildEditingModel(t, "select id from orders", 0)
	model.render()

	held := findButtonOfAction(model, ActionShowPalette)
	model.readMouse(tea.MouseClickMsg{X: held.from, Y: held.row, Button: tea.MouseLeft})
	if connection.Overlay.Kind != app.OverlayPalette {
		t.Errorf("a press on the palette key opened %q", connection.Overlay.Kind)
	}
}

// A key that quits is never a button, because a press meant for the row above it would close
// the client.
func TestTheKeyThatQuitsIsNotAButton(t *testing.T) {
	model, _, _ := buildEditingModel(t, "", 0)
	model.render()
	for _, held := range model.layout.buttons {
		if held.action == "" {
			t.Errorf("a key with no action was recorded as a button")
		}
	}
	if _, _, _, pressed := findButton(
		model.layout.buttons, model.width-2, model.layout.hintRow); pressed {
		t.Error("the last cells of the bar answer a press")
	}
}

// findButtonOfAction answers the box of one action, so a test presses the key it means.
func findButtonOfAction(model *Model, action ActionID) buttonHit {
	for _, held := range model.layout.buttons {
		if held.action == action {
			return held
		}
	}
	return buttonHit{row: -1}
}

// A hint of a pair runs the first action from the first half of its key and the second from
// the other half, so one button steps both ways.
func TestAPairOfKeysAnswersFromEitherHalf(t *testing.T) {
	buttons := []buttonHit{{
		row: 3, from: 10, to: 24, keyTo: 13,
		scope: "tree", action: ActionFoldRow, second: ActionUnfoldRow,
	}}
	if _, action, _, _ := findButton(buttons, 10, 3); action != ActionFoldRow {
		t.Errorf("the first half answers %q", action)
	}
	if _, action, _, _ := findButton(buttons, 13, 3); action != ActionUnfoldRow {
		t.Errorf("the second half answers %q", action)
	}
	if _, _, _, pressed := findButton(buttons, 25, 3); pressed {
		t.Error("a press past the key answered it")
	}
	if _, _, _, pressed := findButton(buttons, 10, 4); pressed {
		t.Error("a press on another row answered it")
	}
}

// The banner names the keys that clear the sort and the filter, so a press on the word does
// what the key does.
func TestThePressOnTheBannerClearsTheRewrite(t *testing.T) {
	model, _, tab := buildGridModel(t)
	tab.Sort = []core.SortState{{Column: "id", Direction: core.SortDescending}}
	frame := strings.Split(model.render(), "\n")

	held := findButtonOfAction(model, ActionClearRewrites)
	if held.row < 0 {
		t.Fatal("the banner recorded no key to clear the rewrite")
	}
	if text := cutRowText(frame[held.row], held.from, held.to); !strings.Contains(text, "clear") {
		t.Errorf("the key covers %q", text)
	}
}

// The row that reports a fault steps to the next one, and the words of the fault are the key
// that steps.
func TestThePressOnTheFaultRowStepsToTheNextFault(t *testing.T) {
	model, _, tab := buildScannedModel(t)
	tab.Focus = app.PaneEditor
	tab.Editor = app.NewEditorBuffer("select * from public.nope", 0)
	frame := strings.Split(model.render(), "\n")

	stepped := findButtonOfAction(model, ActionNextProblem)
	if stepped.row < 0 {
		t.Fatal("the fault row recorded no key of its own")
	}
	if text := cutRowText(frame[stepped.row], stepped.from, stepped.to); !strings.Contains(
		text, "no table called") {
		t.Errorf("the fault covers %q", text)
	}
}

// The list that stands over the statement takes a press, so a candidate is chosen without
// the keyboard.
func TestAPressOnTheCompletionListTakesTheCandidate(t *testing.T) {
	model, connection, tab := buildScannedModel(t)
	tab.Focus = app.PaneEditor
	tab.Editor = app.NewEditorBuffer("select * from ord", len("select * from ord"))
	model.refreshCompletion(connection, tab)
	if !tab.Completion.IsListing() {
		t.Skip("the completion list offered nothing for this statement")
	}
	model.render()

	if model.layout.completionRows.count == 0 {
		t.Fatal("the list recorded no row of its own")
	}
	wanted := tab.Completion.Candidates[0].Text
	model.readMouse(tea.MouseClickMsg{
		X: model.layout.completionRows.from, Y: model.layout.completionRows.top,
		Button: tea.MouseLeft,
	})
	if !strings.Contains(tab.Editor.Text, wanted) {
		t.Errorf("the statement reads %q and does not hold %q", tab.Editor.Text, wanted)
	}
}

// The middle button closes a tab, as it does on the tabs of a browser.
func TestTheMiddleButtonClosesATab(t *testing.T) {
	model, connection, _ := buildEditingModel(t, "select id", 0)
	connection.OpenQueryTab("select two")
	model.render()
	before := len(connection.Tabs)

	held := model.layout.tabs[0]
	model.readMouse(tea.MouseClickMsg{
		X: held.from, Y: model.layout.tabRow, Button: tea.MouseMiddle,
	})
	if len(connection.Tabs) != before-1 {
		t.Errorf("the tabs went from %d to %d", before, len(connection.Tabs))
	}
}
