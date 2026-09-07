package app_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/notebook"
)

// splitOnSemicolons stands in for the splitter of an engine.
func splitOnSemicolons(text string) []string {
	parts := []string{}
	for _, held := range strings.Split(text, ";") {
		if strings.TrimSpace(held) != "" {
			parts = append(parts, strings.TrimSpace(held))
		}
	}
	return parts
}

func buildNotebookTab(t *testing.T, text string) *app.Tab {
	t.Helper()
	return app.NewNotebookTab(1, notebook.Parse(text), "", notebook.OriginPersonal)
}

// A notebook tab holds the buffer of the focused cell, so every key of the editor works on
// the cell the list stands on.
func TestNotebookTabEditsTheFocusedCell(t *testing.T) {
	tab := buildNotebookTab(t, "```sql\nselect 1\n```\n\n```sql\nselect 2\n```\n")
	if tab.Kind != app.TabNotebook {
		t.Fatalf("kind: %q", tab.Kind)
	}
	if tab.Editor.Text != "select 1" {
		t.Errorf("editor: %q", tab.Editor.Text)
	}
	tab.Notebook.StepCell(1)
	tab.Editor = tab.Notebook.GetFocusedCell().Editor
	if tab.Editor.Text != "select 2" {
		t.Errorf("editor after a step: %q", tab.Editor.Text)
	}
}

// A notebook with no cell still opens with one, so there is always a cell to write in.
func TestNotebookOpensWithOneCell(t *testing.T) {
	tab := buildNotebookTab(t, "")
	if tab.Notebook.CountCells() != 1 {
		t.Fatalf("cells: %d", tab.Notebook.CountCells())
	}
	if tab.Notebook.GetFocusedCell().Kind != notebook.CellSQL {
		t.Errorf("kind: %q", tab.Notebook.GetFocusedCell().Kind)
	}
}

// The run plan holds one statement per statement of every cell it covers, and says which
// cell each one came from. A wrong map would report the failure on the wrong cell.
func TestBuildRunPlanNamesTheCellOfEveryStatement(t *testing.T) {
	tab := buildNotebookTab(t, "prose\n\n```sql\nselect 1; select 2\n```\n\n"+
		"```sql\nselect 3\n```\n")
	plan := tab.Notebook.BuildRunPlan(app.RunEveryCell, splitOnSemicolons)
	if len(plan.Statements) != 3 {
		t.Fatalf("statements: %v", plan.Statements)
	}
	wanted := []int{1, 1, 2}
	for at := range wanted {
		if plan.CellOf[at] != wanted[at] {
			t.Errorf("statement %d came from cell %d, want %d",
				at+1, plan.CellOf[at], wanted[at])
		}
	}
}

// A run of one cell sends that cell only, and a run from a cell sends the cells below it
// as well.
func TestBuildRunPlanFollowsTheScope(t *testing.T) {
	tab := buildNotebookTab(t, "```sql\nselect 1\n```\n\n```sql\nselect 2\n```\n\n"+
		"```sql\nselect 3\n```\n")
	tab.Notebook.FocusCell(1)

	one := tab.Notebook.BuildRunPlan(app.RunCell, splitOnSemicolons)
	if len(one.Statements) != 1 || one.Statements[0] != "select 2" {
		t.Errorf("one cell: %v", one.Statements)
	}
	below := tab.Notebook.BuildRunPlan(app.RunFromCell, splitOnSemicolons)
	if len(below.Statements) != 2 {
		t.Errorf("from the cell: %v", below.Statements)
	}
	tab.Notebook.MarkCell()
	marked := tab.Notebook.BuildRunPlan(app.RunMarkedCells, splitOnSemicolons)
	if len(marked.Statements) != 1 || marked.Statements[0] != "select 2" {
		t.Errorf("marked: %v", marked.Statements)
	}
}

// A cell that did not run holds no result, so the list reports it as not run rather than
// drawing the result of another cell.
func TestReadCellOutcomeReportsACellThatDidNotRun(t *testing.T) {
	tab := buildNotebookTab(t, "```sql\nselect 1\n```\n")
	if held := tab.ReadCellOutcome(tab.Notebook.GetFocusedCell()); held.Kind != app.CellIdle {
		t.Errorf("outcome: %q", held.Kind)
	}
}

