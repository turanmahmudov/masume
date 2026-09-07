package ui

import (
	"strconv"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/notebook"
)

const twoCells = "```sql id=first\nselect 1\n```\n\n```sql id=second\nselect 2\n```\n"

// buildNotebookModel returns a model with one notebook tab and the caret in the cell list.
func buildNotebookModel(t *testing.T, text string) (*Model, *app.Tab) {
	t.Helper()
	model := buildOfflineModel(t, 110, 30)
	connection := model.Active()
	tab := connection.OpenNotebook(notebook.Parse(text), "", notebook.OriginPersonal)
	tab.Focus = app.PaneEditor
	return model, tab
}

// seedCellResult gives one cell of the notebook a result of two named columns.
func seedCellResult(tab *app.Tab, at int) {
	cell := tab.Notebook.Cells[at]
	cell.FirstResult, cell.ResultCount = 0, 1
	tab.Results.Start([]string{"select ..."}, 100)
	tab.Results.Succeed(0, db.ComposedRead{Text: "select ..."}, db.QueryResult{
		Columns: []db.ResultColumn{
			{Name: "country", DataType: "text"}, {Name: "revenue", DataType: "numeric"},
		},
		Rows: [][]any{{"ES", 140908.53}, {"DE", 138984.92}},
	})
}

// pressNotebookKey hands one press to the model and returns the model it answered with.
func pressNotebookKey(t *testing.T, model *Model, key tea.KeyPressMsg) *Model {
	t.Helper()
	held, _ := model.Update(key)
	next, is := held.(*Model)
	if !is {
		t.Fatalf("the press answered with %T", held)
	}
	return next
}

