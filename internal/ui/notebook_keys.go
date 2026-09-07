package ui

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/notebook"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// The cell list has a scope of its own, so a single letter moves between cells while the
// same letter types inside a cell.

// runNotebookAction returns one action of the cell list.
func (model *Model) runNotebookAction(
	connection *app.Connection, tab *app.Tab, match Match,
) (tea.Model, tea.Cmd) {
	book := tab.Notebook
	if book == nil {
		return model, nil
	}

	switch match.Action {
	case ActionCursorUp:
		return model.focusCell(connection, tab, book.Focused-1)
	case ActionCursorDown:
		return model.focusCell(connection, tab, book.Focused+1)
	case ActionCursorFirstRow:
		return model.focusCell(connection, tab, 0)
	case ActionCursorLastRow:
		return model.focusCell(connection, tab, book.CountCells()-1)

	case ActionEditCellSource:
		return model.editCellSource(connection, tab)

	case ActionRunCell:
		return model.runNotebook(connection, tab, app.RunCell)
	case ActionRunFromCell:
		return model.runNotebook(connection, tab, app.RunFromCell)
	case ActionRunMarkedCells:
		if len(book.ListMarkedCells()) == 0 {
			connection.Show("no marked cell; mark one first")
			return model, nil
		}
		return model.runNotebook(connection, tab, app.RunMarkedCells)

	case ActionAddCellBelow, ActionAddCellAbove:
		tab.KeepFocusedCell()
		book.AddCell(match.Action == ActionAddCellBelow)
		tab.SettleFocusedCell()
		return model.askNewCellKind(connection, tab)

	case ActionSetCellKind:
		return model.askCellKind(connection, tab)

	case ActionDeleteCell:
		tab.KeepFocusedCell()
		if book.DeleteCell() {
			tab.SettleFocusedCell()
			connection.Show("cell deleted; undo brings it back")
		}
	case ActionUndoCellChange:
		tab.KeepFocusedCell()
		if !book.UndoCellChange() {
			connection.Show("nothing to undo")
		}
		tab.SettleFocusedCell()
	case ActionRedoCellChange:
		tab.KeepFocusedCell()
		if !book.RedoCellChange() {
			connection.Show("nothing to redo")
		}
		tab.SettleFocusedCell()

	case ActionMoveCellUp:
		book.MoveCell(-1)
	case ActionMoveCellDown:
		book.MoveCell(1)

	case ActionCopyCell:
		connection.NotebookClip = book.CopyCell()
		connection.Show("cell " + strconv.Itoa(book.Focused+1) + " copied")
	case ActionCutCell:
		connection.NotebookClip = book.CopyCell()
		tab.KeepFocusedCell()
		if book.DeleteCell() {
			tab.SettleFocusedCell()
			connection.Show("cell cut")
		}
	case ActionPasteCell:
		tab.KeepFocusedCell()
		if !book.PasteCell(connection.NotebookClip) {
			connection.Show("no copied cell")
			return model, nil
		}
		tab.SettleFocusedCell()

	case ActionToggleCellOutput:
		book.FoldCell()
	case ActionToggleEveryOutput:
		book.FoldEveryCell()
	case ActionMarkCell:
		book.MarkCell()

	case ActionNameCell:
		named := statement.FindQueryName(book.GetFocusedCell().Editor.Text)
		connection.Overlay = app.Overlay{
			Kind: app.OverlayPrompt, Prompt: app.PromptCellName, Title: "name",
			Draft: app.NewEditorBuffer(named, len(named)),
		}
	}
	return model, nil
}

// pressCellRow returns a press on one row of the cell list: the first press moves to the
// cell the row belongs to, and the second opens it.
func (model *Model) pressCellRow(
	connection *app.Connection, tab *app.Tab, mouse tea.Mouse, row int,
) (tea.Model, tea.Cmd) {
	tab.Focus = app.PaneEditor
	if row >= len(model.cellsOfRows) {
		return model, nil
	}
	at := model.cellsOfRows[row]
	held := model.clicks.count("cell-"+strconv.Itoa(at), time.Now())
	if at != tab.Notebook.Focused {
		return model.focusCell(connection, tab, at)
	}
	if held < 2 {
		return model, nil
	}
	return model.editCellSource(connection, tab)
}