// The results of a run are keyed by cell, so the list draws the answer of every cell and
// the failure of one names its cell.
func TestApplyRunPlanKeepsTheResultsOfEveryCell(t *testing.T) {
	tab := buildNotebookTab(t, "```sql\nselect 1; select 2\n```\n\n```sql\nselect 3\n```\n")
	plan := tab.Notebook.BuildRunPlan(app.RunEveryCell, splitOnSemicolons)
	tab.Notebook.ApplyRunPlan(plan)

	first, second := tab.Notebook.Cells[0], tab.Notebook.Cells[1]
	if first.FirstResult != 0 || first.ResultCount != 2 {
		t.Errorf("first cell: %d and %d", first.FirstResult, first.ResultCount)
	}
	if second.FirstResult != 2 || second.ResultCount != 1 {
		t.Errorf("second cell: %d and %d", second.FirstResult, second.ResultCount)
	}
	if at := tab.Notebook.FindCellOfResult(2); at != 1 {
		t.Errorf("result 2 belongs to cell %d", at)
	}
}

// A structural change is undone on its own, apart from the undo of a buffer, so a deleted
// cell comes back with the text it held.
func TestUndoCellChangeBringsADeletedCellBack(t *testing.T) {
	tab := buildNotebookTab(t, "```sql\nselect 1\n```\n\n```sql\nselect 2\n```\n")
	tab.Notebook.FocusCell(0)
	tab.Notebook.DeleteCell()
	if tab.Notebook.CountCells() != 1 {
		t.Fatalf("cells after the delete: %d", tab.Notebook.CountCells())
	}
	if !tab.Notebook.UndoCellChange() {
		t.Fatalf("the undo did nothing")
	}
	if tab.Notebook.CountCells() != 2 {
		t.Errorf("cells after the undo: %d", tab.Notebook.CountCells())
	}
	if tab.Notebook.Cells[0].Editor.Text != "select 1" {
		t.Errorf("text after the undo: %q", tab.Notebook.Cells[0].Editor.Text)
	}
}

// A cell of a copy carries the text and none of the results, so a paste never shows the
// rows of the cell it was copied from.
func TestPasteCellCarriesNoResult(t *testing.T) {
	tab := buildNotebookTab(t, "```sql\nselect 1\n```\n")
	tab.Notebook.Cells[0].FirstResult, tab.Notebook.Cells[0].ResultCount = 0, 1
	copied := tab.Notebook.CopyCell()
	if !tab.Notebook.PasteCell(copied) {
		t.Fatalf("the paste did nothing")
	}
	pasted := tab.Notebook.GetFocusedCell()
	if pasted.FirstResult != -1 || pasted.ResultCount != 0 {
		t.Errorf("the pasted cell holds a result: %d and %d",
			pasted.FirstResult, pasted.ResultCount)
	}
	if pasted.ID == tab.Notebook.Cells[0].ID {
		t.Errorf("both cells are named %q", pasted.ID)
	}
}

// The parameter cells give one value set to the whole notebook, so every cell binds the
// same `:name`.
func TestReadParameterValuesReadsEveryParameterCell(t *testing.T) {
	tab := buildNotebookTab(t, "```param\nday = \"2026-09-01\"\n```\n\n"+
		"```param\nregion = \"EU\"\n```\n")
	values, problems := tab.Notebook.ReadParameterValues()
	if len(problems) != 0 {
		t.Errorf("problems: %v", problems)
	}
	if values["day"] != "2026-09-01" || values["region"] != "EU" {
		t.Errorf("values: %#v", values)
	}
}

// The document a save writes holds the text of every cell as it stands, so what is on
// screen is what the file gets.
func TestBuildDocumentHoldsTheTextOfEveryCell(t *testing.T) {
	tab := buildNotebookTab(t, "```sql\nselect 1\n```\n")
	tab.Notebook.GetFocusedCell().Editor.SetText("select 2")
	document := tab.Notebook.BuildDocument()
	if len(document.Cells) != 1 || document.Cells[0].Source != "select 2" {
		t.Errorf("document: %#v", document.Cells)
	}
}