// A letter moves between cells while the list holds the keyboard, and types into the cell
// once the caret is inside it. A letter that did both would make the list unusable.
func TestNotebookKeysMoveTheListAndTypeInACell(t *testing.T) {
	model, tab := buildNotebookModel(t, twoCells)

	pressNotebookKey(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if tab.Notebook.Focused != 1 {
		t.Fatalf("the list stands on cell %d", tab.Notebook.Focused+1)
	}
	if tab.Editor.Text != "select 2" {
		t.Errorf("the editor holds %q", tab.Editor.Text)
	}

	pressNotebookKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !tab.Notebook.Editing {
		t.Fatalf("the caret is not in the cell")
	}
	pressNotebookKey(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if tab.Editor.Text != "select 2j" {
		t.Errorf("the cell holds %q", tab.Editor.Text)
	}

	// The first Escape dismisses the completion list the edit opened, as it does in the
	// editor of a query tab, and the second one leaves the cell.
	model = pressNotebookKey(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	pressNotebookKey(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if tab.Notebook.Editing {
		t.Errorf("the caret is still in the cell")
	}
}

// The keys of the list add and delete a cell, and the undo of the list brings a deleted
// cell back.
func TestNotebookKeysChangeTheList(t *testing.T) {
	model, tab := buildNotebookModel(t, twoCells)

	model = pressNotebookKey(t, model, tea.KeyPressMsg{Code: 'b', Text: "b"})
	if tab.Notebook.CountCells() != 3 {
		t.Fatalf("cells after the add: %d", tab.Notebook.CountCells())
	}
	// A new cell asks for its kind first, and the kind it takes opens it for writing.
	if connection := model.Active(); connection.Overlay.Kind != app.OverlayActionMenu {
		t.Fatalf("the kind menu did not open: %q", connection.Overlay.Kind)
	}
	model = pressNotebookKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !tab.Notebook.Editing {
		t.Errorf("a new cell does not hold the caret")
	}
	model = pressNotebookKey(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})

	model = pressNotebookKey(t, model, tea.KeyPressMsg{Code: 'd', Text: "d"})
	model = pressNotebookKey(t, model, tea.KeyPressMsg{Code: 'd', Text: "d"})
	if tab.Notebook.CountCells() != 2 {
		t.Fatalf("cells after the delete: %d", tab.Notebook.CountCells())
	}
	pressNotebookKey(t, model, tea.KeyPressMsg{Code: 'u', Text: "u"})
	if tab.Notebook.CountCells() != 3 {
		t.Errorf("cells after the undo: %d", tab.Notebook.CountCells())
	}
}

// The kind key opens a menu, and the row it chooses changes what the cell holds.
func TestNotebookKindMenuChangesTheCell(t *testing.T) {
	model, tab := buildNotebookModel(t, twoCells)
	connection := model.Active()

	model = pressNotebookKey(t, model, tea.KeyPressMsg{Code: 'c', Text: "c"})
	if connection.Overlay.Kind != app.OverlayActionMenu {
		t.Fatalf("the menu did not open: %q", connection.Overlay.Kind)
	}
	// The rows stand in the order of the kinds, so the second one is the text cell.
	connection.Overlay.List.Cursor = 1
	pressNotebookKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if held := tab.Notebook.GetFocusedCell().Kind; held != notebook.CellText {
		t.Errorf("the cell holds %q", held)
	}
	if connection.Overlay.IsOpen() {
		t.Errorf("the menu is still open")
	}
}

// The mark key marks a cell for a partial run, and the run of the marked cells refuses to
// run with nothing marked.
func TestNotebookMarksCellsForAPartialRun(t *testing.T) {
	model, tab := buildNotebookModel(t, twoCells)
	connection := model.Active()

	pressNotebookKey(t, model, tea.KeyPressMsg{Code: 'm', Text: "m"})
	if connection.Notice == nil {
		t.Fatalf("nothing was reported for a run with no mark")
	}
	pressNotebookKey(t, model, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if marked := tab.Notebook.ListMarkedCells(); len(marked) != 1 || marked[0] != 0 {
		t.Errorf("marked cells: %v", marked)
	}
}

// A chart cell holds no text, so the key that edits a cell opens the form of the chart, and
// the form writes the fence. A chart that could only be written by hand in the file would
// be no feature of the app at all.
func TestNotebookChartFormWritesTheFence(t *testing.T) {
	model, tab := buildNotebookModel(t,
		"```sql id=countries\n-- countries\nselect country, revenue from x\n```\n")
	connection := model.Active()
	seedCellResult(tab, 0)

	// A new cell takes the chart kind, and the form opens with it.
	model = pressNotebookKey(t, model, tea.KeyPressMsg{Code: 'b', Text: "b"})
	connection.Overlay.List.Cursor = 3
	model = pressNotebookKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if connection.Overlay.Kind != app.OverlayChart {
		t.Fatalf("the chart form did not open: %q", connection.Overlay.Kind)
	}
	// The source cell has run, so the form opens on its columns.
	if held := connection.Overlay.Chart; held.Source != "countries" ||
		held.Label != "country" || held.Value != "revenue" {
		t.Errorf("the form opened on %#v", held)
	}

	// The shape steps to a line, and the form writes what it holds.
	model = pressNotebookKey(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	model = pressNotebookKey(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	model = pressNotebookKey(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	pressNotebookKey(t, model, tea.KeyPressMsg{Code: tea.KeyRight})
	pressNotebookKey(t, model, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if connection.Overlay.IsOpen() {
		t.Fatalf("the form is still open")
	}

	cell := tab.Notebook.Cells[1]
	for name, wanted := range map[string]string{
		"source": "countries", "label": "country", "value": "revenue",
		"kind": string(notebook.ChartLine),
	} {
		held, found := cell.FindAttr(name)
		if !found || held != wanted {
			t.Errorf("the fence holds %s=%q, wanted %q", name, held, wanted)
		}
	}
}

// A cell that holds prose is checked against no schema and completed against no catalog, so
// a heading is no failed statement.
func TestNotebookProseIsNoStatement(t *testing.T) {
	model, tab := buildNotebookModel(t, "```md\n# a heading\n```\n")
	connection := model.Active()

	model = pressNotebookKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if tab.EditsStatements() {
		t.Fatalf("a prose cell is read as statements")
	}
	pressNotebookKey(t, model, tea.KeyPressMsg{Code: 's', Text: "s"})
	if tab.Completion.IsListing() {
		t.Errorf("the prose cell offers completions")
	}
	if found := model.findDiagnostics(connection, tab); len(found) > 0 {
		t.Errorf("the prose cell reports %d problems", len(found))
	}
}

// The wheel moves the cell list, and the list stays where the wheel left it until the focus
// moves. A list that snapped back to the focused cell could not be read with the mouse.
func TestNotebookWheelMovesTheList(t *testing.T) {
	text := ""
	for at := 1; at <= 8; at++ {
		text += "```sql id=cell-" + strconv.Itoa(at) + "\n-- cell " + strconv.Itoa(at) +
			"\nselect " + strconv.Itoa(at) + "\n```\n\n"
	}
	model, tab := buildNotebookModel(t, text)
	// The frame records where the rows were drawn, which a press reads.
	_ = model.View()
	inside := model.layout.cellRows.from + 2

	for turn := 0; turn < 4; turn++ {
		held, _ := model.Update(tea.MouseWheelMsg{
			Button: tea.MouseWheelDown, X: inside, Y: model.layout.cellRows.top + 1,
		})
		model = held.(*Model)
	}
	if tab.Notebook.Offset != 4 || !tab.Notebook.Rolled {
		t.Fatalf("the list stands at %d, rolled %v",
			tab.Notebook.Offset, tab.Notebook.Rolled)
	}
	if tab.Notebook.Focused != 0 {
		t.Errorf("the wheel moved the focus to cell %d", tab.Notebook.Focused+1)
	}

	// A press on a row moves to the cell that row belongs to.
	_ = model.View()
	row := model.layout.cellRows.top + 3
	wanted := model.cellsOfRows[model.layout.cellRows.offset+3]
	model.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: inside, Y: row})
	if tab.Notebook.Focused != wanted {
		t.Errorf("the press moved to cell %d, wanted %d",
			tab.Notebook.Focused+1, wanted+1)
	}
	if tab.Notebook.Rolled {
		t.Errorf("the list still stands where the wheel left it")
	}
}

// The fold keys fold one cell and every cell, so a long notebook can be read as a list of
// names.
func TestNotebookFoldsCells(t *testing.T) {
	model, tab := buildNotebookModel(t, twoCells)

	model = pressNotebookKey(t, model, tea.KeyPressMsg{Code: 'o', Text: "o"})
	if !tab.Notebook.Cells[0].Folded || tab.Notebook.Cells[1].Folded {
		t.Errorf("folds after one key: %v and %v",
			tab.Notebook.Cells[0].Folded, tab.Notebook.Cells[1].Folded)
	}
	pressNotebookKey(t, model, tea.KeyPressMsg{Code: 'O', Text: "O"})
	if !tab.Notebook.Cells[1].Folded {
		t.Errorf("the second cell is not folded")
	}
}