// focusCell moves the list to one cell, and the result pane follows it.
func (model *Model) focusCell(
	connection *app.Connection, tab *app.Tab, index int,
) (tea.Model, tea.Cmd) {
	book := tab.Notebook
	// The sort and the filter belong to the cell the reader set them on, so they are kept
	// with it and the next cell brings its own.
	tab.KeepFocusedCell()
	book.FocusCell(index)
	tab.SettleFocusedCell()
	cell := book.GetFocusedCell()
	tab.Completion.Close()
	if cell.FirstResult >= 0 {
		tab.Results.SelectResult(cell.FirstResult)
		return model.showSelectedResult(connection, tab)
	}
	return model, nil
}

// editCellSource puts the caret inside the focused cell.
func (model *Model) editCellSource(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	book := tab.Notebook
	cell := book.GetFocusedCell()
	// A chart cell holds no text, so the same key opens the form that writes its fence.
	if cell.Kind == notebook.CellChart {
		cell.Folded = false
		return model.openChartForm(connection, tab)
	}
	cell.Folded = false
	book.Editing = true
	tab.Editor = cell.Editor
	tab.Focus = app.PaneEditor
	return model, nil
}

// leaveCellSource takes the caret out of the cell and back to the list.
func (model *Model) leaveCellSource(tab *app.Tab) {
	if tab.Notebook == nil {
		return
	}
	tab.Notebook.Editing = false
	tab.Completion.Close()
	tab.Editor.ClearSelection()
}

// The ids of the rows of the menu that sets the kind of a cell. A cell that was just added
// carries the second prefix, so the kind it takes opens it for writing.
const (
	cellKindPrefix    = "cell-kind:"
	newCellKindPrefix = "new-cell-kind:"
)

// cellKinds are the kinds the menu offers, in the order it draws them.
var cellKinds = []notebook.CellKind{
	notebook.CellSQL, notebook.CellText, notebook.CellParam, notebook.CellChart,
}

// askCellKind opens the menu that changes what the focused cell holds.
func (model *Model) askCellKind(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	return model.openCellKindMenu(connection, tab, cellKindPrefix, " cell kind ")
}

// askNewCellKind opens the menu of a cell that was just added.
func (model *Model) askNewCellKind(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	return model.openCellKindMenu(connection, tab, newCellKindPrefix, " new cell ")
}

// openCellKindMenu opens the menu of the kinds a cell can hold.
func (model *Model) openCellKindMenu(
	connection *app.Connection, tab *app.Tab, prefix, title string,
) (tea.Model, tea.Cmd) {
	held := tab.Notebook.GetFocusedCell().Kind
	actions := []app.MenuAction{}
	for _, kind := range cellKinds {
		detail := describeCellKind(kind)
		if kind == held && prefix == cellKindPrefix {
			detail += " · current"
		}
		actions = append(actions, app.MenuAction{
			ID: prefix + string(kind), Label: string(kind), Detail: detail,
		})
	}
	connection.Overlay = app.Overlay{
		Kind: app.OverlayActionMenu, Title: title, Actions: actions,
		Draft: app.NewEditorBuffer("", 0),
	}
	return model, nil
}

// describeCellKind returns what one kind of cell holds.
func describeCellKind(kind notebook.CellKind) string {
	switch kind {
	case notebook.CellSQL:
		return "statements of the engine"
	case notebook.CellText:
		return "prose"
	case notebook.CellParam:
		return "values every cell binds"
	case notebook.CellChart:
		return "a bar or a line from another cell"
	}
	return ""
}

// applyCellKind sets the kind the menu chose. A cell that was just added is opened for
// writing with the kind it took.
func (model *Model) applyCellKind(
	connection *app.Connection, tab *app.Tab, id string,
) (tea.Model, tea.Cmd) {
	if tab.Notebook == nil {
		return model, nil
	}
	written, isNew := strings.CutPrefix(id, newCellKindPrefix)
	if !isNew {
		held, found := strings.CutPrefix(id, cellKindPrefix)
		if !found {
			return model, nil
		}
		written = held
	}

	kind := notebook.CellKind(written)
	tab.Notebook.SetCellKind(kind)
	if kind == notebook.CellChart {
		model.seedChartAttrs(tab)
		// A chart cell holds no text to write, so the form of the chart opens with it.
		return model.openChartForm(connection, tab)
	}
	if isNew {
		return model.editCellSource(connection, tab)
	}
	connection.Show("cell " + strconv.Itoa(tab.Notebook.Focused+1) + " holds " + written)
	return model, nil
}

// seedChartAttrs gives a new chart cell the nearest statement cell above it as its source,
// and the two columns of the result of that cell where it has already run.
func (model *Model) seedChartAttrs(tab *app.Tab) {
	book := tab.Notebook
	cell := book.GetFocusedCell()
	if _, held := cell.FindAttr("source"); held {
		return
	}
	source := findSourceCell(tab)
	if source == nil {
		return
	}
	held := app.ChartRequest{
		Cell: cell.ID, Source: source.ID, Shape: string(notebook.ChartBar),
	}
	held.Label, held.Value = model.pickChartColumns(tab, source)
	if held.Value == "" {
		// A source that has not run offers no column, so only the source is written and
		// the form asks for the rest.
		cell.Attrs = append(cell.Attrs,
			notebook.Attribute{Name: "source", Value: held.Source},
			notebook.Attribute{Name: "kind", Value: held.Shape})
		return
	}
	cell.Attrs = buildChartAttrs(held, cell.Attrs)
}

// findSourceCell returns the statement cell a new chart reads: the nearest one above it, or
// the first one of the notebook.
func findSourceCell(tab *app.Tab) *app.NotebookCell {
	book := tab.Notebook
	for at := book.Focused - 1; at >= 0; at-- {
		if book.Cells[at].RunsStatements() {
			return book.Cells[at]
		}
	}
	for _, cell := range book.Cells {
		if cell.RunsStatements() {
			return cell
		}
	}
	return nil
}

// pickChartColumns returns the first column that is no number and the first that is one, so
// a chart of a result of a name and a count draws without being written.
func (model *Model) pickChartColumns(
	tab *app.Tab, source *app.NotebookCell,
) (string, string) {
	if tab.ReadCellOutcome(source).Kind != app.CellDone {
		return "", ""
	}
	held := tab.Results.ResultAt(source.FirstResult)
	if held == nil || len(held.State.Result.Rows) == 0 {
		return "", ""
	}
	label, value := "", ""
	row := held.State.Result.Rows[0]
	for at, column := range held.State.Result.Columns {
		if at >= len(row) {
			break
		}
		_, isNumber := present.ReadChartValue(row[at])
		switch {
		case isNumber && value == "":
			value = column.Name
		case !isNumber && label == "":
			label = column.Name
		}
	}
	return label, value
}

// The ids of the rows of the menu that sets the run policy.
const (
	policyTransactionPrefix = "notebook-transaction:"
	policyErrorPrefix       = "notebook-on-error:"
)

// askNotebookPolicy opens the menu that sets the transaction and the error policy.
func (model *Model) askNotebookPolicy(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	if tab.Notebook == nil {
		connection.Show("this tab holds no notebook")
		return model, nil
	}
	policy := tab.Notebook.Run
	mark := func(held bool) string {
		if held {
			return " · current"
		}
		return ""
	}
	connection.Overlay = app.Overlay{
		Kind: app.OverlayActionMenu, Title: " run policy ", Actions: []app.MenuAction{
			{
				ID:    policyTransactionPrefix + notebook.TransactionAutocommit,
				Label: "autocommit", Detail: "each statement commits" +
					mark(!policy.RunsInOneTransaction()),
			},
			{
				ID:    policyTransactionPrefix + notebook.TransactionSingle,
				Label: "one transaction", Detail: "commit after the last cell" +
					mark(policy.RunsInOneTransaction()),
			},
			{
				ID: policyErrorPrefix + notebook.ErrorStop, Label: "stop on error",
				Detail: "the first failure ends it" + mark(policy.StopsOnError()),
			},
			{
				ID: policyErrorPrefix + notebook.ErrorContinue, Label: "continue on error",
				Detail: "the later cells still run" + mark(!policy.StopsOnError()),
			},
		},
		Draft: app.NewEditorBuffer("", 0),
	}
	return model, nil
}

// applyNotebookPolicy sets the policy the menu chose.
func (model *Model) applyNotebookPolicy(
	connection *app.Connection, tab *app.Tab, id string,
) (tea.Model, tea.Cmd) {
	if tab.Notebook == nil {
		return model, nil
	}
	if written, held := strings.CutPrefix(id, policyTransactionPrefix); held {
		tab.Notebook.Run.Transaction = written
		tab.Notebook.Dirty = true
		connection.Show("run policy: " + describeTransactionPolicy(written))
		return model, nil
	}
	if written, held := strings.CutPrefix(id, policyErrorPrefix); held {
		tab.Notebook.Run.OnError = written
		tab.Notebook.Dirty = true
		connection.Show("run policy: " + written + " on error")
	}
	return model, nil
}
